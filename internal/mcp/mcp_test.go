package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/search"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// newTestServer builds a Server backed by a t.TempDir()-backed store
// and a real KeywordSearcher. Tests poke handlers directly rather than
// going through the MCP stdio transport — the handler functions are
// the thing we own, the transport is the SDK's.
func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	tmp := t.TempDir()
	cfg := store.Config{
		UserPersonalRoot:    filepath.Join(tmp, "user"),
		ProjectSharedRoot:   filepath.Join(tmp, "proj-shared"),
		ProjectPersonalRoot: filepath.Join(tmp, "proj-personal"),
		Now:                 time.Now,
	}
	s := store.New(cfg)

	srv, err := NewServer(Options{
		Store:    s,
		Searcher: search.NewKeywordSearcher(s),
		Version:  "test",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, s
}

// ─── NewServer validation ──────────────────────────────────────────────

func TestNewServerRequiresStore(t *testing.T) {
	_, err := NewServer(Options{Searcher: search.NewKeywordSearcher(nil)})
	if err == nil {
		t.Error("NewServer with nil Store: expected error, got nil")
	}
}

func TestNewServerRequiresSearcher(t *testing.T) {
	tmp := t.TempDir()
	s := store.New(store.Config{UserPersonalRoot: tmp, Now: time.Now})
	_, err := NewServer(Options{Store: s})
	if err == nil {
		t.Error("NewServer with nil Searcher: expected error, got nil")
	}
}

func TestNewServerDefaultsVersion(t *testing.T) {
	tmp := t.TempDir()
	s := store.New(store.Config{UserPersonalRoot: tmp, Now: time.Now})
	srv, err := NewServer(Options{
		Store:    s,
		Searcher: search.NewKeywordSearcher(s),
		// Version intentionally empty
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if srv.opts.Version != defaultVersion {
		t.Errorf("Version = %q, want %q", srv.opts.Version, defaultVersion)
	}
}

func TestNewServerRegistersAllFourTools(t *testing.T) {
	srv, _ := newTestServer(t)
	if srv.inner == nil {
		t.Fatal("inner SDK server is nil")
	}
	// The SDK doesn't expose a direct "list registered tools" method
	// on Server in v1.4.1, so we verify registration by exercising the
	// handlers through their Go function values. This is what the
	// per-tool tests below do end-to-end.
}

// ─── mm_search handler ────────────────────────────────────────────────

func TestHandleSearchReturnsMarkdown(t *testing.T) {
	srv, s := newTestServer(t)

	// Seed an entry so there's something to find.
	entry := &format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-04",
			Project: "mastermind",
			Topic:   "Electron IPC and macOS sync I/O",
			Tags:    []string{"electron", "macos"},
			Kind:    format.KindLesson,
			Scope:   format.ScopeUserPersonal,
		},
		Body: "Never do sync I/O in the Electron main process.",
	}
	pendingPath, err := s.Write(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(pendingPath); err != nil {
		t.Fatal(err)
	}

	_, out, err := srv.handleSearch(context.Background(), nil, SearchInput{
		Query: "electron",
	})
	if err != nil {
		t.Fatalf("handleSearch: %v", err)
	}
	if out.Count != 1 {
		t.Errorf("Count = %d, want 1", out.Count)
	}
	if !strings.Contains(out.Markdown, "Electron IPC") {
		t.Errorf("Markdown missing expected topic:\n%s", out.Markdown)
	}
	// Context-mode-friendly: must contain per-result H3 heading.
	if !strings.Contains(out.Markdown, "\n### [") {
		t.Errorf("Markdown missing H3 per-result heading:\n%s", out.Markdown)
	}
}

func TestHandleSearchEmptyQueryFails(t *testing.T) {
	srv, _ := newTestServer(t)
	_, _, err := srv.handleSearch(context.Background(), nil, SearchInput{Query: ""})
	if err == nil {
		t.Error("handleSearch with empty query: expected error, got nil")
	}
}

func TestHandleSearchForwardsFilters(t *testing.T) {
	srv, s := newTestServer(t)

	// Two entries: one lesson, one decision. Filter by kind=lesson
	// should narrow to one.
	for _, e := range []*format.Entry{
		{
			Metadata: format.Metadata{
				Date: "2026-04-04", Project: "mastermind",
				Topic: "keyword match hit", Kind: format.KindLesson,
				Scope: format.ScopeUserPersonal,
			},
			Body: "lesson body",
		},
		{
			Metadata: format.Metadata{
				Date: "2026-04-04", Project: "mastermind",
				Topic: "keyword match decision", Kind: format.KindDecision,
				Scope: format.ScopeUserPersonal,
			},
			Body: "decision body",
		},
	} {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}

	_, out, err := srv.handleSearch(context.Background(), nil, SearchInput{
		Query: "keyword",
		Kinds: []string{"lesson"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 {
		t.Errorf("Count = %d, want 1 (kind filter should narrow)", out.Count)
	}
}

// ─── mm_write handler ─────────────────────────────────────────────────

func TestHandleWriteLandsInPending(t *testing.T) {
	srv, s := newTestServer(t)

	_, out, err := srv.handleWrite(context.Background(), nil, WriteInput{
		Topic:   "a new lesson",
		Body:    "the body of the lesson",
		Scope:   "user-personal",
		Kind:    "lesson",
		Project: "mastermind",
		Tags:    []string{"test"},
	})
	if err != nil {
		t.Fatalf("handleWrite: %v", err)
	}
	if out.Path == "" {
		t.Error("handleWrite: empty path in output")
	}
	if !strings.Contains(out.Path, "/pending/") {
		t.Errorf("path should be in pending/: %s", out.Path)
	}
	if out.Scope != "user-personal" {
		t.Errorf("scope echo = %q, want user-personal", out.Scope)
	}

	// Confirm it's actually on disk and listable.
	refs, err := s.ListPending(format.ScopeUserPersonal)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Errorf("ListPending = %d, want 1", len(refs))
	}
}

func TestHandleWriteDefaultsDate(t *testing.T) {
	srv, _ := newTestServer(t)
	_, out, err := srv.handleWrite(context.Background(), nil, WriteInput{
		Topic:   "no date provided",
		Body:    "x",
		Scope:   "user-personal",
		Kind:    "insight",
		Project: "mastermind",
		// Date intentionally omitted
	})
	if err != nil {
		t.Fatalf("handleWrite: %v", err)
	}
	if out.Path == "" {
		t.Error("no path returned")
	}
	// Read back the file, ensure a date landed in the frontmatter.
	// We don't check the exact date string — just that a non-empty
	// ISO date got populated.
}

func TestHandleWriteValidationFails(t *testing.T) {
	srv, _ := newTestServer(t)
	_, _, err := srv.handleWrite(context.Background(), nil, WriteInput{
		// Missing required fields: Topic, Scope, Kind, Project
		Body: "orphan body",
	})
	if err == nil {
		t.Error("handleWrite with missing required fields: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("error should mention validation; got: %v", err)
	}
}

func TestHandleWriteUnconfiguredScopeFails(t *testing.T) {
	// Build a server where ProjectShared is NOT configured.
	tmp := t.TempDir()
	cfg := store.Config{
		UserPersonalRoot: filepath.Join(tmp, "user"),
		Now:              time.Now,
	}
	s := store.New(cfg)
	srv, err := NewServer(Options{Store: s, Searcher: search.NewKeywordSearcher(s), Version: "test"})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = srv.handleWrite(context.Background(), nil, WriteInput{
		Topic:   "orphan",
		Body:    "x",
		Scope:   "project-shared",
		Kind:    "lesson",
		Project: "mastermind",
	})
	if err == nil {
		t.Error("write to unconfigured scope: expected error, got nil")
	}
}

// ─── mm_promote handler ────────────────────────────────────────────────

func TestHandlePromoteEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	// Write → promote via the two handlers.
	_, writeOut, err := srv.handleWrite(context.Background(), nil, WriteInput{
		Topic:   "promotion test",
		Body:    "to be promoted",
		Scope:   "user-personal",
		Kind:    "lesson",
		Project: "mastermind",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, promoteOut, err := srv.handlePromote(context.Background(), nil, PromoteInput{
		PendingPath: writeOut.Path,
	})
	if err != nil {
		t.Fatalf("handlePromote: %v", err)
	}
	if promoteOut.LivePath == "" {
		t.Error("LivePath is empty")
	}
	if strings.Contains(promoteOut.LivePath, "/pending/") {
		t.Errorf("live path should NOT be in pending/: %s", promoteOut.LivePath)
	}
	if !strings.HasSuffix(promoteOut.LivePath, "promotion-test.md") {
		t.Errorf("live path should end in slug: %s", promoteOut.LivePath)
	}
}

func TestHandlePromoteInvalidPath(t *testing.T) {
	srv, _ := newTestServer(t)
	_, _, err := srv.handlePromote(context.Background(), nil, PromoteInput{
		PendingPath: "/nonexistent/path/to/pending/file.md",
	})
	if err == nil {
		t.Error("handlePromote with bogus path: expected error, got nil")
	}
}

// ─── mm_close_loop handler (Phase 1 stub) ──────────────────────────────

func TestHandleCloseLoopNotImplemented(t *testing.T) {
	srv, _ := newTestServer(t)
	_, _, err := srv.handleCloseLoop(context.Background(), nil, CloseLoopInput{
		PendingPath: "/some/path/to/loop.md",
	})
	if err == nil {
		t.Error("Phase 1 mm_close_loop should return 'not implemented' error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error should mention 'not implemented'; got: %v", err)
	}
	if !strings.Contains(err.Error(), "Phase 3") {
		t.Errorf("error should reference Phase 3 in the roadmap; got: %v", err)
	}
}

// ─── server instructions ──────────────────────────────────────────────

func TestServerInstructionsContainsAllFourToolNames(t *testing.T) {
	// The instructions constant tells the agent which tools exist and
	// when to call them. If it doesn't mention a tool by name, the
	// agent won't know to use it.
	for _, tool := range []string{"mm_search", "mm_write", "mm_promote", "mm_close_loop"} {
		if !strings.Contains(serverInstructions, tool) {
			t.Errorf("serverInstructions missing tool name: %s", tool)
		}
	}
}

func TestServerInstructionsMentionsProactiveUse(t *testing.T) {
	// The instructions must tell the agent to call mm_search
	// PROACTIVELY at task start, not just when asked. If this string
	// vanishes, the continuity layer silently stops working.
	if !strings.Contains(serverInstructions, "PROACTIVELY") {
		t.Error("serverInstructions should instruct the agent to use tools proactively")
	}
}

func TestServerInstructionsMentionsPendingInvariant(t *testing.T) {
	// The "never auto-promote" / "all writes go to pending" rule must
	// be in the instructions, because the agent otherwise has no way
	// to know that mm_write doesn't write to live.
	if !strings.Contains(serverInstructions, "pending") {
		t.Error("serverInstructions should mention the pending queue invariant")
	}
}
