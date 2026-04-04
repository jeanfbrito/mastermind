// Command mastermind is a personal engineering second brain.
//
// It exposes an MCP server over stdio with a small set of tools for
// querying and curating markdown-based knowledge stores across three
// scopes: user-personal, project-shared, and project-personal.
//
// See the project docs for the design: docs/ARCHITECTURE.md,
// docs/FORMAT.md, docs/EXTRACTION.md, docs/ARCHIVE.md.
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
