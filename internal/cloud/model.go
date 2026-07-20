package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
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

// ReportInput is the only payload accepted from a client contribution. It
// deliberately has no command, path, environment, or terminal-output field.
type ReportInput struct {
	Fingerprint PublicFingerprint `json:"fingerprint"`
	Fix         Fix               `json:"fix"`
}

const (
	ReportPending   = "pending"
	ReportHeld      = "held"
	ReportApproved  = "approved"
	ReportRejected  = "rejected"
	ReportPublished = "published"
)

// Report is an operator-facing queue item. It is never returned by the
// public knowledge lookup endpoint.
type Report struct {
	ID                string            `json:"id"`
	Fingerprint       PublicFingerprint `json:"fingerprint"`
	Fix               Fix               `json:"fix"`
	ClaimedVerified   bool              `json:"claimed_verified"`
	ClaimedConfidence float64           `json:"claimed_confidence"`
	Status            string            `json:"status"`
	ReceivedAt        time.Time         `json:"received_at"`
	ReviewedAt        *time.Time        `json:"reviewed_at,omitempty"`
	DecisionReason    string            `json:"decision_reason,omitempty"`
	PublishedAt       *time.Time        `json:"published_at,omitempty"`
}

type ReviewAction struct {
	ID           string   `json:"id"`
	Decision     string   `json:"decision"`
	Reason       string   `json:"reason,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Verification string   `json:"verification,omitempty"`
	Confidence   *float64 `json:"confidence,omitempty"`
}

type ReviewBatch struct {
	Reviews []ReviewAction `json:"reviews"`
}

type PublishRequest struct {
	IDs []string `json:"ids,omitempty"`
}

var fingerprintID = regexp.MustCompile(`^cp1-[a-f0-9]{20}$`)
var reportID = regexp.MustCompile(`^rpt-[a-f0-9]{24}$`)
var fixID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,95}$`)
var errorKind = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

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
	switch fingerprint.Shell {
	case core.ShellPowerShell, core.ShellBash, core.ShellSh, core.ShellCmd:
	default:
		return fmt.Errorf("unsupported shell %q", fingerprint.Shell)
	}
	if !errorKind.MatchString(fingerprint.ErrorKind) {
		return fmt.Errorf("invalid error_kind")
	}
	if fingerprint.Tool != "" && fingerprint.Tool != core.NormalizeTool(fingerprint.Tool) {
		if fingerprint.Tool != "<unknown>" && fingerprint.Tool != "<custom>" {
			return fmt.Errorf("tool contains unsafe characters")
		}
	}
	return nil
}

func (input ReportInput) Validate() error {
	if err := input.Fingerprint.Validate(); err != nil {
		return fmt.Errorf("fingerprint: %w", err)
	}
	if strings.TrimSpace(input.Fix.Summary) == "" {
		return fmt.Errorf("fix summary is required")
	}
	if strings.TrimSpace(input.Fix.Verification) == "" {
		return fmt.Errorf("fix verification is required")
	}
	if len(input.Fix.Summary) > 1200 || len(input.Fix.Verification) > 1200 {
		return fmt.Errorf("fix text is too long")
	}
	if input.Fix.Shell != "" {
		switch input.Fix.Shell {
		case core.ShellPowerShell, core.ShellBash, core.ShellSh, core.ShellCmd:
		default:
			return fmt.Errorf("unsupported fix shell %q", input.Fix.Shell)
		}
	}
	if len(input.Fix.ToolVersion) > 120 {
		return fmt.Errorf("tool_version is too long")
	}
	if input.Fix.ID != "" && !fixID.MatchString(input.Fix.ID) {
		return fmt.Errorf("invalid fix id")
	}
	if input.Fix.Confidence < 0 || input.Fix.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}

func (report Report) Validate() error {
	if !reportID.MatchString(report.ID) {
		return fmt.Errorf("invalid report id")
	}
	if report.Status != ReportPending && report.Status != ReportHeld && report.Status != ReportApproved && report.Status != ReportRejected && report.Status != ReportPublished {
		return fmt.Errorf("invalid report status")
	}
	if err := (ReportInput{Fingerprint: report.Fingerprint, Fix: report.Fix}).Validate(); err != nil {
		return err
	}
	if report.ReceivedAt.IsZero() {
		return fmt.Errorf("report received_at is required")
	}
	if report.ClaimedConfidence < 0 || report.ClaimedConfidence > 1 {
		return fmt.Errorf("claimed_confidence must be between 0 and 1")
	}
	return nil
}

func (action ReviewAction) Validate() error {
	if !reportID.MatchString(action.ID) {
		return fmt.Errorf("invalid report id")
	}
	switch strings.ToLower(strings.TrimSpace(action.Decision)) {
	case "approve", "approved", "reject", "rejected", "hold", "held":
	default:
		return fmt.Errorf("decision must be approve, reject, or hold")
	}
	if len(action.Reason) > 1200 || len(action.Summary) > 1200 || len(action.Verification) > 1200 {
		return fmt.Errorf("review text is too long")
	}
	if action.Confidence != nil && (*action.Confidence < 0 || *action.Confidence > 1) {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}

func normalizedDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "approve", "approved":
		return ReportApproved
	case "reject", "rejected":
		return ReportRejected
	case "hold", "held":
		return ReportHeld
	default:
		return ""
	}
}

func candidateFixID(input ReportInput) string {
	canonical := input.Fingerprint.ID + "|" + input.Fix.Summary + "|" + input.Fix.Verification
	digest := sha256.Sum256([]byte(canonical))
	return "community-" + hex.EncodeToString(digest[:])[:16]
}
