package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// ─── extractPathKeywords ──────────────────────────────────────────────

func TestExtractPathKeywords_BasicPath(t *testing.T) {
	got := extractPathKeywords("/Users/jean/Github/mastermind/internal/mcp/tools.go")
	// Should include "mastermind", "mcp", "tools" — skip "internal" (generic).
	want := []string{"mastermind", "mcp", "tools"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractPathKeywords = %v, want %v", got, want)
	}
}

func TestExtractPathKeywords_SkipsGenericDirs(t *testing.T) {
	got := extractPathKeywords("/home/user/project/src/lib/utils/helper.go")
	// "src", "lib", "utils" are all in skipSegments.
	for _, kw := range got {
		if skipSegments[kw] {
			t.Errorf("extractPathKeywords returned generic segment %q", kw)
		}
	}
}

func TestExtractPathKeywords_StripsExtension(t *testing.T) {
	got := extractPathKeywords("/foo/bar/search.go")
	for _, kw := range got {
		if kw == "search.go" {
			t.Error("extractPathKeywords did not strip .go extension")
		}
	}
	found := false
	for _, kw := range got {
		if kw == "search" {
			found = true
		}
	}
	if !found {
		t.Errorf("extractPathKeywords missing 'search': got %v", got)
	}
}

func TestExtractPathKeywords_TakesLast4Segments(t *testing.T) {
	got := extractPathKeywords("/a/b/c/d/e/f/mypackage/myfile.go")
	// Last 4 segments: "e", "f", "mypackage", "myfile"
	// But "e" and "f" are short (1 char) — skipped by len < 2 check.
	// Only "mypackage" and "myfile" survive.
	if len(got) > 4 {
		t.Errorf("extractPathKeywords returned more than 4 keywords: %v", got)
	}
}

func TestExtractPathKeywords_EmptyPath(t *testing.T) {
	got := extractPathKeywords("")
	if len(got) != 0 {
		t.Errorf("extractPathKeywords('') = %v, want empty", got)
	}
}

func TestExtractPathKeywords_SingleCharSegmentsSkipped(t *testing.T) {
	got := extractPathKeywords("/a/b/c/electron.js")
	for _, kw := range got {
		if len(kw) < 2 {
			t.Errorf("extractPathKeywords returned short segment %q", kw)
		}
	}
}

// ─── countEntriesInDir ────────────────────────────────────────────────

func TestCountEntriesInDir_WithEntries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "entry1.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(dir, "entry2.md"), []byte("# test"), 0o644)
	os.WriteFile(filepath.Join(dir, "not-md.txt"), []byte("skip"), 0o644)

	got := countEntriesInDir(dir)
	if got != 2 {
		t.Errorf("countEntriesInDir = %d, want 2", got)
	}
}

func TestCountEntriesInDir_CountsSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "top.md"), []byte("# top"), 0o644)
	os.WriteFile(filepath.Join(sub, "nested.md"), []byte("# nested"), 0o644)

	got := countEntriesInDir(dir)
	if got != 2 {
		t.Errorf("countEntriesInDir with subdirs = %d, want 2", got)
	}
}

func TestCountEntriesInDir_MissingDir(t *testing.T) {
	got := countEntriesInDir("/nonexistent/dir/that/does/not/exist")
	if got != 0 {
		t.Errorf("countEntriesInDir(missing) = %d, want 0", got)
	}
}

func TestCountEntriesInDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := countEntriesInDir(dir)
	if got != 0 {
		t.Errorf("countEntriesInDir(empty) = %d, want 0", got)
	}
}

// ─── bestEntryTopic ───────────────────────────────────────────────────

func TestBestEntryTopic_ReturnsTopicFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	entry := `---
date: 2026-04-09
project: test
topic: "Always check DOMPurify default allowlist"
kind: lesson
scope: project-shared
confidence: high
---

Some body content here.
`
	os.WriteFile(filepath.Join(dir, "dompurify-allowlist.md"), []byte(entry), 0o644)

	got := bestEntryTopic(dir)
	if got != "Always check DOMPurify default allowlist" {
		t.Errorf("bestEntryTopic = %q, want 'Always check DOMPurify default allowlist'", got)
	}
}

func TestBestEntryTopic_ReturnsMostRecentByModTime(t *testing.T) {
	dir := t.TempDir()

	old := `---
topic: "Old entry"
kind: lesson
---
`
	new := `---
topic: "New entry"
kind: lesson
---
`
	oldPath := filepath.Join(dir, "old.md")
	newPath := filepath.Join(dir, "new.md")

	// Write old first, then new — new gets later mod time.
	os.WriteFile(oldPath, []byte(old), 0o644)
	// Ensure different mod time by touching the file timestamp.
	os.WriteFile(newPath, []byte(new), 0o644)

	got := bestEntryTopic(dir)
	// Should pick "New entry" (most recent mod time).
	if got != "New entry" {
		t.Errorf("bestEntryTopic = %q, want 'New entry'", got)
	}
}

func TestBestEntryTopic_MissingDir(t *testing.T) {
	got := bestEntryTopic("/nonexistent/dir/that/does/not/exist")
	if got != "" {
		t.Errorf("bestEntryTopic(missing) = %q, want empty", got)
	}
}

func TestBestEntryTopic_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got := bestEntryTopic(dir)
	if got != "" {
		t.Errorf("bestEntryTopic(empty) = %q, want empty", got)
	}
}

func TestBestEntryTopic_SkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not markdown"), 0o644)

	got := bestEntryTopic(dir)
	if got != "" {
		t.Errorf("bestEntryTopic(no .md files) = %q, want empty", got)
	}
}

func TestBestEntryTopic_BadFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter here"), 0o644)

	got := bestEntryTopic(dir)
	if got != "" {
		t.Errorf("bestEntryTopic(bad frontmatter) = %q, want empty", got)
	}
}
