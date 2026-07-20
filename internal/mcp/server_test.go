package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/cloud"
	"github.com/cocojojo5213/command-preflight/internal/core"
)

func TestServeWithKnowledgeLookup(t *testing.T) {
	entry := cloud.Entry{Fingerprint: cloud.PublicFingerprint{
		Version:   "v1",
		ID:        "cp1-0123456789abcdef0123",
		Shell:     core.ShellBash,
		Tool:      "git",
		ErrorKind: "unknown_option",
		ExitCode:  129,
	}}
	knowledgeServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/knowledge/"+entry.Fingerprint.ID {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(entry)
	}))
	defer knowledgeServer.Close()

	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"lookup_fingerprint","arguments":{"fingerprint_id":"cp1-0123456789abcdef0123"}}}`,
	}, "\n"))
	var output bytes.Buffer
	if err := ServeWithConfig(input, &output, Config{KnowledgeURL: knowledgeServer.URL}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `"lookup_fingerprint"`) {
		t.Fatalf("tool was not advertised: %s", text)
	}
	if !strings.Contains(text, `"found":true`) || !strings.Contains(text, entry.Fingerprint.ID) {
		t.Fatalf("lookup result missing: %s", text)
	}
}

func TestServeWithExplicitReportSubmission(t *testing.T) {
	var received cloud.ReportInput
	reportServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/reports" {
			t.Fatalf("unexpected report request: %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(writer).Encode(cloud.Report{
			ID:          "rpt-0123456789abcdef01234567",
			Fingerprint: received.Fingerprint,
			Fix:         received.Fix,
			Status:      cloud.ReportPending,
			ReceivedAt:  time.Now().UTC(),
		})
	}))
	defer reportServer.Close()

	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"submit_resolution","arguments":{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"powershell","tool":"git","error_kind":"unknown_option","exit_code":129},"summary":"Use the supported flag.","verification":"Check local help before retrying.","verified":true}}}`,
	}, "\n"))
	var output bytes.Buffer
	if err := ServeWithConfig(input, &output, Config{ReportURL: reportServer.URL, Reporting: true}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `"submit_resolution"`) || !strings.Contains(text, `"submitted":true`) {
		t.Fatalf("report tool result missing: %s", text)
	}
	if received.Fingerprint.ID != "cp1-0123456789abcdef0123" || received.Fix.Summary != "Use the supported flag." {
		t.Fatalf("unexpected submitted input: %+v", received)
	}
}
