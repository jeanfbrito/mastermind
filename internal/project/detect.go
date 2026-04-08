package project

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Unknown is the project name returned when detection produces no usable
// signal (empty dir, non-existent path, no git, no basename).
const Unknown = "unknown"

// Detect resolves a canonical project name for the given directory.
//
// Priority order:
//  1. git remote origin repo name (e.g., "mastermind" from
//     "git@github.com:jeanfbrito/mastermind.git")
//  2. git root basename (the directory name at the top of the working tree)
//  3. cwd basename (the last path component of dir)
//
// The returned name is always non-empty and normalized (lowercase, trimmed).
// Returns "unknown" only when every signal fails.
//
// This is the canonical "what project am I in right now?" function used by
// session-start injection, session-close extraction, and project-scoped
// queries. Every component that needs a project name should call Detect
// so the answer is consistent across the system.
//
// The algorithm is adapted from engram's internal/project/detect.go.
func Detect(dir string) string {
	if dir == "" {
		return Unknown
	}

	// Guard against argument injection: a dir starting with "-" would be
	// interpreted as a git flag when passed to `git -C <dir>`. Prepend ./
	// to force it to be read as a path.
	if strings.HasPrefix(dir, "-") {
		dir = "./" + dir
	}

	if name := detectFromGitRemote(dir); name != "" {
		return normalize(name)
	}
	if name := detectFromGitRoot(dir); name != "" {
		return normalize(name)
	}

	base := filepath.Base(dir)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return Unknown
	}
	return normalize(base)
}

// DetectFromGit resolves a project name using git signals only: git
// remote origin first, git working-tree root basename second. Returns
// an empty string (not Unknown) if dir is not inside a git repository.
//
// Unlike Detect, this does NOT fall back to the cwd basename. Callers
// that need to gate behavior on "is this dir part of a real project"
// — such as mastermind's project-personal scope wire-up, which must
// not create garbage directories under ~/.claude/projects for every
// tmpdir the binary is spawned in — should use DetectFromGit. Callers
// that always want SOME name (for display, for project-scoped search
// output) should use Detect.
//
// The returned name is normalized the same way Detect normalizes:
// lowercase, trimmed.
func DetectFromGit(dir string) string {
	if dir == "" {
		return ""
	}
	if strings.HasPrefix(dir, "-") {
		dir = "./" + dir
	}
	if name := detectFromGitRemote(dir); name != "" {
		return normalize(name)
	}
	if name := detectFromGitRoot(dir); name != "" {
		return normalize(name)
	}
	return ""
}

// normalize applies canonical project name rules: lowercase + trim.
// Empty results collapse to Unknown so callers never have to null-check.
func normalize(name string) string {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		return Unknown
	}
	return n
}

// detectFromGitRemote runs `git -C <dir> remote get-url origin` and extracts
// the repo name from whatever URL form git returns (SSH or HTTPS, with or
// without .git suffix, with or without trailing slash).
//
// Returns empty string on any failure — not in a git repo, no origin remote,
// git binary missing, command timeout. Callers fall through to the next
// detection step.
func detectFromGitRemote(dir string) string {
	out, err := runGit(dir, 2*time.Second, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	return extractRepoName(strings.TrimSpace(out))
}

// detectFromGitRoot runs `git -C <dir> rev-parse --show-toplevel` and
// returns the basename of the working tree root. This handles the case
// where we're in a subdirectory of a git repo that has no origin remote
// (e.g., a fresh `git init`) — we still want the project name.
func detectFromGitRoot(dir string) string {
	out, err := runGit(dir, 2*time.Second, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

// extractRepoName pulls the repo name out of a git remote URL.
//
// Handles:
//   - "git@github.com:owner/repo.git"       → "repo"
//   - "git@github.com:owner/repo"           → "repo"
//   - "https://github.com/owner/repo.git"   → "repo"
//   - "https://github.com/owner/repo"       → "repo"
//   - "https://github.com/owner/repo/"      → "repo"
//   - "git@gitlab.com:org/sub/repo.git"     → "repo"
//
// Returns empty string on unparseable input.
func extractRepoName(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")

	// Find the last path-like separator. SSH URLs use ":" before the path,
	// HTTPS URLs use "/". Scanning from the right for any of ":/" hits
	// both cases.
	cut := -1
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == '/' || url[i] == ':' {
			cut = i
			break
		}
	}
	if cut < 0 || cut == len(url)-1 {
		return ""
	}
	return url[cut+1:]
}

// GitRoot returns the absolute path of the git working tree root for dir,
// or empty string if dir is not inside a git repo. This is the full path
// (not just the basename) — used by callers that need to create directories
// at the repo root (e.g., auto-initializing .knowledge/).
func GitRoot(dir string) string {
	if dir == "" {
		return ""
	}
	if strings.HasPrefix(dir, "-") {
		dir = "./" + dir
	}
	out, err := runGit(dir, 2*time.Second, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// runGit executes `git -C <dir> <args...>` with a timeout and returns stdout.
// Errors (including timeout, non-zero exit, missing binary) are returned
// opaquely — callers only care whether detection succeeded, not why.
func runGit(dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
