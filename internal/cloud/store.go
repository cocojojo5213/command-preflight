package cloud

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type diskData struct {
	Entries map[string]Entry `json:"entries"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	entries map[string]Entry
}

func OpenStore(path string) (*Store, error) {
	store := &Store{path: path, entries: map[string]Entry{}}
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
	store.entries[entry.Fingerprint.ID] = entry
	return store.persistLocked()
}

func (store *Store) persistLocked() error {
	if store.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(diskData{Entries: store.entries}, "", "  ")
	if err != nil {
		return err
	}
	temporary := store.path + ".tmp"
	if err := os.WriteFile(temporary, data, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, store.path)
}
