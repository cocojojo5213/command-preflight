package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/core"
)

func testEntry() Entry {
	return Entry{
		Fingerprint: PublicFingerprint{
			Version:   "v1",
			ID:        "cp1-0123456789abcdef0123",
			Shell:     core.ShellPowerShell,
			Tool:      "git",
			ErrorKind: "unknown_option",
			ExitCode:  129,
		},
	}
}

func TestStoreAndLookup(t *testing.T) {
	store, err := OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	entry := testEntry()
	if err := store.Upsert(entry); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Lookup(entry.Fingerprint.ID)
	if !ok || got.Fingerprint.ErrorKind != entry.Fingerprint.ErrorKind {
		t.Fatalf("lookup failed: %+v %t", got, ok)
	}
}

func TestReportQueuePersistsAcrossStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "knowledge.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	report, _, err := store.SubmitReport(ReportInput{
		Fingerprint: testEntry().Fingerprint,
		Fix: Fix{
			Summary:      "Use the supported flag.",
			Verification: "Confirm it in local help.",
			Verified:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.GetReport(report.ID)
	if !ok || got.Status != ReportPending {
		t.Fatalf("reopened report = %+v, found=%t", got, ok)
	}
}

func TestDeleteReportRollsBackOnPersistFailure(t *testing.T) {
	store := mustTestStore(t)
	report, _, err := store.SubmitReport(ReportInput{
		Fingerprint: testEntry().Fingerprint,
		Fix: Fix{
			Summary:      "Use the supported flag.",
			Verification: "Confirm it in local help.",
			Verified:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	blockedPath := filepath.Join(t.TempDir(), "directory-instead-of-file")
	if err := os.Mkdir(blockedPath, 0700); err != nil {
		t.Fatal(err)
	}
	store.path = blockedPath
	if err := store.DeleteReport(report.ID); err == nil {
		t.Fatal("expected delete persistence error")
	}
	if _, ok := store.GetReport(report.ID); !ok {
		t.Fatal("report was not restored after persistence failure")
	}
}

func TestPruneReportsRemovesOnlyOldTerminalReports(t *testing.T) {
	store := mustTestStore(t)
	makeInput := func(id, summary string) ReportInput {
		fingerprint := testEntry().Fingerprint
		fingerprint.ID = id
		return ReportInput{
			Fingerprint: fingerprint,
			Fix: Fix{
				Summary:      summary,
				Verification: "Confirm the supported flag in local help.",
				Verified:     true,
			},
		}
	}
	rejected, _, err := store.SubmitReport(makeInput("cp1-aaaaaaaaaaaaaaaaaaaa", "Use the rejected proposal."))
	if err != nil {
		t.Fatal(err)
	}
	approved, _, err := store.SubmitReport(makeInput("cp1-bbbbbbbbbbbbbbbbbbbb", "Use the approved proposal."))
	if err != nil {
		t.Fatal(err)
	}
	pending, _, err := store.SubmitReport(makeInput("cp1-cccccccccccccccccccc", "Keep the pending proposal."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewReports([]ReviewAction{
		{ID: rejected.ID, Decision: "reject"},
		{ID: approved.ID, Decision: "approve"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishReports([]string{approved.ID}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	store.mu.Lock()
	for _, id := range []string{rejected.ID, approved.ID, pending.ID} {
		report := store.reports[id]
		report.ReceivedAt = old
		store.reports[id] = report
	}
	store.mu.Unlock()

	removed, err := store.PruneReports(time.Now().UTC().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed reports = %d, want 2", removed)
	}
	if _, ok := store.GetReport(rejected.ID); ok {
		t.Fatal("rejected report was not pruned")
	}
	if _, ok := store.GetReport(approved.ID); ok {
		t.Fatal("published report was not pruned")
	}
	if _, ok := store.GetReport(pending.ID); !ok {
		t.Fatal("pending report was pruned")
	}
	if _, ok := store.Lookup(approved.Fingerprint.ID); !ok {
		t.Fatal("published knowledge was removed with queue history")
	}
}

func TestPruneStaleReportsRemovesOnlyOldUnresolvedReports(t *testing.T) {
	store := mustTestStore(t)
	makeInput := func(id, summary string) ReportInput {
		fingerprint := testEntry().Fingerprint
		fingerprint.ID = id
		return ReportInput{
			Fingerprint: fingerprint,
			Fix: Fix{
				Summary:      summary,
				Verification: "Confirm the supported flag in local help.",
				Verified:     true,
			},
		}
	}
	pending, _, err := store.SubmitReport(makeInput("cp1-aaaaaaaaaaaaaaaaaaaa", "Keep pending."))
	if err != nil {
		t.Fatal(err)
	}
	held, _, err := store.SubmitReport(makeInput("cp1-bbbbbbbbbbbbbbbbbbbb", "Keep held."))
	if err != nil {
		t.Fatal(err)
	}
	approved, _, err := store.SubmitReport(makeInput("cp1-cccccccccccccccccccc", "Keep approved."))
	if err != nil {
		t.Fatal(err)
	}
	rejected, _, err := store.SubmitReport(makeInput("cp1-dddddddddddddddddddd", "Keep rejected."))
	if err != nil {
		t.Fatal(err)
	}
	published, _, err := store.SubmitReport(makeInput("cp1-eeeeeeeeeeeeeeeeeeee", "Keep published."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewReports([]ReviewAction{
		{ID: held.ID, Decision: "hold"},
		{ID: approved.ID, Decision: "approve"},
		{ID: rejected.ID, Decision: "reject"},
		{ID: published.ID, Decision: "approve"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishReports([]string{published.ID}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-48 * time.Hour)
	store.mu.Lock()
	for _, id := range []string{pending.ID, held.ID, approved.ID, rejected.ID, published.ID} {
		report := store.reports[id]
		report.ReceivedAt = old
		store.reports[id] = report
	}
	store.mu.Unlock()

	removed, err := store.PruneStaleReports(time.Now().UTC().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("removed reports = %d, want 3", removed)
	}
	for _, id := range []string{pending.ID, held.ID, approved.ID} {
		if _, ok := store.GetReport(id); ok {
			t.Fatalf("stale report %s was not pruned", id)
		}
	}
	for _, id := range []string{rejected.ID, published.ID} {
		if _, ok := store.GetReport(id); !ok {
			t.Fatalf("terminal report %s was pruned", id)
		}
	}
	if _, ok := store.Lookup(published.Fingerprint.ID); !ok {
		t.Fatal("published knowledge was removed with stale queue history")
	}
}

func TestServerLookupAndReportAuth(t *testing.T) {
	store, _ := OpenStore("")
	_ = store.Upsert(testEntry())
	server := &Server{Store: store, AllowReport: true, ReportToken: "test-token"}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	response, err := http.Get(testServer.URL + "/v1/knowledge/cp1-0123456789abcdef0123")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d", response.StatusCode)
	}
	response.Body.Close()

	request, _ := http.NewRequest(http.MethodPut, testServer.URL+"/v1/knowledge/cp1-0123456789abcdef0123", nil)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthorized report status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestServerIndex(t *testing.T) {
	server := &Server{Store: mustTestStore(t)}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	response, err := http.Get(testServer.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("index status = %d", response.StatusCode)
	}
	if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("content type options header = %q", got)
	}
	var body struct {
		Service          string `json:"service"`
		Access           string `json:"access"`
		ReportingEnabled bool   `json:"reporting_enabled"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Service != "command-preflight-knowledge" || body.Access != "public-lookups; moderated-report-queue" || body.ReportingEnabled {
		t.Fatalf("unexpected index response: %+v", body)
	}

	head, err := http.Head(testServer.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer head.Body.Close()
	if head.StatusCode != http.StatusOK {
		t.Fatalf("health HEAD status = %d", head.StatusCode)
	}

	unknown, err := http.Get(testServer.URL + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	defer unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path status = %d", unknown.StatusCode)
	}
}

func TestReportQueueReviewAndPublish(t *testing.T) {
	store := mustTestStore(t)
	server := &Server{Store: store, AllowReport: true, ReportToken: "admin-token"}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	body := `{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"powershell","tool":"git","error_kind":"unknown_option","exit_code":129},"fix":{"summary":"Use the flag supported by the local command help.","verification":"Run the command help and confirm the replacement flag before retrying.","confidence":0.8,"verified":true}}`
	request, err := http.NewRequest(http.MethodPost, testServer.URL+"/v1/reports", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("submit status = %d", response.StatusCode)
	}
	var report Report
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if report.Status != ReportPending || report.ID == "" || report.Fix.Verified {
		t.Fatalf("unexpected queued report: %+v", report)
	}

	lookup, err := http.Get(testServer.URL + "/v1/knowledge/" + report.Fingerprint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.StatusCode != http.StatusNotFound {
		lookup.Body.Close()
		t.Fatalf("unreviewed lookup status = %d", lookup.StatusCode)
	}
	lookup.Body.Close()

	adminList, err := http.NewRequest(http.MethodGet, testServer.URL+"/v1/admin/reports?status=pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	adminList.Header.Set("Authorization", "Bearer admin-token")
	response, err = http.DefaultClient.Do(adminList)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("admin list status = %d", response.StatusCode)
	}
	response.Body.Close()

	reviewBody := `{"reviews":[{"id":"` + report.ID + `","decision":"approve","reason":"The fix is short, shell-specific, and safe."}]}`
	reviewRequest, _ := http.NewRequest(http.MethodPost, testServer.URL+"/v1/admin/reports/review", bytes.NewBufferString(reviewBody))
	reviewRequest.Header.Set("Authorization", "Bearer admin-token")
	reviewRequest.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(reviewRequest)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("review status = %d", response.StatusCode)
	}
	response.Body.Close()

	publishRequest, _ := http.NewRequest(http.MethodPost, testServer.URL+"/v1/admin/reports/publish", bytes.NewBufferString(`{}`))
	publishRequest.Header.Set("Authorization", "Bearer admin-token")
	publishRequest.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(publishRequest)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("publish status = %d", response.StatusCode)
	}
	response.Body.Close()

	lookup, err = http.Get(testServer.URL + "/v1/knowledge/" + report.Fingerprint.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lookup.Body.Close()
	if lookup.StatusCode != http.StatusOK {
		t.Fatalf("published lookup status = %d", lookup.StatusCode)
	}
	var entry Entry
	if err := json.NewDecoder(lookup.Body).Decode(&entry); err != nil {
		t.Fatal(err)
	}
	if len(entry.Fixes) != 1 || !entry.Fixes[0].Verified || entry.Fixes[0].Source != "community-reviewed" {
		t.Fatalf("unexpected published entry: %+v", entry)
	}

	duplicateRequest, _ := http.NewRequest(http.MethodPost, testServer.URL+"/v1/reports", strings.NewReader(body))
	response, err = http.DefaultClient.Do(duplicateRequest)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		t.Fatalf("duplicate submit status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestReportEndpointRejectsUnknownFieldsAndAdminAuth(t *testing.T) {
	server := &Server{Store: mustTestStore(t), AllowReport: true, ReportToken: "admin-token"}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	request, _ := http.NewRequest(http.MethodPost, testServer.URL+"/v1/reports", strings.NewReader(`{"command":"secret"}`))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		response.Body.Close()
		t.Fatalf("unknown field status = %d", response.StatusCode)
	}
	response.Body.Close()

	unverifiedBody := `{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"bash","tool":"git","error_kind":"unknown_option","exit_code":129},"fix":{"summary":"Use local help.","verification":"Confirm the supported flag.","verified":false}}`
	unverified, _ := http.NewRequest(http.MethodPost, testServer.URL+"/v1/reports", strings.NewReader(unverifiedBody))
	response, err = http.DefaultClient.Do(unverified)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest {
		response.Body.Close()
		t.Fatalf("unverified report status = %d", response.StatusCode)
	}
	response.Body.Close()

	admin, _ := http.NewRequest(http.MethodGet, testServer.URL+"/v1/admin/reports", nil)
	response, err = http.DefaultClient.Do(admin)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		response.Body.Close()
		t.Fatalf("unauthorized admin status = %d", response.StatusCode)
	}
	response.Body.Close()

	proxied, _ := http.NewRequest(http.MethodGet, testServer.URL+"/v1/admin/reports", nil)
	proxied.Header.Set("Authorization", "Bearer admin-token")
	proxied.Header.Set("X-Forwarded-For", "203.0.113.10")
	response, err = http.DefaultClient.Do(proxied)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		response.Body.Close()
		t.Fatalf("proxied admin status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestAdminDeleteReport(t *testing.T) {
	store := mustTestStore(t)
	report, _, err := store.SubmitReport(ReportInput{
		Fingerprint: testEntry().Fingerprint,
		Fix: Fix{
			Summary:      "Use local help.",
			Verification: "Confirm the supported flag.",
			Verified:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: store, AllowReport: true, ReportToken: "admin-token"}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	unauthorized, _ := http.NewRequest(http.MethodDelete, testServer.URL+"/v1/admin/reports/"+report.ID, nil)
	response, err := http.DefaultClient.Do(unauthorized)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthorized delete status = %d", response.StatusCode)
	}

	request, _ := http.NewRequest(http.MethodDelete, testServer.URL+"/v1/admin/reports/"+report.ID, nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", response.StatusCode)
	}

	missing, _ := http.NewRequest(http.MethodDelete, testServer.URL+"/v1/admin/reports/"+report.ID, nil)
	missing.Header.Set("Authorization", "Bearer admin-token")
	response, err = http.DefaultClient.Do(missing)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete status = %d", response.StatusCode)
	}
}

func TestReportEndpointGlobalRateLimit(t *testing.T) {
	server := &Server{Store: mustTestStore(t), AllowReport: true, ReportToken: "admin-token", ReportsPerMinute: 1}
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()
	body := `{"fingerprint":{"version":"v1","id":"cp1-0123456789abcdef0123","shell":"bash","tool":"git","error_kind":"unknown_option","exit_code":129},"fix":{"summary":"Use local help.","verification":"Confirm the supported flag.","verified":true}}`
	first, err := http.Post(testServer.URL+"/v1/reports", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first report status = %d", first.StatusCode)
	}
	second, err := http.Post(testServer.URL+"/v1/reports", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") != "60" {
		t.Fatalf("rate limited status = %d, retry-after = %q", second.StatusCode, second.Header.Get("Retry-After"))
	}
}

func TestReportRateLimitResetsAtUtcDayBoundary(t *testing.T) {
	server := &Server{ReportsPerMinute: 100, ReportsPerDay: 1}
	firstAt := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	if allowed, _ := server.allowReportRequest(firstAt); !allowed {
		t.Fatal("first report was unexpectedly limited")
	}
	if allowed, retry := server.allowReportRequest(firstAt.Add(time.Minute)); allowed || retry <= 11*time.Hour {
		t.Fatalf("daily limit result = allowed %t, retry %s", allowed, retry)
	}
	if allowed, _ := server.allowReportRequest(firstAt.Add(24 * time.Hour)); !allowed {
		t.Fatal("daily limit did not reset at UTC midnight")
	}
}

func TestClientSubmitReport(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/reports" {
			t.Fatalf("unexpected report request: %s %s", request.Method, request.URL.Path)
		}
		var input ReportInput
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(input.Fix.Summary, "alice@example.com") {
			t.Fatal("client sent unredacted report text")
		}
		_ = json.NewEncoder(writer).Encode(Report{
			ID:          "rpt-0123456789abcdef01234567",
			Fingerprint: input.Fingerprint,
			Fix:         input.Fix,
			Status:      ReportPending,
			ReceivedAt:  time.Now().UTC(),
		})
	}))
	defer testServer.Close()
	client, err := NewClient(testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	report, err := client.SubmitReport(context.Background(), ReportInput{
		Fingerprint: testEntry().Fingerprint,
		Fix: Fix{
			Summary:      "Contact alice@example.com if this fails.",
			Verification: "Run the local help command.",
			Verified:     true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != ReportPending {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func mustTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestClientLookup(t *testing.T) {
	entry := testEntry()
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/knowledge/"+entry.Fingerprint.ID {
			t.Fatalf("lookup path = %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(entry)
	}))
	defer testServer.Close()

	client, err := NewClient(testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := client.Lookup(context.Background(), entry.Fingerprint.ID)
	if err != nil || !found || got.Fingerprint.ID != entry.Fingerprint.ID {
		t.Fatalf("lookup = %+v, found=%t, err=%v", got, found, err)
	}
}

func TestClientLookupNotFound(t *testing.T) {
	testServer := httptest.NewServer(http.NotFoundHandler())
	defer testServer.Close()
	client, err := NewClient(testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := client.Lookup(context.Background(), "cp1-0123456789abcdef0123"); err != nil || found {
		t.Fatalf("not found lookup = found=%t, err=%v", found, err)
	}
}

func TestClientRejectsSecretURL(t *testing.T) {
	if _, err := NewClient("https://user:pass@example.test/knowledge?token=secret"); err == nil {
		t.Fatal("expected URL validation error")
	}
}
