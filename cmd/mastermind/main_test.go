package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// needGit skips the caller if git isn't on PATH. Kept local to this
// test file rather than factored out of the project package because
// cmd/mastermind should not depend on test helpers from its own deps.
func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH; skipping integration test")
	}
}

// initGitRepo initialises an empty git repo at dir. Any failure fails
// the test immediately.
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

// withFakeHome points $HOME at a fresh tempdir for the duration of
// the test. buildSessionConfig's call to os.UserHomeDir reads $HOME
// on Unix, so this is how we keep tests hermetic without touching
// the real home directory.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// TestBuildSessionConfigUserPersonalOnly exercises the baseline case:
// cwd is a plain directory with no git and no .knowledge/ anywhere in its
// ancestors. UserPersonalRoot must be populated from $HOME, the
// other two scopes must be empty (disabled silently).
func TestBuildSessionConfigUserPersonalOnly(t *testing.T) {
	home := withFakeHome(t)

	cwd := t.TempDir()

	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		t.Fatalf("buildSessionConfig: %v", err)
	}

	wantUser := filepath.Join(home, ".knowledge")
	if cfg.UserPersonalRoot != wantUser {
		t.Errorf("UserPersonalRoot = %q, want %q", cfg.UserPersonalRoot, wantUser)
	}
	if cfg.ProjectSharedRoot != "" {
		t.Errorf("ProjectSharedRoot = %q, want empty (no .knowledge/ in tree)", cfg.ProjectSharedRoot)
	}
	if cfg.ProjectPersonalRoot != "" {
		t.Errorf("ProjectPersonalRoot = %q, want empty (no git in tree)", cfg.ProjectPersonalRoot)
	}
}

// TestBuildSessionConfigProjectSharedWhenMmExists covers the case
// where cwd is inside a repo with a .knowledge/ directory. ProjectSharedRoot
// must point at that .knowledge/. ProjectPersonalRoot stays empty because
// the directory is not a git repo.
func TestBuildSessionConfigProjectSharedWhenMmExists(t *testing.T) {
	withFakeHome(t)

	repoRoot := t.TempDir()
	mmDir := filepath.Join(repoRoot, ".knowledge")
	if err := os.MkdirAll(mmDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := buildSessionConfig(repoRoot)
	if err != nil {
		t.Fatalf("buildSessionConfig: %v", err)
	}

	if cfg.ProjectSharedRoot != mmDir {
		t.Errorf("ProjectSharedRoot = %q, want %q", cfg.ProjectSharedRoot, mmDir)
	}
	if cfg.ProjectPersonalRoot != "" {
		t.Errorf("ProjectPersonalRoot = %q, want empty (no git)", cfg.ProjectPersonalRoot)
	}
}

// TestBuildSessionConfigProjectPersonalWhenInGitRepo is the key
// regression test for the project-personal wiring: a git repo with no
// remote should map to ~/.claude/projects/<slug>/memory where <slug>
// is the working-tree basename, not a dash-encoded path.
func TestBuildSessionConfigProjectPersonalWhenInGitRepo(t *testing.T) {
	needGit(t)
	home := withFakeHome(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "acme-service")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	cfg, err := buildSessionConfig(repoDir)
	if err != nil {
		t.Fatalf("buildSessionConfig: %v", err)
	}

	wantPersonal := filepath.Join(home, ".claude", "projects", "acme-service", "memory")
	if cfg.ProjectPersonalRoot != wantPersonal {
		t.Errorf("ProjectPersonalRoot = %q, want %q", cfg.ProjectPersonalRoot, wantPersonal)
	}
}

// TestBuildSessionConfigProjectPersonalPrefersRemoteOverBasename
// locks the invariant that when a git remote is present, the slug
// comes from the remote (stable across clone locations), not from
// the working-tree basename. This is the cross-machine continuity
// guarantee — the whole reason we picked slug over dash-encoded cwd.
func TestBuildSessionConfigProjectPersonalPrefersRemoteOverBasename(t *testing.T) {
	needGit(t)
	home := withFakeHome(t)

	parent := t.TempDir()
	// Deliberately different local dir name so the test fails loudly
	// if buildSessionConfig ever falls back to the basename when a
	// remote is available.
	repoDir := filepath.Join(parent, "local-clone-path-that-differs")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin",
		"git@github.com:jeanfbrito/mastermind.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}

	cfg, err := buildSessionConfig(repoDir)
	if err != nil {
		t.Fatalf("buildSessionConfig: %v", err)
	}

	wantPersonal := filepath.Join(home, ".claude", "projects", "mastermind", "memory")
	if cfg.ProjectPersonalRoot != wantPersonal {
		t.Errorf("ProjectPersonalRoot = %q, want %q (remote name should win over basename %q)",
			cfg.ProjectPersonalRoot, wantPersonal, "local-clone-path-that-differs")
	}
}

// TestBuildSessionConfigAllThreeScopesWhenRepoHasMmAndGit covers the
// happy-path for a fully-configured session: a git repo that also has
// a .knowledge/ directory. All three scope roots must be populated.
func TestBuildSessionConfigAllThreeScopesWhenRepoHasMmAndGit(t *testing.T) {
	needGit(t)
	home := withFakeHome(t)

	parent := t.TempDir()
	repoDir := filepath.Join(parent, "fullproject")
	if err := os.MkdirAll(filepath.Join(repoDir, ".knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoDir)

	cfg, err := buildSessionConfig(repoDir)
	if err != nil {
		t.Fatalf("buildSessionConfig: %v", err)
	}

	if cfg.UserPersonalRoot != filepath.Join(home, ".knowledge") {
		t.Errorf("UserPersonalRoot = %q, want %q", cfg.UserPersonalRoot, filepath.Join(home, ".knowledge"))
	}
	if cfg.ProjectSharedRoot != filepath.Join(repoDir, ".knowledge") {
		t.Errorf("ProjectSharedRoot = %q, want %q", cfg.ProjectSharedRoot, filepath.Join(repoDir, ".knowledge"))
	}
	if cfg.ProjectPersonalRoot != filepath.Join(home, ".claude", "projects", "fullproject", "memory") {
		t.Errorf("ProjectPersonalRoot = %q, want ~/.claude/projects/fullproject/memory", cfg.ProjectPersonalRoot)
	}
}
