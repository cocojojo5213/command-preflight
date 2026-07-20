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
	allowReport := flags.Bool("allow-report", envBool("COMMAND_PREFLIGHT_ALLOW_REPORT", false), "enable authenticated PUT reports")
	reportToken := flags.String("report-token", os.Getenv("COMMAND_PREFLIGHT_REPORT_TOKEN"), "Bearer token required when reporting is enabled")
	flags.Parse(os.Args[1:])
	if *allowReport && *reportToken == "" {
		log.Fatal("--report-token is required with --allow-report")
	}
	store, err := cloud.OpenStore(*data)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	server := &cloud.Server{Store: store, AllowReport: *allowReport, ReportToken: *reportToken}
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	fmt.Printf("command-preflight-server listening on http://%s\n", *listen)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(cloud.AddressError(*listen, err))
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
