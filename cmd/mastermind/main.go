// Command mastermind is the ADHD cure for agents that you always
// dreamed for yourself.
//
// It runs as an MCP server over stdio plus two CLI subcommands
// (session-start, session-close) wired to Claude Code hooks. Together
// they form a continuity layer: context is surfaced automatically at
// session start, lessons are extracted automatically at session close,
// and the user's working memory is never taxed by the tool itself.
//
// See the project docs for the design:
//   - docs/CONTINUITY.md   — the load-bearing behaviors
//   - docs/ARCHITECTURE.md — module layout and MCP tool surface
//   - docs/FORMAT.md       — the entry schema (the long-term contract)
//   - docs/EXTRACTION.md   — the capture pipeline
//   - docs/ARCHIVE.md      — working set vs lifelong archive
//   - docs/DECISIONS.md    — the why behind every architectural choice
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/jeanfbrito/mastermind/internal/mcp"
	"github.com/jeanfbrito/mastermind/internal/project"
	"github.com/jeanfbrito/mastermind/internal/search"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// version is set at build time via -ldflags "-X main.version=..."
// Falls back to debug.ReadBuildInfo() for `go install` builds, then to
// "dev" as a last resort. Pattern borrowed from engram's main.go.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("mastermind %s\n", version)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "session-start":
			// TODO(phase3c): implement in CONTINUITY.md phase 3c.
			fmt.Fprintln(os.Stderr, "mastermind session-start: not implemented yet — see docs/CONTINUITY.md and docs/ROADMAP.md Phase 3c")
			os.Exit(1)
		case "session-close":
			// TODO(phase3b): implement in CONTINUITY.md phase 3b.
			fmt.Fprintln(os.Stderr, "mastermind session-close: not implemented yet — see docs/EXTRACTION.md and docs/ROADMAP.md Phase 3b")
			os.Exit(1)
		case "mcp":
			// Explicit MCP mode (matches engram's convention: `engram mcp`
			// to start the stdio server). Fall through to default.
		default:
			fmt.Fprintf(os.Stderr, "mastermind: unknown command %q\n\n", os.Args[1])
			printHelp()
			os.Exit(2)
		}
	}

	// Default: start the MCP server over stdio. This is the mode
	// Claude Code spawns mastermind in.
	if err := runMCPServer(); err != nil {
		fmt.Fprintf(os.Stderr, "mastermind: %s\n", err)
		os.Exit(1)
	}
}

// buildSessionConfig constructs a store.Config with all three scope
// roots populated for the current session:
//
//   - UserPersonalRoot: ~/.mm (from store.DefaultConfig, which resolves
//     $HOME).
//   - ProjectSharedRoot: <root>/.mm when walking upward from cwd finds
//     a .mm/ directory. Left empty otherwise — the scope disables
//     silently rather than creating a new .mm/ the user never asked for.
//   - ProjectPersonalRoot: ~/.claude/projects/<slug>/memory when cwd is
//     inside a git repository. The slug comes from project.DetectFromGit,
//     which reads the origin remote first and falls back to the git
//     working-tree basename. If cwd is NOT inside a git repo (or
//     git is unavailable), the scope is left empty — this is a
//     deliberate guard against spawning garbage directories under
//     ~/.claude/projects/<random-tmpdir-name>/ every time the binary
//     is run from a non-project cwd.
//
// The chosen naming convention for project-personal — slug, not
// dash-encoded cwd — means two clones of the same project on two
// machines (e.g., ~/Github/mastermind and ~/code/mastermind) map to
// the same directory and the entries merge cleanly on sync. This is
// load-bearing for the cross-machine memory story. See the promoted
// pattern entry .mm/nodes/store-defaultconfig-returns-a-skeleton-...md
// and the closed open-loop that originally flagged this design call.
//
// Escape hatch for the edge case where a slug collision is unwanted
// (two unrelated projects that normalize to the same name): a future
// MASTERMIND_PROJECT_DIR env var can override this path. Not
// implemented yet — add it when a real collision surfaces, not before.
func buildSessionConfig(cwd string) (store.Config, error) {
	cfg, err := store.DefaultConfig()
	if err != nil {
		return store.Config{}, err
	}

	if root := store.FindProjectRoot(cwd); root != "" {
		cfg.ProjectSharedRoot = filepath.Join(root, ".mm")
	}

	if slug := project.DetectFromGit(cwd); slug != "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.ProjectPersonalRoot = filepath.Join(home, ".claude", "projects", slug, "memory")
		}
	}

	return cfg, nil
}

// runMCPServer boots the three-scope store, wires up the searcher and
// the MCP server, and runs until the client disconnects or a signal
// arrives. Returns any error that escapes the SDK run loop.
func runMCPServer() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return fmt.Errorf("build session config: %w", err)
	}

	s := store.New(cfg)

	// Silent pending auto-expire: the first thing any session does is
	// drop stale candidates older than PendingTTL. No log, no count, no
	// user-visible output — see CONTINUITY.md on "silent unless needed".
	_, _ = s.PruneStale()

	searcher := search.NewKeywordSearcher(s)

	server, err := mcp.NewServer(mcp.Options{
		Store:    s,
		Searcher: searcher,
		Version:  version,
	})
	if err != nil {
		return fmt.Errorf("build mcp server: %w", err)
	}

	// Run the server in a context that's cancelled by SIGINT/SIGTERM
	// so a clean shutdown happens on Ctrl-C or kill. The SDK's Run
	// returns when the transport closes, which normally happens when
	// the parent (Claude Code) exits.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx)
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `mastermind %s — the ADHD cure for agents that you always dreamed for yourself

Usage:
  mastermind                    Start MCP server over stdio (default; used by Claude Code)
  mastermind mcp                Explicit: start MCP server
  mastermind session-start      Claude Code session-start hook (phase 3c, not implemented)
  mastermind session-close      Claude Code session-close hook (phase 3b, not implemented)
  mastermind version            Print version and exit
  mastermind help               Show this help

MCP tools (for agent use):
  mm_search       Search persistent knowledge across scopes
  mm_write        Write a candidate entry to the pending-review queue
  mm_promote      Move a pending entry to the live store (user-gated)
  mm_close_loop   Mark an open-loop as resolved (phase 3, not implemented)

Setup:
  mastermind expects a ~/.mm/ directory. Initialize it as a git repo
  with a remote for cross-machine sync. Then add mastermind to your
  Claude Code MCP config:

    {
      "mcpServers": {
        "mastermind": {
          "type": "stdio",
          "command": "mastermind"
        }
      }
    }

Docs: see the project docs/ directory for the full design.
`, version)
}
