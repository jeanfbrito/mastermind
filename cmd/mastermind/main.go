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

// runMCPServer boots the three-scope store, wires up the searcher and
// the MCP server, and runs until the client disconnects or a signal
// arrives. Returns any error that escapes the SDK run loop.
func runMCPServer() error {
	cfg, err := store.DefaultConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Project-shared scope detection: walk upward from cwd looking for
	// a .mm/ directory. If found, point the store at it so mm_write
	// with scope=project-shared has somewhere to land. An absent .mm/
	// leaves ProjectSharedRoot empty, which disables the scope silently
	// — other scopes continue to work. This is per-session: every
	// spawn of the server re-detects, so moving between projects works.
	if cwd, err := os.Getwd(); err == nil {
		if root := store.FindProjectRoot(cwd); root != "" {
			cfg.ProjectSharedRoot = filepath.Join(root, ".mm")
		}
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
