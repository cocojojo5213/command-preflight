package core

const Version = "0.1.0"

// Shell identifies the parser and command conventions used for a preflight.
type Shell string

const (
	ShellPowerShell Shell = "powershell"
	ShellBash       Shell = "bash"
	ShellSh         Shell = "sh"
	ShellCmd        Shell = "cmd"
)

type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Subject  string `json:"subject,omitempty"`
}

type Result struct {
	Version            string            `json:"version"`
	Status             string            `json:"status"`
	Shell              Shell             `json:"shell"`
	CommandRedacted    string            `json:"command_redacted,omitempty"`
	CWD                string            `json:"cwd,omitempty"`
	Executable         string            `json:"executable,omitempty"`
	Syntax             string            `json:"syntax"`
	ResolutionComplete bool              `json:"resolution_complete"`
	Risk               string            `json:"risk"`
	Diagnostics        []Diagnostic      `json:"diagnostics"`
	Fingerprint        *ErrorFingerprint `json:"fingerprint,omitempty"`
}

type PreflightOptions struct {
	Shell   Shell
	Command string
	CWD     string
}

type ErrorInput struct {
	Shell    Shell
	Command  string
	ExitCode int
	Stderr   string
	Stdout   string
}

type ErrorFingerprint struct {
	Version           string `json:"version"`
	ID                string `json:"id"`
	Shell             Shell  `json:"shell"`
	Tool              string `json:"tool,omitempty"`
	ErrorKind         string `json:"error_kind"`
	ExitCode          int    `json:"exit_code"`
	NormalizedError   string `json:"normalized_error"`
	NormalizedCommand string `json:"normalized_command,omitempty"`
}
