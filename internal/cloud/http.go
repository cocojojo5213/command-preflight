package cloud

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Store             *Store
	AllowReport       bool
	ReportToken       string // kept for backwards-compatible operator writes
	AdminToken        string // protects the moderation API
	ReportSubmitToken string // optional token for private deployments
	AllowProxiedAdmin bool   // opt-in for authenticated admin access through a reverse proxy
	ReportsPerMinute  int    // zero uses the privacy-preserving global default
	ReportsPerDay     int    // zero uses the UTC daily default; no client address is retained
	reportRateMu      sync.Mutex
	reportRateWindow  time.Time
	reportRateCount   int
	reportRateDay     time.Time
	reportDailyCount  int
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.index)
	mux.HandleFunc("/healthz", server.health)
	mux.HandleFunc("/v1/knowledge/", server.knowledge)
	mux.HandleFunc("/v1/reports", server.reports)
	mux.HandleFunc("/v1/admin/reports", server.adminReports)
	mux.HandleFunc("/v1/admin/reports/review", server.adminReview)
	mux.HandleFunc("/v1/admin/reports/publish", server.adminPublish)
	mux.HandleFunc("/v1/admin/reports/", server.adminReport)
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
		"access":            "public-lookups; moderated-report-queue",
		"reporting_enabled": server.AllowReport,
		"privacy":           "The application accepts only a public fingerprint and short pattern-redacted fix text; it has no raw-command, path, environment, or terminal-log fields.",
		"health":            "/healthz",
		"lookup":            "/v1/knowledge/{fingerprint_id}",
		"report":            "/v1/reports",
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
		"mode":              "lookup plus moderated queue",
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
		if !server.AllowReport || !server.authorizedAdmin(request) {
			writeJSON(writer, http.StatusForbidden, map[string]string{"error": "reporting is disabled or unauthorized"})
			return
		}
		var entry Entry
		if err := decodeJSON(request, &entry); err != nil {
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

func (server *Server) reports(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
		return
	}
	if !server.AllowReport || !server.authorizedSubmit(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "reporting is disabled or unauthorized"})
		return
	}
	allowed, retryAfter := server.allowReportRequest(time.Now().UTC())
	if !allowed {
		writer.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
		writeJSON(writer, http.StatusTooManyRequests, map[string]string{"error": "report rate limit exceeded"})
		return
	}
	var input ReportInput
	if err := decodeJSON(request, &input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid report", "detail": err.Error()})
		return
	}
	report, duplicate, err := server.Store.SubmitReport(input)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "report rejected", "detail": err.Error()})
		return
	}
	status := http.StatusAccepted
	if duplicate {
		status = http.StatusOK
	}
	writeJSON(writer, status, report)
}

func (server *Server) adminReports(writer http.ResponseWriter, request *http.Request) {
	if !server.authorizedAdmin(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "admin authorization required"})
		return
	}
	if request.Method != http.MethodGet {
		methodNotAllowed(writer)
		return
	}
	limit := 100
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 500 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 500"})
			return
		}
		limit = parsed
	}
	status := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("status")))
	if status != "" && status != "all" && status != ReportPending && status != ReportHeld && status != ReportApproved && status != ReportRejected && status != ReportPublished {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid report status"})
		return
	}
	reports := server.Store.ListReports(status, limit)
	writeJSON(writer, http.StatusOK, map[string]interface{}{"reports": reports, "count": len(reports)})
}

func (server *Server) adminReport(writer http.ResponseWriter, request *http.Request) {
	if !server.authorizedAdmin(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "admin authorization required"})
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/v1/admin/reports/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid report id"})
		return
	}
	switch request.Method {
	case http.MethodGet:
		report, ok := server.Store.GetReport(id)
		if !ok {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "report not found"})
			return
		}
		writeJSON(writer, http.StatusOK, report)
	case http.MethodDelete:
		err := server.Store.DeleteReport(id)
		if errors.Is(err, ErrReportNotFound) {
			writeJSON(writer, http.StatusNotFound, map[string]string{"error": "report not found"})
			return
		}
		if err != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": "delete report failed"})
			return
		}
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(writer)
	}
}

func (server *Server) adminReview(writer http.ResponseWriter, request *http.Request) {
	if !server.authorizedAdmin(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "admin authorization required"})
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
		return
	}
	var batch ReviewBatch
	if err := decodeJSON(request, &batch); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid review batch", "detail": err.Error()})
		return
	}
	reports, err := server.Store.ReviewReports(batch.Reviews)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "review failed", "detail": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]interface{}{"reports": reports, "count": len(reports)})
}

func (server *Server) adminPublish(writer http.ResponseWriter, request *http.Request) {
	if !server.authorizedAdmin(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "admin authorization required"})
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(writer)
		return
	}
	var input PublishRequest
	if err := decodeJSON(request, &input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "invalid publish request", "detail": err.Error()})
		return
	}
	entries, err := server.Store.PublishReports(input.IDs)
	if err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "publish failed", "detail": err.Error()})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]interface{}{"entries": entries, "count": len(entries)})
}

func (server *Server) authorizedAdmin(request *http.Request) bool {
	if !server.AllowProxiedAdmin && (request.Header.Get("CF-Connecting-IP") != "" || request.Header.Get("X-Forwarded-For") != "" || request.Header.Get("Forwarded") != "") {
		return false
	}
	token := server.AdminToken
	if token == "" {
		token = server.ReportToken
	}
	return bearerMatches(request, token)
}

func (server *Server) authorizedSubmit(request *http.Request) bool {
	if server.ReportSubmitToken == "" {
		return true
	}
	return bearerMatches(request, server.ReportSubmitToken)
}

func (server *Server) allowReportRequest(now time.Time) (bool, time.Duration) {
	minuteLimit := server.ReportsPerMinute
	if minuteLimit == 0 {
		minuteLimit = 60
	}
	dailyLimit := server.ReportsPerDay
	if dailyLimit == 0 {
		dailyLimit = 500
	}
	if minuteLimit < 0 && dailyLimit < 0 {
		return true, 0
	}
	now = now.UTC()
	server.reportRateMu.Lock()
	defer server.reportRateMu.Unlock()
	if server.reportRateWindow.IsZero() || now.Before(server.reportRateWindow) || now.Sub(server.reportRateWindow) >= time.Minute {
		server.reportRateWindow = now
		server.reportRateCount = 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if server.reportRateDay.IsZero() || !server.reportRateDay.Equal(today) {
		server.reportRateDay = today
		server.reportDailyCount = 0
	}
	if dailyLimit >= 0 && server.reportDailyCount >= dailyLimit {
		return false, today.Add(24 * time.Hour).Sub(now)
	}
	if minuteLimit >= 0 && server.reportRateCount >= minuteLimit {
		return false, server.reportRateWindow.Add(time.Minute).Sub(now)
	}
	if dailyLimit >= 0 {
		server.reportDailyCount++
	}
	if minuteLimit >= 0 {
		server.reportRateCount++
	}
	return true, 0
}

func retryAfterSeconds(delay time.Duration) int {
	if delay <= 0 {
		return 1
	}
	seconds := int(delay / time.Second)
	if delay%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func bearerMatches(request *http.Request, token string) bool {
	if token == "" {
		return false
	}
	want := "Bearer " + token
	got := request.Header.Get("Authorization")
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func decodeJSON(request *http.Request, target interface{}) error {
	if request.Body == nil {
		return io.EOF
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request contains multiple JSON values")
		}
		return err
	}
	return nil
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
