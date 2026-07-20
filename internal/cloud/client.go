package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client performs opt-in, read-only lookups against a knowledge service.
// It never sends command text, output, environment data, or local paths.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
