package core

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRedactSecrets(t *testing.T) {
	input := `curl -H "Authorization: Bearer abc123" "https://example.test/?token=secret-value" --password='hidden'`
	output := RedactSecrets(input)
	for _, forbidden := range []string{"abc123", "secret-value", "hidden"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("secret %q survived redaction: %s", forbidden, output)
		}
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("expected redaction marker, got %s", output)
	}
}

func TestRedactPublicText(t *testing.T) {
	input := `Use token=secret-value from C:\Users\alice\project, /srv/private/repo, or src/private/file.go; contact alice@example.com at https://internal.example.test and set PROJECT_HOME=/opt/private or $HOME; host 192.0.2.10.`
	got := RedactPublicText(input)
	for _, unwanted := range []string{"secret-value", "alice@example.com", `C:\Users\alice`, "/srv/private", "src/private", "internal.example.test", "PROJECT_HOME", "$HOME", "192.0.2.10"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("public text leaked %q: %q", unwanted, got)
		}
	}
	for _, marker := range []string{"[REDACTED]", "<EMAIL>", "<PATH>", "<URL>", "<ENV>", "<IP>"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("expected %s marker in %q", marker, got)
		}
	}

	long := RedactPublicText(strings.Repeat("界", 500))
	if len(long) > 1200 || !utf8.ValidString(long) || !strings.HasSuffix(long, "...") {
		t.Fatalf("invalid public text truncation: bytes=%d valid=%t", len(long), utf8.ValidString(long))
	}
	if strings.ContainsRune(long, utf8.RuneError) {
		t.Fatalf("public text truncation introduced replacement characters: %q", long)
	}
}

func TestNormalizeErrorAndFingerprint(t *testing.T) {
	input := ErrorInput{
		Shell:    ShellPowerShell,
		Command:  `git checkout --token=abc123 C:\Users\alice\repo`,
		ExitCode: 129,
		Stderr:   `fatal: unknown option --token at C:\Users\alice\repo\file.txt request 123456`,
	}
	fingerprint := BuildFingerprint(input)
	if fingerprint.ErrorKind != "unknown_option" {
		t.Fatalf("unexpected error kind: %s", fingerprint.ErrorKind)
	}
	if strings.Contains(fingerprint.NormalizedError, "alice") || strings.Contains(fingerprint.NormalizedCommand, "abc123") {
		t.Fatalf("fingerprint contains private data: %+v", fingerprint)
	}
	if !strings.HasPrefix(fingerprint.ID, "cp1-") {
		t.Fatalf("unexpected fingerprint id: %s", fingerprint.ID)
	}
}

func TestFirstToken(t *testing.T) {
	tests := map[string]string{
		`git status`: "git",
		`& "C:\Program Files\tool.exe" --version`: `C:\Program Files\tool.exe`,
		`'quoted tool' --flag`:                    "quoted tool",
	}
	for input, expected := range tests {
		if got := FirstToken(input); got != expected {
			t.Errorf("FirstToken(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNormalizeToolDoesNotKeepLocalPath(t *testing.T) {
	if got := NormalizeTool(`/home/alice/bin/custom-tool`); got != "custom-tool" {
		t.Fatalf("unexpected normalized tool: %q", got)
	}
	if got := NormalizeTool(`tool name`); got != "<custom>" {
		t.Fatalf("unsafe tool was not collapsed: %q", got)
	}
}

func TestRunPreflightEmptyCommand(t *testing.T) {
	result := RunPreflight(PreflightOptions{Shell: ShellBash})
	if result.Status != "failed" || result.Diagnostics[0].Code != "empty_command" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunPreflightBash(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash is not installed")
	}
	valid := RunPreflight(PreflightOptions{Shell: ShellBash, Command: "printf '%s\\n' ok"})
	if valid.Syntax != "passed" {
		t.Fatalf("valid command was not parsed: %+v", valid)
	}
	invalid := RunPreflight(PreflightOptions{Shell: ShellBash, Command: "if true; then"})
	if invalid.Status != "failed" || invalid.Syntax != "failed" {
		t.Fatalf("invalid command was not rejected: %+v", invalid)
	}
}
