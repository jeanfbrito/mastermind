package format

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseValidFull(t *testing.T) {
	data := readFixture(t, "valid_full.md")

	entry, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	md := entry.Metadata
	if md.Date != "2024-03-14" {
		t.Errorf("Date = %q, want 2024-03-14", md.Date)
	}
	if md.Project != "Rocket.Chat.Electron" {
		t.Errorf("Project = %q, want Rocket.Chat.Electron", md.Project)
	}
	wantTags := []string{"electron", "ipc", "macos", "debugging"}
	if !reflect.DeepEqual(md.Tags, wantTags) {
		t.Errorf("Tags = %v, want %v", md.Tags, wantTags)
	}
	if md.Kind != KindLesson {
		t.Errorf("Kind = %q, want %q", md.Kind, KindLesson)
	}
	if md.Scope != ScopeUserPersonal {
		t.Errorf("Scope = %q, want %q", md.Scope, ScopeUserPersonal)
	}
	if md.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q, want %q", md.Confidence, ConfidenceHigh)
	}

	if !strings.Contains(entry.Body, "# macOS Electron IPC hangs") {
		t.Errorf("Body missing expected heading; got: %q", entry.Body)
	}

	// The full fixture has nothing to validate against.
	if errs := entry.Validate(); len(errs) != 0 {
		t.Errorf("Validate: expected no errors, got %v", errs)
	}
}

func TestParseValidMinimal(t *testing.T) {
	data := readFixture(t, "valid_minimal.md")

	entry, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Before Normalize, optional fields should be their zero values.
	if entry.Metadata.Confidence != "" {
		t.Errorf("pre-Normalize Confidence = %q, want empty", entry.Metadata.Confidence)
	}
	if entry.Metadata.Tags != nil {
		t.Errorf("pre-Normalize Tags = %v, want nil", entry.Metadata.Tags)
	}

	// After Normalize, defaults should be present.
	entry.Normalize()
	if entry.Metadata.Confidence != ConfidenceHigh {
		t.Errorf("post-Normalize Confidence = %q, want %q", entry.Metadata.Confidence, ConfidenceHigh)
	}
	if entry.Metadata.Tags == nil {
		t.Errorf("post-Normalize Tags is nil, want empty slice")
	}

	if errs := entry.Validate(); len(errs) != 0 {
		t.Errorf("Validate: expected no errors, got %v", errs)
	}
}

func TestValidateMissingRequired(t *testing.T) {
	data := readFixture(t, "invalid_missing_required.md")

	entry, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	errs := entry.Validate()
	// Expect four errors: date, project, topic, kind.
	if len(errs) != 4 {
		t.Fatalf("Validate: got %d errors, want 4; errors: %v", len(errs), errs)
	}

	joined := joinErrs(errs)
	for _, want := range []string{"date", "project", "topic", "kind"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Validate errors missing mention of %q; errors: %s", want, joined)
		}
	}
}

func TestValidateBadKind(t *testing.T) {
	data := readFixture(t, "invalid_bad_kind.md")

	entry, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	errs := entry.Validate()
	if len(errs) != 1 {
		t.Fatalf("Validate: got %d errors, want 1; errors: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "kind") || !strings.Contains(errs[0].Error(), "lessson") {
		t.Errorf("Validate error should mention kind and the bad value; got: %s", errs[0])
	}
}

func TestParseNoFrontmatterFails(t *testing.T) {
	data := readFixture(t, "no_frontmatter.md")

	_, err := Parse(data)
	if err == nil {
		t.Fatal("Parse: expected error for file without frontmatter, got nil")
	}
	if !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("error should mention frontmatter; got: %v", err)
	}
}

func TestParseRejectsMissingClosingDelim(t *testing.T) {
	// Opening delim but never closed.
	data := []byte("---\ndate: 2026-04-04\nproject: x\ntopic: oops\nkind: lesson\n")

	_, err := Parse(data)
	if err == nil {
		t.Fatal("Parse: expected error for missing closing delim, got nil")
	}
	if !strings.Contains(err.Error(), "closing") {
		t.Errorf("error should mention closing delimiter; got: %v", err)
	}
}

func TestRoundTripStable(t *testing.T) {
	// Start from a fully-specified entry so Normalize adds nothing.
	original := &Entry{
		Metadata: Metadata{
			Date:       "2026-04-04",
			Project:    "mastermind",
			Tags:       []string{"format", "roundtrip"},
			Topic:      "Round-trip must be deterministic",
			Kind:       KindPattern,
			Scope:      ScopeUserPersonal,
			Confidence: ConfidenceHigh,
		},
		Body: "Body content that should survive the trip.",
	}

	// First serialization.
	first, err := original.MarshalMarkdown()
	if err != nil {
		t.Fatalf("first MarshalMarkdown: %v", err)
	}

	// Parse it back.
	parsed, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	parsed.Normalize()

	// Serialize again.
	second, err := parsed.MarshalMarkdown()
	if err != nil {
		t.Fatalf("second MarshalMarkdown: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("round-trip not stable\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if !reflect.DeepEqual(parsed.Metadata, original.Metadata) {
		t.Errorf("metadata changed through round trip\noriginal: %+v\nparsed:   %+v", original.Metadata, parsed.Metadata)
	}
	if parsed.Body != original.Body {
		t.Errorf("body changed through round trip\noriginal: %q\nparsed:   %q", original.Body, parsed.Body)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	e := &Entry{
		Metadata: Metadata{
			Date:    "2026-04-04",
			Project: "mastermind",
			Topic:   "idempotent",
			Kind:    KindLesson,
		},
	}
	e.Normalize()
	first := e.Metadata
	e.Normalize()
	if !reflect.DeepEqual(first, e.Metadata) {
		t.Errorf("Normalize not idempotent\nfirst:  %+v\nsecond: %+v", first, e.Metadata)
	}
}

func TestAllKindsValid(t *testing.T) {
	for _, k := range AllKinds() {
		if !k.Valid() {
			t.Errorf("Kind %q reported as invalid but is in AllKinds()", k)
		}
	}
}

func TestAllScopesValid(t *testing.T) {
	for _, s := range AllScopes() {
		if !s.Valid() {
			t.Errorf("Scope %q reported as invalid but is in AllScopes()", s)
		}
	}
	// Empty scope is deliberately valid.
	if !Scope("").Valid() {
		t.Error("empty Scope should be valid")
	}
	if Scope("bogus").Valid() {
		t.Error("bogus Scope should be invalid")
	}
}

func TestInvalidDateFormatFails(t *testing.T) {
	e := &Entry{
		Metadata: Metadata{
			Date:    "April 4 2026", // not ISO
			Project: "mastermind",
			Topic:   "bad date",
			Kind:    KindLesson,
		},
	}
	errs := e.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for non-ISO date")
	}
	if !strings.Contains(joinErrs(errs), "date") {
		t.Errorf("expected date error; got %v", errs)
	}
}

func joinErrs(errs []error) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, " | ")
}
