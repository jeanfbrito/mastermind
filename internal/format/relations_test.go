package format

import (
	"strings"
	"testing"
)

// TestParseSupersedesAndContradicts verifies YAML round-trip for the
// two new relation fields (2026-04-10). Both are optional []string
// slices — omission must produce nil/empty slices, not errors.
func TestParseSupersedesAndContradicts(t *testing.T) {
	input := `---
date: "2026-04-10"
project: mastermind
topic: "Use mutex instead of channel"
kind: decision
scope: user-personal
supersedes:
  - old-decision-use-channel
  - even-older-lockfree
contradicts:
  - outdated-benchmark
---

Body text.
`
	entry, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := len(entry.Metadata.Supersedes); got != 2 {
		t.Errorf("Supersedes len = %d, want 2", got)
	}
	if entry.Metadata.Supersedes[0] != "old-decision-use-channel" {
		t.Errorf("Supersedes[0] = %q", entry.Metadata.Supersedes[0])
	}
	if got := len(entry.Metadata.Contradicts); got != 1 {
		t.Errorf("Contradicts len = %d, want 1", got)
	}
	if entry.Metadata.Contradicts[0] != "outdated-benchmark" {
		t.Errorf("Contradicts[0] = %q", entry.Metadata.Contradicts[0])
	}
}

// TestParseWithoutRelationsFields is a regression guard: entries
// written before 2026-04-10 have no supersedes/contradicts fields.
// Parse must still accept them and leave the slices empty.
func TestParseWithoutRelationsFields(t *testing.T) {
	input := `---
date: "2026-04-05"
project: mastermind
topic: "Legacy entry"
kind: lesson
scope: user-personal
---

Body.
`
	entry, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(entry.Metadata.Supersedes) != 0 {
		t.Errorf("Supersedes = %v, want empty", entry.Metadata.Supersedes)
	}
	if len(entry.Metadata.Contradicts) != 0 {
		t.Errorf("Contradicts = %v, want empty", entry.Metadata.Contradicts)
	}
}

// TestMarshalPreservesRelations verifies the round-trip: parse an
// entry with supersedes/contradicts, marshal it back, and confirm
// the fields survive. Also confirms that empty slices do NOT emit
// (omitempty) so legacy entries don't grow new frontmatter lines
// on every rewrite.
func TestMarshalPreservesRelations(t *testing.T) {
	input := `---
date: "2026-04-10"
project: mastermind
topic: "Use mutex instead of channel"
kind: decision
scope: user-personal
supersedes:
  - old-slug
contradicts:
  - conflict-slug
---

Body.
`
	entry, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := entry.MarshalMarkdown()
	if err != nil {
		t.Fatalf("MarshalMarkdown: %v", err)
	}
	if !strings.Contains(string(out), "supersedes:") {
		t.Errorf("marshal missing supersedes:\n%s", out)
	}
	if !strings.Contains(string(out), "- old-slug") {
		t.Errorf("marshal missing old-slug:\n%s", out)
	}
	if !strings.Contains(string(out), "contradicts:") {
		t.Errorf("marshal missing contradicts:\n%s", out)
	}

	// Empty slices must NOT emit.
	empty := &Entry{
		Metadata: Metadata{
			Date:    "2026-04-10",
			Project: "mm",
			Topic:   "no relations",
			Kind:    KindLesson,
			Scope:   ScopeUserPersonal,
		},
		Body: "body",
	}
	out2, err := empty.MarshalMarkdown()
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	if strings.Contains(string(out2), "supersedes") {
		t.Errorf("empty entry emitted supersedes:\n%s", out2)
	}
	if strings.Contains(string(out2), "contradicts") {
		t.Errorf("empty entry emitted contradicts:\n%s", out2)
	}
}
