package extract

import (
	"strings"
	"testing"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// ─── deriveTopic ──────────────────────────────────────────────────────

func TestDeriveTopic_CleansMarkdownPrefixes(t *testing.T) {
	got := deriveTopic("## The fix was to revert the migration")
	if strings.HasPrefix(got, "#") {
		t.Errorf("deriveTopic kept markdown prefix: %q", got)
	}
	if got == "" {
		t.Error("deriveTopic returned empty for valid line")
	}
}

func TestDeriveTopic_CleansTranscriptPrefixes(t *testing.T) {
	got := deriveTopic(`"content": "The fix was to revert the migration"`)
	if strings.Contains(strings.ToLower(got), "content") {
		t.Errorf("deriveTopic kept transcript prefix: %q", got)
	}
}

func TestDeriveTopic_RejectsShortStrings(t *testing.T) {
	if got := deriveTopic("short"); got != "" {
		t.Errorf("deriveTopic(%q) = %q, want empty", "short", got)
	}
	if got := deriveTopic("  hi  "); got != "" {
		t.Errorf("deriveTopic(%q) = %q, want empty", "hi", got)
	}
}

func TestDeriveTopic_TruncatesLongStrings(t *testing.T) {
	long := strings.Repeat("word ", 50) // 250 chars
	got := deriveTopic(long)
	if len(got) > 120 {
		t.Errorf("deriveTopic returned %d chars, want <= 120", len(got))
	}
	// Should truncate at word boundary.
	if strings.HasSuffix(got, " ") {
		t.Errorf("deriveTopic has trailing space: %q", got)
	}
}

func TestDeriveTopic_TrimsQuotesAndPunctuation(t *testing.T) {
	got := deriveTopic(`"The solution was to use a mutex lock."`)
	if strings.HasPrefix(got, "\"") || strings.HasSuffix(got, ".") {
		t.Errorf("deriveTopic kept quotes/punctuation: %q", got)
	}
}

// ─── isDuplicate ──────────────────────────────────────────────────────

func TestIsDuplicate_SubstringMatch(t *testing.T) {
	existing := map[string]bool{
		"fix migration rollback":  true,
		"electron ipc deadlock":   true,
	}

	// Exact match.
	if !isDuplicate("fix migration rollback", existing) {
		t.Error("exact match should be duplicate")
	}
	// Existing contains candidate.
	if !isDuplicate("migration", existing) {
		t.Error("substring of existing should be duplicate")
	}
	// Candidate contains existing.
	if !isDuplicate("fix migration rollback in staging", existing) {
		t.Error("candidate containing existing should be duplicate")
	}
	// No match.
	if isDuplicate("goroutine leak in http handler", existing) {
		t.Error("unrelated topic should not be duplicate")
	}
}

// ─── KeywordExtractor.Extract ─────────────────────────────────────────

func TestExtract_EmptyTranscript(t *testing.T) {
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("empty transcript returned %d entries, want nil", len(entries))
	}
}

func TestExtract_WhitespaceTranscript(t *testing.T) {
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract("   \n\n  \t  ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("whitespace transcript returned %d entries, want nil", len(entries))
	}
}

func TestExtract_MatchesLessonPatterns(t *testing.T) {
	transcript := `line 1
line 2
line 3
The fix was to add a mutex around the shared map access.
This prevented the race condition we'd been seeing.
line 6
line 7
line 8`

	k := &KeywordExtractor{ProjectName: "myproject"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one lesson entry")
	}

	found := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindLesson {
			found = true
			if e.Metadata.Project != "myproject" {
				t.Errorf("project = %q, want %q", e.Metadata.Project, "myproject")
			}
			if e.Metadata.Confidence != format.ConfidenceMedium {
				t.Errorf("confidence = %q, want %q", e.Metadata.Confidence, format.ConfidenceMedium)
			}
		}
	}
	if !found {
		t.Error("no entry with kind=lesson found")
	}
}

func TestExtract_MatchesWarStoryPattern(t *testing.T) {
	transcript := "We wasted hours debugging the flaky test before finding the timezone issue."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasWarStory := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindWarStory {
			hasWarStory = true
		}
	}
	if !hasWarStory {
		t.Error("expected war-story entry for 'wasted hours debugging'")
	}
}

func TestExtract_MatchesDecisionPattern(t *testing.T) {
	transcript := "We decided to use PostgreSQL over MySQL for the better JSON support and ACID compliance."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasDecision := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindDecision {
			hasDecision = true
		}
	}
	if !hasDecision {
		t.Error("expected decision entry for 'decided to'")
	}
}

func TestExtract_MatchesPatternKind(t *testing.T) {
	transcript := "The pattern is to always wrap database calls in a transaction context."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasPattern := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindPattern {
			hasPattern = true
		}
	}
	if !hasPattern {
		t.Error("expected pattern entry for 'the pattern is'")
	}
}

func TestExtract_MatchesOpenLoopPattern(t *testing.T) {
	transcript := "TODO: we still need to add rate limiting to the public API endpoints."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasOpenLoop := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindOpenLoop {
			hasOpenLoop = true
		}
	}
	if !hasOpenLoop {
		t.Error("expected open-loop entry for 'TODO:'")
	}
}

func TestExtract_MatchesInsightPattern(t *testing.T) {
	transcript := "I realized that the ORM was generating N+1 queries for every nested relation."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasInsight := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindInsight {
			hasInsight = true
		}
	}
	if !hasInsight {
		t.Error("expected insight entry for 'realized that'")
	}
}

func TestExtract_ContextWindow(t *testing.T) {
	// Build a transcript where the match is on line 5 (index 4).
	// Context should be lines 1-9 (3 before + match + 5 after).
	var lines []string
	for i := 1; i <= 15; i++ {
		if i == 5 {
			lines = append(lines, "The fix was to restart the service after config changes.")
		} else {
			lines = append(lines, "context line "+string(rune('A'-1+i)))
		}
	}
	transcript := strings.Join(lines, "\n")

	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}

	body := entries[0].Body
	// Should include line 2 (3 before line 5 = lines 2,3,4).
	if !strings.Contains(body, "context line B") {
		t.Errorf("body missing 3-lines-before context: %q", body)
	}
	// Should include line 10 (5 after line 5 = lines 6,7,8,9,10).
	if !strings.Contains(body, "context line J") {
		t.Errorf("body missing 5-lines-after context: %q", body)
	}
	// Should NOT include line 11.
	if strings.Contains(body, "context line K") {
		t.Errorf("body includes too much after-context: %q", body)
	}
}

func TestExtract_DedupAgainstExistingTopics(t *testing.T) {
	transcript := "The fix was to add a mutex around the shared map."
	existing := []string{"add a mutex around the shared map"}

	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries (dedup against existing), got %d", len(entries))
	}
}

func TestExtract_DedupAgainstSelf(t *testing.T) {
	// Two lines that would extract the same topic.
	transcript := `The fix was to add connection pooling.
The solution was to add connection pooling.`

	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Both match lesson patterns, but topics are identical — should dedup.
	topics := make(map[string]bool)
	for _, e := range entries {
		topics[strings.ToLower(e.Metadata.Topic)] = true
	}
	// We can't assert exactly 1 because deriveTopic might produce slightly
	// different results. But there should be fewer than 2 identical topics.
	if len(entries) > len(topics) {
		t.Errorf("got %d entries but only %d unique topics — dedup failed", len(entries), len(topics))
	}
}

func TestExtract_DefaultProjectName(t *testing.T) {
	transcript := "The fix was to increase the connection timeout to 30 seconds."
	k := &KeywordExtractor{ProjectName: ""}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	if entries[0].Metadata.Project != "general" {
		t.Errorf("project = %q, want %q", entries[0].Metadata.Project, "general")
	}
}

// ─── parseExtractionResponse ──────────────────────────────────────────

func TestParseExtractionResponse_ValidJSON(t *testing.T) {
	raw := `[{"topic":"Use mutex for shared maps","kind":"lesson","body":"Always protect shared maps with sync.Mutex in Go.","tags":["go","concurrency"],"category":"go"}]`
	entries, err := parseExtractionResponse(raw, "myproject", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Metadata.Kind != format.KindLesson {
		t.Errorf("kind = %q, want lesson", e.Metadata.Kind)
	}
	if e.Metadata.Project != "myproject" {
		t.Errorf("project = %q, want myproject", e.Metadata.Project)
	}
	if e.Metadata.Confidence != format.ConfidenceMedium {
		t.Errorf("confidence = %q, want medium", e.Metadata.Confidence)
	}
	if e.Metadata.Category != "go" {
		t.Errorf("category = %q, want go", e.Metadata.Category)
	}
}

func TestParseExtractionResponse_CodeFences(t *testing.T) {
	raw := "```json\n" + `[{"topic":"Wrap DB calls in transactions","kind":"pattern","body":"Always use a transaction context.","tags":["db"],"category":"database"}]` + "\n```"
	entries, err := parseExtractionResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}

func TestParseExtractionResponse_InvalidKindFallsToLesson(t *testing.T) {
	raw := `[{"topic":"Something interesting happened here","kind":"bogus","body":"Details about what happened.","tags":["misc"],"category":"misc"}]`
	entries, err := parseExtractionResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Metadata.Kind != format.KindLesson {
		t.Errorf("invalid kind should fallback to lesson, got %q", entries[0].Metadata.Kind)
	}
}

func TestParseExtractionResponse_SkipsEmptyTopicOrBody(t *testing.T) {
	raw := `[{"topic":"","kind":"lesson","body":"has body","tags":[],"category":"x"},{"topic":"has topic","kind":"lesson","body":"","tags":[],"category":"x"}]`
	entries, err := parseExtractionResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (empty topic/body skipped)", len(entries))
	}
}

func TestParseExtractionResponse_DedupAgainstExisting(t *testing.T) {
	raw := `[{"topic":"Fix migration rollback","kind":"lesson","body":"Details here.","tags":["db"],"category":"db"}]`
	existing := []string{"fix migration rollback"}
	entries, err := parseExtractionResponse(raw, "test", existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (dedup against existing)", len(entries))
	}
}

func TestParseExtractionResponse_InvalidJSON(t *testing.T) {
	_, err := parseExtractionResponse("not json at all", "test", nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseExtractionResponse_EmptyArray(t *testing.T) {
	entries, err := parseExtractionResponse("[]", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestParseExtractionResponse_DefaultProject(t *testing.T) {
	raw := `[{"topic":"Some useful insight about caching strategies","kind":"insight","body":"Cache invalidation details.","tags":["cache"],"category":"infra"}]`
	entries, err := parseExtractionResponse(raw, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected 1 entry")
	}
	if entries[0].Metadata.Project != "general" {
		t.Errorf("project = %q, want general", entries[0].Metadata.Project)
	}
}

// ─── NewExtractor factory ─────────────────────────────────────────────

func TestNewExtractor_DefaultIsKeyword(t *testing.T) {
	ext := NewExtractor(DefaultConfig())
	if _, ok := ext.(*KeywordExtractor); !ok {
		t.Errorf("default extractor should be KeywordExtractor, got %T", ext)
	}
}

func TestNewExtractor_LLMFallsBackToKeyword(t *testing.T) {
	cfg := Config{Mode: "llm", LLMProvider: "anthropic"} // no API key
	ext := NewExtractor(cfg)
	if _, ok := ext.(*KeywordExtractor); !ok {
		t.Errorf("LLM without key should fallback to KeywordExtractor, got %T", ext)
	}
}
