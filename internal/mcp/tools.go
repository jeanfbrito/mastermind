package mcp

import (
	"context"
	"fmt"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/search"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools adds the four mastermind tools to the underlying SDK
// server. Called once from NewServer. The order of registration is
// the order tools are advertised to clients — search first because
// it's the most common call path.
func (s *Server) registerTools() {
	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "mm_search",
		Description: mmSearchDescription,
	}, s.handleSearch)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "mm_write",
		Description: mmWriteDescription,
	}, s.handleWrite)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "mm_promote",
		Description: mmPromoteDescription,
	}, s.handlePromote)

	mcpsdk.AddTool(s.inner, &mcpsdk.Tool{
		Name:        "mm_close_loop",
		Description: mmCloseLoopDescription,
	}, s.handleCloseLoop)
}

// ─── mm_search ──────────────────────────────────────────────────────────

const mmSearchDescription = `Search the user's persistent knowledge base across all scopes.
Call at the start of any non-trivial task and whenever the user
references prior work. Returns ranked markdown results with per-entry
sections. Supports optional filters by scope, kind, project, and tags.`

// SearchInput is the wire schema for mm_search. Fields are optional
// except Query. Every field has a jsonschema tag so the SDK can
// publish an accurate schema to clients.
type SearchInput struct {
	Query          string   `json:"query" jsonschema:"natural language or keyword query; required"`
	Scopes         []string `json:"scopes,omitempty" jsonschema:"optional scope filter: user-personal, project-shared, project-personal"`
	Kinds          []string `json:"kinds,omitempty" jsonschema:"optional kind filter: lesson, insight, war-story, decision, pattern, open-loop"`
	Project        string   `json:"project,omitempty" jsonschema:"optional project name filter (case-insensitive)"`
	Tags           []string `json:"tags,omitempty" jsonschema:"optional tag filter; ALL listed tags must be present (AND semantics)"`
	IncludePending bool     `json:"include_pending,omitempty" jsonschema:"if true, also search pending/ (unreviewed candidates); default false"`
	Limit          int      `json:"limit,omitempty" jsonschema:"max results; default 10"`
}

// SearchOutput is the wire schema for mm_search results. The Markdown
// field is the main payload — it's formatted for human reading AND for
// context-mode's automatic session indexing (per-result H3 headings let
// context-mode chunk it cleanly so warm follow-ups within the same
// session can be answered from the cache without re-calling mastermind).
type SearchOutput struct {
	Markdown string `json:"markdown" jsonschema:"markdown-formatted ranked results with per-entry H3 sections"`
	Count    int    `json:"count" jsonschema:"number of results returned after ranking and limit"`
}

func (s *Server) handleSearch(ctx context.Context, req *mcpsdk.CallToolRequest, in SearchInput) (*mcpsdk.CallToolResult, SearchOutput, error) {
	q := search.Query{
		QueryText:      in.Query,
		Project:        in.Project,
		Tags:           in.Tags,
		IncludePending: in.IncludePending,
		Limit:          in.Limit,
	}
	for _, scope := range in.Scopes {
		q.Scopes = append(q.Scopes, format.Scope(scope))
	}
	for _, kind := range in.Kinds {
		q.Kinds = append(q.Kinds, format.Kind(kind))
	}

	results, err := s.opts.Searcher.Search(q)
	if err != nil {
		return nil, SearchOutput{}, fmt.Errorf("mm_search: %w", err)
	}

	return nil, SearchOutput{
		Markdown: search.FormatResultsMarkdown(in.Query, results),
		Count:    len(results),
	}, nil
}

// ─── mm_write ───────────────────────────────────────────────────────────

const mmWriteDescription = `Write an entry directly to the user's live knowledge store.
Use for explicit in-session captures — the user is present and the
write is their decision, so no second review step is needed. Never
use for session summaries — those are extracted automatically at
session close into pending/ for review.`

// WriteInput is the wire schema for mm_write. The shape mirrors
// format.Metadata but uses plain strings instead of enum types so the
// JSON Schema published to clients is stable and human-readable.
type WriteInput struct {
	Topic      string   `json:"topic" jsonschema:"one-line human summary of the entry; required"`
	Body       string   `json:"body" jsonschema:"markdown body of the entry; what/why/how/lesson sections recommended"`
	Scope      string   `json:"scope" jsonschema:"required: user-personal, project-shared, or project-personal"`
	Kind       string   `json:"kind" jsonschema:"required: lesson, insight, war-story, decision, pattern, or open-loop"`
	Project    string   `json:"project" jsonschema:"project name; use 'general' for cross-project entries; required"`
	Tags       []string `json:"tags,omitempty" jsonschema:"free-form lowercase tags"`
	Category   string   `json:"category" jsonschema:"topic directory path (1-2 segments); e.g. 'electron/ipc', 'go/modules', 'testing'. Classify by SUBJECT, not context."`
	Date       string   `json:"date,omitempty" jsonschema:"ISO 8601 capture date (YYYY-MM-DD); defaults to today UTC if omitted"`
	Confidence string   `json:"confidence,omitempty" jsonschema:"high, medium, or low; defaults to high"`
}

// WriteOutput reports where the entry landed and what scope/kind it
// was routed to after normalization. Since mm_write goes directly to
// the live store, the path is the final resting place — no promotion
// needed.
type WriteOutput struct {
	Path  string `json:"path" jsonschema:"absolute path to the written live entry"`
	Scope string `json:"scope" jsonschema:"scope the entry was routed to"`
	Kind  string `json:"kind" jsonschema:"kind of the written entry"`
}

func (s *Server) handleWrite(ctx context.Context, req *mcpsdk.CallToolRequest, in WriteInput) (*mcpsdk.CallToolResult, WriteOutput, error) {
	entry := &format.Entry{
		Metadata: format.Metadata{
			Date:       in.Date,
			Project:    in.Project,
			Tags:       in.Tags,
			Topic:      in.Topic,
			Kind:       format.Kind(in.Kind),
			Scope:      format.Scope(in.Scope),
			Category:   in.Category,
			Confidence: format.Confidence(in.Confidence),
		},
		Body: in.Body,
	}

	// Date defaults to today UTC if the caller omitted it. This matches
	// the session-close extractor's timestamp-grounding behavior.
	if entry.Metadata.Date == "" {
		entry.Metadata.Date = s.opts.Store.Config().Now().UTC().Format("2006-01-02")
	}

	// Validate before writing so the error surface is the tool's
	// return value, not a corrupted pending file.
	if errs := entry.Validate(); len(errs) > 0 {
		return nil, WriteOutput{}, fmt.Errorf("mm_write: validation failed: %v", errs)
	}

	path, err := s.opts.Store.WriteLive(entry)
	if err != nil {
		return nil, WriteOutput{}, fmt.Errorf("mm_write: %w", err)
	}

	return nil, WriteOutput{
		Path:  path,
		Scope: string(entry.Metadata.Scope),
		Kind:  string(entry.Metadata.Kind),
	}, nil
}

// ─── mm_promote ─────────────────────────────────────────────────────────

const mmPromoteDescription = `Move an entry from pending/ to the live store.
Only call when the user has explicitly reviewed and approved a pending
candidate. Do NOT auto-promote — promotion is the user's decision.
Returns the new live-store path.`

type PromoteInput struct {
	PendingPath string `json:"pending_path" jsonschema:"absolute path to the pending entry to promote; must live under a <scope>/pending/ directory"`
}

type PromoteOutput struct {
	LivePath string `json:"live_path" jsonschema:"absolute path where the entry now lives in the live store"`
}

func (s *Server) handlePromote(ctx context.Context, req *mcpsdk.CallToolRequest, in PromoteInput) (*mcpsdk.CallToolResult, PromoteOutput, error) {
	livePath, err := s.opts.Store.Promote(in.PendingPath)
	if err != nil {
		return nil, PromoteOutput{}, fmt.Errorf("mm_promote: %w", err)
	}
	return nil, PromoteOutput{LivePath: livePath}, nil
}

// ─── mm_close_loop ──────────────────────────────────────────────────────

const mmCloseLoopDescription = `Mark an open-loop as resolved.
Call when the user signals closure of something previously captured
as an open-loop ("ok, that refactor is done", "shipped the fix",
"that bug is closed"). Moves the entry so it stops appearing in
future session-start injections. Does NOT delete — resolved loops are
archived for history.`

type CloseLoopInput struct {
	PendingPath string `json:"entry_path" jsonschema:"absolute path to the open-loop entry to resolve (from a prior mm_search result)"`
	Resolution  string `json:"resolution,omitempty" jsonschema:"optional one-line note about how the loop was resolved; appended to the entry before archiving"`
}

type CloseLoopOutput struct {
	ResolvedPath string `json:"resolved_path" jsonschema:"absolute path where the resolved loop now lives"`
}

func (s *Server) handleCloseLoop(ctx context.Context, req *mcpsdk.CallToolRequest, in CloseLoopInput) (*mcpsdk.CallToolResult, CloseLoopOutput, error) {
	// mm_close_loop is intentionally a thin wrapper. The real behavior
	// (verify it's an open-loop, append resolution note, move to
	// resolved-loops/) lives in a store.CloseLoop method that doesn't
	// exist yet — that's Phase 3c territory (CONTINUITY.md → open-loop
	// management).
	//
	// For Phase 1, we register the tool so the schema is published and
	// the agent learns about it from serverInstructions, but the handler
	// returns an explicit "not implemented" error. That's better than
	// silently dropping calls: the agent learns the tool exists, tries
	// it, gets a clear error, and the user sees the Phase 3c work is
	// still pending.
	_ = in
	return nil, CloseLoopOutput{}, fmt.Errorf("mm_close_loop: not implemented in Phase 1 — see docs/CONTINUITY.md and docs/ROADMAP.md Phase 3c")
}
