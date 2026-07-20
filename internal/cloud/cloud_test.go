package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
