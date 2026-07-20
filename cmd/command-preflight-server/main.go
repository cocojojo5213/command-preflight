package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/cloud"
)

func main() {
	flags := flag.NewFlagSet("command-preflight-server", flag.ExitOnError)
	listen := flags.String("listen", envOrDefault("COMMAND_PREFLIGHT_BIND", "127.0.0.1:8787"), "HTTP listen address")
	data := flags.String("data", envOrDefault("COMMAND_PREFLIGHT_DATA", "./data/knowledge.json"), "JSON knowledge store path; empty disables persistence")
	allowReport := flags.Bool("allow-report", envBool("COMMAND_PREFLIGHT_ALLOW_REPORT", false), "enable the opt-in public report queue")
	reportToken := flags.String("report-token", os.Getenv("COMMAND_PREFLIGHT_REPORT_TOKEN"), "legacy Bearer token for operator writes and moderation")
	adminToken := flags.String("admin-token", os.Getenv("COMMAND_PREFLIGHT_ADMIN_TOKEN"), "Bearer token for moderation APIs (defaults to report-token)")
	submitToken := flags.String("report-submit-token", os.Getenv("COMMAND_PREFLIGHT_REPORT_SUBMIT_TOKEN"), "optional Bearer token required for report submissions")
	allowProxiedAdmin := flags.Bool("allow-proxied-admin", envBool("COMMAND_PREFLIGHT_ALLOW_PROXIED_ADMIN", false), "allow moderation APIs through a reverse proxy")
	reportsPerMinute := flags.Int("reports-per-minute", envInt("COMMAND_PREFLIGHT_REPORTS_PER_MINUTE", 60), "global report submission limit per minute")
	reportsPerDay := flags.Int("reports-per-day", envInt("COMMAND_PREFLIGHT_REPORTS_PER_DAY", 500), "global UTC-day report submission limit")
	retentionDays := flags.Int("report-retention-days", envInt("COMMAND_PREFLIGHT_REPORT_RETENTION_DAYS", 30), "days to retain terminal queue records; zero disables pruning")
	pendingRetentionDays := flags.Int("pending-retention-days", envInt("COMMAND_PREFLIGHT_PENDING_RETENTION_DAYS", 7), "days to retain pending, held, or approved queue records; zero disables pruning")
	flags.Parse(os.Args[1:])
	if *allowReport && *reportToken == "" && *adminToken == "" {
		log.Fatal("--report-token or --admin-token is required with --allow-report")
	}
	adminCredential := *adminToken
	if adminCredential == "" {
		adminCredential = *reportToken
	}
	if *allowReport && len(adminCredential) < 32 {
		log.Fatal("the moderation token must contain at least 32 characters")
	}
	if *submitToken != "" && len(*submitToken) < 32 {
		log.Fatal("--report-submit-token must contain at least 32 characters when set")
	}
	store, err := cloud.OpenStore(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	if *retentionDays < 0 {
		log.Fatal("--report-retention-days must be zero or greater")
	}
	if *reportsPerMinute < 1 {
		log.Fatal("--reports-per-minute must be at least 1")
	}
	if *reportsPerDay < 1 {
		log.Fatal("--reports-per-day must be at least 1")
	}
	if *pendingRetentionDays < 0 {
		log.Fatal("--pending-retention-days must be zero or greater")
	}
	if *retentionDays > 0 || *pendingRetentionDays > 0 {
		retention := time.Duration(*retentionDays) * 24 * time.Hour
		pendingRetention := time.Duration(*pendingRetentionDays) * 24 * time.Hour
		if err := pruneReportQueues(store, retention, pendingRetention); err != nil {
			log.Fatalf("prune reports: %v", err)
		}
		go runReportPruner(store, retention, pendingRetention)
	}
	server := &cloud.Server{
		Store:             store,
		AllowReport:       *allowReport,
		ReportToken:       *reportToken,
		AdminToken:        *adminToken,
		ReportSubmitToken: *submitToken,
		AllowProxiedAdmin: *allowProxiedAdmin,
		ReportsPerMinute:  *reportsPerMinute,
		ReportsPerDay:     *reportsPerDay,
	}
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	fmt.Printf("command-preflight-server listening on http://%s\n", *listen)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(cloud.AddressError(*listen, err))
	}
}

func pruneReports(store *cloud.Store, retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	removed, err := store.PruneReports(time.Now().UTC().Add(-retention))
	if err != nil {
		return err
	}
	if removed > 0 {
		log.Printf("pruned %d terminal report(s)", removed)
	}
	return nil
}

func pruneStaleReports(store *cloud.Store, retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	removed, err := store.PruneStaleReports(time.Now().UTC().Add(-retention))
	if err != nil {
		return err
	}
	if removed > 0 {
		log.Printf("pruned %d stale moderation report(s)", removed)
	}
	return nil
}

func pruneReportQueues(store *cloud.Store, retention, pendingRetention time.Duration) error {
	if err := pruneReports(store, retention); err != nil {
		return err
	}
	return pruneStaleReports(store, pendingRetention)
}

func runReportPruner(store *cloud.Store, retention, pendingRetention time.Duration) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := pruneReportQueues(store, retention, pendingRetention); err != nil {
			log.Printf("prune reports: %v", err)
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("%s must be true or false", name)
	}
	return parsed
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("%s must be an integer", name)
	}
	return parsed
}
