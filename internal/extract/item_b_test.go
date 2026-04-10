package extract

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// ─── Filler filter (2026-04-10) ────────────────────────────────────────

func TestFillerPattern_MatchesCommonOpeners(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"Ok, let me look at this.", true},
		{"ok let me check", true},
		{"Sure, I'll go with option A.", true}, // "Sure," is filler
		{"  Let me think.", true},              // leading whitespace
		{"Here's what I found:", true},
		{"Here is what I found", false}, // "Here is" isn't in the filler list
		{"Alright, moving on.", true},
		{"Got it. The fix was to revert.", true},
		{"Understood — will fix now.", true},
		{"Let's see the logs.", true},
		{"Sounds good. I'll use Redis.", true},
		{"On it. Starting now.", true},
		{"Looking at the file.", true},

		// Must NOT match legitimate content lines.
		{"The fix was to revert the migration.", false},
		{"I'll use Redis because it's faster.", false}, // must still fire as a decision
		{"The issue was a race condition.", false},
		{"Going to look at this.", false},  // "going to" alone isn't filler
		{"Decided to use mutex.", false},
		{"", false},
		{"    ", false},
	}
	for _, tc := range cases {
		got := fillerPattern.MatchString(tc.line)
		if got != tc.want {
			t.Errorf("fillerPattern.MatchString(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// TestKeywordExtractor_SkipsFillerLines verifies that a filler opener
// line does not trigger an extraction even if it contains a token a
// lower-precision regex would otherwise match (e.g. "going to" or
// "because"). The precision win is exactly this: filler lines carry
// no signal and shouldn't produce candidate entries.
func TestKeywordExtractor_SkipsFillerLines(t *testing.T) {
	// "Let me use the context7 approach because it's faster" would
	// previously fire the "because" (decision) pattern. With the
	// filler filter, the leading "Let me" drops the whole line.
	transcript := "Let me use the context7 approach because it's faster to iterate."

	kw := &KeywordExtractor{ProjectName: "test"}
	entries, err := kw.Extract(transcript, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// All the signal regexes that could match ("going to", "because",
	// "i'll use") are gated behind the filler filter on this line.
	if len(entries) != 0 {
		t.Errorf("filler line produced %d entries, want 0: %+v", len(entries), entries)
	}
}

// TestKeywordExtractor_FillerFilterDoesNotHurtRealContent verifies
// that non-filler decision lines still produce entries. The filter
// must be precise — anchored to line-start with a word boundary — so
// legitimate content containing filler-adjacent words isn't dropped.
func TestKeywordExtractor_FillerFilterDoesNotHurtRealContent(t *testing.T) {
	transcript := `The fix was to revert the migration.
We decided to use mutex locking.
The plan is to ship Friday.`

	kw := &KeywordExtractor{ProjectName: "test"}
	entries, err := kw.Extract(transcript, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry from real content lines")
	}
}

// ─── Session timestamp header (2026-04-10) ─────────────────────────────

func TestSessionTimestampHeader_Format(t *testing.T) {
	// Freeze sessionNow for deterministic output.
	oldNow := sessionNow
	sessionNow = func() time.Time {
		return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() { sessionNow = oldNow })

	got := sessionTimestampHeader()
	want := "Session time: 2026-04-10 (Friday)\n\n"
	if got != want {
		t.Errorf("sessionTimestampHeader() = %q, want %q", got, want)
	}
}

func TestSessionTimestampHeader_RealClockNotZero(t *testing.T) {
	// Without the test hook, sessionNow is time.Now. The header must
	// at minimum start with "Session time: " and contain a date —
	// guards against an accidental empty return.
	got := sessionTimestampHeader()
	if !strings.HasPrefix(got, "Session time: ") {
		t.Errorf("header missing prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("header missing trailing blank line: %q", got)
	}
}

// ─── Gap-fill skip (2026-04-10) ────────────────────────────────────────

// TestLLMExtractor_GapFillSkipWhenKeywordRich verifies that when the
// keyword tier already produces >= GapFillThreshold entries, the LLM
// call is skipped and the keyword entries are returned directly.
//
// This is the load-bearing test for the gap-fill skip path: we construct
// an LLMExtractor with an unreachable LLM endpoint (localhost:1 — no
// service, connection refused). If the skip works, the test passes
// silently because the LLM is never called. If the skip fails, we'd
// see an HTTP error.
func TestLLMExtractor_GapFillSkipWhenKeywordRich(t *testing.T) {
	// Build a transcript rich enough that the keyword tier extracts
	// >= 5 entries. Multiple distinct signal lines across kinds.
	transcript := `The fix was to revert the migration.
We decided to use mutex locking.
The plan is to ship Friday.
The pattern is to lock before read.
TODO: investigate the race condition next week.
We should add metrics.
`

	// Sanity: run the keyword tier directly to confirm it extracts
	// at least 5 entries from this transcript.
	kw := &KeywordExtractor{ProjectName: "test"}
	kwEntries, err := kw.Extract(transcript, nil)
	if err != nil {
		t.Fatalf("keyword Extract: %v", err)
	}
	if len(kwEntries) < 5 {
		t.Fatalf("fixture only produced %d keyword entries, want >= 5 — add more signal lines", len(kwEntries))
	}

	// Now build an LLMExtractor pointed at an unreachable endpoint.
	// If the gap-fill skip works, no HTTP call is made and this
	// test passes silently.
	l := &LLMExtractor{
		cfg: Config{
			LLMProvider:      "openai",
			LLMModel:         "test-model",
			BaseURL:          "http://127.0.0.1:1/v1", // guaranteed connection refused
			APIKey:           "test-key",
			ProjectName:      "test",
			GapFillThreshold: 5,
		},
		keyword: &KeywordExtractor{ProjectName: "test"},
	}

	entries, err := l.Extract(transcript, nil)
	if err != nil {
		t.Fatalf("gap-fill skip failed — LLM was called: %v", err)
	}
	if len(entries) < 5 {
		t.Errorf("got %d entries, want >= 5 (keyword results)", len(entries))
	}
}

// TestLLMExtractor_GapFillThresholdZeroDisablesSkip verifies that
// GapFillThreshold = 0 means "always run the LLM" — the pre-2026-04-10
// default behavior. Set to zero, the unreachable endpoint should cause
// a fallback to the keyword tier (Strict=false), not bypass it.
func TestLLMExtractor_GapFillThresholdZeroDisablesSkip(t *testing.T) {
	transcript := `The fix was to revert the migration.
We decided to use mutex locking.
The plan is to ship Friday.`

	l := &LLMExtractor{
		cfg: Config{
			LLMProvider:      "openai",
			LLMModel:         "test-model",
			BaseURL:          "http://127.0.0.1:1/v1",
			APIKey:           "test-key",
			ProjectName:      "test",
			GapFillThreshold: 0, // skip disabled
			Strict:           false,
		},
		keyword: &KeywordExtractor{ProjectName: "test"},
		httpClient: &http.Client{
			// Short timeout — we expect connection refused, not a hang.
			Timeout: 2 * time.Second,
		},
	}

	// LLM call will fail (connection refused). With Strict=false, we
	// fall back to the cached keyword results. Should succeed.
	entries, err := l.Extract(transcript, nil)
	if err != nil {
		t.Fatalf("fallback failed: %v", err)
	}
	if len(entries) == 0 {
		t.Error("fallback returned zero entries, want the keyword results")
	}
}
