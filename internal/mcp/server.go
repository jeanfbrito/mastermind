// Package mcp wires mastermind's functionality into the Model Context
// Protocol using github.com/modelcontextprotocol/go-sdk.
//
// This package is the ONLY place that imports the MCP SDK. All other
// packages (format, store, search, project) are MCP-agnostic. If the
// SDK ever needs to be swapped or upgraded, this is the only file that
// changes — a deliberate isolation boundary.
//
// Tool surface (four, forever — see DECISIONS.md on "no fifth tool
// without a recorded justification"):
//
//   - mm_search      read:  query the three scopes, return ranked results
//   - mm_write       write: store a candidate entry in <scope>/pending/
//   - mm_promote     write: move a pending entry into the live store
//   - mm_close_loop  write: resolve an open-loop, move to resolved-loops/
//
// All tool input/output types live in this package (see tools.go), not
// in format/store. This is deliberate: the MCP tool API is an external
// contract that must stay stable even as internal types evolve. Having
// separate wire types and internal types lets each evolve independently
// and prevents a rename in internal/format from silently breaking the
// tool schema.
//
// Server instructions (see serverInstructions below) are shipped to the
// MCP client at session start and tell the agent *when* to call each
// tool. This is as load-bearing as the code itself — it's how an agent
// learns to call mm_close_loop proactively when the user says "ok I
// finished that auth refactor," rather than waiting to be asked. The
// pattern is borrowed from engram's serverInstructions constant.
package mcp

import (
	"context"
	"fmt"

	"github.com/jeanfbrito/mastermind/internal/search"
	"github.com/jeanfbrito/mastermind/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the server version string exposed to MCP clients.
// It's set from main at startup via Options.
const defaultVersion = "dev"

// Options configures the MCP server.
//
// Store and Searcher are required. Version defaults to "dev" if empty.
type Options struct {
	Store    *store.Store
	Searcher search.Searcher
	Version  string
}

// Server wraps the underlying MCP SDK server with the mastermind
// tool set pre-registered. Use NewServer to construct one, then call
// Run with a transport.
//
// A Server is not re-entrant across Run calls — create a fresh one per
// process. In practice mastermind is a single-process, single-session
// subprocess spawned by Claude Code per-session, so this constraint is
// free.
type Server struct {
	inner *mcpsdk.Server
	opts  Options
}

// NewServer builds a Server with all four mm_* tools registered.
// Returns an error only if required Options fields are missing.
func NewServer(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("mcp: Options.Store is required")
	}
	if opts.Searcher == nil {
		return nil, fmt.Errorf("mcp: Options.Searcher is required")
	}
	if opts.Version == "" {
		opts.Version = defaultVersion
	}

	inner := mcpsdk.NewServer(
		&mcpsdk.Implementation{
			Name:    "mastermind",
			Version: opts.Version,
		},
		&mcpsdk.ServerOptions{
			Instructions: serverInstructions,
		},
	)

	s := &Server{inner: inner, opts: opts}
	s.registerTools()
	return s, nil
}

// Run starts the MCP server over stdio and blocks until the client
// disconnects or ctx is canceled. This is the normal entry point from
// main when mastermind runs as an MCP subprocess.
func (s *Server) Run(ctx context.Context) error {
	return s.inner.Run(ctx, &mcpsdk.StdioTransport{})
}

// Inner returns the underlying SDK server. Exposed for tests that need
// to poke at the server state directly. Not part of the stable API.
func (s *Server) Inner() *mcpsdk.Server {
	return s.inner
}

// serverInstructions is sent to every MCP client at session start.
// It tells the agent *when* to call each tool, which is the single
// thing that determines whether the continuity layer actually works in
// practice: if the agent doesn't know to call mm_search at the start
// of every task, the whole design of "retrieval surfaces automatically"
// fails even though the code is perfect.
//
// This constant is versioned as code. Changes are commits. It's the
// closest thing mastermind has to a "prompt," and like any prompt, it
// will need tuning during dogfooding.
const serverInstructions = `mastermind is a personal engineering second brain with persistent
memory across sessions, scoped per user/project. When you are helping
the user with coding work, you should use these tools PROACTIVELY —
not just when asked.

## Tools (four total, always available)

### mm_search
Search the user's persistent knowledge. Call this at the START of any
non-trivial task to surface prior lessons, decisions, patterns, and
war stories that may apply. Also call it when the user references
something they "solved before" or when you're about to attempt a
pattern you suspect has been tried.

Returns markdown with per-result sections the user can read directly.

### mm_write
Write a candidate entry to the user's pending-review queue. Use when
you discover a lesson worth preserving mid-session. Do NOT use this
for full session summaries — mastermind extracts those automatically
at session close. Prefer mm_write for in-session captures the user
explicitly asks you to save, or for open-loops the user surfaces
mid-conversation.

Never writes directly to the live store. All writes land in
<scope>/pending/ for the user's review.

### mm_promote
Move an entry from pending/ to the live store. Only call this when
the user has explicitly reviewed and approved a pending candidate.
Do NOT auto-promote — promotion is the user's decision, not yours.

### mm_close_loop
Mark an open-loop as resolved. Call this when the user indicates
they've finished something they previously marked as in-progress
("ok, auth refactor is done", "I shipped that fix", "that bug is
finally closed"). Moves the entry to resolved-loops/ so it stops
appearing in future session-start injections.

## Critical rules

- mm_search is read-only. Use it liberally.
- mm_write always goes to pending/. Never to live.
- mm_promote is user-gated. Never auto-promote.
- mm_close_loop should be called whenever the user signals closure,
  not just when they explicitly say "mark that done."
- Session-start context injection and session-close extraction happen
  via external hooks (mastermind session-start / session-close
  subcommands), not via these MCP tools. You do not need to trigger
  them.
`
