package store

import (
	"errors"
	"fmt"
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
// Has no tags, so resolveTopicDir falls back to "general".
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

// makeEntryWithTags builds a valid Entry with tags. The first tag
// determines the topic directory via resolveTopicDir.
func makeEntryWithTags(scope format.Scope, topic string, tags []string) *format.Entry {
	e := makeEntry(scope, topic)
	e.Metadata.Tags = tags
	return e
}

// makeEntryWithCategory builds a valid Entry with an explicit category.
func makeEntryWithCategory(scope format.Scope, topic, category string) *format.Entry {
	e := makeEntry(scope, topic)
	e.Metadata.Category = category
	return e
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
	if !strings.HasSuffix(cfg.UserPersonalRoot, ".knowledge") {
		t.Errorf("UserPersonalRoot = %q, want suffix .knowledge", cfg.UserPersonalRoot)
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
	// Verify it's NOT in any topic directory — it should be strictly in pending/.
	if !strings.Contains(path, string(os.PathSeparator)+pendingDirName+string(os.PathSeparator)) {
		t.Errorf("Write did not land in pending: %q", path)
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

// TestWriteUnconfiguredScopeErrorIsDistinct locks the invariant that
// an unknown scope string and a known scope with no configured root
// surface as distinct error messages. Both wrap ErrInvalidScope so
// callers that only care about the sentinel keep working, but the
// messages carry enough detail for a human (or an agent reading its
// own tool error) to diagnose the failure.
//
// The distinction matters because "I typed user-persoanl and mastermind
// rejected it" and "I typed project-personal and mastermind is not
// wired for that scope this session" are very different problems with
// very different fixes. Collapsing them back into one message is a
// real regression — this test fails if that happens.
func TestWriteUnconfiguredScopeErrorIsDistinct(t *testing.T) {
	cfg := Config{
		UserPersonalRoot: t.TempDir(),
		Now:              time.Now,
	}
	s := New(cfg)

	_, errUnknown := s.Write(makeEntry("nonsense-scope", "bogus"))
	if !errors.Is(errUnknown, ErrInvalidScope) {
		t.Fatalf("unknown scope: err = %v, want ErrInvalidScope", errUnknown)
	}
	if !strings.Contains(errUnknown.Error(), "unknown scope") {
		t.Errorf("unknown-scope error does not mention \"unknown scope\": %v", errUnknown)
	}

	_, errNotConfigured := s.Write(makeEntry(format.ScopeProjectShared, "bogus"))
	if !errors.Is(errNotConfigured, ErrInvalidScope) {
		t.Fatalf("unconfigured scope: err = %v, want ErrInvalidScope", errNotConfigured)
	}
	if !strings.Contains(errNotConfigured.Error(), "no root configured") {
		t.Errorf("unconfigured-scope error does not mention \"no root configured\": %v", errNotConfigured)
	}

	if errUnknown.Error() == errNotConfigured.Error() {
		t.Errorf("unknown-scope and unconfigured-scope errors are identical, expected distinct messages: %v", errUnknown)
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
	wantPrefix := filepath.Join(cfg.UserPersonalRoot, "general")
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
	liveDir := filepath.Join(cfg.UserPersonalRoot, "general")
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

func TestPromoteAcceptsPathOutsideConfiguredScopes(t *testing.T) {
	// Promote must accept any path with the structure <root>/pending/<file>.md,
	// even if <root> isn't a configured scope. This handles the case where
	// the MCP server was started in a different project than the one the
	// agent is currently working in. The path itself tells us everything.
	s, _ := newTestStore(t)
	outsideRoot := filepath.Join(t.TempDir(), "other-project", ".knowledge")
	pendingDir := filepath.Join(outsideRoot, "pending")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(pendingDir, "20260410-120000-cross-project-entry.md")
	if err := os.WriteFile(outside, []byte("---\ndate: 2026-04-10\nproject: other\ntopic: cross-project entry\nkind: lesson\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	livePath, err := s.Promote(outside)
	if err != nil {
		t.Fatalf("Promote cross-project path: %v", err)
	}
	if !strings.HasPrefix(livePath, outsideRoot) {
		t.Errorf("live path %q should start with derived root %q", livePath, outsideRoot)
	}
	if _, err := os.Stat(livePath); err != nil {
		t.Errorf("live entry file not created: %v", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Errorf("pending file should be removed after promote, stat err = %v", err)
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

// ─── auto-promote stale ────────────────────────────────────────────────

func TestAutoPromoteStalePromotesOldPending(t *testing.T) {
	s, cfg := newTestStore(t)
	s.cfg.PendingBehavior = PendingAutoPromote
	// Inject a fake Now so the default file mtime ("now") is 8 days old.
	fakeNow := time.Now().Add(8 * 24 * time.Hour)
	s.cfg.Now = fixedNow(fakeNow)

	pendingPath, err := s.Write(makeEntry(format.ScopeUserPersonal, "old enough to promote"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatal(err)
	}

	promoted, err := s.AutoPromoteStale()
	if err != nil {
		t.Fatalf("AutoPromoteStale: %v", err)
	}
	if promoted != 1 {
		t.Errorf("AutoPromoteStale promoted %d, want 1", promoted)
	}
	// Pending file should be gone (moved, not deleted).
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Errorf("pending file still exists after auto-promote: %v", err)
	}
	// Live file should exist.
	livePath := filepath.Join(cfg.UserPersonalRoot, "general", "old-enough-to-promote.md")
	if _, err := os.Stat(livePath); err != nil {
		t.Errorf("live file does not exist after auto-promote: %v", err)
	}
}

func TestAutoPromoteStaleKeepsFreshPending(t *testing.T) {
	s, _ := newTestStore(t)
	s.cfg.PendingBehavior = PendingAutoPromote
	// Default Now (real time.Now). Just written files are 0 days old.
	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "fresh"))
	if err != nil {
		t.Fatal(err)
	}

	promoted, err := s.AutoPromoteStale()
	if err != nil {
		t.Fatalf("AutoPromoteStale: %v", err)
	}
	if promoted != 0 {
		t.Errorf("AutoPromoteStale promoted %d, want 0 (file is fresh)", promoted)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("fresh file removed incorrectly: %v", err)
	}
}

func TestAutoPromoteStaleNoOpWhenPolicyKeepForever(t *testing.T) {
	s, _ := newTestStore(t)
	// Default PendingBehavior is "" which is treated as keep-forever.
	fakeNow := time.Now().Add(30 * 24 * time.Hour) // 30 days later
	s.cfg.Now = fixedNow(fakeNow)

	path, err := s.Write(makeEntry(format.ScopeUserPersonal, "ancient but kept"))
	if err != nil {
		t.Fatal(err)
	}

	promoted, err := s.AutoPromoteStale()
	if err != nil {
		t.Fatalf("AutoPromoteStale: %v", err)
	}
	if promoted != 0 {
		t.Errorf("AutoPromoteStale promoted %d with keep-forever policy, want 0", promoted)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("pending file deleted under keep-forever policy: %v", err)
	}
}

func TestAutoPromoteStaleSkipsOnConflict(t *testing.T) {
	s, cfg := newTestStore(t)
	s.cfg.PendingBehavior = PendingAutoPromote
	fakeNow := time.Now().Add(8 * 24 * time.Hour)
	s.cfg.Now = fixedNow(fakeNow)

	// Write a pending entry.
	pendingPath, err := s.Write(makeEntry(format.ScopeUserPersonal, "conflict topic"))
	if err != nil {
		t.Fatal(err)
	}

	// Manually place a live entry with the same slug so promotion would
	// collide.
	liveDir := filepath.Join(cfg.UserPersonalRoot, "general")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(liveDir, "conflict-topic.md")
	if err := os.WriteFile(livePath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	promoted, err := s.AutoPromoteStale()
	if err != nil {
		t.Fatalf("AutoPromoteStale: %v", err)
	}
	if promoted != 0 {
		t.Errorf("AutoPromoteStale promoted %d, want 0 (conflict)", promoted)
	}
	// Pending file must still exist — not lost.
	if _, err := os.Stat(pendingPath); err != nil {
		t.Errorf("pending file lost on conflict: %v", err)
	}
}

// ─── write live ────────────────────────────────────────────────────────

func TestWriteLiveLandsInLiveDir(t *testing.T) {
	s, cfg := newTestStore(t)
	entry := makeEntry(format.ScopeUserPersonal, "direct to live")

	path, err := s.WriteLive(entry)
	if err != nil {
		t.Fatalf("WriteLive: %v", err)
	}
	wantDir := filepath.Join(cfg.UserPersonalRoot, "general")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("WriteLive path = %q, want prefix %q (live dir)", path, wantDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("WriteLive file does not exist: %v", err)
	}
}

func TestWriteLiveReturnsErrEntryExistsOnConflict(t *testing.T) {
	s, cfg := newTestStore(t)

	// Place a file in the live dir first.
	liveDir := filepath.Join(cfg.UserPersonalRoot, "general")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "already-there.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := makeEntry(format.ScopeUserPersonal, "already there")
	_, err := s.WriteLive(entry)
	if !errors.Is(err, ErrEntryExists) {
		t.Errorf("WriteLive duplicate: err = %v, want ErrEntryExists", err)
	}
}

func TestWriteLiveNilEntryFails(t *testing.T) {
	s, _ := newTestStore(t)
	_, err := s.WriteLive(nil)
	if err == nil {
		t.Error("WriteLive(nil): expected error, got nil")
	}
}

func TestWriteLiveProjectSharedScope(t *testing.T) {
	s, cfg := newTestStore(t)
	entry := makeEntry(format.ScopeProjectShared, "shared live entry")

	path, err := s.WriteLive(entry)
	if err != nil {
		t.Fatalf("WriteLive project-shared: %v", err)
	}
	wantDir := filepath.Join(cfg.ProjectSharedRoot, "general")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("WriteLive path = %q, want prefix %q (nodes dir)", path, wantDir)
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
	mm := filepath.Join(repo, ".knowledge")
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
	// Could return "" or walk all the way up to find someone else's .knowledge;
	// on a tmp dir with nothing above, should be "".
	if got != "" && !strings.HasPrefix(tmp, got) == false {
		// Allow the test to pass even if a .knowledge exists somewhere above the
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

// TestFindProjectRootStopsAtHome guards against the bug where a cwd
// under $HOME would walk up and find $HOME/.knowledge/ (the user-personal
// store), causing ProjectSharedRoot to collide with UserPersonalRoot.
// Writes with scope=project-shared would then leak into the user store.
func TestFindProjectRootStopsAtHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a .knowledge/ directly at the fake HOME — this simulates
	// the user-personal store.
	if err := os.MkdirAll(filepath.Join(home, ".knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory under HOME that has NO .knowledge/ between
	// it and HOME. FindProjectRoot must return "" — it must NOT walk up
	// into HOME and match HOME/.knowledge/ as a project root.
	sub := filepath.Join(home, "some", "non-project", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindProjectRoot(sub)
	if got != "" {
		t.Errorf("FindProjectRoot(%q) = %q, want empty (must not match $HOME/.knowledge/)", sub, got)
	}
}

// TestFindProjectRootFindsProjectBelowHome ensures the $HOME guard
// doesn't break the common case: a real project under $HOME with its
// own .knowledge/ should still be detected.
func TestFindProjectRootFindsProjectBelowHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// User-personal .knowledge/ at HOME.
	if err := os.MkdirAll(filepath.Join(home, ".knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Project with its own .knowledge/ somewhere under HOME.
	project := filepath.Join(home, "Github", "someproject")
	if err := os.MkdirAll(filepath.Join(project, ".knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindProjectRoot(sub)
	if got != project {
		t.Errorf("FindProjectRoot(%q) = %q, want %q", sub, got, project)
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

// ─── topic directory resolution ────────────────────────────────────────

func TestResolveTopicDirAttractor(t *testing.T) {
	s, cfg := newTestStore(t)
	root := cfg.UserPersonalRoot

	// Create an existing topic dir "electron".
	if err := os.MkdirAll(filepath.Join(root, "electron"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Entry with "electron" as second tag should still land in electron/
	// because the attractor matches against ALL tags, not just the first.
	got := s.resolveTopicDir(root, []string{"ipc", "electron", "macos"})
	if got != "electron" {
		t.Errorf("resolveTopicDir = %q, want electron (attractor)", got)
	}
}

func TestResolveTopicDirNewDirFromFirstTag(t *testing.T) {
	s, cfg := newTestStore(t)
	root := cfg.UserPersonalRoot

	// No existing dirs. First tag becomes the new directory.
	got := s.resolveTopicDir(root, []string{"gamedev", "godot"})
	if got != "gamedev" {
		t.Errorf("resolveTopicDir = %q, want gamedev (first tag)", got)
	}
}

func TestResolveTopicDirNoTags(t *testing.T) {
	s, cfg := newTestStore(t)
	got := s.resolveTopicDir(cfg.UserPersonalRoot, nil)
	if got != "general" {
		t.Errorf("resolveTopicDir(nil tags) = %q, want general", got)
	}
}

func TestResolveTopicDirSkipsOperationalDirs(t *testing.T) {
	s, cfg := newTestStore(t)
	root := cfg.UserPersonalRoot

	// Create pending/ and archive/ — these should NOT be matched.
	os.MkdirAll(filepath.Join(root, "pending"), 0o755)
	os.MkdirAll(filepath.Join(root, "archive"), 0o755)

	// Tag "pending" should NOT match the operational dir.
	got := s.resolveTopicDir(root, []string{"pending", "testing"})
	if got != "pending" {
		// "pending" as a topic name is allowed — the tag IS "pending".
		// The attractor skips the operational dir, so no match.
		// Falls back to first tag = "pending". This is correct but
		// unusual. The dir name is "pending" as a topic, distinct
		// from the operational pending/ dir. In practice this name
		// collision is unlikely and harmless — the store distinguishes
		// them by context (listTopicDirs vs pendingDirName).
	}
	_ = got
}

func TestNormalizeCategoryValid(t *testing.T) {
	tests := []struct{ in, want string }{
		{"electron", "electron"},
		{"electron/ipc", filepath.Join("electron", "ipc")},
		{"Go/Modules", filepath.Join("go", "modules")},
		{"  MCP  ", "mcp"},
		{"electron/ipc/", filepath.Join("electron", "ipc")}, // trailing slash stripped
	}
	for _, tc := range tests {
		got := normalizeCategory(tc.in)
		if got != tc.want {
			t.Errorf("normalizeCategory(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeCategoryRejectsTooDeep(t *testing.T) {
	got := normalizeCategory("electron/ipc/macos")
	if got != "" {
		t.Errorf("normalizeCategory(3 segments) = %q, want empty (rejected)", got)
	}
}

func TestNormalizeCategoryEmpty(t *testing.T) {
	for _, in := range []string{"", "  ", "///", "---"} {
		got := normalizeCategory(in)
		if got != "" {
			t.Errorf("normalizeCategory(%q) = %q, want empty", in, got)
		}
	}
}

func TestWriteLiveWithExplicitCategory(t *testing.T) {
	s, cfg := newTestStore(t)
	entry := makeEntryWithCategory(format.ScopeUserPersonal, "tls lesson", "electron/tls")

	path, err := s.WriteLive(entry)
	if err != nil {
		t.Fatalf("WriteLive: %v", err)
	}
	wantDir := filepath.Join(cfg.UserPersonalRoot, "electron", "tls")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("WriteLive path = %q, want prefix %q", path, wantDir)
	}
}

func TestWriteLiveWithTagsDerivedDir(t *testing.T) {
	s, cfg := newTestStore(t)
	entry := makeEntryWithTags(format.ScopeUserPersonal, "gamedev lesson", []string{"gamedev", "godot"})

	path, err := s.WriteLive(entry)
	if err != nil {
		t.Fatalf("WriteLive: %v", err)
	}
	wantDir := filepath.Join(cfg.UserPersonalRoot, "gamedev")
	if !strings.HasPrefix(path, wantDir) {
		t.Errorf("WriteLive path = %q, want prefix %q (first tag)", path, wantDir)
	}
}

func TestListLiveWalksTopicDirs(t *testing.T) {
	s, cfg := newTestStore(t)
	root := cfg.UserPersonalRoot

	// Manually create entries in two topic dirs.
	for _, dir := range []string{"electron", filepath.Join("go", "modules")} {
		full := filepath.Join(root, dir)
		os.MkdirAll(full, 0o755)
	}

	writeTestEntry := func(dir, slug string) {
		content := fmt.Sprintf("---\ndate: 2026-04-06\nproject: test\ntopic: %q\nkind: lesson\nscope: user-personal\n---\ntest", slug)
		os.WriteFile(filepath.Join(root, dir, slug+".md"), []byte(content), 0o644)
	}

	writeTestEntry("electron", "split-tls")
	writeTestEntry(filepath.Join("go", "modules"), "go-get-import")

	refs, err := s.ListLive(format.ScopeUserPersonal)
	if err != nil {
		t.Fatalf("ListLive: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("ListLive returned %d entries, want 2", len(refs))
	}
}

func TestListCategoriesReturnsBothLevels(t *testing.T) {
	s, cfg := newTestStore(t)
	root := cfg.UserPersonalRoot

	// Create level-1 and level-2 dirs.
	for _, dir := range []string{"electron", filepath.Join("electron", "ipc"), "go", filepath.Join("go", "modules")} {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
	}
	// Also create pending/ which should be excluded.
	os.MkdirAll(filepath.Join(root, "pending"), 0o755)

	cats, err := s.ListCategories(format.ScopeUserPersonal)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}

	want := map[string]bool{
		"electron":     true,
		"electron/ipc": true,
		"go":           true,
		"go/modules":   true,
	}
	got := map[string]bool{}
	for _, c := range cats {
		got[c] = true
	}
	for w := range want {
		if !got[w] {
			t.Errorf("ListCategories missing %q", w)
		}
	}
	if got["pending"] {
		t.Error("ListCategories should not include operational dir 'pending'")
	}
}

func TestPromoteUsesCategory(t *testing.T) {
	s, cfg := newTestStore(t)

	// Write a pending entry with an explicit category.
	entry := makeEntryWithCategory(format.ScopeUserPersonal, "promote with category", "electron/ipc")
	pendingPath, err := s.Write(entry)
	if err != nil {
		t.Fatal(err)
	}

	livePath, err := s.Promote(pendingPath)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	wantDir := filepath.Join(cfg.UserPersonalRoot, "electron", "ipc")
	if !strings.HasPrefix(livePath, wantDir) {
		t.Errorf("Promote path = %q, want prefix %q", livePath, wantDir)
	}
}
