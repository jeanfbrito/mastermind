package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"empty", "", ""},
		{"ssh with .git", "git@github.com:jeanfbrito/mastermind.git", "mastermind"},
		{"ssh without .git", "git@github.com:jeanfbrito/mastermind", "mastermind"},
		{"https with .git", "https://github.com/jeanfbrito/mastermind.git", "mastermind"},
		{"https without .git", "https://github.com/jeanfbrito/mastermind", "mastermind"},
		{"https trailing slash", "https://github.com/jeanfbrito/mastermind/", "mastermind"},
		{"ssh with org dots", "git@github.com:Gentleman-Programming/engram.git", "engram"},
		{"gitlab nested path", "git@gitlab.com:org/sub/repo.git", "repo"},
		{"https nested path", "https://gitlab.com/org/sub/repo", "repo"},
		{"just a name no separator", "mastermind", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractRepoName(tc.url)
			if got != tc.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"mastermind", "mastermind"},
		{"MasterMind", "mastermind"},
		{"  trimmed  ", "trimmed"},
		{"", Unknown},
		{"   ", Unknown},
		{"Rocket.Chat.Electron", "rocket.chat.electron"},
	}

	for _, tc := range tests {
		got := normalize(tc.in)
		if got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectEmptyDir(t *testing.T) {
	if got := Detect(""); got != Unknown {
		t.Errorf("Detect(\"\") = %q, want %q", got, Unknown)
	}
}

func TestDetectNonexistentDirFallsBackToBasename(t *testing.T) {
	// Detect should not blow up on a non-existent path. git will fail,
	// basename of the path remains — that's what we return.
	got := Detect("/nonexistent/path/to/coolproject")
	if got != "coolproject" {
		t.Errorf("Detect nonexistent path = %q, want coolproject", got)
	}
}

func TestDetectLeadingDashIsEscaped(t *testing.T) {
	// A dir starting with "-" must not be treated as a git flag. We
	// can't directly observe the escape, but we can verify Detect
	// doesn't panic or return Unknown.
	got := Detect("-trick")
	// After ./ prefix, basename is "-trick" → normalized to "-trick".
	if got != "-trick" {
		t.Errorf("Detect(%q) = %q, want %q", "-trick", got, "-trick")
	}
}

// needGit skips the calling test if git isn't on PATH.
func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH; skipping integration test")
	}
}

// initGitRepo creates an empty git repo at dir. Fails the test on error.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "Test User")
}

func TestDetectFromGitRoot(t *testing.T) {
	needGit(t)

	// Create a temp dir, init git (no remote), then verify Detect
	// returns the directory basename via the git-root path.
	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "coolproject")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	// Also test from a subdirectory — the git rev-parse --show-toplevel
	// should still find the root and return "coolproject".
	subDir := filepath.Join(repoDir, "subdir", "deeper")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := Detect(subDir)
	if got != "coolproject" {
		t.Errorf("Detect from subdir = %q, want coolproject", got)
	}
}

func TestDetectFromGitRemote(t *testing.T) {
	needGit(t)

	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "local-name-ignored")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	// Add a fake remote. Detect should prefer the remote name
	// over the directory name.
	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin",
		"git@github.com:jeanfbrito/mastermind.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	got := Detect(repoDir)
	if got != "mastermind" {
		t.Errorf("Detect with remote = %q, want mastermind (directory was %q)", got, "local-name-ignored")
	}
}

func TestDetectFromCwdBasenameNoGit(t *testing.T) {
	// A directory that exists but is NOT a git repo. Detect should
	// fall through to the basename.
	tmp := t.TempDir()
	plainDir := filepath.Join(tmp, "plainproject")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := Detect(plainDir)
	if got != "plainproject" {
		t.Errorf("Detect non-git dir = %q, want plainproject", got)
	}
}

// TestDetectFromGitReturnsEmptyWithoutGit is the critical invariant
// that DetectFromGit does NOT fall back to the cwd basename. A
// non-git directory must return empty string so callers like the
// project-personal scope wire-up can gate on "real project" without
// creating garbage directories.
func TestDetectFromGitReturnsEmptyWithoutGit(t *testing.T) {
	tmp := t.TempDir()
	plainDir := filepath.Join(tmp, "not-a-repo")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := DetectFromGit(plainDir)
	if got != "" {
		t.Errorf("DetectFromGit non-git dir = %q, want empty string", got)
	}
}

// TestDetectFromGitUsesRemoteWhenPresent mirrors TestDetectFromGitRemote
// but through the git-gated entry point. A repo with a remote should
// produce the repo name from the remote URL.
func TestDetectFromGitUsesRemoteWhenPresent(t *testing.T) {
	needGit(t)

	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "local-name-ignored")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin",
		"git@github.com:jeanfbrito/mastermind.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	got := DetectFromGit(repoDir)
	if got != "mastermind" {
		t.Errorf("DetectFromGit with remote = %q, want mastermind", got)
	}
}

// TestDetectFromGitUsesRootBasenameWithoutRemote covers the second
// detection tier: a git repo with no origin remote should still
// produce a name via `git rev-parse --show-toplevel`.
func TestDetectFromGitUsesRootBasenameWithoutRemote(t *testing.T) {
	needGit(t)

	tmp := t.TempDir()
	repoDir := filepath.Join(tmp, "sologitrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	got := DetectFromGit(repoDir)
	if got != "sologitrepo" {
		t.Errorf("DetectFromGit git root no remote = %q, want sologitrepo", got)
	}
}

func TestDetectNormalizesResult(t *testing.T) {
	// Verify the output is always lowercase/trimmed, regardless of
	// which detection tier wins.
	tmp := t.TempDir()
	mixedDir := filepath.Join(tmp, "MixedCase_Project")
	if err := os.MkdirAll(mixedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got := Detect(mixedDir)
	if got != strings.ToLower("MixedCase_Project") {
		t.Errorf("Detect = %q, want lowercase", got)
	}
}
