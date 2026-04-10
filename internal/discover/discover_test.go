package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// ─── parseResponse ────────────────────────────────────────────────────

func TestParseResponse_ValidJSON(t *testing.T) {
	raw := `[{"topic":"Always use sync.Map for concurrent access","kind":"lesson","body":"The shared config map had race conditions.","tags":["go","concurrency"],"category":"go","source":"abc1234 — Fix race in config map"}]`
	entries, err := parseResponse(raw, "myproject", nil)
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
	if !strings.Contains(e.Body, "## Source") {
		t.Error("body missing ## Source section")
	}
	if !strings.Contains(e.Body, "abc1234") {
		t.Error("body missing source hash")
	}
}

func TestParseResponse_CodeFences(t *testing.T) {
	raw := "```json\n" + `[{"topic":"Use transactions for batch writes","kind":"pattern","body":"All batch ops use tx.","tags":["db"],"category":"database","source":"internal/store/store.go"}]` + "\n```"
	entries, err := parseResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}

func TestParseResponse_InvalidKindDefaultsToLesson(t *testing.T) {
	raw := `[{"topic":"Something worth noting about the architecture","kind":"bogus","body":"Details.","tags":[],"category":"arch","source":"def5678"}]`
	entries, err := parseResponse(raw, "test", nil)
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

func TestParseResponse_SkipsEmptyTopicOrBody(t *testing.T) {
	raw := `[{"topic":"","kind":"lesson","body":"has body","tags":[],"category":"x","source":"a"},{"topic":"has topic","kind":"lesson","body":"","tags":[],"category":"x","source":"b"}]`
	entries, err := parseResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (empty topic/body skipped)", len(entries))
	}
}

func TestParseResponse_DedupAgainstExisting(t *testing.T) {
	raw := `[{"topic":"Fix migration rollback","kind":"lesson","body":"Details.","tags":[],"category":"db","source":"abc1234"}]`
	existing := []string{"fix migration rollback"}
	entries, err := parseResponse(raw, "test", existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (dedup)", len(entries))
	}
}

func TestParseResponse_DedupWithinBatch(t *testing.T) {
	raw := `[
		{"topic":"Use mutex for maps","kind":"lesson","body":"Body 1.","tags":[],"category":"go","source":"aaa1111"},
		{"topic":"Use mutex for maps","kind":"lesson","body":"Body 2.","tags":[],"category":"go","source":"bbb2222"}
	]`
	entries, err := parseResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (within-batch dedup)", len(entries))
	}
}

func TestParseResponse_EmptyArray(t *testing.T) {
	entries, err := parseResponse("[]", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := parseResponse("not json", "test", nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseResponse_DefaultProject(t *testing.T) {
	raw := `[{"topic":"Some general engineering insight here","kind":"insight","body":"Details.","tags":[],"category":"general","source":"xyz"}]`
	entries, err := parseResponse(raw, "", nil)
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

func TestParseResponse_NoSourceOmitsSection(t *testing.T) {
	raw := `[{"topic":"Entry without source provenance info","kind":"lesson","body":"Some body text.","tags":[],"category":"misc"}]`
	entries, err := parseResponse(raw, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if strings.Contains(entries[0].Body, "## Source") {
		t.Error("body should not have ## Source when source is empty")
	}
}

// ─── collectKnownHashes ──────────────────────────────────────────────

func TestCollectKnownHashes_FindsHashesInSource(t *testing.T) {
	dir := t.TempDir()
	entryDir := filepath.Join(dir, "topic")
	os.MkdirAll(entryDir, 0o755)

	entry := `---
topic: "Test entry"
kind: lesson
---

Some body.

## Source
abc1234 — Fix the thing
def5678 — Another commit
`
	os.WriteFile(filepath.Join(entryDir, "test.md"), []byte(entry), 0o644)

	// Create a minimal discoverer with the dir as a scope root.
	d := &Discoverer{
		store: newTestStore(t, dir),
	}

	hashes := d.collectKnownHashes()
	if !hashes["abc1234"] {
		t.Error("missing hash abc1234")
	}
	if !hashes["def5678"] {
		t.Error("missing hash def5678")
	}
}

func TestCollectKnownHashes_IgnoresNonSourceHashes(t *testing.T) {
	dir := t.TempDir()
	entry := `---
topic: "Test entry"
kind: lesson
---

The hash abc1234 appears in body but not in Source section.
`
	os.WriteFile(filepath.Join(dir, "test.md"), []byte(entry), 0o644)

	d := &Discoverer{
		store: newTestStore(t, dir),
	}

	hashes := d.collectKnownHashes()
	if hashes["abc1234"] {
		t.Error("should NOT find hash outside ## Source section")
	}
}

func TestCollectKnownHashes_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	d := &Discoverer{
		store: newTestStore(t, dir),
	}
	hashes := d.collectKnownHashes()
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes, got %d", len(hashes))
	}
}

// ─── isDuplicate ──────────────────────────────────────────────────────

func TestIsDuplicate(t *testing.T) {
	existing := map[string]bool{
		"fix migration rollback": true,
	}
	if !isDuplicate("fix migration rollback in staging", existing) {
		t.Error("candidate containing existing should be duplicate")
	}
	if !isDuplicate("migration", existing) {
		t.Error("existing containing candidate should be duplicate")
	}
	if isDuplicate("goroutine leak", existing) {
		t.Error("unrelated should not be duplicate")
	}
}

// ─── findPackages ─────────────────────────────────────────────────────

func TestFindPackages_FindsGoPackages(t *testing.T) {
	dir := t.TempDir()
	pkg1 := filepath.Join(dir, "cmd", "app")
	pkg2 := filepath.Join(dir, "internal", "store")
	os.MkdirAll(pkg1, 0o755)
	os.MkdirAll(pkg2, 0o755)
	os.WriteFile(filepath.Join(pkg1, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(pkg2, "store.go"), []byte("package store"), 0o644)

	d := &Discoverer{cfg: Config{Cwd: dir}}
	packages := d.findPackages()

	if len(packages) != 2 {
		t.Fatalf("found %d packages, want 2: %v", len(packages), packages)
	}
}

func TestFindPackages_SkipsVendor(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vendor", "lib"), 0o755)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "lib", "lib.go"), []byte("package lib"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package app"), 0o644)

	d := &Discoverer{cfg: Config{Cwd: dir}}
	packages := d.findPackages()

	for _, p := range packages {
		if strings.Contains(p, "vendor") {
			t.Errorf("found vendor package: %s", p)
		}
	}
}

func TestFindPackages_SkipsTestFiles(t *testing.T) {
	dir := t.TempDir()
	// Directory with only test files should not be found.
	testOnly := filepath.Join(dir, "testpkg")
	os.MkdirAll(testOnly, 0o755)
	os.WriteFile(filepath.Join(testOnly, "foo_test.go"), []byte("package testpkg"), 0o644)

	d := &Discoverer{cfg: Config{Cwd: dir}}
	packages := d.findPackages()

	if len(packages) != 0 {
		t.Errorf("found %d packages from test-only dir, want 0: %v", len(packages), packages)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────

func newTestStore(t *testing.T, root string) *store.Store {
	t.Helper()
	return store.New(store.Config{
		UserPersonalRoot: root,
	})
}
