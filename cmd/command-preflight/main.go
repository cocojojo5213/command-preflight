package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cocojojo5213/command-preflight/internal/core"
	"github.com/cocojojo5213/command-preflight/internal/mcp"
)

var buildVersion = core.Version

// The embedded skill keeps the one-command installer independent of the source checkout.
//
//go:embed assets/skill/SKILL.md
var embeddedSkill []byte

//go:embed assets/skill/agents/openai.yaml
var embeddedSkillMetadata []byte

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}

	var exitCode int
	switch os.Args[1] {
	case "preflight":
		exitCode = runPreflight(os.Args[2:])
	case "fingerprint":
		exitCode = runFingerprint(os.Args[2:])
	case "mcp":
		if err := mcp.Serve(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "command-preflight mcp: %v\n", err)
			exitCode = 1
		}
	case "doctor":
		exitCode = runDoctor()
	case "install-skill":
		exitCode = runInstallSkill(os.Args[2:])
	case "setup":
		exitCode = runSetup(os.Args[2:])
	case "version", "--version", "-V":
		fmt.Printf("command-preflight %s (%s)\n", buildVersion, core.RuntimeSummary())
	default:
		if os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
			printUsage(os.Stdout)
			return
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage(os.Stderr)
		exitCode = 2
	}
	os.Exit(exitCode)
}

func runPreflight(args []string) int {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	shell := defaultShell()
	command := ""
	cwd := ""
	stderr := ""
	stdout := ""
	exitCode := 0
	jsonOutput := false
	fs.StringVar(&shell, "shell", shell, "shell: powershell, bash, sh, or cmd")
	fs.StringVar(&command, "command", "", "command text; use - to read it from stdin")
	fs.StringVar(&cwd, "cwd", "", "working directory to validate")
	fs.StringVar(&stderr, "stderr", "", "optional stderr from a previous attempt")
	fs.StringVar(&stdout, "stdout", "", "optional stdout from a previous attempt")
	fs.IntVar(&exitCode, "exit-code", 0, "exit code from a previous attempt")
	fs.BoolVar(&jsonOutput, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if command == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read command: %v\n", err)
			return 1
		}
		command = string(data)
	}
	result := core.RunPreflight(core.PreflightOptions{
		Shell:   core.Shell(shell),
		Command: command,
		CWD:     cwd,
	})
	if stderr != "" || stdout != "" || exitCode != 0 {
		fingerprint := core.BuildFingerprint(core.ErrorInput{
			Shell:    core.Shell(shell),
			Command:  command,
			ExitCode: exitCode,
			Stderr:   stderr,
			Stdout:   stdout,
		})
		result.Fingerprint = &fingerprint
	}
	if jsonOutput {
		return writeJSONResult(result)
	}
	printResult(result)
	return statusExitCode(result.Status)
}

func runFingerprint(args []string) int {
	fs := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	shell := defaultShell()
	command := ""
	stderr := ""
	stdout := ""
	exitCode := 1
	jsonOutput := true
	fs.StringVar(&shell, "shell", shell, "shell: powershell, bash, sh, or cmd")
	fs.StringVar(&command, "command", "", "command text")
	fs.StringVar(&stderr, "stderr", "", "stderr text")
	fs.StringVar(&stdout, "stdout", "", "stdout text")
	fs.IntVar(&exitCode, "exit-code", 1, "process exit code")
	fs.BoolVar(&jsonOutput, "json", true, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fingerprint := core.BuildFingerprint(core.ErrorInput{
		Shell:    core.Shell(shell),
		Command:  command,
		ExitCode: exitCode,
		Stderr:   stderr,
		Stdout:   stdout,
	})
	if jsonOutput {
		return writeJSON(fingerprint)
	}
	fmt.Printf("%s %s\n", fingerprint.ID, fingerprint.ErrorKind)
	return 0
}

func runDoctor() int {
	fmt.Printf("version: %s\n", core.Version)
	fmt.Printf("runtime: %s\n", core.RuntimeSummary())
	fmt.Printf("default shell: %s\n", defaultShell())
	fmt.Println("telemetry: disabled (local-only MVP)")
	fmt.Println("mcp: run `command-preflight mcp` over stdio")
	if _, err := os.Stat("."); err != nil {
		fmt.Printf("cwd: unavailable (%v)\n", err)
	} else {
		cwd, _ := os.Getwd()
		fmt.Printf("cwd: %s\n", cwd)
	}
	return 0
}

func runInstallSkill(args []string) int {
	fs := flag.NewFlagSet("install-skill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := "codex"
	force := false
	fs.StringVar(&target, "target", target, "target client: codex, claude, or both")
	fs.BoolVar(&force, "force", false, "replace an existing bundled skill")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find home directory: %v\n", err)
		return 1
	}
	paths := map[string]string{
		"codex":  filepath.Join(home, ".agents", "skills", "command-preflight"),
		"claude": filepath.Join(home, ".claude", "skills", "command-preflight"),
	}
	var targets []string
	switch target {
	case "codex", "claude":
		targets = []string{target}
	case "both":
		targets = []string{"codex", "claude"}
	default:
		fmt.Fprintf(os.Stderr, "unsupported target %q\n", target)
		return 2
	}
	for _, name := range targets {
		dir := paths[name]
		if !force {
			if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
				fmt.Printf("skip %s: %s already exists (use --force to replace)\n", name, dir)
				continue
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "agents"), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "create %s: %v\n", dir, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), embeddedSkill, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", dir, err)
			return 1
		}
		if err := os.WriteFile(filepath.Join(dir, "agents", "openai.yaml"), embeddedSkillMetadata, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write metadata %s: %v\n", dir, err)
			return 1
		}
		fmt.Printf("installed %s skill at %s\n", name, dir)
	}
	return 0
}

func runSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	client := "both"
	apply := false
	fs.StringVar(&client, "client", client, "client: codex, claude, or both")
	fs.BoolVar(&apply, "apply", false, "apply the MCP configuration instead of only printing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve executable: %v\n", err)
		return 1
	}
	clients := []string{}
	switch client {
	case "codex", "claude":
		clients = []string{client}
	case "both":
		clients = []string{"codex", "claude"}
	default:
		fmt.Fprintf(os.Stderr, "unsupported client %q\n", client)
		return 2
	}
	for _, name := range clients {
		var args []string
		switch name {
		case "codex":
			args = []string{"mcp", "add", "command-preflight", "--", executable, "mcp"}
		case "claude":
			args = []string{"mcp", "add", "--scope", "user", "command-preflight", "--", executable, "mcp"}
		}
		fmt.Printf("%s: %s %s\n", name, name, shellJoin(args))
		if !apply {
			continue
		}
		if _, err := exec.LookPath(name); err != nil {
			fmt.Fprintf(os.Stderr, "%s is not installed: %v\n", name, err)
			continue
		}
		command := exec.Command(name, args...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "configure %s: %v\n", name, err)
			return 1
		}
	}
	if !apply {
		fmt.Println("dry run only; add --apply to change client configuration")
	}
	return 0
}

func shellJoin(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\\\"") {
			parts = append(parts, fmt.Sprintf("%q", arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func defaultShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

func statusExitCode(status string) int {
	switch status {
	case "passed":
		return 0
	case "review":
		return 2
	default:
		return 1
	}
}

func printResult(result core.Result) {
	fmt.Printf("Preflight: %s\n", strings.ToUpper(result.Status))
	fmt.Printf("Shell: %s; syntax: %s; resolution complete: %t; risk: %s\n", result.Shell, result.Syntax, result.ResolutionComplete, result.Risk)
	if result.Executable != "" {
		fmt.Printf("Executable: %s\n", result.Executable)
	}
	for _, diagnostic := range result.Diagnostics {
		subject := ""
		if diagnostic.Subject != "" {
			subject = " [" + diagnostic.Subject + "]"
		}
		fmt.Printf("[%s] %s: %s%s\n", diagnostic.Severity, diagnostic.Code, diagnostic.Message, subject)
	}
	if result.Fingerprint != nil {
		fmt.Printf("Fingerprint: %s (%s)\n", result.Fingerprint.ID, result.Fingerprint.ErrorKind)
	}
}

func writeJSONResult(result core.Result) int {
	if err := writeJSON(result); err != 0 {
		return err
	}
	return statusExitCode(result.Status)
}

func writeJSON(value any) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(os.Stderr, "write JSON: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `command-preflight: local, non-executing command checks

Usage:
  command-preflight preflight --shell <shell> --command <text> [--cwd <dir>] [--json]
  command-preflight fingerprint --shell <shell> --command <text> --stderr <text> [--json]
  command-preflight mcp
  command-preflight doctor
  command-preflight install-skill --target codex|claude|both
  command-preflight setup --client codex|claude|both [--apply]
  command-preflight version

The MVP never executes the command being checked and keeps telemetry disabled.
Use the MCP subcommand as a stdio server for Codex, Claude Code, or another MCP client.`)
}
