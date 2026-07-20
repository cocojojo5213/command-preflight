package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func BuildFingerprint(input ErrorInput) ErrorFingerprint {
	errorText := input.Stderr
	if strings.TrimSpace(errorText) == "" {
		errorText = input.Stdout
	}
	normalizedError := NormalizeError(errorText)
	kind := classifyError(normalizedError)
	tool := NormalizeTool(FirstToken(input.Command))
	normalizedCommand := NormalizeCommand(input.Command)
	canonical := fmt.Sprintf("v1|%s|%s|%s|%d|%s", input.Shell, tool, kind, input.ExitCode, normalizedError)
	digest := sha256.Sum256([]byte(canonical))

	return ErrorFingerprint{
		Version:           "v1",
		ID:                "cp1-" + hex.EncodeToString(digest[:])[:20],
		Shell:             input.Shell,
		Tool:              tool,
		ErrorKind:         kind,
		ExitCode:          input.ExitCode,
		NormalizedError:   normalizedError,
		NormalizedCommand: normalizedCommand,
	}
}

// NormalizeTool keeps a custom executable from leaking a local path into a
// fingerprint. The basename is useful for grouping while the full path is not.
func NormalizeTool(tool string) string {
	tool = RedactSecrets(strings.Trim(tool, "\"'"))
	if index := strings.LastIndexAny(tool, `/\\`); index >= 0 {
		tool = tool[index+1:]
	}
	if tool == "" {
		return "<unknown>"
	}
	for _, r := range tool {
		if !(r == '.' || r == '_' || r == '-' || r == '+' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return "<custom>"
		}
	}
	if len(tool) > 96 {
		return "<custom>"
	}
	return strings.ToLower(tool)
}

func classifyError(text string) string {
	switch {
	case text == "":
		return "empty_error"
	case strings.Contains(text, "not recognized as an internal or external command"),
		strings.Contains(text, "command not found"),
		strings.Contains(text, "is not recognized"):
		return "command_not_found"
	case strings.Contains(text, "unknown option"),
		strings.Contains(text, "unrecognized option"),
		strings.Contains(text, "unknown argument"),
		strings.Contains(text, "invalid option"):
		return "unknown_option"
	case strings.Contains(text, "syntax error"),
		strings.Contains(text, "parsererror"),
		strings.Contains(text, "unexpected token"),
		strings.Contains(text, "missing terminator"):
		return "syntax_error"
	case strings.Contains(text, "permission denied"),
		strings.Contains(text, "access is denied"):
		return "permission_denied"
	case strings.Contains(text, "no such file or directory"),
		strings.Contains(text, "cannot find path"),
		strings.Contains(text, "path does not exist"):
		return "path_not_found"
	case strings.Contains(text, "the token '&&' is not a valid statement separator"),
		strings.Contains(text, "operator is reserved for future use"):
		return "shell_operator_unsupported"
	default:
		return "other"
	}
}

func FirstToken(command string) string {
	command = strings.TrimSpace(command)
	for strings.HasPrefix(command, "&") || strings.HasPrefix(command, ".\\") || strings.HasPrefix(command, "./") {
		command = strings.TrimSpace(strings.TrimLeft(command, "&"))
	}
	if command == "" {
		return ""
	}
	var token strings.Builder
	quote := rune(0)
	for _, r := range command {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				token.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			break
		}
		token.WriteRune(r)
	}
	return token.String()
}
