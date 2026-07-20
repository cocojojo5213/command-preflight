package cloud

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/core"
)

type PublicFingerprint struct {
	Version   string     `json:"version"`
	ID        string     `json:"id"`
	Shell     core.Shell `json:"shell"`
	Tool      string     `json:"tool,omitempty"`
	ErrorKind string     `json:"error_kind"`
	ExitCode  int        `json:"exit_code"`
}

type Fix struct {
	ID           string     `json:"id"`
	Summary      string     `json:"summary"`
	Verification string     `json:"verification"`
	Shell        core.Shell `json:"shell,omitempty"`
	ToolVersion  string     `json:"tool_version,omitempty"`
	Source       string     `json:"source,omitempty"`
	Confidence   float64    `json:"confidence"`
	Verified     bool       `json:"verified"`
}

type Entry struct {
	Fingerprint PublicFingerprint `json:"fingerprint"`
	Fixes       []Fix             `json:"fixes,omitempty"`
	ReportCount int               `json:"report_count"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

var fingerprintID = regexp.MustCompile(`^cp1-[a-f0-9]{20}$`)

func PublicFromCore(input core.ErrorFingerprint) PublicFingerprint {
	return PublicFingerprint{
		Version:   input.Version,
		ID:        input.ID,
		Shell:     input.Shell,
		Tool:      input.Tool,
		ErrorKind: input.ErrorKind,
		ExitCode:  input.ExitCode,
	}
}

func (fingerprint PublicFingerprint) Validate() error {
	if !fingerprintID.MatchString(fingerprint.ID) {
		return fmt.Errorf("invalid fingerprint id")
	}
	if fingerprint.Version != "v1" {
		return fmt.Errorf("unsupported fingerprint version %q", fingerprint.Version)
	}
	if fingerprint.Shell == "" || fingerprint.ErrorKind == "" {
		return fmt.Errorf("shell and error_kind are required")
	}
	if fingerprint.Tool != "" && fingerprint.Tool != core.NormalizeTool(fingerprint.Tool) {
		return fmt.Errorf("tool contains unsafe characters")
	}
	return nil
}
