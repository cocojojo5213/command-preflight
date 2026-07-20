package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/core"
)

// Client performs opt-in lookups and constrained report submissions against a
// knowledge service. It never sends command text, output, environment data, or
// local paths.
type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	ReportToken string
}

func NewClient(baseURL string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("knowledge URL is empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("knowledge URL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("knowledge URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("knowledge URL must not contain credentials, query, or fragment")
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 3 * time.Second},
	}, nil
}

// Lookup returns false without an error when the server has no matching entry.
func (client *Client) Lookup(ctx context.Context, id string) (Entry, bool, error) {
	if client == nil {
		return Entry{}, false, fmt.Errorf("knowledge client is nil")
	}
	if !fingerprintID.MatchString(id) {
		return Entry{}, false, fmt.Errorf("invalid fingerprint id")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/v1/knowledge/"+url.PathEscape(id), nil)
	if err != nil {
		return Entry{}, false, fmt.Errorf("create knowledge request: %w", err)
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Entry{}, false, fmt.Errorf("knowledge lookup: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return Entry{}, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return Entry{}, false, fmt.Errorf("knowledge lookup returned HTTP %d", response.StatusCode)
	}
	var entry Entry
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	if err := decoder.Decode(&entry); err != nil {
		return Entry{}, false, fmt.Errorf("decode knowledge entry: %w", err)
	}
	if err := entry.Fingerprint.Validate(); err != nil {
		return Entry{}, false, fmt.Errorf("invalid knowledge entry: %w", err)
	}
	if entry.Fingerprint.ID != id {
		return Entry{}, false, fmt.Errorf("knowledge entry id mismatch")
	}
	return entry, true, nil
}

// SubmitReport sends only a public fingerprint and short, locally redacted
// explanatory text. The MCP adapter exposes it only after explicit opt-in.
func (client *Client) SubmitReport(ctx context.Context, input ReportInput) (Report, error) {
	if client == nil {
		return Report{}, fmt.Errorf("knowledge client is nil")
	}
	input.Fix.Summary = core.RedactPublicText(input.Fix.Summary)
	input.Fix.Verification = core.RedactPublicText(input.Fix.Verification)
	input.Fix.ToolVersion = core.RedactPublicText(input.Fix.ToolVersion)
	if err := input.Validate(); err != nil {
		return Report{}, fmt.Errorf("invalid report: %w", err)
	}
	body, err := json.Marshal(input)
	if err != nil {
		return Report{}, fmt.Errorf("encode report: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/v1/reports", bytes.NewReader(body))
	if err != nil {
		return Report{}, fmt.Errorf("create report request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if client.ReportToken != "" {
		request.Header.Set("Authorization", "Bearer "+client.ReportToken)
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return Report{}, fmt.Errorf("report upload: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {
		var detail struct {
			Error  string `json:"error"`
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 16*1024)).Decode(&detail)
		message := detail.Error
		if detail.Detail != "" {
			message += ": " + detail.Detail
		}
		if message == "" {
			message = response.Status
		}
		return Report{}, fmt.Errorf("report upload returned HTTP %d: %s", response.StatusCode, message)
	}
	var report Report
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	if err := decoder.Decode(&report); err != nil {
		return Report{}, fmt.Errorf("decode report response: %w", err)
	}
	if err := report.Validate(); err != nil {
		return Report{}, fmt.Errorf("invalid report response: %w", err)
	}
	return report, nil
}
