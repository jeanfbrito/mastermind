package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// ─── helpers ──────────────────────────────────────────────────────────

// makeEntry builds a minimal Entry for use in post-compact tests.
func makeEntry(topic string, kind format.Kind, scope format.Scope) *format.Entry {
	return &format.Entry{
		Metadata: format.Metadata{
			Date:    time.Now().Format("2006-01-02"),
			Topic:   topic,
			Kind:    kind,
			Scope:   scope,
			Project: "testproject",
		},
		Body: "test body",
	}
}

// seedEntry writes an entry to the live store.
func seedEntry(t *testing.T, s *store.Store, entry *format.Entry) {
	t.Helper()
	if _, err := s.WriteLive(entry); err != nil {
		t.Fatalf("seedEntry: %v", err)
	}
}

// makeProjectStore sets up a temporary project-shared .knowledge/ directory
// and returns a store.Config scoped to that project only (user-personal
// and project-personal are both pointing at temp dirs that are empty).
func makeProjectStore(t *testing.T) (store.Config, string) {
	t.Helper()
	home := withFakeHome(t)
	projectRoot := t.TempDir()
	knowledgeDir := filepath.Join(projectRoot, ".knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := store.Config{
		UserPersonalRoot:    filepath.Join(home, ".knowledge"),
		ProjectSharedRoot:   knowledgeDir,
		ProjectPersonalRoot: filepath.Join(home, ".claude", "projects", "testproject", "memory"),
	}
	return cfg, projectRoot
}

// ─── collectProjectOpenLoops ──────────────────────────────────────────

// TestCollectProjectOpenLoops_Empty verifies that when there is no
// knowledge at all, the function returns an empty slice (not an error).
func TestCollectProjectOpenLoops_Empty(t *testing.T) {
	cfg, _ := makeProjectStore(t)
	s := store.New(cfg)

	loops, err := collectProjectOpenLoops(s)
	if err != nil {
		t.Fatalf("collectProjectOpenLoops: %v", err)
	}
	if len(loops) != 0 {
		t.Errorf("got %d loops, want 0", len(loops))
	}
}

// TestCollectProjectOpenLoops_ProjectScopedOnly verifies that open loops
// from project-shared are included and open loops from user-personal are
// NOT included. This is the key scope-narrowing invariant for PostCompact.
func TestCollectProjectOpenLoops_ProjectScopedOnly(t *testing.T) {
	cfg, _ := makeProjectStore(t)

	// Write a user-personal open loop — must NOT appear in post-compact output.
	if err := os.MkdirAll(cfg.UserPersonalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	s := store.New(cfg)
	seedEntry(t, s, makeEntry("user-level open loop", format.KindOpenLoop, format.ScopeUserPersonal))

	// Write a project-shared open loop — MUST appear.
	seedEntry(t, s, makeEntry("project open loop", format.KindOpenLoop, format.ScopeProjectShared))

	loops, err := collectProjectOpenLoops(s)
	if err != nil {
		t.Fatalf("collectProjectOpenLoops: %v", err)
	}

	if len(loops) != 1 {
		t.Fatalf("got %d loops, want 1 (project-scoped only)", len(loops))
	}
	if loops[0].Metadata.Topic != "project open loop" {
		t.Errorf("got topic %q, want %q", loops[0].Metadata.Topic, "project open loop")
	}
}

// TestCollectProjectOpenLoops_NonLoopEntriesExcluded checks that lesson
// entries in project-shared scope do not appear in open-loop results.
func TestCollectProjectOpenLoops_NonLoopEntriesExcluded(t *testing.T) {
	cfg, _ := makeProjectStore(t)
	s := store.New(cfg)

	seedEntry(t, s, makeEntry("a lesson entry", format.KindLesson, format.ScopeProjectShared))

	loops, err := collectProjectOpenLoops(s)
	if err != nil {
		t.Fatalf("collectProjectOpenLoops: %v", err)
	}
	if len(loops) != 0 {
		t.Errorf("got %d loops, want 0 (lessons are not open-loops)", len(loops))
	}
}

// ─── formatPostCompact ────────────────────────────────────────────────

// TestFormatPostCompact_EmptyReturnsEmpty verifies the silent-unless-needed
// rule: when there is nothing to surface, output must be empty string.
func TestFormatPostCompact_EmptyReturnsEmpty(t *testing.T) {
	out := formatPostCompact(nil, nil)
	if out != "" {
		t.Errorf("formatPostCompact(nil, nil) = %q, want empty string", out)
	}
}

// TestFormatPostCompact_NonEmptyWhenLoopsExist checks that output is
// non-empty when there are open loops to surface.
func TestFormatPostCompact_NonEmptyWhenLoopsExist(t *testing.T) {
	loops := []store.EntryRef{
		{Metadata: format.Metadata{Topic: "implement auth", Kind: format.KindOpenLoop, Date: "2026-04-10"}},
	}
	out := formatPostCompact(loops, nil)
	if out == "" {
		t.Fatal("formatPostCompact returned empty string with open loops present")
	}
	if !strings.Contains(out, "implement auth") {
		t.Errorf("output missing open loop topic: %q", out)
	}
	if !strings.Contains(out, "post-compact") {
		t.Errorf("output missing post-compact header: %q", out)
	}
}

// TestFormatPostCompact_NonEmptyWhenProjectEntriesExist checks that
// output is non-empty when there are project knowledge entries.
func TestFormatPostCompact_NonEmptyWhenProjectEntriesExist(t *testing.T) {
	entries := []store.EntryRef{
		{Metadata: format.Metadata{Topic: "store package uses three scopes", Kind: format.KindLesson, Date: "2026-04-10"}},
	}
	out := formatPostCompact(nil, entries)
	if out == "" {
		t.Fatal("formatPostCompact returned empty string with project entries present")
	}
	if !strings.Contains(out, "store package uses three scopes") {
		t.Errorf("output missing project entry topic: %q", out)
	}
}

// TestFormatPostCompact_OmitsPendingCount verifies that the pending review
// count notice (present in session-start output) is absent from post-compact
// output — it's noise after compaction.
func TestFormatPostCompact_OmitsPendingCount(t *testing.T) {
	entries := []store.EntryRef{
		{Metadata: format.Metadata{Topic: "some lesson", Kind: format.KindLesson, Date: "2026-04-10"}},
	}
	out := formatPostCompact(nil, entries)
	if strings.Contains(out, "Pending review") {
		t.Errorf("formatPostCompact output should not contain pending count, got: %q", out)
	}
}

// ─── dispatch integration ─────────────────────────────────────────────

// TestPostCompactDispatch_SubcommandExists verifies that the post-compact
// subcommand is wired into the dispatch table. We run the binary against
// an empty directory with no knowledge store and expect silent success
// (exit 0, empty stdout) rather than "unknown command" (exit 2).
func TestPostCompactDispatch_SubcommandExists(t *testing.T) {
	// This test exercises the function directly rather than via subprocess
	// since we're in the same package. We simulate the dispatch by calling
	// runPostCompact with a controlled env (fake home, no stdin).
	withFakeHome(t)

	// Redirect stdin to /dev/null so json.Decoder gets EOF immediately.
	oldStdin := os.Stdin
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	t.Cleanup(func() {
		os.Stdin = oldStdin
		devNull.Close()
	})

	// Set --cwd to a temp dir with no .knowledge/.
	oldArgs := os.Args
	os.Args = []string{"mastermind", "post-compact", "--cwd", t.TempDir()}
	t.Cleanup(func() { os.Args = oldArgs })

	if err := runPostCompact(); err != nil {
		t.Fatalf("runPostCompact: %v", err)
	}
}

// TestPostCompactDispatch_GracefulWhenNoKnowledge verifies that when
// there is no project knowledge store at all, runPostCompact succeeds
// silently (no output, no error). This is the common case for new repos.
func TestPostCompactDispatch_GracefulWhenNoKnowledge(t *testing.T) {
	withFakeHome(t)

	// Point stdin at /dev/null (no hook JSON).
	oldStdin := os.Stdin
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	t.Cleanup(func() {
		os.Stdin = oldStdin
		devNull.Close()
	})

	cwd := t.TempDir()
	oldArgs := os.Args
	os.Args = []string{"mastermind", "post-compact", "--cwd", cwd}
	t.Cleanup(func() { os.Args = oldArgs })

	// Capture stdout to verify silence.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	if err := runPostCompact(); err != nil {
		t.Fatalf("runPostCompact: %v", err)
	}

	w.Close()
	var buf strings.Builder
	bufBytes := make([]byte, 4096)
	for {
		n, err := r.Read(bufBytes)
		if n > 0 {
			buf.Write(bufBytes[:n])
		}
		if err != nil {
			break
		}
	}
	r.Close()

	if buf.Len() > 0 {
		t.Errorf("runPostCompact produced unexpected output: %q", buf.String())
	}
}
