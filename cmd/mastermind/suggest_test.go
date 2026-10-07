package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

// ─── scanTopicDir ─────────────────────────────────────────────────────

func writeEntry(t *testing.T, path, topic, project string) {
	t.Helper()
	body := "---\ntopic: \"" + topic + "\"\nkind: lesson\n"
	if project != "" {
		body += "project: " + project + "\n"
	}
	body += "---\n\nBody.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanTopicDir_CountsMdEntries(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, filepath.Join(dir, "entry1.md"), "One", "")
	writeEntry(t, filepath.Join(dir, "entry2.md"), "Two", "")
	os.WriteFile(filepath.Join(dir, "not-md.txt"), []byte("skip"), 0o644)

	if got, _ := scanTopicDir(dir, "test"); got != 2 {
		t.Errorf("scanTopicDir count = %d, want 2", got)
	}
}

func TestScanTopicDir_CountsSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	writeEntry(t, filepath.Join(dir, "top.md"), "Top", "")
	writeEntry(t, filepath.Join(sub, "nested.md"), "Nested", "")

	if got, _ := scanTopicDir(dir, "test"); got != 2 {
		t.Errorf("scanTopicDir with subdirs = %d, want 2", got)
	}
}

func TestScanTopicDir_MissingOrEmptyDir(t *testing.T) {
	for _, dir := range []string{"/nonexistent/dir/that/does/not/exist", t.TempDir()} {
		if n, topic := scanTopicDir(dir, "test"); n != 0 || topic != "" {
			t.Errorf("scanTopicDir(%s) = %d %q, want 0 \"\"", dir, n, topic)
		}
	}
}

func TestScanTopicDir_ReturnsTopicFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, filepath.Join(dir, "dompurify-allowlist.md"), "Always check DOMPurify default allowlist", "test")

	if _, got := scanTopicDir(dir, "test"); got != "Always check DOMPurify default allowlist" {
		t.Errorf("scanTopicDir topic = %q, want 'Always check DOMPurify default allowlist'", got)
	}
}

func TestScanTopicDir_ReturnsMostRecentByModTime(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.md")
	newPath := filepath.Join(dir, "new.md")
	writeEntry(t, oldPath, "Old entry", "")
	writeEntry(t, newPath, "New entry", "")
	past := time.Now().Add(-time.Hour)
	os.Chtimes(oldPath, past, past)

	if _, got := scanTopicDir(dir, "test"); got != "New entry" {
		t.Errorf("scanTopicDir topic = %q, want 'New entry'", got)
	}
}

func TestScanTopicDir_SkipsBadFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "broken.md"), []byte("no frontmatter here"), 0o644)

	if n, topic := scanTopicDir(dir, "test"); n != 0 || topic != "" {
		t.Errorf("scanTopicDir(bad frontmatter) = %d %q, want 0 \"\"", n, topic)
	}
}

// ─── project scoping ──────────────────────────────────────────────────

func TestProjectRelativePath(t *testing.T) {
	root := t.TempDir() // not a git repo: cwd is the root
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, file, want string
	}{
		{"inside", filepath.Join(root, "src", "main.ts"), filepath.Join("src", "main.ts")},
		{"outside", filepath.Join(filepath.Dir(root), "other", "x.go"), ""},
		{"relative path", "src/main.ts", ""},
	}
	for _, c := range cases {
		if got := projectRelativePath(root, c.file); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The parent directory of a checkout (for example ~/Github) must not
// become a keyword: it matched the "github" topic on every Read.
func TestSuggestKeywordsIgnoreDirsAboveProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Github", "Rocket.Chat.Electron")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	kws := extractPathKeywords(projectRelativePath(root, filepath.Join(root, "src", "main.ts")))
	for _, k := range kws {
		if k == "github" || k == "rocket.chat.electron" {
			t.Fatalf("keyword %q comes from above the project root: %v", k, kws)
		}
	}
}

func TestScanTopicDirFiltersByProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "search")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, project string) {
		body := "---\ndate: \"2026-10-07\"\nproject: " + project + "\ntopic: " + name + "\nkind: lesson\nscope: user-personal\nconfidence: high\n---\n\nBody.\n"
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("general-entry", "general")
	write("mine", "mastermind")
	write("other-repo", "rocket-cli")

	if n, _ := scanTopicDir(dir, "mastermind"); n != 2 {
		t.Errorf("mastermind: got %d entries, want 2 (general + own)", n)
	}
	n, topic := scanTopicDir(dir, "rocket.chat.electron")
	if n != 1 || topic != "general-entry" {
		t.Errorf("unrelated project: got %d %q, want 1 \"general-entry\"", n, topic)
	}
}
