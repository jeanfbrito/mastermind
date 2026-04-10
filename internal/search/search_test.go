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

func TestAccessBoost(t *testing.T) {
	// Zero accesses → no boost.
	if b := accessBoost(0); b != 0 {
		t.Errorf("accessBoost(0) = %v, want 0", b)
	}
	if b := accessBoost(-1); b != 0 {
		t.Errorf("accessBoost(-1) = %v, want 0", b)
	}

	// One access is meaningful under ACT-R fast mode — proves the log
	// shape rewards even the first hit (linear would have been 0.05).
	b1 := accessBoost(1)
	if b1 < 0.12 || b1 > 0.16 {
		t.Errorf("accessBoost(1) = %v, want ~0.139 (ln(2)*0.2)", b1)
	}

	// Monotonic growth across the unsaturated range.
	prev := accessBoost(1)
	for n := 2; n <= 10; n++ {
		cur := accessBoost(n)
		if cur <= prev {
			t.Errorf("accessBoost(%d)=%v not > accessBoost(%d)=%v", n, cur, n-1, prev)
		}
		prev = cur
	}

	// Hard cap at 0.5: preserves load-bearing invariant that a single
	// topic hit (2.0) dominates any access boost.
	if b := accessBoost(1000); b > 0.5 {
		t.Errorf("accessBoost(1000) = %v, want <= 0.5 (cap)", b)
	}
	if b := accessBoost(100000); b > 0.5 {
		t.Errorf("accessBoost(100000) = %v, want <= 0.5 (cap)", b)
	}

	// Saturation: 100 vs 1000 accesses should differ by less than 0.01
	// (both hit the 0.5 cap). This ensures frequently-accessed entries
	// don't runaway-dominate merely-familiar ones.
	diff := accessBoost(1000) - accessBoost(100)
	if diff > 0.01 || diff < -0.01 {
		t.Errorf("accessBoost saturation: 1000-100 diff = %v, want |diff| < 0.01", diff)
	}

	// Topic-dominance invariant check: even with max access boost, the
	// boost alone cannot promote a body-only match (score <= 0.75) above
	// a single topic hit (2.0). This is the ranking contract.
	const topicHit = 2.0
	const maxBodyHit = 0.75
	if maxBodyHit+accessBoost(100000) >= topicHit {
		t.Errorf("access boost breaks topic dominance: body+boost = %v, topic = %v",
			maxBodyHit+accessBoost(100000), topicHit)
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

	// Project is a soft ranking signal by default — matchesMetadataFilters
	// ignores it unless StrictProject is set. Both matching and mismatched
	// projects pass the filter; ranking is handled later via
	// projectMultiplier.
	if !matchesMetadataFilters(ref, Query{Project: "MasterMind"}) {
		t.Error("non-strict project match should pass")
	}
	if !matchesMetadataFilters(ref, Query{Project: "other"}) {
		t.Error("non-strict project mismatch should pass (ranking, not filter)")
	}

	// StrictProject restores the old hard-filter behavior.
	if !matchesMetadataFilters(ref, Query{Project: "MasterMind", StrictProject: true}) {
		t.Error("strict project match should pass (case-insensitive)")
	}
	if matchesMetadataFilters(ref, Query{Project: "other", StrictProject: true}) {
		t.Error("strict project mismatch should fail")
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

func TestKeywordSearcherProjectBoostRanksSameProjectFirst(t *testing.T) {
	// Under the 2026-04-10 soft-filter refactor, Query.Project is a
	// ranking multiplier (1.3× same-project, 0.8× cross-project), not
	// a hard filter. Both rocket-chat AND mastermind results should
	// surface for "macos"; rocket-chat must rank first because its
	// entry is same-project AND sits in a higher class (class 3 vs
	// class 4 for the mastermind tag-only hit).
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{
		QueryText: "macos",
		Project:   "rocket-chat",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want >= 2 (soft filter must surface both)", len(results))
	}
	if results[0].Metadata.Project != "rocket-chat" {
		t.Errorf("results[0].Project = %q, want rocket-chat (same-project boost)", results[0].Metadata.Project)
	}
	// The mastermind entry must also appear — cross-project demoted,
	// not dropped.
	var sawMastermind bool
	for _, r := range results {
		if r.Metadata.Project == "mastermind" {
			sawMastermind = true
			break
		}
	}
	if !sawMastermind {
		t.Error("mastermind entry dropped — soft filter should surface cross-project entries")
	}
}

func TestKeywordSearcherStrictProjectRestoresHardFilter(t *testing.T) {
	// StrictProject=true is the escape hatch for callers that truly
	// want only-this-project results (e.g., a future `mastermind
	// discover --project foo` CLI flag). It restores the pre-refactor
	// behavior: cross-project entries are dropped entirely.
	s := populateStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{
		QueryText:     "macos",
		Project:       "rocket-chat",
		StrictProject: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want exactly 1 (hard filter)", len(results))
	}
	if results[0].Metadata.Project != "rocket-chat" {
		t.Errorf("result project = %q, want rocket-chat", results[0].Metadata.Project)
	}
}

func TestProjectMultiplierCases(t *testing.T) {
	cases := []struct {
		name         string
		queryProject string
		entryProject string
		want         float64
	}{
		{"no query project → neutral", "", "mastermind", 1.0},
		{"same project match → boost", "mastermind", "mastermind", 1.3},
		{"same project case-insensitive", "MasterMind", "mastermind", 1.3},
		{"general entry → neutral", "mastermind", "general", 1.0},
		{"general case-insensitive", "mastermind", "General", 1.0},
		{"empty entry project → neutral", "mastermind", "", 1.0},
		{"cross-project → demote", "mastermind", "rocket-chat", 0.8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := projectMultiplier(tc.queryProject, tc.entryProject)
			if got != tc.want {
				t.Errorf("projectMultiplier(%q, %q) = %v, want %v",
					tc.queryProject, tc.entryProject, got, tc.want)
			}
		})
	}
}

func TestKeywordSearcherProjectBoostIsWithinClassOnly(t *testing.T) {
	// Cross-project class-3 (topic tokens) must still beat same-project
	// class-5 (body keyword) — the multiplier is a within-class
	// tiebreaker, it can NEVER bridge a class gap. This locks in the
	// orthogonality of the tier-class sort and the project boost.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Entry A: cross-project, but topic contains the query token → class 3.
	entA := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "other-project",
			Topic: "widgets and things",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "unrelated body",
	}
	// Entry B: same-project, but query token only in body → class 5.
	entB := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "my-project",
			Topic: "unrelated",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "this talks about widgets extensively",
	}
	for _, e := range []*format.Entry{entA, entB} {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{
		QueryText: "widgets",
		Project:   "my-project",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// Class 3 (cross-project) must beat class 5 (same-project) — the
	// multiplier is only a within-class signal.
	if results[0].Metadata.Project != "other-project" {
		t.Errorf("results[0].Project = %q, want other-project (class 3 beats class 5)",
			results[0].Metadata.Project)
	}
	if results[0].class != classTopicTokens {
		t.Errorf("results[0].class = %d, want classTopicTokens (3)", results[0].class)
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

// populateExactPhraseStore sets up three entries designed to exercise
// the exact-phrase tiers (classes 0/1/2) independently of the normal
// keyword classes.
func populateExactPhraseStore(t *testing.T) *store.Store {
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
				Topic:   "Hook extraction runs inside PreCompact",
				Tags:    []string{"hooks", "precompact"},
				Kind:    format.KindLesson,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "Pipeline is keyword-first, LLM fallback. Nothing about the magic phrase here.",
		},
		{
			Metadata: format.Metadata{
				Date:    "2026-03-01",
				Project: "mastermind",
				Topic:   "Unrelated discovery",
				Tags:    []string{"hook extraction", "misc"}, // exact phrase in tags
				Kind:    format.KindInsight,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "A totally different body about caching strategies.",
		},
		{
			Metadata: format.Metadata{
				Date:    "2026-02-01",
				Project: "mastermind",
				Topic:   "Random topic",
				Tags:    []string{"misc"},
				Kind:    format.KindInsight,
				Scope:   format.ScopeUserPersonal,
			},
			// Exact phrase "hook extraction" appears verbatim in the body,
			// but not in topic or tags. Should land in classExactBody (2).
			Body: "Long body that eventually mentions hook extraction as a design term.",
		},
		{
			Metadata: format.Metadata{
				Date:    "2026-01-01",
				Project: "mastermind",
				Topic:   "Cat video catalog",
				Tags:    []string{"cats"},
				Kind:    format.KindInsight,
				Scope:   format.ScopeUserPersonal,
			},
			// Contains both words "hook" and "extraction" but NOT adjacent.
			// Should fall into classKeyword (5), the baseline.
			Body: "The hook pattern is separate from extraction of data.",
		},
	}
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

func TestKeywordSearcherExactPhraseTiers(t *testing.T) {
	s := populateExactPhraseStore(t)
	searcher := NewKeywordSearcher(s)

	results, err := searcher.Search(Query{QueryText: "hook extraction"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("got %d results, want >= 3", len(results))
	}

	// Expected ordering by class:
	//   results[0] — "Hook extraction runs inside PreCompact" (class 0, topic)
	//   results[1] — "Unrelated discovery" (class 1, tag)
	//   results[2] — "Random topic" (class 2, body)
	//   results[3] — "Cat video catalog" (class 5, keyword; words not adjacent)

	if got := results[0].class; got != classExactTopic {
		t.Errorf("results[0].class = %d, want classExactTopic (0)", got)
	}
	if !strings.Contains(results[0].Metadata.Topic, "PreCompact") {
		t.Errorf("results[0] topic = %q, want the PreCompact entry", results[0].Metadata.Topic)
	}

	if got := results[1].class; got != classExactTag {
		t.Errorf("results[1].class = %d, want classExactTag (1)", got)
	}

	if got := results[2].class; got != classExactBody {
		t.Errorf("results[2].class = %d, want classExactBody (2)", got)
	}

	// The "Cat video catalog" entry, if present, must come after all
	// exact-phrase hits even though it contains both words.
	for i := 0; i < 3; i++ {
		if strings.Contains(results[i].Metadata.Topic, "Cat video") {
			t.Errorf("results[%d] is the non-adjacent keyword entry, but it's ranked in the exact-phrase slot", i)
		}
	}
}

func TestKeywordSearcherSingleTokenSkipsExactPhrase(t *testing.T) {
	s := populateExactPhraseStore(t)
	searcher := NewKeywordSearcher(s)

	// Single-token query: exact phrase and token-level match are
	// identical, so every match should land in a keyword class — NOT
	// in classExactTopic. Class 0 is reserved for multi-word phrases
	// where the exact-phrase signal carries meaningful information
	// beyond the individual tokens.
	results, err := searcher.Search(Query{QueryText: "hook"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for i, r := range results {
		if r.class == classExactTopic || r.class == classExactTag || r.class == classExactBody {
			t.Errorf("results[%d] class = %d, single-token queries should skip exact-phrase tiers", i, r.class)
		}
	}
}

func TestKeywordSearcherClassDominatesAccessBoost(t *testing.T) {
	// Build a store where the body-only keyword match has been
	// artificially accessed many times (maxing out the access boost),
	// and the exact-phrase topic match has zero access history.
	// Class 0 must still win — access boost cannot bridge a class gap.
	tmp := t.TempDir()
	cfg := store.Config{
		UserPersonalRoot: filepath.Join(tmp, "user"),
		Now:              time.Now,
	}
	s := store.New(cfg)

	// Entry A: topic exact phrase, zero accesses.
	entA := &format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-01",
			Project: "mastermind",
			Topic:   "Hook extraction basics",
			Tags:    []string{"misc"},
			Kind:    format.KindLesson,
			Scope:   format.ScopeUserPersonal,
		},
		Body: "Short body.",
	}
	pathA, err := s.Write(entA)
	if err != nil {
		t.Fatal(err)
	}
	if pathA, err = s.Promote(pathA); err != nil {
		t.Fatal(err)
	}

	// Entry B: no exact phrase, many accesses.
	entB := &format.Entry{
		Metadata: format.Metadata{
			Date:    "2026-04-02",
			Project: "mastermind",
			Topic:   "Unrelated item",
			Tags:    []string{"popular"},
			Kind:    format.KindInsight,
			Scope:   format.ScopeUserPersonal,
		},
		Body: "This entry talks about hook patterns and extraction of values in separate sentences.",
	}
	pathB, err := s.Write(entB)
	if err != nil {
		t.Fatal(err)
	}
	if pathB, err = s.Promote(pathB); err != nil {
		t.Fatal(err)
	}

	// Simulate heavy usage on entry B.
	now := time.Now()
	for i := 0; i < 500; i++ {
		s.IncrementAccess(pathB, now)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "hook extraction"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].class != classExactTopic {
		t.Errorf("results[0].class = %d, want classExactTopic — access boost bridged a class gap", results[0].class)
	}
	if !strings.Contains(results[0].Metadata.Topic, "Hook extraction basics") {
		t.Errorf("results[0] topic = %q, want the exact-phrase entry", results[0].Metadata.Topic)
	}
}

func TestKeywordSearcherKeywordClassSplit(t *testing.T) {
	// Build a store with three entries designed to land in
	// classTopicTokens (3), classMetaTokens (4), and classKeyword (5)
	// respectively, for the query "debug macos".
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	entries := []*format.Entry{
		{
			// Class 3: both tokens in topic, NOT adjacent (so no exact-phrase hit).
			Metadata: format.Metadata{
				Date:    "2026-04-01",
				Project: "mastermind",
				Topic:   "Fixing macOS hangs when debug mode breaks",
				Tags:    []string{"tools"},
				Kind:    format.KindLesson,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "Content about profiling.",
		},
		{
			// Class 4: one token in topic ("debug" ⊂ "debugging"), other in tags ("macos").
			Metadata: format.Metadata{
				Date:    "2026-03-01",
				Project: "mastermind",
				Topic:   "Session debugging tricks",
				Tags:    []string{"macos", "general"},
				Kind:    format.KindLesson,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "Unrelated body content.",
		},
		{
			// Class 5: tokens only in body.
			Metadata: format.Metadata{
				Date:    "2026-02-01",
				Project: "mastermind",
				Topic:   "Kernel tracing notes",
				Tags:    []string{"kernel"},
				Kind:    format.KindInsight,
				Scope:   format.ScopeUserPersonal,
			},
			Body: "debugging macos problems with dtrace is painful.",
		},
	}
	for _, e := range entries {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "debug macos"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Expected: class 3 (both tokens in topic) → class 4 (across topic+tags) → class 5 (body match)
	wantClasses := []tierClass{classTopicTokens, classMetaTokens, classKeyword}
	for i, want := range wantClasses {
		if got := results[i].class; got != want {
			t.Errorf("results[%d].class = %d, want %d", i, got, want)
		}
	}
}

func TestKeywordSearcherShortCircuitFires(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Three entries with the query token in their topic (class 3).
	// One entry with the token only in its body (class 5 candidate for pass 2).
	paths := make([]string, 0, 4)
	topicEntries := []*format.Entry{
		{Metadata: format.Metadata{Date: "2026-04-01", Project: "mm", Topic: "goroutines explained", Kind: format.KindLesson, Scope: format.ScopeUserPersonal}, Body: "A"},
		{Metadata: format.Metadata{Date: "2026-03-01", Project: "mm", Topic: "goroutines and channels", Kind: format.KindLesson, Scope: format.ScopeUserPersonal}, Body: "B"},
		{Metadata: format.Metadata{Date: "2026-02-01", Project: "mm", Topic: "goroutines for beginners", Kind: format.KindLesson, Scope: format.ScopeUserPersonal}, Body: "C"},
	}
	for _, e := range topicEntries {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		promoted, err := s.Promote(p)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, promoted)
	}
	// Bump access_count ≥ 3 on the first topic entry to "earn" short-circuit.
	now := time.Now()
	for i := 0; i < 5; i++ {
		s.IncrementAccess(paths[0], now)
	}

	// A body-only entry — present in pass-2 candidates but should be
	// skipped by the short-circuit.
	bodyEntry := &format.Entry{
		Metadata: format.Metadata{Date: "2026-01-01", Project: "mm", Topic: "unrelated", Kind: format.KindInsight, Scope: format.ScopeUserPersonal},
		Body:     "goroutines are great",
	}
	p, err := s.Write(bodyEntry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	before := searcher.shortCircuitCount
	results, err := searcher.Search(Query{QueryText: "goroutines"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if searcher.shortCircuitCount != before+1 {
		t.Errorf("shortCircuitCount = %d, want %d — short-circuit did not fire",
			searcher.shortCircuitCount, before+1)
	}
	// Body-only entry should NOT be in results (pass 2 skipped).
	for _, r := range results {
		if strings.Contains(r.Metadata.Topic, "unrelated") {
			t.Errorf("short-circuit skipped pass 2 but body entry %q still surfaced", r.Metadata.Topic)
		}
	}
}

func TestKeywordSearcherShortCircuitNeedsEarnedAccess(t *testing.T) {
	// Same three class-3 entries, but NONE have access_count ≥ 3.
	// Short-circuit must NOT fire — the access gate is the second
	// condition and prevents structural matches from short-circuiting
	// before they've proven useful.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	for i, topic := range []string{"goroutines explained", "goroutines channels", "goroutines beginners"} {
		p, err := s.Write(&format.Entry{
			Metadata: format.Metadata{
				Date: "2026-04-0" + string(rune('1'+i)), Project: "mm",
				Topic: topic, Kind: format.KindLesson, Scope: format.ScopeUserPersonal,
			},
			Body: "body " + topic,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}
	// Body-only entry that SHOULD surface since short-circuit won't fire.
	bodyEntry := &format.Entry{
		Metadata: format.Metadata{Date: "2026-01-01", Project: "mm", Topic: "body match", Kind: format.KindInsight, Scope: format.ScopeUserPersonal},
		Body:     "goroutines appear only here",
	}
	p, err := s.Write(bodyEntry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	before := searcher.shortCircuitCount
	results, err := searcher.Search(Query{QueryText: "goroutines"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if searcher.shortCircuitCount != before {
		t.Errorf("shortCircuitCount incremented when no entry had earned access; want stable")
	}
	// Body entry should be in results because pass 2 ran.
	foundBody := false
	for _, r := range results {
		if strings.Contains(r.Metadata.Topic, "body match") {
			foundBody = true
			break
		}
	}
	if !foundBody {
		t.Error("pass 2 did not run; body-only entry missing from results")
	}
}

func TestKeywordSearcherFuzzyTypo(t *testing.T) {
	// Entry with topic "extraction pipeline" — query with typo
	// "extrction" should still find it via the fuzzy tier.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	p, err := s.Write(&format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "extraction pipeline overview",
			Tags:  []string{"pipeline"},
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "body text",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "extrction"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("fuzzy typo query returned zero results")
	}
	if results[0].class != classFuzzy {
		t.Errorf("results[0].class = %d, want classFuzzy (6)", results[0].class)
	}
	if !strings.Contains(results[0].Metadata.Topic, "extraction") {
		t.Errorf("fuzzy hit topic = %q, want contains 'extraction'", results[0].Metadata.Topic)
	}
}

func TestKeywordSearcherFuzzyGapMatch(t *testing.T) {
	// sahilm/fuzzy does Sublime-style gap matching. Query "hookex"
	// should match topic "hook extraction" — non-contiguous but
	// in-order char match.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	p, err := s.Write(&format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "hook extraction overview",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "hookex"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("fuzzy gap query returned zero results")
	}
	if results[0].class != classFuzzy {
		t.Errorf("results[0].class = %d, want classFuzzy (6)", results[0].class)
	}
}

func TestKeywordSearcherFuzzyLengthGuard(t *testing.T) {
	// Query "go" (2 chars) is below the length guard (>= 4). Fuzzy
	// tier should NOT run — engram's lesson is that short queries
	// drown precision.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Entry whose topic contains "go" only via fuzzy gap (no exact token).
	p, err := s.Write(&format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "green options",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
		},
		Body: "unrelated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	// Single-char tokens are dropped by tokenize, but "go" is 2 chars
	// (and Search needs >= 4 for the fuzzy guard). Use a dedicated
	// 3-char query that would fuzzy-match "green" without the guard.
	results, err := searcher.Search(Query{QueryText: "gre"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.class == classFuzzy {
			t.Errorf("fuzzy tier fired for 3-char query; length guard should have suppressed it")
		}
	}
}

func TestKeywordSearcherFuzzyRanksBelowKeyword(t *testing.T) {
	// A class-5 (body keyword) hit must always rank above a class-6
	// (fuzzy) hit, regardless of access_count on the fuzzy entry.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Body-only keyword hit entry.
	bodyEntry := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "unrelated topic",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "The word extraction appears here in the body text.",
	}
	// Fuzzy-candidate entry with massive access history.
	// Topic "extractor functions efficiently" contains all chars of
	// "extraction" in order (e-x-t-r-a-c-t-i-o-n), so sahilm gap-
	// matches — but does NOT contain "extraction" as a substring, so
	// the token-level keyword pipeline rejects it (bodyScore = 0).
	fuzzyEntry := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-02", Project: "mm",
			Topic: "extractor functions efficiently",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
		},
		Body: "something",
	}
	bp, err := s.Write(bodyEntry)
	if err != nil {
		t.Fatal(err)
	}
	bp, err = s.Promote(bp)
	if err != nil {
		t.Fatal(err)
	}
	fp, err := s.Write(fuzzyEntry)
	if err != nil {
		t.Fatal(err)
	}
	fp, err = s.Promote(fp)
	if err != nil {
		t.Fatal(err)
	}
	// Inflate fuzzy entry access to maximum.
	now := time.Now()
	for i := 0; i < 500; i++ {
		s.IncrementAccess(fp, now)
	}
	_ = bp

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "extraction"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].class != classKeyword {
		t.Errorf("results[0].class = %d, want classKeyword (5) — fuzzy overtook keyword", results[0].class)
	}
	// Fuzzy result must be strictly after any keyword result.
	var sawKeyword, sawFuzzy bool
	for _, r := range results {
		if r.class == classFuzzy {
			if !sawKeyword {
				t.Error("fuzzy result appeared before any keyword result")
			}
			sawFuzzy = true
		}
		if r.class == classKeyword {
			if sawFuzzy {
				t.Error("keyword result appeared after a fuzzy result")
			}
			sawKeyword = true
		}
	}
}

func TestKeywordSearcherFuzzyDedupes(t *testing.T) {
	// An entry that already matched via class 3 (topic tokens) must
	// not appear a second time via class 6 fuzzy.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	p, err := s.Write(&format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "extraction pipeline",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(p); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "extraction"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1 (fuzzy must dedupe against earlier tiers)", len(results))
	}
}

func TestKeywordSearcherStrictClassOrderingInvariant(t *testing.T) {
	// Contract test: results must always be ordered by class ascending.
	// No combination of score, access count, recency, or path can
	// invert that. Produces one entry per class, gives each a different
	// date + access count, then asserts the sort is stable on class.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Seven entries, each targeting a different class for query "alpha beta".
	// The "low-class" entries are given inferior dates / zero access to
	// prove they still win on class alone.
	entries := []struct {
		topic string
		tags  []string
		body  string
		date  string
	}{
		{"alpha beta found here", nil, "", "2024-01-01"},                                  // class 0 — exact topic, old
		{"totally different", []string{"alpha beta"}, "", "2024-01-01"},                   // class 1 — exact tag, old
		{"nothing relevant", nil, "the phrase alpha beta is hidden in body", "2024-01-01"}, // class 2 — exact body, old
		{"alpha standing next to beta", nil, "", "2024-01-01"},                            // class 3 — both tokens in topic
		{"alpha only in topic", []string{"beta"}, "", "2024-01-01"},                       // class 4 — topic + tag
		{"unrelated", nil, "alpha here and beta here body only", "2024-01-01"},            // class 5 — body keyword
		{"alphen beeto entries", nil, "misc", "2024-01-01"},                               // class 6 — fuzzy match (gap)
	}
	for i, e := range entries {
		p, err := s.Write(&format.Entry{
			Metadata: format.Metadata{
				Date: e.date, Project: "mm",
				Topic: e.topic, Tags: e.tags,
				Kind: format.KindLesson, Scope: format.ScopeUserPersonal,
			},
			Body: e.body,
		})
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatalf("promote %d: %v", i, err)
		}
	}
	// Deliberately NOT inflating access_count — that would trigger
	// the short-circuit (top-K pass-1 results all in class ≤ 4 with
	// one at access ≥ 3) and suppress pass 2, which is where classes
	// 2 and 5 surface. Class-gap-over-access is tested separately in
	// TestKeywordSearcherClassDominatesAccessBoost.

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "alpha beta", Limit: 20})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Expected: one result per class, strictly ordered by class.
	wantMinimum := 6 // class 6 fuzzy may or may not match; others must
	if len(results) < wantMinimum {
		t.Fatalf("got %d results, want at least %d", len(results), wantMinimum)
	}

	// Monotonic non-decreasing class sequence.
	for i := 1; i < len(results); i++ {
		if results[i].class < results[i-1].class {
			t.Errorf("class inversion at i=%d: class[%d]=%d < class[%d]=%d (topics: %q / %q)",
				i, i, results[i].class, i-1, results[i-1].class,
				results[i].Metadata.Topic, results[i-1].Metadata.Topic)
		}
	}

	// Spot-check the expected lineup for the first six classes.
	expectClass := []tierClass{
		classExactTopic, classExactTag, classExactBody,
		classTopicTokens, classMetaTokens, classKeyword,
	}
	for i, want := range expectClass {
		if results[i].class != want {
			t.Errorf("results[%d].class = %d, want %d (topic=%q)",
				i, results[i].class, want, results[i].Metadata.Topic)
		}
	}
}

func TestKeywordSearcherWithinClassTiebreakByACTR(t *testing.T) {
	// Two entries in the same class (class 3: both tokens in topic),
	// same date. One has high access_count, one has zero. ACT-R fast
	// mode must tiebreak toward the frequently-accessed entry.
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	hot := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "gamma delta signals", // both tokens in topic
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "x",
	}
	cold := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "delta gamma rhythms", // both tokens in topic
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "x",
	}
	hp, err := s.Write(hot)
	if err != nil {
		t.Fatal(err)
	}
	hp, err = s.Promote(hp)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := s.Write(cold)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(cp); err != nil {
		t.Fatal(err)
	}

	// Inflate hot entry's access count to 20 (saturates ACT-R boost).
	now := time.Now()
	for i := 0; i < 20; i++ {
		s.IncrementAccess(hp, now)
	}

	searcher := NewKeywordSearcher(s)
	// Use a single-token query to avoid exact-phrase classes.
	results, err := searcher.Search(Query{QueryText: "gamma"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// Both should be in classTopicTokens (class 3).
	if results[0].class != classTopicTokens || results[1].class != classTopicTokens {
		t.Fatalf("expected both in class 3, got %d and %d",
			results[0].class, results[1].class)
	}
	// Hot entry wins the tiebreak via access boost.
	if !strings.Contains(results[0].Metadata.Topic, "gamma delta signals") {
		t.Errorf("results[0] topic = %q, want hot entry (gamma delta signals)", results[0].Metadata.Topic)
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
