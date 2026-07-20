package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const powershellParser = `$text = [Console]::In.ReadToEnd()
$tokens = $null
$errors = $null
[System.Management.Automation.Language.Parser]::ParseInput($text, [ref]$tokens, [ref]$errors) | Out-Null
if ($errors.Count -gt 0) {
  $errors | ForEach-Object { $_.Message }
  exit 1
}`

var bashBuiltins = map[string]bool{
	"[": true, "alias": true, "cd": true, "command": true, "echo": true,
	"eval": true, "exec": true, "export": true, "false": true, "local": true,
	"printf": true, "pwd": true, "read": true, "return": true, "set": true,
	"shift": true, "source": true, "test": true, "trap": true, "true": true,
	"type": true, "ulimit": true, "umask": true, "unset": true,
}

var shellKeywords = map[string]bool{
	"!": true, "case": true, "coproc": true, "do": true, "done": true,
	"elif": true, "else": true, "esac": true, "fi": true, "for": true,
	"function": true, "if": true, "in": true, "select": true, "then": true,
	"time": true, "until": true, "while": true, "{": true, "}": true,
}

var powershellCommands = map[string]bool{
	"clear-host": true, "copy-item": true, "get-childitem": true, "get-command": true,
	"get-content": true, "get-help": true, "get-location": true, "get-member": true,
	"get-process": true, "get-service": true, "invoke-expression": true,
	"invoke-restmethod": true, "invoke-webrequest": true, "move-item": true,
	"new-item": true, "remove-item": true, "resolve-path": true, "select-object": true,
	"set-content": true, "set-location": true, "start-process": true, "test-path": true,
	"where-object": true, "write-error": true, "write-output": true,
}

func RunPreflight(options PreflightOptions) Result {
	result := Result{
		Version:            Version,
		Status:             "passed",
		Shell:              normalizeShell(options.Shell),
		CommandRedacted:    NormalizeCommand(options.Command),
		CWD:                options.CWD,
		Syntax:             "not_checked",
		ResolutionComplete: true,
		Risk:               "low",
		Diagnostics:        []Diagnostic{},
	}

	if strings.TrimSpace(options.Command) == "" {
		addDiagnostic(&result, "error", "empty_command", "The command is empty.", "")
		return finalize(result)
	}
	if !validShell(result.Shell) {
		addDiagnostic(&result, "error", "unsupported_shell", "Use powershell, bash, sh, or cmd.", string(result.Shell))
		return finalize(result)
	}
	if options.CWD != "" {
		info, err := os.Stat(options.CWD)
		if err != nil {
			addDiagnostic(&result, "error", "cwd_missing", "The working directory does not exist.", options.CWD)
		} else if !info.IsDir() {
			addDiagnostic(&result, "error", "cwd_not_directory", "The working directory is not a directory.", options.CWD)
		}
	}

	first := FirstToken(options.Command)
	result.Executable = first
	if first == "" {
		addDiagnostic(&result, "error", "command_target_missing", "The command has no executable target.", "")
	} else {
		resolveCommand(&result, first)
	}
	validateSyntax(&result, result.Shell, options.Command, options.CWD)
	assessRisk(&result, options.Command)
	return finalize(result)
}

func normalizeShell(shell Shell) Shell {
	switch strings.ToLower(strings.TrimSpace(string(shell))) {
	case "pwsh", "powershell.exe":
		return ShellPowerShell
	case "sh", "dash", "zsh":
		return ShellSh
	case "cmd.exe", "command.com":
		return ShellCmd
	default:
		return Shell(strings.ToLower(strings.TrimSpace(string(shell))))
	}
}

func validShell(shell Shell) bool {
	return shell == ShellPowerShell || shell == ShellBash || shell == ShellSh || shell == ShellCmd
}

func resolveCommand(result *Result, token string) {
	clean := strings.Trim(token, "\"'")
	if result.Shell == ShellPowerShell && powershellCommands[strings.ToLower(clean)] {
		return
	}
	if (result.Shell == ShellBash || result.Shell == ShellSh) && bashBuiltins[clean] {
		return
	}
	if (result.Shell == ShellBash || result.Shell == ShellSh) && shellKeywords[clean] {
		return
	}
	if result.Shell == ShellPowerShell && (strings.EqualFold(clean, "if") || strings.EqualFold(clean, "foreach") || strings.EqualFold(clean, "for") || strings.EqualFold(clean, "while") || strings.EqualFold(clean, "switch")) {
		return
	}
	if strings.HasPrefix(clean, "$") || strings.HasPrefix(clean, "{") {
		result.ResolutionComplete = false
		addDiagnostic(result, "warning", "dynamic_command", "The executable is computed at runtime and cannot be resolved safely.", clean)
		return
	}
	if strings.ContainsAny(clean, `/\\`) {
		if _, err := os.Stat(clean); err != nil {
			addDiagnostic(result, "error", "command_path_missing", "The command path does not exist.", clean)
		}
		return
	}
	if _, err := exec.LookPath(clean); err != nil {
		if result.Shell == ShellPowerShell && looksLikePowerShellCmdlet(clean) {
			result.ResolutionComplete = false
			addDiagnostic(result, "warning", "powershell_resolution_deferred", "PowerShell command resolution requires the target PowerShell session.", clean)
			return
		}
		addDiagnostic(result, "error", "command_not_found", "The executable cannot be resolved on this host.", clean)
	}
}

func looksLikePowerShellCmdlet(name string) bool {
	parts := strings.Split(name, "-")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func validateSyntax(result *Result, shell Shell, command, cwd string) {
	switch shell {
	case ShellBash, ShellSh:
		bash := "bash"
		if shell == ShellSh {
			bash = "sh"
		}
		if _, err := exec.LookPath(bash); err != nil {
			result.Syntax = "unavailable"
			addDiagnostic(result, "warning", "syntax_checker_unavailable", "No standalone shell parser is available on this host.", bash)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := []string{"-n", "-c", command}
		if shell == ShellBash {
			args = []string{"--noprofile", "--norc", "-n", "-c", command}
		}
		cmd := exec.CommandContext(ctx, bash, args...)
		if cwd != "" {
			cmd.Dir = cwd
		}
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			result.Syntax = "failed"
			addDiagnostic(result, "error", "shell_syntax", strings.TrimSpace(output.String()), "")
			return
		}
		result.Syntax = "passed"
		addDiagnostic(result, "info", "syntax_only", "Shell syntax passed; external flags and runtime effects still require separate validation.", "")
	case ShellPowerShell:
		ps, err := findPowerShell()
		if err != nil {
			result.Syntax = "unavailable"
			addDiagnostic(result, "warning", "powershell_unavailable", "PowerShell is not available on this host; syntax was not checked.", "")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", powershellParser)
		if cwd != "" {
			cmd.Dir = cwd
		}
		cmd.Stdin = strings.NewReader(command)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			result.Syntax = "failed"
			message := strings.TrimSpace(output.String())
			if message == "" {
				message = err.Error()
			}
			addDiagnostic(result, "error", "powershell_syntax", message, "")
			return
		}
		result.Syntax = "passed"
	case ShellCmd:
		result.Syntax = "unavailable"
		addDiagnostic(result, "warning", "cmd_parser_unavailable", "cmd.exe has no reliable general-purpose syntax parser; review the command manually.", "")
	}
}

func findPowerShell() (string, error) {
	for _, name := range []string{"pwsh", "powershell"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("PowerShell not found")
}

func assessRisk(result *Result, command string) {
	lower := strings.ToLower(command)
	for _, marker := range []string{"rm ", "remove-item", "del ", "rmdir", "git reset", "git clean", "docker system prune", "chmod ", "chown ", "kill ", "stop-process", "publish", "deploy", "migrate", "git push"} {
		if strings.Contains(lower, marker) {
			result.Risk = "review"
			addDiagnostic(result, "warning", "high_risk_effect", "The command appears to change, delete, publish, or terminate external state; inspect the exact target before execution.", marker)
			return
		}
	}
}

func addDiagnostic(result *Result, severity, code, message, subject string) {
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Severity: severity, Code: code, Message: message, Subject: subject})
}

func finalize(result Result) Result {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == "error" {
			result.Status = "failed"
			return result
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == "warning" {
			result.Status = "review"
			return result
		}
	}
	return result
}

func RuntimeSummary() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func IsAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	return len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}
