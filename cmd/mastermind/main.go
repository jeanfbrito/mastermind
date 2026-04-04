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
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("mastermind %s\n", version)
			return
		}
	}

	// TODO(phase1): start the MCP server over stdio and register tools:
	//   - mm_search(query, scopes?, include_archive?)
	//   - mm_write(content, scope, kind)
	//   - mm_promote(pending_path, target_scope)
	//
	// See docs/ARCHITECTURE.md for the tool surface.
	fmt.Fprintln(os.Stderr, "mastermind: not implemented yet — see docs/ROADMAP.md")
	os.Exit(1)
}
