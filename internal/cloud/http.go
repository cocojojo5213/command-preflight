package cloud

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	Store       *Store
	AllowReport bool
	ReportToken string
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.index)
	mux.HandleFunc("/healthz", server.health)
	mux.HandleFunc("/v1/knowledge/", server.knowledge)
	return maxBody(mux, 64*1024)
}

func (server *Server) index(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		methodNotAllowed(writer)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]interface{}{
		"service":           "command-preflight-knowledge",
		"status":            "ok",
		"access":            "read-only-lookups",
		"reporting_enabled": server.AllowReport,
		"privacy":           "Lookups use only public cp1 fingerprint IDs; commands, paths, environment variables, and terminal output are never accepted.",
		"health":            "/healthz",
		"lookup":            "/v1/knowledge/{fingerprint_id}",
		"source":            "https://github.com/cocojojo5213/command-preflight",
	})
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		methodNotAllowed(writer)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]interface{}{
		"status":            "ok",
		"service":           "command-preflight-knowledge",
		"mode":              "offline-by-default",
		"reporting_enabled": server.AllowReport,
	})
}

func (server *Server) knowledge(writer http.ResponseWriter, request *http.Request) {
	id := strings.TrimPrefix(request.URL.Path, "/v1/knowledge/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid fingerprint id"})
		return
	}
	switch request.Method {
	case http.MethodGet, http.MethodHead:
		entry, ok := server.Store.Lookup(id)
		if !ok {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "knowledge entry not found"})
			return
		}
		writeJSON(writer, http.StatusOK, entry)
	case http.MethodPut:
		if !server.AllowReport || !server.authorized(request) {
			writeJSON(writer, http.StatusForbidden, map[string]string{"error": "reporting is disabled or unauthorized"})
			return
		}
		var entry Entry
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid entry", "detail": err.Error()})
			return
		}
		if entry.Fingerprint.ID != id {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "path and fingerprint id differ"})
			return
		}
		// The server owns the timestamp; clients must not forge freshness metadata.
		entry.UpdatedAt = time.Now().UTC()
		if err := server.Store.Upsert(entry); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(writer, http.StatusAccepted, entry)
	default:
		methodNotAllowed(writer)
	}
}

func (server *Server) authorized(request *http.Request) bool {
	return server.ReportToken != "" && request.Header.Get("Authorization") == "Bearer "+server.ReportToken
}

func maxBody(handler http.Handler, bytes int64) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(writer, request.Body, bytes)
		handler.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func methodNotAllowed(writer http.ResponseWriter) {
	writeJSON(writer, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func AddressError(address string, err error) error {
	return fmt.Errorf("listen %s: %w", address, err)
}
