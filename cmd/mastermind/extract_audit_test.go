package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeanfbrito/mastermind/internal/extract"
	"github.com/jeanfbrito/mastermind/internal/format"
)

// mockExtractor lets audit tests control exactly what comes out of the
// extraction step so we can assert matching logic in isolation.
type mockExtractor struct {
	entries []format.Entry
}

func (m *mockExtractor) Extract(_ string, _ []string) ([]format.Entry, error) {
	return m.entries, nil
}

// ensure mockExtractor satisfies the extract.Extractor interface.
var _ extract.Extractor = (*mockExtractor)(nil)

func TestAuditLabel_InScope(t *testing.T) {
	cases := []struct {
		name  string
		tier  string
		mode  string
		want  bool
	}{
		{"empty tier defaults to both (keyword mode)", "", "keyword", true},
		{"empty tier defaults to both (llm mode)", "", "llm", true},
		{"explicit both matches any mode", "both", "keyword", true},
		{"keyword label in keyword mode", "keyword", "keyword", true},
		{"keyword label in llm mode", "keyword", "llm", false},
		{"llm label in llm mode", "llm", "llm", true},
		{"llm label in keyword mode", "llm", "keyword", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := auditLabel{Tier: tc.tier}.inScope(tc.mode)
			if got != tc.want {
				t.Errorf("inScope(%q) with tier=%q: got %v, want %v", tc.mode, tc.tier, got, tc.want)
			}
		})
	}
}

func TestAuditOneTranscript_GreedyMatchingAndTierFilter(t *testing.T) {
	// The transcript must contain every label's key_phrase so the
	// audit's pre-flight validation passes. The mock extractor ignores
	// the content, but pre-flight checks the normalized prose for
	// every label. Non-JSONL content passes through unchanged so a
	// plain-text transcript is fine here.
	dir := t.TempDir()
	txPath := filepath.Join(dir, "tiny.txt")
	transcriptText := "the fix was to unplug it\naudit the ranker tomorrow\nspent six hours debugging\n"
	if err := os.WriteFile(txPath, []byte(transcriptText), 0o644); err != nil {
		t.Fatal(err)
	}

	// The mock returns three entries: one lesson that matches a
	// keyword-tier label, one open-loop that matches an llm-tier label,
	// and one decision that matches nothing.
	mock := &mockExtractor{
		entries: []format.Entry{
			{
				Metadata: format.Metadata{Kind: format.KindLesson, Topic: "the fix was to unplug it"},
				Body:     "context around the fix was: cord was loose",
			},
			{
				Metadata: format.Metadata{Kind: format.KindOpenLoop, Topic: "need to audit the ranker tomorrow"},
				Body:     "we said we'd come back to the ranker",
			},
			{
				Metadata: format.Metadata{Kind: format.KindDecision, Topic: "unrelated extraction"},
				Body:     "nothing to see here",
			},
		},
	}

	tr := auditTranscript{
		ID:   "test-tiny",
		Path: txPath,
		Labels: []auditLabel{
			// keyword-tier lesson: should match the first entry
			{Kind: "lesson", Tier: "keyword", KeyPhrase: "the fix was to unplug"},
			// llm-tier open-loop: should match the second entry
			{Kind: "open-loop", Tier: "llm", KeyPhrase: "audit the ranker tomorrow"},
			// llm-tier war-story: should NOT match (no matching entry)
			{Kind: "war-story", Tier: "llm", KeyPhrase: "spent six hours debugging"},
		},
	}

	// Run against --mode=keyword: only the first label is in scope.
	resKeyword, err := auditOneTranscript(mock, tr, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if resKeyword.NumLabelsInScope != 1 {
		t.Errorf("keyword mode: want 1 in-scope label, got %d", resKeyword.NumLabelsInScope)
	}
	if resKeyword.NumLabelsTotal != 3 {
		t.Errorf("keyword mode: want 3 total labels, got %d", resKeyword.NumLabelsTotal)
	}
	if got := resKeyword.PerKind["lesson"].Matched; got != 1 {
		t.Errorf("keyword mode: want 1 matched lesson, got %d", got)
	}
	if len(resKeyword.UnmatchedLabels) != 0 {
		t.Errorf("keyword mode: want 0 unmatched labels, got %d", len(resKeyword.UnmatchedLabels))
	}

	// Run against --mode=llm: two labels in scope (open-loop + war-story).
	resLLM, err := auditOneTranscript(mock, tr, "llm")
	if err != nil {
		t.Fatal(err)
	}
	if resLLM.NumLabelsInScope != 2 {
		t.Errorf("llm mode: want 2 in-scope labels, got %d", resLLM.NumLabelsInScope)
	}
	if got := resLLM.PerKind["open-loop"].Matched; got != 1 {
		t.Errorf("llm mode: want 1 matched open-loop, got %d", got)
	}
	if got := resLLM.PerKind["war-story"].Matched; got != 0 {
		t.Errorf("llm mode: want 0 matched war-story, got %d", got)
	}
	if len(resLLM.UnmatchedLabels) != 1 {
		t.Errorf("llm mode: want 1 unmatched label, got %d", len(resLLM.UnmatchedLabels))
	}
}

func TestAuditOneTranscript_ClassifiesMissTypes(t *testing.T) {
	// Two unmatched labels, one of each miss type. Exercises the
	// diagnostic second-pass logic that classifies misses. The
	// transcript needs to contain both key_phrases so pre-flight
	// validation passes — the miss-type classification is about
	// WHAT EXTRACTIONS CONTAIN, not what the transcript contains.
	dir := t.TempDir()
	txPath := filepath.Join(dir, "t.txt")
	transcriptText := "root cause was a nil pointer dereference in the handler\nalways validate upstream before trusting input\n"
	if err := os.WriteFile(txPath, []byte(transcriptText), 0o644); err != nil {
		t.Fatal(err)
	}

	mock := &mockExtractor{
		entries: []format.Entry{
			// Contains "root cause ..." but tagged as lesson, not war-story.
			{
				Metadata: format.Metadata{Kind: format.KindLesson, Topic: "root cause of the crash"},
				Body:     "root cause was a nil pointer dereference",
			},
		},
	}

	tr := auditTranscript{
		ID:   "miss-types",
		Path: txPath,
		Labels: []auditLabel{
			// kind-mismatch: phrase appears in the lesson extraction,
			// but label says war-story.
			{Kind: "war-story", KeyPhrase: "root cause was a nil pointer"},
			// phrase-miss: nothing in any extraction contains this.
			{Kind: "pattern", KeyPhrase: "always validate upstream"},
		},
	}

	res, err := auditOneTranscript(mock, tr, "keyword")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.UnmatchedLabels) != 2 {
		t.Fatalf("want 2 unmatched, got %d", len(res.UnmatchedLabels))
	}

	var kindMismatch, phraseMiss *unmatchedLabel
	for i := range res.UnmatchedLabels {
		u := &res.UnmatchedLabels[i]
		switch u.MissType {
		case "kind-mismatch":
			kindMismatch = u
		case "phrase-miss":
			phraseMiss = u
		}
	}
	if kindMismatch == nil {
		t.Fatal("want a kind-mismatch entry, got none")
	}
	if kindMismatch.Kind != "war-story" {
		t.Errorf("kind-mismatch: want label kind war-story, got %q", kindMismatch.Kind)
	}
	if len(kindMismatch.ActualKinds) != 1 || kindMismatch.ActualKinds[0] != "lesson" {
		t.Errorf("kind-mismatch: want ActualKinds=[lesson], got %v", kindMismatch.ActualKinds)
	}
	if phraseMiss == nil {
		t.Fatal("want a phrase-miss entry, got none")
	}
	if phraseMiss.Kind != "pattern" {
		t.Errorf("phrase-miss: want label kind pattern, got %q", phraseMiss.Kind)
	}
	if len(phraseMiss.ActualKinds) != 0 {
		t.Errorf("phrase-miss: ActualKinds should be empty, got %v", phraseMiss.ActualKinds)
	}
}

func TestLoadAuditCorpus_ValidatesTierAndKind(t *testing.T) {
	dir := t.TempDir()
	corpusPath := filepath.Join(dir, "corpus.json")

	// Valid corpus — should load without error.
	valid := `{"transcripts":[{"id":"x","path":"foo.jsonl","labels":[
    {"kind":"lesson","tier":"keyword","key_phrase":"the fix was"}
  ]}]}`
	if err := os.WriteFile(corpusPath, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := loadAuditCorpus(corpusPath)
	if err != nil {
		t.Fatalf("valid corpus failed to load: %v", err)
	}
	// Relative path should be resolved against the corpus dir.
	want := filepath.Join(dir, "foo.jsonl")
	if c.Transcripts[0].Path != want {
		t.Errorf("relative path not resolved: got %q, want %q", c.Transcripts[0].Path, want)
	}

	// Invalid tier.
	badTier := `{"transcripts":[{"id":"x","path":"f","labels":[
    {"kind":"lesson","tier":"maybe","key_phrase":"x"}
  ]}]}`
	if err := os.WriteFile(corpusPath, []byte(badTier), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAuditCorpus(corpusPath); err == nil {
		t.Error("expected error on invalid tier, got nil")
	}

	// Invalid kind.
	badKind := `{"transcripts":[{"id":"x","path":"f","labels":[
    {"kind":"bogus","key_phrase":"x"}
  ]}]}`
	if err := os.WriteFile(corpusPath, []byte(badKind), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAuditCorpus(corpusPath); err == nil {
		t.Error("expected error on invalid kind, got nil")
	}

	// Empty key_phrase.
	emptyKey := `{"transcripts":[{"id":"x","path":"f","labels":[
    {"kind":"lesson","key_phrase":""}
  ]}]}`
	if err := os.WriteFile(corpusPath, []byte(emptyKey), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAuditCorpus(corpusPath); err == nil {
		t.Error("expected error on empty key_phrase, got nil")
	}
}
