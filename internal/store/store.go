// Package store implements the three-scope markdown-backed knowledge store.
//
// The store is the only component that reads and writes markdown files on
// disk. Every path that produces or consumes an Entry goes through it.
// Other packages (search, mcp, extraction) must not touch the filesystem
// for store content directly.
//
// Responsibilities:
//   - Locate the three scope roots via Config.
//   - Glob and load entries from the live and pending directories.
//   - Enforce the pending/ invariant: all Writes go to <scope>/pending/,
//     never directly to the live directory. Entries reach the live
//     directory only via Promote.
//   - Prune stale pending entries (older than PendingTTL) silently.
//   - Walk-up project root detection (FindProjectRoot).
//
// Non-responsibilities:
//   - Search and FTS indexing (see internal/search).
//   - Frontmatter parsing and validation (see internal/format).
//   - Sync and git operations (git is run by the user, not by mastermind).
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// ErrInvalidScope is returned when a caller passes a scope value that
// isn't one of the three known scopes, or passes a scope for which the
// store has no root configured.
var ErrInvalidScope = errors.New("store: invalid or unconfigured scope")

// ErrEntryExists is returned when Promote would overwrite a live entry.
// The caller must decide whether to force, rename, or reject.
var ErrEntryExists = errors.New("store: target entry already exists")

// Store reads and writes entries across the three scopes described in
// Config. Store is safe for use by a single goroutine; concurrent writes
// to the same path are not coordinated (filesystem atomics cover the
// single-writer case, which is all mastermind needs).
type Store struct {
	cfg Config
}

// New constructs a Store from the given Config. It does NOT create any
// directories on disk — that happens lazily on the first write to each
// scope. Read-only callers (search, list) should never cause directory
// creation.
func New(cfg Config) *Store {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Store{cfg: cfg}
}

// Config returns a copy of the store's configuration. Useful for tests
// and for components that need to know which roots are active.
func (s *Store) Config() Config {
	return s.cfg
}

// EntryRef is a lightweight handle to an entry on disk. It contains the
// frontmatter metadata and the absolute file path, but not the body.
// Listing operations return EntryRefs so callers can filter and sort
// without paying the cost of loading every body into memory.
//
// Load a full Entry with (*Store).LoadRef.
type EntryRef struct {
	Path     string          // absolute path to the .md file
	Metadata format.Metadata // parsed frontmatter (not yet Normalized)
	Scope    format.Scope    // which scope this ref was found in
	Pending  bool            // true if the ref lives in <scope>/pending/
}

// FindProjectRoot walks upward from cwd looking for the nearest directory
// containing a .knowledge/ subdirectory. Returns the absolute path of the repo
// root (the directory containing .knowledge/), or "" if nothing is found before
// reaching the filesystem root.
//
// This is used by session-start and session-close to locate the
// project-shared store without requiring the user to pass a path.
func FindProjectRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}

	// Walk upward. filepath.Dir on a root path returns the root itself,
	// so compare before/after to detect the top.
	for {
		candidate := filepath.Join(abs, ".knowledge")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return abs
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return ""
		}
		abs = parent
	}
}

// Write stores a new pending entry under the appropriate scope.
//
// The scope comes from entry.Metadata.Scope — callers MUST set it before
// calling Write. If the scope is empty or unknown, Write returns
// ErrInvalidScope. This is the single chokepoint that enforces the
// pending/ invariant: there is no path from Write to the live directory.
//
// The file is written atomically (tempfile + rename in the same
// directory). The returned path is absolute and points at the freshly
// written pending file.
func (s *Store) Write(entry *format.Entry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("store: write: nil entry")
	}
	// Normalize before writing so defaults land on disk.
	entry.Normalize()

	scope := scopeFromFormat(entry.Metadata.Scope)
	if scope == scopeUnknown {
		return "", fmt.Errorf("%w: unknown scope %q (expected user-personal, project-shared, or project-personal)", ErrInvalidScope, entry.Metadata.Scope)
	}
	root := s.cfg.rootFor(scope)
	if root == "" {
		return "", fmt.Errorf("%w: scope %q is valid but has no root configured in this session (caller forgot to wire it, see cmd/mastermind/main.go:runMCPServer)", ErrInvalidScope, entry.Metadata.Scope)
	}

	pendingPath := filepath.Join(root, pendingDirName)
	if err := os.MkdirAll(pendingPath, 0o755); err != nil {
		return "", fmt.Errorf("store: mkdir pending: %w", err)
	}

	name := pendingFileName(s.cfg.Now(), entry.Metadata.Topic)
	target := filepath.Join(pendingPath, name)

	data, err := entry.MarshalMarkdown()
	if err != nil {
		return "", fmt.Errorf("store: marshal entry: %w", err)
	}

	if err := writeFileAtomic(target, data); err != nil {
		return "", fmt.Errorf("store: atomic write %s: %w", target, err)
	}
	return target, nil
}

// Promote moves a pending entry to the live directory of its scope.
// pendingPath must be an absolute path to a file under one of the
// configured <scope>/pending/ directories.
//
// The destination filename is derived from the entry's topic (the
// timestamp prefix used in pending/ is stripped). If a file with the
// target name already exists, Promote returns ErrEntryExists — callers
// must decide whether to rename, force, or reject.
//
// Promote is not atomic in the filesystem sense — it does two operations
// (load + rename), and a crash between them leaves the entry in pending/,
// which is the safe side to fail on.
func (s *Store) Promote(pendingPath string) (string, error) {
	abs, err := filepath.Abs(pendingPath)
	if err != nil {
		return "", fmt.Errorf("store: abs %s: %w", pendingPath, err)
	}

	// Figure out which scope this path belongs to by matching roots.
	scope, root := s.scopeOfPath(abs)
	if scope == scopeUnknown {
		return "", fmt.Errorf("%w: path not under any configured scope: %s", ErrInvalidScope, abs)
	}

	// Confirm it's actually a pending file, not a live one.
	if !strings.Contains(abs, string(os.PathSeparator)+pendingDirName+string(os.PathSeparator)) {
		return "", fmt.Errorf("store: promote: path is not in pending/: %s", abs)
	}

	// Load the entry so we can derive the live filename from its topic.
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("store: read pending: %w", err)
	}
	entry, err := format.Parse(data)
	if err != nil {
		return "", fmt.Errorf("store: parse pending %s: %w", abs, err)
	}

	topicDir := normalizeCategory(entry.Metadata.Category)
	if topicDir == "" {
		topicDir = s.resolveTopicDir(root, entry.Metadata.Tags)
	}

	liveDirPath := filepath.Join(root, topicDir)
	if err := os.MkdirAll(liveDirPath, 0o755); err != nil {
		return "", fmt.Errorf("store: mkdir live: %w", err)
	}

	name := liveFileName(entry.Metadata.Topic)
	target := filepath.Join(liveDirPath, name)

	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%w: %s", ErrEntryExists, target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("store: stat target: %w", err)
	}

	if err := os.Rename(abs, target); err != nil {
		return "", fmt.Errorf("store: rename %s -> %s: %w", abs, target, err)
	}
	return target, nil
}

// ListPending returns all pending refs across the given scope. If the
// scope's root isn't configured, returns nil, nil (not an error — some
// scopes are legitimately unavailable in some sessions).
//
// Order: sorted by path ascending, which (given the pendingFileName
// timestamp prefix) also yields chronological order.
func (s *Store) ListPending(scope format.Scope) ([]EntryRef, error) {
	sk := scopeFromFormat(scope)
	root := s.cfg.rootFor(sk)
	if sk == scopeUnknown {
		return nil, fmt.Errorf("%w: %q", ErrInvalidScope, scope)
	}
	if root == "" {
		return nil, nil
	}
	dir := filepath.Join(root, pendingDirName)
	return s.listDir(dir, scope, true)
}

// ListLive returns all live (promoted) refs for the given scope.
// Same semantics as ListPending: unconfigured scopes return nil, nil.
func (s *Store) ListLive(scope format.Scope) ([]EntryRef, error) {
	sk := scopeFromFormat(scope)
	root := s.cfg.rootFor(sk)
	if sk == scopeUnknown {
		return nil, fmt.Errorf("%w: %q", ErrInvalidScope, scope)
	}
	if root == "" {
		return nil, nil
	}
	return s.listLiveRecursive(root, scope)
}

// listLiveRecursive walks all topic subdirectories under root,
// collecting .md entries. It skips operational dirs (pending/,
// archive/) and also picks up .md files at the root level
// (uncategorized entries). This is the retrieval side of the
// topic-directory system — it doesn't care about the tree shape,
// it just finds every .md entry.
func (s *Store) listLiveRecursive(root string, scope format.Scope) ([]EntryRef, error) {
	var refs []EntryRef

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}

		// Skip operational directories entirely.
		if d.IsDir() && d.Name() != filepath.Base(root) {
			if operationalDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil // descend into topic dirs
		}

		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		// Skip files directly inside pending/ (handled by ListPending).
		rel, _ := filepath.Rel(root, path)
		if strings.HasPrefix(rel, pendingDirName+string(filepath.Separator)) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil // skip unreadable files
		}
		entry, parseErr := format.Parse(data)
		if parseErr != nil {
			return nil // skip malformed files
		}
		refs = append(refs, EntryRef{
			Path:     path,
			Metadata: entry.Metadata,
			Scope:    scope,
			Pending:  false,
		})
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("store: walk live %s: %w", root, err)
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Path < refs[j].Path
	})
	return refs, nil
}

// LoadRef reads the full entry body from a ref and returns a complete
// format.Entry with metadata re-parsed (so the returned entry is
// authoritative for both frontmatter and body).
func (s *Store) LoadRef(ref EntryRef) (*format.Entry, error) {
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return nil, fmt.Errorf("store: read %s: %w", ref.Path, err)
	}
	entry, err := format.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("store: parse %s: %w", ref.Path, err)
	}
	return entry, nil
}

// Reject deletes a pending entry. Used by the review flow when the user
// decides a candidate isn't worth keeping. Idempotent — a missing file
// is not an error.
func (s *Store) Reject(pendingPath string) error {
	abs, err := filepath.Abs(pendingPath)
	if err != nil {
		return fmt.Errorf("store: abs %s: %w", pendingPath, err)
	}
	scope, _ := s.scopeOfPath(abs)
	if scope == scopeUnknown {
		return fmt.Errorf("%w: path not under any configured scope: %s", ErrInvalidScope, abs)
	}
	if !strings.Contains(abs, string(os.PathSeparator)+pendingDirName+string(os.PathSeparator)) {
		return fmt.Errorf("store: reject: path is not in pending/: %s", abs)
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: remove %s: %w", abs, err)
	}
	return nil
}

// AutoPromoteStale promotes old pending entries to the live store when
// the store's PendingBehavior is PendingAutoPromote. When
// PendingBehavior is PendingKeepForever (the default), this is a no-op.
//
// Returns the number of entries promoted. Entries that would collide
// with an existing live entry (ErrEntryExists) are silently skipped —
// they stay in pending for manual review rather than being lost.
//
// This replaced the original PruneStale, which DELETED old entries.
// That behavior was reversed because auto-delete is the only
// irreversible failure mode in the system. See DECISIONS.md.
func (s *Store) AutoPromoteStale() (int, error) {
	behavior := s.cfg.PendingBehavior
	if behavior == "" {
		behavior = PendingKeepForever
	}
	if behavior == PendingKeepForever {
		return 0, nil
	}

	promoteAfter := s.cfg.AutoPromoteAfter
	if promoteAfter == 0 {
		promoteAfter = DefaultAutoPromoteAfter
	}
	cutoff := s.cfg.Now().Add(-promoteAfter)
	var promoted int

	for _, scope := range []format.Scope{
		format.ScopeUserPersonal,
		format.ScopeProjectShared,
		format.ScopeProjectPersonal,
	} {
		refs, err := s.ListPending(scope)
		if err != nil {
			return promoted, err
		}
		for _, ref := range refs {
			info, err := os.Stat(ref.Path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return promoted, fmt.Errorf("store: stat %s: %w", ref.Path, err)
			}
			if info.ModTime().Before(cutoff) {
				_, err := s.Promote(ref.Path)
				if err != nil {
					if errors.Is(err, ErrEntryExists) {
						// Skip silently — the entry stays in pending
						// for manual review rather than being lost.
						continue
					}
					return promoted, fmt.Errorf("store: auto-promote %s: %w", ref.Path, err)
				}
				promoted++
			}
		}
	}
	return promoted, nil
}

// WriteLive writes an entry directly to the live store, bypassing
// pending entirely. This is the path for user-initiated captures
// (mm_write during a conversation) where the user IS the review —
// they're present, they can see what the agent is writing, they chose
// to create it. Forcing a second approve step via mm_promote would be
// pointless ceremony.
//
// Automatic extraction (Phase 3 session-close) should use Write
// (which targets pending/) because the user isn't present to review.
//
// WriteLive uses the same atomic write pattern as Write (tempfile +
// rename). Returns ErrEntryExists if a live entry with the same slug
// already exists.
func (s *Store) WriteLive(entry *format.Entry) (string, error) {
	if entry == nil {
		return "", fmt.Errorf("store: write-live: nil entry")
	}
	entry.Normalize()

	scope := scopeFromFormat(entry.Metadata.Scope)
	if scope == scopeUnknown {
		return "", fmt.Errorf("%w: unknown scope %q (expected user-personal, project-shared, or project-personal)", ErrInvalidScope, entry.Metadata.Scope)
	}
	root := s.cfg.rootFor(scope)
	if root == "" {
		return "", fmt.Errorf("%w: scope %q is valid but has no root configured in this session (caller forgot to wire it, see cmd/mastermind/main.go:runMCPServer)", ErrInvalidScope, entry.Metadata.Scope)
	}

	topicDir := normalizeCategory(entry.Metadata.Category)
	if topicDir == "" {
		topicDir = s.resolveTopicDir(root, entry.Metadata.Tags)
	}

	liveDirPath := filepath.Join(root, topicDir)
	if err := os.MkdirAll(liveDirPath, 0o755); err != nil {
		return "", fmt.Errorf("store: mkdir live: %w", err)
	}

	name := liveFileName(entry.Metadata.Topic)
	target := filepath.Join(liveDirPath, name)

	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%w: %s", ErrEntryExists, target)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("store: stat target: %w", err)
	}

	data, err := entry.MarshalMarkdown()
	if err != nil {
		return "", fmt.Errorf("store: marshal entry: %w", err)
	}

	if err := writeFileAtomic(target, data); err != nil {
		return "", fmt.Errorf("store: atomic write %s: %w", target, err)
	}
	return target, nil
}

// ─── topic directory resolution ────────────────────────────────────────

// resolveTopicDir determines the topic directory for an entry based on
// its tags and the existing directory structure. This is the fallback
// path for entries without an explicit Category field.
//
// The algorithm uses an "attractor" pattern:
//  1. List existing subdirectories under root (skip operational dirs)
//  2. For each tag, check if it matches an existing directory name
//  3. If a match is found → use that directory (first match wins)
//  4. If no match → use the first tag as a new directory
//  5. If no tags → "general"
//
// Fallback resolution only produces level-1 directories (single
// segment). Level-2 subdirectories require the agent's explicit
// Category field — the store doesn't guess at sub-topics.
func (s *Store) resolveTopicDir(root string, tags []string) string {
	existing := s.listTopicDirs(root)

	// Attractor: match any tag against existing directory names.
	for _, tag := range tags {
		normalized := slugifySegment(tag)
		if normalized == "" {
			continue
		}
		for _, dir := range existing {
			if normalized == dir {
				return dir
			}
		}
	}

	// No match — use first tag as a new directory.
	if len(tags) > 0 {
		if s := slugifySegment(tags[0]); s != "" {
			return s
		}
	}

	return "general"
}

// listTopicDirs returns the names of immediate subdirectories under
// root that are NOT operational directories. These are the level-1
// topic directories used by the attractor pattern.
func (s *Store) listTopicDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if operationalDirs[e.Name()] {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	return dirs
}

// ListCategories returns all existing topic directory paths across
// both levels for the given scope. Useful for agents to check existing
// categories before creating new ones.
//
// Returns paths like ["electron", "electron/ipc", "go", "mcp"].
func (s *Store) ListCategories(scope format.Scope) ([]string, error) {
	sk := scopeFromFormat(scope)
	root := s.cfg.rootFor(sk)
	if sk == scopeUnknown {
		return nil, fmt.Errorf("%w: %q", ErrInvalidScope, scope)
	}
	if root == "" {
		return nil, nil
	}

	var categories []string
	level1 := s.listTopicDirs(root)
	for _, d1 := range level1 {
		categories = append(categories, d1)
		// Check for level-2 subdirs.
		sub, err := os.ReadDir(filepath.Join(root, d1))
		if err != nil {
			continue
		}
		for _, d2 := range sub {
			if d2.IsDir() && !operationalDirs[d2.Name()] {
				categories = append(categories, d1+"/"+d2.Name())
			}
		}
	}
	return categories, nil
}

// ─── internal helpers ───────────────────────────────────────────────────

// listDir reads a directory and returns refs for every .md file it
// contains. Missing directories return an empty slice, not an error —
// the store's scopes are legitimately sparse (not every scope exists
// in every session).
func (s *Store) listDir(dir string, scope format.Scope, pending bool) ([]EntryRef, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: read dir %s: %w", dir, err)
	}

	var refs []EntryRef
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("store: read %s: %w", fullPath, err)
		}
		entry, err := format.Parse(data)
		if err != nil {
			// Skip malformed files; don't fail the entire list.
			// The review flow and the search layer can both live
			// with a malformed entry being invisible until fixed.
			continue
		}
		refs = append(refs, EntryRef{
			Path:     fullPath,
			Metadata: entry.Metadata,
			Scope:    scope,
			Pending:  pending,
		})
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Path < refs[j].Path
	})
	return refs, nil
}

// scopeOfPath identifies which configured scope a filesystem path belongs
// to, by prefix-matching against each configured root. Returns
// scopeUnknown if no root matches.
func (s *Store) scopeOfPath(path string) (scopeKind, string) {
	for _, sk := range []scopeKind{scopeUser, scopeProjectShared, scopeProjectPersonal} {
		root := s.cfg.rootFor(sk)
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		// Normalize trailing separator for robust prefix match.
		prefix := rootAbs + string(os.PathSeparator)
		if strings.HasPrefix(path, prefix) || path == rootAbs {
			return sk, root
		}
	}
	return scopeUnknown, ""
}

// writeFileAtomic writes data to path using the tempfile + rename
// pattern. The tempfile lives in the same directory as the target so
// rename is an atomic operation on the same filesystem.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mm-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	// If anything below fails, make a best-effort to remove the temp.
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// slugify turns a free-form topic string into a filesystem-safe slug.
// Lowercase, ASCII letters and digits only, dashes between words, no
// leading/trailing dashes, capped at 80 characters so filenames stay
// manageable.
//
// The slug is not meant to be unique on its own — pending files also
// get a timestamp prefix, and live files use the slug as the identity
// (so collisions are intentional deduplication).
func slugify(topic string) string {
	var b strings.Builder
	b.Grow(len(topic))
	prevDash := false
	for _, r := range strings.ToLower(topic) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if s == "" {
		s = "untitled"
	}
	if len(s) > 80 {
		s = strings.TrimRight(s[:80], "-")
	}
	return s
}

// pendingFileName builds the canonical pending filename:
//
//	YYYYMMDD-HHMMSS-<slug>.md
//
// The timestamp prefix ensures chronological listing and makes
// accidental collisions vanishingly unlikely even for entries with the
// same topic.
func pendingFileName(now time.Time, topic string) string {
	return fmt.Sprintf("%s-%s.md", now.UTC().Format("20060102-150405"), slugify(topic))
}

// liveFileName builds the canonical live filename:
//
//	<slug>.md
//
// Live entries use slug as identity — the same topic always collides
// with itself, and Promote surfaces the collision to the caller via
// ErrEntryExists.
func liveFileName(topic string) string {
	return slugify(topic) + ".md"
}
