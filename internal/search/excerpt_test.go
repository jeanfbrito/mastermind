package search

import (
	"strings"
	"testing"
)

// ─── BodyExcerpt ──────────────────────────────────────────────────────────

func TestBodyExcerptShortBodyReturnedVerbatim(t *testing.T) {
	body := "A short body under the threshold."
	got := BodyExcerpt(body, "anything")
	if got != body {
		t.Errorf("short body: got %q, want verbatim %q", got, body)
	}
}

func TestBodyExcerptMatchOnFirstLine(t *testing.T) {
	// Build a body long enough to exceed shortBodyThreshold.
	var sb strings.Builder
	sb.WriteString("electron ipc hangs on sync io\n")
	for i := 0; i < 50; i++ {
		sb.WriteString("unrelated content line that is somewhat long to pad the body size\n")
	}
	body := sb.String()

	got := BodyExcerpt(body, "electron")

	// The matched line should appear in the excerpt.
	if !strings.Contains(got, "electron ipc hangs") {
		t.Errorf("match line missing from excerpt: %q", got)
	}
	// Should not return the full body.
	if len(got) >= len(body) {
		t.Errorf("excerpt not shorter than full body: got %d chars, body %d chars", len(got), len(body))
	}
}

func TestBodyExcerptMatchOnMiddleLine(t *testing.T) {
	// Build body with the match in the middle, surrounded by padding.
	// Use a query token that is NOT present in the filler lines.
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("generic padding line that is long enough to bulk up the body size here\n")
	}
	sb.WriteString("the zorblax sentinel lives right here in this sentence\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("more generic padding below the match to ensure we exceed the threshold\n")
	}
	body := sb.String()

	got := BodyExcerpt(body, "zorblax sentinel")

	if !strings.Contains(got, "zorblax sentinel lives right here") {
		t.Errorf("match line missing from excerpt: %q", got)
	}
}

func TestBodyExcerptMatchOnLastLine(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("unrelated filler content that helps us exceed the body size threshold\n")
	}
	sb.WriteString("last line has the special token")
	body := sb.String()

	got := BodyExcerpt(body, "special token")

	if !strings.Contains(got, "special token") {
		t.Errorf("last-line match missing from excerpt: %q", got)
	}
}

func TestBodyExcerptNoMatchFallsBackToFirstSection(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("## What\nThis is the what section.\n")
	sb.WriteString("## Why\nThis is the why section with lots of detail.\n")
	// Pad to exceed threshold.
	for i := 0; i < 20; i++ {
		sb.WriteString("filler filler filler filler filler filler filler filler filler filler\n")
	}
	body := sb.String()

	// Query token "kubernetes" won't match anything in the body.
	got := BodyExcerpt(body, "kubernetes")

	if !strings.Contains(got, "## What") {
		t.Errorf("expected first ## section in fallback excerpt, got: %q", got)
	}
	// Should not include the second section header.
	if strings.Contains(got, "## Why") {
		t.Errorf("excerpt crossed section boundary into ## Why: %q", got)
	}
}

func TestBodyExcerptNoSectionsFallsBackToWordTrim(t *testing.T) {
	// Body with no ## sections and long enough to trigger trimming.
	body := strings.Repeat("word ", 300) // 1500 chars, no sections
	got := BodyExcerpt(body, "nonexistent")

	if len(got) >= len(body) {
		t.Errorf("expected trimmed output, got same length as body")
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got: %q", got[max(0, len(got)-5):])
	}
}

func TestBodyExcerptContextWindowSize(t *testing.T) {
	// Verify that the context window includes ±3 lines around the match.
	// Build a body where we can count lines precisely.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "padding line for context window size test that is long enough")
	}
	// Place match at line 25.
	lines[25] = "unique_marker_xyz found here"
	body := strings.Join(lines, "\n")

	got := BodyExcerpt(body, "unique_marker_xyz")
	gotLines := strings.Split(strings.TrimSpace(got), "\n")

	// Should be at most 7 lines (center ± 3).
	if len(gotLines) > 7 {
		t.Errorf("context window returned %d lines, want <= 7", len(gotLines))
	}
	// Must contain the match line itself.
	found := false
	for _, l := range gotLines {
		if strings.Contains(l, "unique_marker_xyz") {
			found = true
		}
	}
	if !found {
		t.Errorf("match line not in context window: %v", gotLines)
	}
}

func TestBodyExcerptEmptyQuery(t *testing.T) {
	// Empty query → skip match step, fall through to first ## section.
	var sb strings.Builder
	sb.WriteString("## Lesson\nThis is the lesson section.\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("filler filler filler filler filler filler filler filler filler filler\n")
	}
	body := sb.String()

	got := BodyExcerpt(body, "")

	if !strings.Contains(got, "## Lesson") {
		t.Errorf("empty query: expected first ## section, got: %q", got)
	}
}

// ─── firstSection ─────────────────────────────────────────────────────────

func TestFirstSectionSingleSection(t *testing.T) {
	lines := []string{"## What", "Content here.", "More content."}
	got := firstSection(lines)
	if !strings.Contains(got, "## What") {
		t.Errorf("firstSection missing header: %q", got)
	}
	if !strings.Contains(got, "Content here.") {
		t.Errorf("firstSection missing content: %q", got)
	}
}

func TestFirstSectionStopsAtNextSection(t *testing.T) {
	lines := []string{"## First", "first content", "## Second", "second content"}
	got := firstSection(lines)
	if strings.Contains(got, "## Second") {
		t.Errorf("firstSection crossed into second section: %q", got)
	}
	if strings.Contains(got, "second content") {
		t.Errorf("firstSection included second section content: %q", got)
	}
}

func TestFirstSectionNoSections(t *testing.T) {
	lines := []string{"no sections here", "just plain text"}
	got := firstSection(lines)
	if got != "" {
		t.Errorf("firstSection with no sections: got %q, want empty", got)
	}
}

// ─── contextWindow ────────────────────────────────────────────────────────

func TestContextWindowClampedAtStart(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4", "l5"}
	got := contextWindow(lines, 0, 3) // center=0, would want lines[-3..-1]
	// Should start at 0, not panic.
	if !strings.Contains(got, "l0") {
		t.Errorf("contextWindow clamped start: %q", got)
	}
}

func TestContextWindowClampedAtEnd(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4", "l5"}
	got := contextWindow(lines, 5, 3) // center=5 (last), would want lines[5+1..5+3]
	if !strings.Contains(got, "l5") {
		t.Errorf("contextWindow clamped end: %q", got)
	}
}
