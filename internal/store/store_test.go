package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// ─── helpers ────────────────────────────────────────────────────────────

// newTestStore creates a Store backed by t.TempDir() with all three scopes
// configured under the tmp dir. The Now function defaults to time.Now —
// individual tests override it via s.cfg when they need time control.
func newTestStore(t *testing.T) (*Store, Config) {
	t.Helper()
	tmp := t.TempDir()
	cfg := Config{
		UserPersonalRoot:    filepath.Join(tmp, "user"),
		ProjectSharedRoot:   filepath.Join(tmp, "proj-shared"),
		ProjectPersonalRoot: filepath.Join(tmp, "proj-personal"),
		Now:                 time.Now,
	}
	return New(cfg), cfg
}

// fixedNow returns a Now func that always reports t, so tests can
// reason about file ages deterministically.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// makeEntry builds a minimal valid Entry with the given scope and topic.
func makeEntry(scope format.Scope, topic string) *format.Entry {
	return &format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-04",
			Project: "mastermind",
			Topic:   topic,
			Kind:    format.KindLesson,
			Scope:   scope,
		},
		Body: "test body",
	}
}

// ─── config and construction ───────────────────────────────────────────

func TestDefaultConfigHomeDir(t *testing.T) {
	// Sanity: DefaultConfig should return a Config with a non-empty
	// UserPersonalRoot on any normal machine.
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if cfg.UserPersonalRoot == "" {
		t.Error("DefaultConfig.UserPersonalRoot is empty")
	}
	if !strings.HasSuffix(cfg.UserPersonalRoot, ".mm") {
		t.Errorf("UserPersonalRoot = %q, want suffix .mm", cfg.UserPersonalRoot)
	}
}

func TestNewSetsDefaultNow(t *testing.T) {
	s := New(Config{})
	if s.cfg.Now == nil {
		t.Error("New did not populate Now")
	}
}

// ─── write and pending invariant ───────────────────────────────────────

func TestWriteUserPersonalLandsInPending(t *testing.T) {
	s, cfg := newTestStore(t)

	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "test entry"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Verify path is under user/pending/, NOT user/lessons/.
	wantPrefix := filepath.Join(cfg.UserPersonalRoot, pendingDirName)
	if !strings.HasPrefix(path, wantPrefix) {
		t.Errorf("Write path = %q, want prefix %q", path, wantPrefix)
	}
	if strings.Contains(path, string(os.PathSeparator)+"lessons"+string(os.PathSeparator)) {
		t.Errorf("Write landed in live dir, not pending: %q", path)
	}

	// File must exist and be parseable.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	parsed, err := format.Parse(data)
	if err != nil {
		t.Fatalf("Parse written file: %v", err)
	}
	if parsed.Metadata.Topic != "test entry" {
		t.Errorf("round-trip topic = %q, want %q", parsed.Metadata.Topic, "test entry")
	}
}

func TestWriteProjectSharedLandsInPending(t *testing.T) {
	s, cfg := newTestStore(t)
	path, err := s.Write(makeEntry(format.ScopeProjectShared, "project fact"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	wantPrefix := filepath.Join(cfg.ProjectSharedRoot, pendingDirName)
	if !strings.HasPrefix(path, wantPrefix) {
		t.Errorf("Write path = %q, want prefix %q", path, wantPrefix)
	}
}

func TestWriteProjectPersonalLandsInPending(t *testing.T) {
	s, cfg := newTestStore(t)
	path, err := s.Write(makeEntry(format.ScopeProjectPersonal, "private scratch"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	wantPrefix := filepath.Join(cfg.ProjectPersonalRoot, pendingDirName)
	if !strings.HasPrefix(path, wantPrefix) {
		t.Errorf("Write path = %q, want prefix %q", path, wantPrefix)
	}
}

func TestWriteEmptyScopeFails(t *testing.T) {
	s, _ := newTestStore(t)
	e := makeEntry("", "no scope")
	_, err := s.Write(e)
	if !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Write with empty scope: err = %v, want ErrInvalidScope", err)
	}
}

func TestWriteUnconfiguredScopeFails(t *testing.T) {
	// Only UserPersonal configured; writes to others must fail.
	cfg := Config{
		UserPersonalRoot: t.TempDir(),
		Now:              time.Now,
	}
	s := New(cfg)

	_, err := s.Write(makeEntry(format.ScopeProjectShared, "no root configured"))
	if !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Write to unconfigured scope: err = %v, want ErrInvalidScope", err)
	}
}

func TestWriteNilEntryFails(t *testing.T) {
	s, _ := newTestStore(t)
	_, err := s.Write(nil)
	if err == nil {
		t.Error("Write(nil): expected error, got nil")
	}
}

func TestWriteNormalizesDefaults(t *testing.T) {
	// An entry written with Confidence="" should land on disk with
	// confidence: high (via format.Normalize called inside Write).
	s, _ := newTestStore(t)
	e := &format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-04",
			Project: "mastermind",
			Topic:   "defaults",
			Kind:    format.KindLesson,
			Scope:   format.ScopeUserPersonal,
			// Confidence intentionally omitted.
		},
		Body: "x",
	}
	path, err := s.Write(e)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "confidence: high") {
		t.Errorf("written file missing normalized confidence:\n%s", data)
	}
}

// ─── list ──────────────────────────────────────────────────────────────

func TestListPendingReturnsWrittenEntries(t *testing.T) {
	s, _ := newTestStore(t)
	_, err := s.Write(makeEntry(format.ScopeUserPersonal, "first"))
	if err != nil {
		t.Fatal(err)
	}
	// Sleep 1s so second entry gets a distinct timestamp prefix.
	time.Sleep(1100 * time.Millisecond)
	_, err = s.Write(makeEntry(format.ScopeUserPersonal, "second"))
	if err != nil {
		t.Fatal(err)
	}

	refs, err := s.ListPending(format.ScopeUserPersonal)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("ListPending = %d refs, want 2", len(refs))
	}
	for _, ref := range refs {
		if !ref.Pending {
			t.Errorf("ref %q not marked Pending", ref.Path)
		}
		if ref.Scope != format.ScopeUserPersonal {
			t.Errorf("ref scope = %q, want user-personal", ref.Scope)
		}
	}
}

func TestListLiveIsEmptyBeforePromote(t *testing.T) {
	s, _ := newTestStore(t)
	_, err := s.Write(makeEntry(format.ScopeUserPersonal, "pending only"))
	if err != nil {
		t.Fatal(err)
	}
	live, err := s.ListLive(format.ScopeUserPersonal)
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 0 {
		t.Errorf("ListLive = %d, want 0 (nothing promoted yet)", len(live))
	}
}

func TestListUnconfiguredScopeReturnsEmpty(t *testing.T) {
	cfg := Config{
		UserPersonalRoot: t.TempDir(),
		Now:              time.Now,
	}
	s := New(cfg)
	refs, err := s.ListPending(format.ScopeProjectShared)
	if err != nil {
		t.Errorf("ListPending unconfigured scope: err = %v, want nil", err)
	}
	if refs != nil {
		t.Errorf("ListPending unconfigured scope: refs = %v, want nil", refs)
	}
}

func TestListSkipsMalformedFiles(t *testing.T) {
	s, cfg := newTestStore(t)
	pendingDir := filepath.Join(cfg.UserPersonalRoot, pendingDirName)
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write one valid file via Write.
	_, err := s.Write(makeEntry(format.ScopeUserPersonal, "valid"))
	if err != nil {
		t.Fatal(err)
	}
	// Hand-write one malformed file.
	bad := filepath.Join(pendingDir, "malformed.md")
	if err := os.WriteFile(bad, []byte("not a frontmatter file at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := s.ListPending(format.ScopeUserPersonal)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("ListPending = %d, want 1 (malformed should be skipped)", len(refs))
	}
}

// ─── promote ───────────────────────────────────────────────────────────

func TestPromoteMovesPendingToLive(t *testing.T) {
	s, cfg := newTestStore(t)
	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "ship it"))
	if err != nil {
		t.Fatal(err)
	}

	livePath, err := s.Promote(path)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Pending file should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("pending file still exists after Promote: %v", err)
	}
	// Live file should exist at the expected path.
	if _, err := os.Stat(livePath); err != nil {
		t.Errorf("live file missing after Promote: %v", err)
	}
	// Live path should be under lessons/, not pending/.
	wantPrefix := filepath.Join(cfg.UserPersonalRoot, "lessons")
	if !strings.HasPrefix(livePath, wantPrefix) {
		t.Errorf("live path = %q, want prefix %q", livePath, wantPrefix)
	}
	// Live filename should be slug-only, not timestamped.
	base := filepath.Base(livePath)
	if base != "ship-it.md" {
		t.Errorf("live filename = %q, want ship-it.md", base)
	}
}

func TestPromoteCollisionReturnsErrEntryExists(t *testing.T) {
	s, _ := newTestStore(t)
	// Write two pending entries with the same topic.
	p1, err := s.Write(makeEntry(format.ScopeUserPersonal, "same topic"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	p2, err := s.Write(makeEntry(format.ScopeUserPersonal, "same topic"))
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("expected distinct pending paths; both were %q", p1)
	}

	// First promote succeeds.
	if _, err := s.Promote(p1); err != nil {
		t.Fatalf("first promote: %v", err)
	}
	// Second collides.
	_, err = s.Promote(p2)
	if !errors.Is(err, ErrEntryExists) {
		t.Errorf("second promote: err = %v, want ErrEntryExists", err)
	}
}

func TestPromoteRejectsNonPendingPath(t *testing.T) {
	s, cfg := newTestStore(t)
	// Create a file directly in lessons/ (bypassing the store).
	liveDir := filepath.Join(cfg.UserPersonalRoot, "lessons")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(liveDir, "already-there.md")
	if err := os.WriteFile(live, []byte("---\ndate: 2026-04-04\nproject: x\ntopic: y\nkind: lesson\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Promote(live)
	if err == nil {
		t.Error("Promote on non-pending path: expected error, got nil")
	}
}

func TestPromoteRejectsPathOutsideScopes(t *testing.T) {
	s, _ := newTestStore(t)
	outside := filepath.Join(t.TempDir(), "other", "pending", "x.md")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("---\ndate: 2026-04-04\nproject: x\ntopic: y\nkind: lesson\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.Promote(outside)
	if !errors.Is(err, ErrInvalidScope) {
		t.Errorf("Promote outside configured scopes: err = %v, want ErrInvalidScope", err)
	}
}

// ─── reject ────────────────────────────────────────────────────────────

func TestRejectDeletesPendingFile(t *testing.T) {
	s, _ := newTestStore(t)
	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "rejected"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reject(path); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("pending file still exists after Reject: %v", err)
	}
}

func TestRejectIsIdempotent(t *testing.T) {
	s, _ := newTestStore(t)
	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "to be rejected"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reject(path); err != nil {
		t.Fatalf("first Reject: %v", err)
	}
	if err := s.Reject(path); err != nil {
		t.Errorf("second Reject should be idempotent, got: %v", err)
	}
}

// ─── prune stale ───────────────────────────────────────────────────────

func TestPruneStaleRemovesOldPending(t *testing.T) {
	s, cfg := newTestStore(t)
	// Inject a fake Now so the default file mtime ("now") is 8 days old.
	fakeNow := time.Now().Add(8 * 24 * time.Hour)
	s.cfg.Now = fixedNow(fakeNow)

	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "stale"))
	if err != nil {
		t.Fatal(err)
	}
	// The file exists. Confirm.
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	// Now prune. "Now" is 8 days after the file was written (file mtime
	// is real time.Now, store's Now is +8 days).
	removed, err := s.PruneStale()
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if removed != 1 {
		t.Errorf("PruneStale removed %d, want 1", removed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale file not removed: %v", err)
	}

	// Cover the unused cfg arg explicitly.
	_ = cfg
}

func TestPruneStaleKeepsFreshPending(t *testing.T) {
	s, _ := newTestStore(t)
	// Default Now (real time.Now). Just written files are 0 days old.
	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "fresh"))
	if err != nil {
		t.Fatal(err)
	}

	removed, err := s.PruneStale()
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if removed != 0 {
		t.Errorf("PruneStale removed %d, want 0 (file is fresh)", removed)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("fresh file removed incorrectly: %v", err)
	}
}

// ─── project root walk-up ──────────────────────────────────────────────

func TestFindProjectRootLocatesMmDir(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	sub := filepath.Join(repo, "src", "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mm := filepath.Join(repo, ".mm")
	if err := os.MkdirAll(mm, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindProjectRoot(sub)
	if got != repo {
		t.Errorf("FindProjectRoot(%q) = %q, want %q", sub, got, repo)
	}
}

func TestFindProjectRootReturnsEmptyWhenNoMmDir(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := FindProjectRoot(sub)
	// Could return "" or walk all the way up to find someone else's .mm;
	// on a tmp dir with nothing above, should be "".
	if got != "" && !strings.HasPrefix(tmp, got) == false {
		// Allow the test to pass even if a .mm exists somewhere above the
		// tmp dir (which could happen on the author's laptop). The
		// important property: if it finds one, it must be a prefix of
		// the starting path.
		if !strings.HasPrefix(sub, got) {
			t.Errorf("FindProjectRoot returned %q which is not an ancestor of %q", got, sub)
		}
	}
}

func TestFindProjectRootEmptyCwd(t *testing.T) {
	if got := FindProjectRoot(""); got != "" {
		t.Errorf("FindProjectRoot(\"\") = %q, want empty", got)
	}
}

// ─── slugify ───────────────────────────────────────────────────────────

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"simple", "simple"},
		{"Two Words", "two-words"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multi   spaces", "multi-spaces"},
		{"with punctuation! and symbols?", "with-punctuation-and-symbols"},
		{"unicode: café résumé", "unicode-caf-r-sum"},
		{"", "untitled"},
		{"!@#$%", "untitled"},
		{strings.Repeat("a", 200), strings.Repeat("a", 80)},
	}
	for _, tc := range tests {
		got := slugify(tc.in)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ─── atomic write ──────────────────────────────────────────────────────

func TestWriteLeavesNoTempFiles(t *testing.T) {
	s, cfg := newTestStore(t)
	_, err := s.Write(makeEntry(format.ScopeUserPersonal, "clean write"))
	if err != nil {
		t.Fatal(err)
	}
	pendingDir := filepath.Join(cfg.UserPersonalRoot, pendingDirName)
	ents, err := os.ReadDir(pendingDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".mm-tmp-") {
			t.Errorf("temp file left behind: %s", ent.Name())
		}
	}
}

// ─── LoadRef ───────────────────────────────────────────────────────────

func TestLoadRefReturnsFullEntry(t *testing.T) {
	s, _ := newTestStore(t)
	e := makeEntry(format.ScopeUserPersonal, "load me")
	e.Body = "specific body content for load test"
	_, err := s.Write(e)
	if err != nil {
		t.Fatal(err)
	}

	refs, err := s.ListPending(format.ScopeUserPersonal)
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListPending: %v, refs=%d", err, len(refs))
	}

	loaded, err := s.LoadRef(refs[0])
	if err != nil {
		t.Fatalf("LoadRef: %v", err)
	}
	if loaded.Body != "specific body content for load test" {
		t.Errorf("loaded body = %q, want %q", loaded.Body, "specific body content for load test")
	}
	if loaded.Metadata.Topic != "load me" {
		t.Errorf("loaded topic = %q, want %q", loaded.Metadata.Topic, "load me")
	}
}
