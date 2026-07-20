package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cocojojo5213/command-preflight/internal/core"
)

type diskData struct {
	Entries map[string]Entry  `json:"entries"`
	Reports map[string]Report `json:"reports,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Entry
	reports map[string]Report
}

const maxQueuedReports = 5000

var ErrReportNotFound = errors.New("report not found")

func OpenStore(path string) (*Store, error) {
	store := &Store{path: path, entries: map[string]Entry{}, reports: map[string]Report{}}
	if path == "" {
		return store, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var disk diskData
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, err
	}
	if disk.Entries != nil {
		store.entries = disk.Entries
	}
	if disk.Reports != nil {
		store.reports = disk.Reports
	}
	return store, nil
}

func (store *Store) Lookup(id string) (Entry, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	entry, ok := store.entries[id]
	return entry, ok
}

func (store *Store) Upsert(entry Entry) error {
	if err := entry.Fingerprint.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}
	previous, existed := store.entries[entry.Fingerprint.ID]
	store.entries[entry.Fingerprint.ID] = entry
	if err := store.persistLocked(); err != nil {
		if existed {
			store.entries[entry.Fingerprint.ID] = previous
		} else {
			delete(store.entries, entry.Fingerprint.ID)
		}
		return err
	}
	return nil
}

// SubmitReport stores a sanitized, untrusted community proposal in the
// moderation queue. The bool result is true when an equivalent proposal was
// already present, allowing callers to avoid creating duplicate work.
func (store *Store) SubmitReport(input ReportInput) (Report, bool, error) {
	input.Fix.Summary = core.RedactPublicText(input.Fix.Summary)
	input.Fix.Verification = core.RedactPublicText(input.Fix.Verification)
	input.Fix.ToolVersion = core.RedactPublicText(input.Fix.ToolVersion)
	input.Fix.ID = ""
	if input.Fix.Shell == "" {
		input.Fix.Shell = input.Fingerprint.Shell
	}
	if err := input.Validate(); err != nil {
		return Report{}, false, err
	}
	if !input.Fix.Verified {
		return Report{}, false, fmt.Errorf("resolution must be verified before reporting")
	}
	input.Fix.ID = candidateFixID(input)
	claimedVerified := input.Fix.Verified
	claimedConfidence := input.Fix.Confidence
	input.Fix.Verified = false
	input.Fix.Confidence = 0
	input.Fix.Source = "community-submission"

	store.mu.Lock()
	defer store.mu.Unlock()
	for _, existing := range store.reports {
		if existing.Status == ReportRejected {
			continue
		}
		if sameProposal(existing, input) {
			return existing, true, nil
		}
	}
	queued := 0
	for _, existing := range store.reports {
		if existing.Status == ReportPending || existing.Status == ReportHeld || existing.Status == ReportApproved {
			queued++
		}
	}
	if queued >= maxQueuedReports {
		return Report{}, false, fmt.Errorf("report queue is full")
	}
	id, err := newReportID()
	if err != nil {
		return Report{}, false, err
	}
	report := Report{
		ID:                id,
		Fingerprint:       input.Fingerprint,
		Fix:               input.Fix,
		ClaimedVerified:   claimedVerified,
		ClaimedConfidence: claimedConfidence,
		Status:            ReportPending,
		ReceivedAt:        time.Now().UTC(),
	}
	if err := report.Validate(); err != nil {
		return Report{}, false, err
	}
	store.reports[report.ID] = report
	if err := store.persistLocked(); err != nil {
		delete(store.reports, report.ID)
		return Report{}, false, err
	}
	return report, false, nil
}

func (store *Store) ListReports(status string, limit int) []Report {
	store.mu.RLock()
	defer store.mu.RUnlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	status = strings.ToLower(strings.TrimSpace(status))
	reports := make([]Report, 0, len(store.reports))
	for _, report := range store.reports {
		if status != "" && status != "all" && report.Status != status {
			continue
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].ReceivedAt.Equal(reports[j].ReceivedAt) {
			return reports[i].ID < reports[j].ID
		}
		return reports[i].ReceivedAt.Before(reports[j].ReceivedAt)
	})
	if len(reports) > limit {
		reports = reports[:limit]
	}
	return reports
}

func (store *Store) GetReport(id string) (Report, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	report, ok := store.reports[id]
	return report, ok
}

func (store *Store) DeleteReport(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	report, ok := store.reports[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrReportNotFound, id)
	}
	delete(store.reports, id)
	if err := store.persistLocked(); err != nil {
		store.reports[id] = report
		return err
	}
	return nil
}

// ReviewReports applies a batch of operator decisions atomically. Report text
// is sanitized again because the review client is also an untrusted input
// boundary.
func (store *Store) ReviewReports(actions []ReviewAction) ([]Report, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("at least one review is required")
	}
	if len(actions) > 100 {
		return nil, fmt.Errorf("review batch is limited to 100 reports")
	}
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if err := action.Validate(); err != nil {
			return nil, err
		}
		if _, ok := seen[action.ID]; ok {
			return nil, fmt.Errorf("duplicate report id %q", action.ID)
		}
		seen[action.ID] = struct{}{}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	originalReports := store.reports
	workingReports := make(map[string]Report, len(store.reports))
	for id, report := range store.reports {
		workingReports[id] = report
	}
	store.reports = workingReports
	committed := false
	defer func() {
		if !committed {
			store.reports = originalReports
		}
	}()
	now := time.Now().UTC()
	updated := make([]Report, 0, len(actions))
	for _, action := range actions {
		report, ok := store.reports[action.ID]
		if !ok {
			return nil, fmt.Errorf("report %q not found", action.ID)
		}
		if report.Status != ReportPending && report.Status != ReportHeld {
			return nil, fmt.Errorf("report %q is already %s", action.ID, report.Status)
		}
		if action.Summary != "" {
			report.Fix.Summary = core.RedactPublicText(action.Summary)
		}
		if action.Verification != "" {
			report.Fix.Verification = core.RedactPublicText(action.Verification)
		}
		if action.Confidence != nil {
			report.Fix.Confidence = *action.Confidence
		}
		if err := (ReportInput{Fingerprint: report.Fingerprint, Fix: report.Fix}).Validate(); err != nil {
			return nil, fmt.Errorf("report %q: %w", action.ID, err)
		}
		report.Status = normalizedDecision(action.Decision)
		report.DecisionReason = core.RedactPublicText(action.Reason)
		report.ReviewedAt = &now
		store.reports[report.ID] = report
		updated = append(updated, report)
	}
	if err := store.persistLocked(); err != nil {
		return nil, err
	}
	committed = true
	return updated, nil
}

// PublishReports promotes approved reports into the public knowledge store.
// An empty ID list publishes every currently approved report.
func (store *Store) PublishReports(ids []string) ([]Entry, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	originalEntries := store.entries
	originalReports := store.reports
	workingEntries := make(map[string]Entry, len(store.entries))
	for id, entry := range store.entries {
		entry.Fixes = append([]Fix(nil), entry.Fixes...)
		workingEntries[id] = entry
	}
	workingReports := make(map[string]Report, len(store.reports))
	for id, report := range store.reports {
		workingReports[id] = report
	}
	store.entries = workingEntries
	store.reports = workingReports
	committed := false
	defer func() {
		if !committed {
			store.entries = originalEntries
			store.reports = originalReports
		}
	}()

	selected := make([]string, 0, len(store.reports))
	if len(ids) == 0 {
		for id, report := range store.reports {
			if report.Status == ReportApproved {
				selected = append(selected, id)
			}
		}
	} else {
		seen := map[string]struct{}{}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
	}
	sort.Strings(selected)
	if len(selected) > 100 {
		return nil, fmt.Errorf("publish batch is limited to 100 reports")
	}
	now := time.Now().UTC()
	entries := make([]Entry, 0, len(selected))
	for _, id := range selected {
		report, ok := store.reports[id]
		if !ok {
			return nil, fmt.Errorf("report %q not found", id)
		}
		if report.Status != ReportApproved {
			return nil, fmt.Errorf("report %q is not approved", id)
		}
		entry := store.entries[report.Fingerprint.ID]
		if entry.Fingerprint.ID == "" {
			entry.Fingerprint = report.Fingerprint
		} else if entry.Fingerprint != report.Fingerprint {
			return nil, fmt.Errorf("report %q fingerprint metadata conflicts with published knowledge", id)
		}
		fix := report.Fix
		fix.Source = "community-reviewed"
		fix.Verified = true
		if fix.Confidence == 0 {
			fix.Confidence = 0.5
		}
		if fix.Shell == "" {
			fix.Shell = report.Fingerprint.Shell
		}
		found := false
		for index, existing := range entry.Fixes {
			if existing.ID == fix.ID || (existing.Summary == fix.Summary && existing.Verification == fix.Verification) {
				entry.Fixes[index] = fix
				found = true
				break
			}
		}
		if !found {
			entry.Fixes = append(entry.Fixes, fix)
		}
		entry.ReportCount++
		entry.UpdatedAt = now
		store.entries[entry.Fingerprint.ID] = entry
		report.Status = ReportPublished
		report.PublishedAt = &now
		store.reports[id] = report
		entries = append(entries, entry)
	}
	if len(selected) == 0 {
		committed = true
		return entries, nil
	}
	if err := store.persistLocked(); err != nil {
		return nil, err
	}
	committed = true
	return entries, nil
}

// PruneReports removes old terminal queue records while leaving their
// published knowledge entries intact.
func (store *Store) PruneReports(before time.Time) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	originalReports := store.reports
	workingReports := make(map[string]Report, len(store.reports))
	for id, report := range store.reports {
		workingReports[id] = report
	}
	store.reports = workingReports
	removed := 0
	for id, report := range store.reports {
		if (report.Status == ReportRejected || report.Status == ReportPublished) && report.ReceivedAt.Before(before) {
			delete(store.reports, id)
			removed++
		}
	}
	if removed == 0 {
		store.reports = originalReports
		return 0, nil
	}
	if err := store.persistLocked(); err != nil {
		store.reports = originalReports
		return 0, err
	}
	return removed, nil
}

// PruneStaleReports removes old reports that still occupy the moderation
// queue. Approved reports that were never published are intentionally treated
// as stale too; published knowledge remains in entries and is not affected.
func (store *Store) PruneStaleReports(before time.Time) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	originalReports := store.reports
	workingReports := make(map[string]Report, len(store.reports))
	for id, report := range store.reports {
		workingReports[id] = report
	}
	store.reports = workingReports
	removed := 0
	for id, report := range store.reports {
		if (report.Status == ReportPending || report.Status == ReportHeld || report.Status == ReportApproved) && report.ReceivedAt.Before(before) {
			delete(store.reports, id)
			removed++
		}
	}
	if removed == 0 {
		store.reports = originalReports
		return 0, nil
	}
	if err := store.persistLocked(); err != nil {
		store.reports = originalReports
		return 0, err
	}
	return removed, nil
}

func sameProposal(report Report, input ReportInput) bool {
	return report.Fingerprint.ID == input.Fingerprint.ID &&
		report.Fix.Summary == input.Fix.Summary &&
		report.Fix.Verification == input.Fix.Verification
}

func newReportID() (string, error) {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate report id: %w", err)
	}
	return "rpt-" + hex.EncodeToString(bytes), nil
}

func (store *Store) persistLocked() error {
	if store.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(diskData{Entries: store.entries, Reports: store.reports}, "", "  ")
	if err != nil {
		return err
	}
	temporary := store.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, store.path)
}
