package search

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// ─── tokenize ──────────────────────────────────────────────────────────

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"electron ipc", []string{"electron", "ipc"}},
		{"Electron IPC", []string{"electron", "ipc"}},
		{"  spaces   everywhere  ", []string{"spaces", "everywhere"}},
		{"punctuation!? mixed.", []string{"punctuation", "mixed"}},
		{"single-letter a b c words", []string{"single", "letter", "words"}},
		{"", nil},
		{"!!??", nil},
		{"macos-sync-io", []string{"macos", "sync", "io"}}, // "io" is 2 chars, kept
	}
	for _, tc := range tests {
		got := tokenize(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// ─── scoring functions ────────────────────────────────────────────────

func TestScoreTopicAndTags(t *testing.T) {
	md := format.Metadata{
		Topic: "macOS Electron IPC hangs when main process blocks",
		Tags:  []string{"electron", "ipc", "macos", "debugging"},
	}

	// Token found in topic (should be dominant — see scoreTopicAndTags comment)
	if s := scoreTopicAndTags(md, []string{"electron"}); s < 2.0 {
		t.Errorf("topic hit score = %v, want >= 2.0 (topic dominates)", s)
	}

	// Token found only in tags
	if s := scoreTopicAndTags(md, []string{"debugging"}); s < 0.6 || s > 0.8 {
		t.Errorf("tag hit score = %v, want ~0.7", s)
	}

	// Token found in neither
	if s := scoreTopicAndTags(md, []string{"banana"}); s != 0 {
		t.Errorf("miss score = %v, want 0", s)
	}

	// Multiple tokens accumulate: topic + tag = 2.0 + 0.7 = 2.7
	s := scoreTopicAndTags(md, []string{"electron", "debugging"})
	if s < 2.5 {
		t.Errorf("combined score = %v, want > 2.5", s)
	}
}

func TestScoreBody(t *testing.T) {
	body := "This lesson is about electron main process sync io. The fix was to move io off main."

	// Single hit
	if s := scoreBody(body, []string{"lesson"}); s != 0.3 {
		t.Errorf("single hit = %v, want 0.3", s)
	}

	// Multiple hits of same token should diminish
	s1 := scoreBody(body, []string{"electron"})
	s2 := scoreBody(body, []string{"io"})
	if s1 < 0.3 || s2 < 0.3 {
		t.Errorf("body hits too low: %v, %v", s1, s2)
	}

	// Miss contributes nothing
	if s := scoreBody(body, []string{"kubernetes"}); s != 0 {
		t.Errorf("miss score = %v, want 0", s)
	}

	// Empty body
	if s := scoreBody("", []string{"anything"}); s != 0 {
		t.Errorf("empty body score = %v, want 0", s)
	}
}

// ─── metadata filter ───────────────────────────────────────────────────

func TestMatchesMetadataFilters(t *testing.T) {
	ref := store.EntryRef{
		Metadata: format.Metadata{
			Kind:    format.KindLesson,
			Project: "mastermind",
			Tags:    []string{"go", "format", "mcp"},
		},
	}

	// No filters: always passes
	if !matchesMetadataFilters(ref, Query{}) {
		t.Error("empty filters should pass")
	}

	// Matching kind
	if !matchesMetadataFilters(ref, Query{Kinds: []format.Kind{format.KindLesson}}) {
		t.Error("kind match should pass")
	}

	// Non-matching kind
	if matchesMetadataFilters(ref, Query{Kinds: []format.Kind{format.KindDecision}}) {
		t.Error("kind mismatch should fail")
	}

	// Matching project (case-insensitive)
	if !matchesMetadataFilters(ref, Query{Project: "MasterMind"}) {
		t.Error("project match should be case-insensitive")
	}

	// Non-matching project
	if matchesMetadataFilters(ref, Query{Project: "other"}) {
		t.Error("project mismatch should fail")
	}

	// Matching all required tags (AND)
	if !matchesMetadataFilters(ref, Query{Tags: []string{"go", "format"}}) {
		t.Error("all-tags match should pass")
	}

	// Missing one required tag
	if matchesMetadataFilters(ref, Query{Tags: []string{"go", "missing"}}) {
		t.Error("partial tag match should fail (AND semantics)")
	}

	// Tag match is case-insensitive
	if !matchesMetadataFilters(ref, Query{Tags: []string{"GO"}}) {
		t.Error("tag match should be case-insensitive")
	}
}

// ─── end-to-end: real store backed by t.TempDir() ─────────────────────

// populateStore writes several live entries to a store and returns the
// store ready for searching. Uses distinct topics, tags, and bodies so
// tests can check ordering and filtering.
func populateStore(t *testing.T) *store.Store {
	t.Helper()
	tmp := t.TempDir()
	cfg := store.Config{
		UserPersonalRoot: filepath.Join(tmp, "user"),
		Now:              time.Now,
	}
	s := store.New(cfg)

	entries := []*format.Entry{
		{
			Metadata: format.Metadata{
				Date:    "2026-04-01",
				Project: "mastermind",
				Topic:   "Never do sync I/O in the Electron main process",
				Tags:    []string{"electron", "ipc", "macos"},
				Kind:    format.KindLesson,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "The Electron main process on macOS hangs IPC when blocked on sync I/O. Use worker or async.",
		},
		{
			Metadata: format.Metadata{
				Date:    "2026-03-15",
				Project: "mastermind",
				Topic:   "FTS5 indexing is overkill for personal knowledge bases",
				Tags:    []string{"search", "architecture"},
				Kind:    format.KindDecision,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "At thousands of small markdown files, stdlib grep is faster than any index build.",
		},
		{
			Metadata: format.Metadata{
				Date:    "2026-02-10",
				Project: "rocket-chat",
				Topic:   "Linux CI passes but macOS hangs",
				Tags:    []string{"ci", "debugging"},
				Kind:    format.KindWarStory,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "Spent three days on this. Root cause was main-process blocking.",
		},
	}

	// Write each entry through the normal path, then promote so it
	// lands in live/ (live is what ListLive returns).
	for _, e := range entries {
		pendingPath, err := s.Write(e)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := s.Promote(pendingPath); err != nil {
			t.Fatalf("promote: %v", err)
		}
	}
	return s
}

func TestKeywordSearcherBasicHit(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{QueryText: "electron"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search: expected at least one hit for 'electron'")
	}
	// Top hit should be the one with "electron" in the topic.
	top := results[0]
	if !strings.Contains(strings.ToLower(top.Metadata.Topic), "electron") {
		t.Errorf("top hit topic = %q, want contains 'electron'", top.Metadata.Topic)
	}
}

func TestKeywordSearcherEmptyQueryFails(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)
	_, err := searcher.Search(Query{QueryText: ""})
	if err == nil {
		t.Error("empty query: expected error, got nil")
	}
}

func TestKeywordSearcherFilterByKind(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	// "macos" matches both the electron lesson and the war-story;
	// kind filter should narrow to just the war-story.
	results, err := searcher.Search(Query{
		QueryText: "macos",
		Kinds:     []format.Kind{format.KindWarStory},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Metadata.Kind != format.KindWarStory {
		t.Errorf("result kind = %q, want war-story", results[0].Metadata.Kind)
	}
}

func TestKeywordSearcherFilterByProject(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{
		QueryText: "macos",
		Project:   "rocket-chat",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Metadata.Project != "rocket-chat" {
		t.Errorf("result project = %q, want rocket-chat", results[0].Metadata.Project)
	}
}

func TestKeywordSearcherFilterByTag(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{
		QueryText: "grep",
		Tags:      []string{"architecture"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d, want 1 (the FTS5 decision)", len(results))
	}
}

func TestKeywordSearcherLimit(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{
		QueryText: "the", // matches every body
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Limit=2 returned %d results", len(results))
	}
}

func TestKeywordSearcherMissesReturnsEmpty(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{QueryText: "kubernetes"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 hits for 'kubernetes', got %d", len(results))
	}
}

func TestKeywordSearcherRankingFavorsTopicOverBody(t *testing.T) {
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	// "macos" is in the topic of the war-story AND in the tags of the
	// electron lesson. Ranking should favor topic matches first.
	results, err := searcher.Search(Query{QueryText: "macos"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(results))
	}
	// The war-story has "macos" directly in the topic string.
	if results[0].Metadata.Kind != format.KindWarStory {
		t.Errorf("top result kind = %q, want war-story (topic hit wins)", results[0].Metadata.Kind)
	}
}

func TestKeywordSearcherIncludesPendingWhenRequested(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{
		UserPersonalRoot: filepath.Join(tmp, "user"),
		Now:              time.Now,
	}
	s := store.New(cfg)

	// Write one entry and leave it in pending (don't promote).
	_, err := s.Write(&format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-04",
			Project: "mastermind",
			Topic:   "Pending discovery",
			Kind:    format.KindInsight,
			Scope:   format.ScopeUserPersonal,
		},
		Body: "Ephemeral insight waiting for review.",
	})
	if err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)

	// Without IncludePending: no results (pending is hidden by default).
	noPending, err := searcher.Search(Query{QueryText: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	if len(noPending) != 0 {
		t.Errorf("default search returned %d pending entries, want 0", len(noPending))
	}

	// With IncludePending: finds the pending entry.
	withPending, err := searcher.Search(Query{QueryText: "pending", IncludePending: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withPending) != 1 {
		t.Errorf("IncludePending search returned %d, want 1", len(withPending))
	}
	if len(withPending) == 1 && !withPending[0].Ref.Pending {
		t.Error("pending result not marked as pending")
	}
}

// ─── format output ────────────────────────────────────────────────────

func TestFormatResultsMarkdownEmpty(t *testing.T) {
	out := FormatResultsMarkdown("electron", nil, false)
	if !strings.Contains(out, "0 results") {
		t.Errorf("empty output missing count: %q", out)
	}
	if !strings.Contains(out, "no matching entries") {
		t.Errorf("empty output missing friendly message: %q", out)
	}
}

func TestFormatResultsMarkdownHasPerResultHeadings(t *testing.T) {
	results := []Result{
		{
			Ref: store.EntryRef{
				Path:  "/tmp/user/lessons/macos-sync-io.md",
				Scope: format.ScopeUserPersonal,
			},
			Metadata: format.Metadata{
				Date:    "2026-04-01",
				Project: "mastermind",
				Topic:   "macOS sync I/O",
				Tags:    []string{"electron", "macos"},
				Kind:    format.KindLesson,
			},
			Body: "Short body.",
		},
		{
			Ref: store.EntryRef{
				Path:  "/tmp/user/lessons/other.md",
				Scope: format.ScopeUserPersonal,
			},
			Metadata: format.Metadata{
				Date:  "2026-03-01",
				Topic: "Another",
				Kind:  format.KindInsight,
			},
			Body: "",
		},
	}

	out := FormatResultsMarkdown("macos", results, false)

	// Top heading
	if !strings.HasPrefix(out, "## mm_search:") {
		t.Errorf("output should start with H2; got: %q", out[:min(80, len(out))])
	}
	// Result count
	if !strings.Contains(out, "2 results") {
		t.Errorf("output should mention 2 results; got: %q", out)
	}
	// Per-result H3 headings — context-mode uses these to chunk
	h3Count := strings.Count(out, "\n### [")
	if h3Count != 2 {
		t.Errorf("expected 2 per-result H3 headings, got %d", h3Count)
	}
	// Scope label appears
	if !strings.Contains(out, "[user-personal]") {
		t.Errorf("scope label missing: %q", out)
	}
	// Body appears for the result that has one (short body → returned verbatim)
	if !strings.Contains(out, "Short body.") {
		t.Errorf("body excerpt missing: %q", out)
	}
	// Path field must appear in the output
	if !strings.Contains(out, "**path**:") {
		t.Errorf("path field missing from result section: %q", out)
	}
}

func TestFormatResultsMarkdownExpandReturnsFullBody(t *testing.T) {
	// Build a multi-line body well over shortBodyThreshold (800 chars).
	// Use many lines so the match-anchored excerpt (±3 lines) is much
	// shorter than the full body.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("unrelated content line with distinct words to fill the body up\n")
	}
	longBody := strings.TrimRight(sb.String(), "\n")

	results := []Result{
		{
			Ref: store.EntryRef{
				Path:  "/tmp/user/lessons/long-entry.md",
				Scope: format.ScopeUserPersonal,
			},
			Metadata: format.Metadata{
				Date:  "2026-04-01",
				Topic: "Long entry",
				Kind:  format.KindLesson,
			},
			Body: longBody,
		},
	}

	// expand=false with a matching query should trim to a context window
	trimmed := FormatResultsMarkdown("content", results, false)
	// expand=true should return full body
	expanded := FormatResultsMarkdown("content", results, true)

	if len(expanded) <= len(trimmed) {
		t.Errorf("expand=true output (%d) should be longer than expand=false (%d)", len(expanded), len(trimmed))
	}
	if !strings.Contains(expanded, longBody) {
		t.Error("expand=true output should contain full body verbatim")
	}
}

func TestWordTrimShortBodyUnchanged(t *testing.T) {
	body := "short body"
	got := wordTrim(body, 500)
	if got != body {
		t.Errorf("wordTrim short body = %q, want %q", got, body)
	}
}

func TestWordTrimLongBodyTruncated(t *testing.T) {
	body := strings.Repeat("word ", 200) // 1000 chars
	got := wordTrim(body, 100)
	if len(got) > 105 || !strings.HasSuffix(got, "…") {
		t.Errorf("wordTrim long body: len=%d, suffix=%q", len(got), got[max(0, len(got)-4):])
	}
}

// min/max are in stdlib from Go 1.21+; safe here since go.mod is 1.22.
// (Avoids depending on them in pre-1.21 if anyone back-ports.)
