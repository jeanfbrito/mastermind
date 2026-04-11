package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// TestEstimateTokensRoughBytes exercises the 4-chars-per-token
// heuristic used by the L0/L1 soft budget check. The formula is
// ceil(len/4); tests pin the expected values for a handful of
// representative strings so the heuristic doesn't drift silently.
func TestEstimateTokensRoughBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{strings.Repeat("x", 100), 25},
		{strings.Repeat("x", 2000), 500},
	}
	for _, tc := range cases {
		if got := estimateTokens(tc.in); got != tc.want {
			t.Errorf("estimateTokens(len=%d) = %d, want %d", len(tc.in), got, tc.want)
		}
	}
}

// TestFormatSessionStartUnderBudgetNoWarning verifies that a
// normally-sized SessionStart block (a handful of entries) emits
// no warning to the budget writer. This is the load-bearing silent-
// unless-needed case — the 99% path for users.
func TestFormatSessionStartUnderBudgetNoWarning(t *testing.T) {
	loops := []store.EntryRef{
		makeLoop("auth refactor blocker", "2026-04-05"),
		makeLoop("electron IPC race condition", "2026-04-06"),
		makeLoop("ingest pipeline backfill retry", "2026-04-07"),
	}
	proj := []store.EntryRef{
		makeProject("Topic one", format.KindLesson),
		makeProject("Topic two", format.KindInsight),
		makeProject("Topic three", format.KindDecision),
	}

	var warn bytes.Buffer
	out := formatSessionStart(loops, proj, 2, &warn)

	if out == "" {
		t.Fatal("expected non-empty output for under-budget input")
	}
	if warn.Len() != 0 {
		t.Errorf("expected no budget warnings, got:\n%s", warn.String())
	}
}

// TestFormatSessionStartL0OverBudgetWarns constructs enough open
// loops to push the L0 block past its soft 500-token budget and
// verifies a single one-line warning is written to warnOut with
// the expected shape.
func TestFormatSessionStartL0OverBudgetWarns(t *testing.T) {
	// Each loop line is "- <topic> (<date>)\n" ≈ 80 bytes. To land
	// well over 500 tokens (~2000 bytes) build 40 loops at ~80
	// bytes each = ~3200 bytes ≈ 800 tokens, comfortably over.
	loops := make([]store.EntryRef, 0, 40)
	for i := 0; i < 40; i++ {
		loops = append(loops, makeLoop(
			fmt.Sprintf("padding topic %02d with enough room to make bytes", i),
			"2026-04-10",
		))
	}

	var warn bytes.Buffer
	out := formatSessionStart(loops, nil, 0, &warn)

	if out == "" {
		t.Fatal("expected non-empty output for over-budget input (no truncation)")
	}
	if warn.Len() == 0 {
		t.Fatal("expected an L0 budget warning, got none")
	}
	warning := warn.String()
	if !strings.Contains(warning, "L0 open-loops") {
		t.Errorf("warning missing L0 label: %q", warning)
	}
	if !strings.Contains(warning, "exceeds soft budget") {
		t.Errorf("warning missing expected phrase: %q", warning)
	}
	if !strings.Contains(warning, "> 500 tokens") {
		t.Errorf("warning missing budget number: %q", warning)
	}
	if strings.Count(warning, "\n") != 1 {
		t.Errorf("expected a single-line warning, got %d newlines: %q",
			strings.Count(warning, "\n"), warning)
	}
}

// TestFormatSessionStartL1OverBudgetWarns verifies the symmetric
// case for the L1 project-knowledge block (budget 2000 tokens).
func TestFormatSessionStartL1OverBudgetWarns(t *testing.T) {
	// L1 budget is 2000 tokens ≈ 8000 bytes. Each line is
	// "- <topic> · <kind>\n". With a ~76-char topic + " · lesson"
	// that's ~88 bytes per line; 120 entries ≈ 10560 bytes ≈ 2640
	// tokens — comfortably over.
	proj := make([]store.EntryRef, 0, 120)
	for i := 0; i < 120; i++ {
		proj = append(proj, makeProject(
			fmt.Sprintf("padding topic %03d with enough bytes to matter, generously padded so it counts", i),
			format.KindLesson,
		))
	}

	var warn bytes.Buffer
	out := formatSessionStart(nil, proj, 0, &warn)

	if out == "" {
		t.Fatal("expected non-empty output for over-budget L1 input")
	}
	warning := warn.String()
	if !strings.Contains(warning, "L1 project knowledge") {
		t.Errorf("warning missing L1 label: %q", warning)
	}
	if !strings.Contains(warning, "> 2000 tokens") {
		t.Errorf("warning missing budget number: %q", warning)
	}
}

// TestFormatSessionStartDoesNotTruncateOverBudget is the load-
// bearing invariant: the budget check warns but never silently
// drops entries. An over-budget block must still render every
// entry so the user sees the full picture and can make an
// informed pruning decision. This locks in the "surface, don't
// hide" rule from the L0-L3 open-loop spec.
func TestFormatSessionStartDoesNotTruncateOverBudget(t *testing.T) {
	loops := make([]store.EntryRef, 0, 40)
	for i := 0; i < 40; i++ {
		loops = append(loops, makeLoop(
			fmt.Sprintf("sentinel-loop-%02d unique marker phrase here", i),
			"2026-04-10",
		))
	}

	var warn bytes.Buffer
	out := formatSessionStart(loops, nil, 0, &warn)

	// Every single loop topic must appear in the output — no
	// silent truncation under any circumstances.
	for i := 0; i < 40; i++ {
		marker := fmt.Sprintf("sentinel-loop-%02d", i)
		if !strings.Contains(out, marker) {
			t.Errorf("output missing loop %s — budget check must not truncate", marker)
		}
	}
	// And the warning still fires.
	if warn.Len() == 0 {
		t.Error("expected a budget warning alongside the full output")
	}
}

// TestFormatSessionStartNilWarnerIsSafe verifies the function
// doesn't panic and behaves normally when warnOut is nil. The
// nil case is used by callers that want to disable the budget
// check entirely (none today, but the contract should be
// resilient).
func TestFormatSessionStartNilWarnerIsSafe(t *testing.T) {
	loops := make([]store.EntryRef, 0, 40)
	for i := 0; i < 40; i++ {
		loops = append(loops, makeLoop("padding topic with plenty of bytes", "2026-04-10"))
	}
	out := formatSessionStart(loops, nil, 0, nil)
	if out == "" {
		t.Fatal("expected non-empty output with nil warnOut")
	}
}

// ─── fixtures ───────────────────────────────────────────────────────────

func makeLoop(topic, date string) store.EntryRef {
	return store.EntryRef{
		Path:  "/tmp/.knowledge/search/" + topic + ".md",
		Scope: format.ScopeProjectShared,
		Metadata: format.Metadata{
			Date:  date,
			Topic: topic,
			Kind:  format.KindOpenLoop,
			Scope: format.ScopeProjectShared,
		},
	}
}

func makeProject(topic string, kind format.Kind) store.EntryRef {
	return store.EntryRef{
		Path:  "/tmp/.knowledge/lessons/" + topic + ".md",
		Scope: format.ScopeProjectShared,
		Metadata: format.Metadata{
			Date:  "2026-04-05",
			Topic: topic,
			Kind:  kind,
			Scope: format.ScopeProjectShared,
		},
	}
}
