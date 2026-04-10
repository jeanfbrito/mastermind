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

// ─── Decision patterns (soulforge WSM additions) ──────────────────────

func TestExtract_DecisionIllUse(t *testing.T) {
	transcript := "I'll use Postgres because it has better JSON support and ACID compliance."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	hasDecision := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindDecision {
			hasDecision = true
			if e.Metadata.Confidence != format.ConfidenceMedium {
				t.Errorf("I'll use: confidence = %q, want medium", e.Metadata.Confidence)
			}
		}
	}
	if !hasDecision {
		t.Error("expected decision entry for \"I'll use\"")
	}
}

func TestExtract_DecisionIllGoWith(t *testing.T) {
	transcript := "I'll go with the goroutine pool approach to cap memory usage."
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
		t.Error("expected decision entry for \"I'll go with\"")
	}
}

func TestExtract_DecisionLetsUse(t *testing.T) {
	transcript := "Let's use Redis for the session store — it's already in the stack."
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
		t.Error("expected decision entry for \"let's use\"")
	}
}

func TestExtract_DecisionThePlanIs(t *testing.T) {
	transcript := "The plan is to migrate the database schema in three stages to minimize downtime."
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
		t.Error("expected decision entry for \"the plan is\"")
	}
}

func TestExtract_DecisionGoingTo_LowConfidence(t *testing.T) {
	transcript := "We're going to refactor the auth module next sprint."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Metadata.Kind == format.KindDecision {
			if e.Metadata.Confidence != format.ConfidenceLow {
				t.Errorf("going to: confidence = %q, want low", e.Metadata.Confidence)
			}
		}
	}
}

func TestExtract_DecisionBecause_LowConfidence(t *testing.T) {
	transcript := "We picked gRPC because it gives us bidirectional streaming out of the box."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundBecauseLow := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindDecision && e.Metadata.Confidence == format.ConfidenceLow {
			foundBecauseLow = true
		}
	}
	if !foundBecauseLow {
		t.Error("expected low-confidence decision entry for \"because\"")
	}
}

// ─── Discovery / insight patterns (soulforge WSM additions) ───────────

func TestExtract_InsightFoundThat(t *testing.T) {
	transcript := "Found that the ORM was issuing a separate query for every nested association."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	hasInsight := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindInsight {
			hasInsight = true
			if e.Metadata.Confidence != format.ConfidenceMedium {
				t.Errorf("found that: confidence = %q, want medium", e.Metadata.Confidence)
			}
		}
	}
	if !hasInsight {
		t.Error("expected insight entry for \"found that\"")
	}
}

func TestExtract_InsightTheIssueWas(t *testing.T) {
	transcript := "The issue was that the lock was being held across the network call."
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
		t.Error("expected insight entry for \"the issue was\"")
	}
}

func TestExtract_InsightDiscovered(t *testing.T) {
	transcript := "I discovered that Go's http.Client does not timeout on slow response bodies by default."
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
		t.Error("expected insight entry for \"discovered\"")
	}
}

func TestExtract_InsightItSeems_LowConfidence(t *testing.T) {
	transcript := "It seems the garbage collector is running more frequently under load."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundLow := false
	for _, e := range entries {
		if e.Metadata.Kind == format.KindInsight && e.Metadata.Confidence == format.ConfidenceLow {
			foundLow = true
		}
	}
	if !foundLow {
		t.Error("expected low-confidence insight entry for \"it seems\"")
	}
}

// ─── Case-insensitivity ───────────────────────────────────────────────

func TestExtract_CaseInsensitive_Decision(t *testing.T) {
	for _, phrase := range []string{
		"I'LL USE gRPC for the new service.",
		"I'll Use Postgres for persistence.",
		"DECIDED TO switch to a message queue.",
		"LET'S USE the stdlib http client here.",
	} {
		k := &KeywordExtractor{ProjectName: "test"}
		entries, err := k.Extract(phrase, nil)
		if err != nil {
			t.Fatalf("phrase %q: %v", phrase, err)
		}
		found := false
		for _, e := range entries {
			if e.Metadata.Kind == format.KindDecision {
				found = true
			}
		}
		if !found {
			t.Errorf("case-insensitive match failed for %q", phrase)
		}
	}
}

func TestExtract_CaseInsensitive_Insight(t *testing.T) {
	for _, phrase := range []string{
		"FOUND THAT the index was missing on the foreign key column.",
		"The Issue Was a race between the writer and the compaction goroutine.",
		"DISCOVERED the cache was being evicted too aggressively.",
	} {
		k := &KeywordExtractor{ProjectName: "test"}
		entries, err := k.Extract(phrase, nil)
		if err != nil {
			t.Fatalf("phrase %q: %v", phrase, err)
		}
		found := false
		for _, e := range entries {
			if e.Metadata.Kind == format.KindInsight {
				found = true
			}
		}
		if !found {
			t.Errorf("case-insensitive match failed for %q", phrase)
		}
	}
}

// ─── Word-boundary anchoring ──────────────────────────────────────────

func TestExtract_WordBoundary_IllUse_NoFalsePositive(t *testing.T) {
	// "I'll use" should NOT match inside words like "illustrate" or "illusive".
	// These phrases don't contain the boundary-anchored pattern.
	transcript := "The illusive bug turned out to be a simple off-by-one error."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Metadata.Kind == format.KindDecision {
			// A decision match here would mean the boundary check failed.
			// The only reason a decision would match is "turned out" → lesson/insight
			// not decision, so specifically check for I'll-use false positive via topic.
			if strings.Contains(strings.ToLower(e.Metadata.Topic), "illusive") {
				t.Errorf("word-boundary failed: matched 'illusive' as decision: %q", e.Metadata.Topic)
			}
		}
	}
}

func TestExtract_WordBoundary_Discovered_NoFalsePositive(t *testing.T) {
	// "undiscovered" should not match \bdiscovered\b.
	transcript := "There are undiscovered edge cases in the parser that we should handle."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Metadata.Kind == format.KindInsight && strings.Contains(strings.ToLower(e.Body), "undiscovered") {
			// If the topic came from the "undiscovered" word specifically, that's a false positive.
			// However, "we should" will also fire an open-loop on this line, so we only
			// check that no insight has a topic derived purely from "undiscovered".
			if strings.HasPrefix(strings.ToLower(e.Metadata.Topic), "undiscovered") {
				t.Errorf("word-boundary failed: 'undiscovered' matched as insight: %q", e.Metadata.Topic)
			}
		}
	}
}

// ─── Overlapping matches / dedup ──────────────────────────────────────

func TestExtract_OverlappingPatterns_NoDuplicates(t *testing.T) {
	// "I'll use Postgres because it's fast" matches both "I'll use" (decision/medium)
	// and "because" (decision/low). The seen map should deduplicate — same topic,
	// only one entry emitted (whichever pattern fires first wins).
	transcript := "I'll use Postgres because it has better JSON support and ACID compliance."
	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	topics := make(map[string]int)
	for _, e := range entries {
		topics[strings.ToLower(e.Metadata.Topic)]++
	}
	for topic, count := range topics {
		if count > 1 {
			t.Errorf("duplicate topic %q: appeared %d times", topic, count)
		}
	}
}

// ─── Multi-line paragraph context ────────────────────────────────────

func TestExtract_MultiLineParagraphContext(t *testing.T) {
	transcript := `We evaluated several options for the cache layer.
Redis has pub/sub which we don't need right now.
Memcached is simpler and faster for pure key-value.

I'll use Memcached for the session store since we only need TTL expiry.

We can always migrate to Redis if pub/sub becomes a requirement.`

	k := &KeywordExtractor{ProjectName: "test"}
	entries, err := k.Extract(transcript, nil)
	if err != nil {
		t.Fatal(err)
	}

	var decisionEntry *format.Entry
	for i := range entries {
		if entries[i].Metadata.Kind == format.KindDecision &&
			strings.Contains(strings.ToLower(entries[i].Body), "memcached") {
			decisionEntry = &entries[i]
			break
		}
	}
	if decisionEntry == nil {
		t.Fatal("expected decision entry mentioning Memcached")
	}
	// Body should include surrounding context lines, not just the match line.
	if !strings.Contains(decisionEntry.Body, "Memcached is simpler") {
		t.Errorf("body missing pre-match context; got: %q", decisionEntry.Body)
	}
	if !strings.Contains(decisionEntry.Body, "always migrate") {
		t.Errorf("body missing post-match context; got: %q", decisionEntry.Body)
	}
}
