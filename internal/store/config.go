package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// PendingPolicy controls what happens to entries that sit in pending/
// without being reviewed.
//
// The original design auto-deleted after 7 days ("no guilt queue"). That
// was reversed because auto-DELETE is the only irreversible failure mode
// in the system, and it punishes exactly the ADHD pattern mastermind is
// designed for — a user who doesn't review for 10 days loses knowledge
// silently. See DECISIONS.md "Reverse auto-expire" entry.
type PendingPolicy string

const (
	// PendingKeepForever is the default: pending entries are never
	// touched by the store. They stay until the user promotes, rejects,
	// or manually deletes them. Old entries are not shameful — they're
	// waiting for a good day.
	PendingKeepForever PendingPolicy = "keep"

	// PendingAutoPromote moves old pending entries to the live store
	// after AutoPromoteAfter has elapsed. This is the "zero-maintenance"
	// option: the knowledge is preserved, just not curated. Noise in the
	// live store is fixable; lost knowledge isn't.
	PendingAutoPromote PendingPolicy = "auto-promote"
)

// DefaultAutoPromoteAfter is the duration after which pending entries
// are auto-promoted when PendingPolicy is PendingAutoPromote.
const DefaultAutoPromoteAfter = 7 * 24 * time.Hour

// Config points the store at the three scope roots on disk. Every field
// is absolute. An empty field disables that scope entirely — useful in
// tests and during Phase 1 dogfooding, when only user-personal may exist.
type Config struct {
	// UserPersonalRoot is the root of the user-personal store.
	// In production: ~/.mm/
	// The "live" entries live directly under this directory in /lessons/.
	// Pending candidates land in /pending/. Archive tier lives in
	// /archive/<year>/<project>/ and is managed separately.
	UserPersonalRoot string

	// ProjectSharedRoot is the root of the project-shared store — the
	// repo-local .mm/ directory at a project root.
	// In production: <repo>/.mm/
	// This is per-session: the value depends on which project the user
	// is currently working in, so it's typically set by the caller after
	// calling store.FindProjectRoot(cwd).
	ProjectSharedRoot string

	// ProjectPersonalRoot is the root of the project-personal store —
	// Claude Code's auto-memory directory for the current project.
	// In production: ~/.claude/projects/<repo>/memory/
	ProjectPersonalRoot string

	// PendingBehavior controls what happens to unreviewed pending entries
	// at startup. Default (zero value ""): treated as PendingKeepForever.
	PendingBehavior PendingPolicy

	// AutoPromoteAfter is the duration after which pending entries are
	// auto-promoted to the live store. Only used when PendingBehavior is
	// PendingAutoPromote. Zero value defaults to DefaultAutoPromoteAfter.
	AutoPromoteAfter time.Duration

	// Now is the time source used by the auto-promote pass. Tests
	// override this to control timing behavior. If nil, time.Now is used.
	Now func() time.Time
}

// DefaultConfig returns a Config populated from the user's environment:
//
//   - UserPersonalRoot = ~/.mm/
//   - ProjectSharedRoot = "" (caller must set via FindProjectRoot)
//   - ProjectPersonalRoot = ~/.claude/projects/<unknown>/memory/
//     (caller should re-set with the real project name)
//
// DefaultConfig never returns an error unless $HOME can't be resolved,
// which on macOS and Linux is a machine in trouble.
func DefaultConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("store: resolve home dir: %w", err)
	}
	return Config{
		UserPersonalRoot: filepath.Join(home, ".mm"),
		// ProjectSharedRoot is left empty on purpose. It's per-session.
		ProjectPersonalRoot: "", // caller fills this after project detection
		Now:                 time.Now,
	}, nil
}

// Scope enumerates the three store locations. Internal to the store
// package — callers pass the format.Scope value from the user-facing
// API and the store maps it to its internal representation.
type scopeKind int

const (
	scopeUnknown scopeKind = iota
	scopeUser
	scopeProjectShared
	scopeProjectPersonal
)

func scopeFromFormat(s format.Scope) scopeKind {
	switch s {
	case format.ScopeUserPersonal:
		return scopeUser
	case format.ScopeProjectShared:
		return scopeProjectShared
	case format.ScopeProjectPersonal:
		return scopeProjectPersonal
	default:
		return scopeUnknown
	}
}

// rootFor returns the configured root directory for the given scope,
// or "" if the scope isn't configured. An empty return value means
// "this scope isn't available in the current session."
func (c Config) rootFor(s scopeKind) string {
	switch s {
	case scopeUser:
		return c.UserPersonalRoot
	case scopeProjectShared:
		return c.ProjectSharedRoot
	case scopeProjectPersonal:
		return c.ProjectPersonalRoot
	default:
		return ""
	}
}

// liveDir is the subdirectory under a scope root where promoted (live)
// entries live. For user-personal, this is "lessons/". For project
// scopes, it's "nodes/". The difference reflects the different roles
// of the two kinds of store: user-personal is a lessons journal,
// project-shared is a team knowledge base.
func liveDir(s scopeKind) string {
	switch s {
	case scopeUser:
		return "lessons"
	case scopeProjectShared, scopeProjectPersonal:
		return "nodes"
	default:
		return ""
	}
}

// pendingDir is "pending" for every scope. Consistency matters more
// than scope-specific naming here — the review flow walks pending/ in
// every scope the same way.
const pendingDirName = "pending"
