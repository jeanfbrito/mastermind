// Package access manages per-entry access telemetry in a sidecar JSON
// cache, keeping markdown files immutable on read.
//
// # Motivation
//
// The original design bumped accessed/last_accessed fields directly in
// the entry's YAML frontmatter on every mm_search call. This caused
// every search to rewrite every returned file, permanently dirtying git
// status for users who version .knowledge/. Access counts are mutable
// telemetry — they are not content — so frontmatter is the wrong home
// for them. The sidecar keeps the markdown source-of-truth clean.
//
// # Sidecar location
//
// Each scope root gets its own sidecar at <root>/.cache/access.json.
// This keeps per-project telemetry local to that repo and user-scope
// telemetry in ~/.knowledge/.cache/access.json.
//
// # Error handling
//
// All errors are silenced. Access tracking is best-effort: a missing
// or corrupt sidecar degrades ranking quality slightly but never blocks
// search.
//
// # Concurrency
//
// The Tracker is guarded by a sync.Mutex. Safe for concurrent use by
// multiple goroutines (the MCP server processes tool calls serially
// today, but the mutex is cheap insurance).
//
// # Migration
//
// On first load, if the JSON file does not exist, the Tracker returns
// zero values for all entries. Callers that hydrate from frontmatter
// will pass those values back through Increment, rebuilding the sidecar
// organically over time. A one-shot seed path is provided by the store
// (see internal/store for how it seeds the sidecar from legacy frontmatter
// on first use).
package access

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// cacheDir is the subdirectory under each scope root where the sidecar lives.
	cacheDir = ".cache"
	// cacheFile is the filename of the sidecar JSON.
	cacheFile = "access.json"
	// schemaVersion is the version field written into the JSON for future
	// schema evolution. Increment when the shape changes incompatibly.
	schemaVersion = 1
)

// entryRecord holds the telemetry for one entry file.
type entryRecord struct {
	Count int    `json:"count"`
	Last  string `json:"last"` // ISO 8601 date, YYYY-MM-DD
}

// sidecar is the on-disk JSON shape.
type sidecar struct {
	Version int                    `json:"version"`
	Entries map[string]entryRecord `json:"entries"`
}

// Tracker manages access telemetry for a single scope root.
//
// Create with New; call Load once (lazy, called automatically on first
// Get/Increment). Increment updates in-memory state and writes the
// sidecar atomically.
type Tracker struct {
	root string // absolute path to the scope root (e.g. ~/.knowledge)
	path string // absolute path to the sidecar JSON

	mu      sync.Mutex
	loaded  bool
	entries map[string]entryRecord // key: absolute entry path
}

// New constructs a Tracker for the given scope root. The root must be
// an absolute path. The tracker is not yet loaded from disk — loading
// happens lazily on the first Get or Increment call.
func New(root string) *Tracker {
	return &Tracker{
		root:    root,
		path:    filepath.Join(root, cacheDir, cacheFile),
		entries: make(map[string]entryRecord),
	}
}

// canonicalize resolves symlinks in path so that callers on macOS (where
// t.TempDir() returns /var/folders/... but WalkDir yields /private/var/folders/...)
// always write and read under the same key. Falls back to the original path
// if EvalSymlinks fails (e.g. the file doesn't exist yet — shouldn't happen
// for entry files, but best-effort is fine here).
func canonicalize(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// Get returns the access count and last-accessed ISO date for the entry
// at the given absolute path. Returns (0, "") if there is no record.
// Triggers a lazy load from disk on the first call.
func (t *Tracker) Get(path string) (count int, last string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	r := t.entries[canonicalize(path)]
	return r.Count, r.Last
}

// Increment bumps the access count for the entry at the given absolute
// path and updates the last-accessed date to now. Writes the sidecar
// atomically after the update. Errors are silenced.
func (t *Tracker) Increment(path string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()

	key := canonicalize(path)
	r := t.entries[key]
	r.Count++
	r.Last = now.UTC().Format("2006-01-02")
	t.entries[key] = r

	t.saveLocked() // best-effort
}

// Seed sets the count and last for a path only if no existing record
// exists (count == 0). Used during migration to prime the sidecar from
// legacy frontmatter values without overwriting real telemetry.
// Triggers a lazy load. Does NOT write to disk — caller must call Save.
func (t *Tracker) Seed(path string, count int, last string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()

	key := canonicalize(path)
	if existing := t.entries[key]; existing.Count > 0 {
		return // already has real data
	}
	if count <= 0 {
		return // nothing to seed
	}
	t.entries[key] = entryRecord{Count: count, Last: last}
}

// Save writes the current in-memory state to disk atomically. Best-effort.
func (t *Tracker) Save() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.saveLocked()
}

// loadLocked reads the sidecar from disk into t.entries. Must be called
// with t.mu held. Idempotent — subsequent calls after the first are no-ops.
// Missing file is not an error (empty/cold start).
func (t *Tracker) loadLocked() {
	if t.loaded {
		return
	}
	t.loaded = true // mark even on failure so we don't retry on every call

	data, err := os.ReadFile(t.path)
	if err != nil {
		// Missing file is normal on first run. Any other error: start cold.
		return
	}
	var sc sidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		// Corrupt file: start cold. Next save will overwrite.
		return
	}
	if sc.Entries != nil {
		t.entries = sc.Entries
	}
}

// saveLocked writes t.entries to disk atomically. Must be called with
// t.mu held. Errors are silenced.
func (t *Tracker) saveLocked() {
	sc := sidecar{
		Version: schemaVersion,
		Entries: t.entries,
	}
	data, err := json.Marshal(sc)
	if err != nil {
		return
	}

	dir := filepath.Dir(t.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	// Atomic write: temp file in same directory + rename.
	tmp, err := os.CreateTemp(dir, ".mm-access-tmp-*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmpName, t.path)
}
