package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexSkillDirectory(t *testing.T) {
	home := filepath.FromSlash("/home/tester")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("COMMAND_PREFLIGHT_CODEX_SKILL_DIR", "")
	if got, want := codexSkillDirectory(home), filepath.Join(home, ".codex", "skills", "command-preflight"); got != want {
		t.Fatalf("default Codex skill directory = %q, want %q", got, want)
	}
	t.Setenv("CODEX_HOME", filepath.FromSlash("/custom/codex"))
	if got, want := codexSkillDirectory(home), filepath.FromSlash("/custom/codex/skills/command-preflight"); got != want {
		t.Fatalf("CODEX_HOME skill directory = %q, want %q", got, want)
	}
	t.Setenv("COMMAND_PREFLIGHT_CODEX_SKILL_DIR", filepath.FromSlash("/custom/skills"))
	if got, want := codexSkillDirectory(home), filepath.FromSlash("/custom/skills/command-preflight"); got != want {
		t.Fatalf("override skill directory = %q, want %q", got, want)
	}
}

func TestSetupCommandOffline(t *testing.T) {
	executable := `/opt/Command Preflight/command-preflight`
	want := []string{"mcp", "add", "command-preflight", "--", executable, "mcp"}
	if got := setupCommand("codex", executable, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("codex setup command = %#v, want %#v", got, want)
	}

	want = []string{"mcp", "add", "--scope", "user", "command-preflight", "--", executable, "mcp"}
	if got := setupCommand("claude", executable, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("claude setup command = %#v, want %#v", got, want)
	}
}

func TestSetupCommandWithKnowledgeURL(t *testing.T) {
	executable := `/opt/command-preflight`
	url := "https://preflight.52131415.xyz"
	want := []string{"mcp", "add", "--env", "COMMAND_PREFLIGHT_KNOWLEDGE_URL=" + url, "command-preflight", "--", executable, "mcp"}
	if got := setupCommand("codex", executable, url); !reflect.DeepEqual(got, want) {
		t.Fatalf("codex connected setup command = %#v, want %#v", got, want)
	}

	want = []string{"mcp", "add", "--scope", "user", "command-preflight", "--env", "COMMAND_PREFLIGHT_KNOWLEDGE_URL=" + url, "--", executable, "mcp"}
	if got := setupCommand("claude", executable, url); !reflect.DeepEqual(got, want) {
		t.Fatalf("claude connected setup command = %#v, want %#v", got, want)
	}
}

func TestSetupCommandWithReporting(t *testing.T) {
	executable := `/opt/command-preflight`
	got := setupCommandWithOptions("codex", executable, "https://preflight.example", "https://preflight.example", "submit-token", true)
	want := []string{
		"mcp", "add",
		"--env", "COMMAND_PREFLIGHT_KNOWLEDGE_URL=https://preflight.example",
		"--env", "COMMAND_PREFLIGHT_REPORTING=on",
		"--env", "COMMAND_PREFLIGHT_REPORT_URL=https://preflight.example",
		"--env", "COMMAND_PREFLIGHT_REPORT_SUBMIT_TOKEN=submit-token",
		"command-preflight", "--", executable, "mcp",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("codex reporting setup command = %#v, want %#v", got, want)
	}
}
