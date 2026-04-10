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
sections. Supports optional filters by scope, kind, project, and tags.

Two input shapes are supported — exactly one must be provided:
  - query: single query string (original shape, back-compat).
  - queries: array of query strings, all run in a single call. Each
    query gets its own ranked result block in the output markdown,
    separated by its own H2 heading. Use when you want multiple
    angles on a task in one round-trip (e.g. ["auth middleware",
    "session tokens", "jwt expiry"]) — avoids N serial tool calls.
    Every query runs the full tiered fallback pipeline independently;
    filters (scopes, kinds, project, tags, limit, expand) apply to
    every query uniformly.

Body verbosity is controlled by the expand field (L2/L3 in the
mastermind memory stack):
  - expand omitted or false (default, L2): returns topic + first ##
    section + a match-anchored excerpt (~200 tokens per result). Each
    result includes a 'path' field — pass it to the Read tool for the
    full entry (L3) without re-searching.
  - expand: true (L3): returns the full body verbatim. Use for deep
    dives when you know you need complete content.

Prefer the default (expand omitted) for session-start queries and
broad topic sweeps. Use expand:true when you have a specific entry
in mind and need its full text.`

// SearchInput is the wire schema for mm_search. All fields are
// optional at the schema level; runtime validation enforces that
// exactly one of Query or Queries is non-empty.
type SearchInput struct {
	Query          string   `json:"query,omitempty" jsonschema:"single query string; exactly one of query or queries is required"`
	Queries        []string `json:"queries,omitempty" jsonschema:"array of query strings for batch search; each runs the full tiered fallback independently and gets its own markdown section in the response"`
	Scopes         []string `json:"scopes,omitempty" jsonschema:"optional scope filter: user-personal, project-shared, project-personal"`
	Kinds          []string `json:"kinds,omitempty" jsonschema:"optional kind filter: lesson, insight, war-story, decision, pattern, open-loop"`
	Project        string   `json:"project,omitempty" jsonschema:"optional project name filter (case-insensitive)"`
	Tags           []string `json:"tags,omitempty" jsonschema:"optional tag filter; ALL listed tags must be present (AND semantics)"`
	IncludePending bool     `json:"include_pending,omitempty" jsonschema:"if true, also search pending/ (unreviewed candidates); default false"`
	Limit          int      `json:"limit,omitempty" jsonschema:"max results per query; default 10"`
	Expand         bool     `json:"expand,omitempty" jsonschema:"if true, return full body (L3 deep dive); default false returns trimmed excerpt (L2)"`
}

// SearchOutput is the wire schema for mm_search results. The Markdown
// field is the main payload — it's formatted for human reading AND for
// context-mode's automatic session indexing (per-result H3 headings let
// context-mode chunk it cleanly so warm follow-ups within the same
// session can be answered from the cache without re-calling mastermind).
//
// For batch mode (queries array), Markdown is the concatenation of
// per-query result blocks separated by blank lines; Count is the sum
// across all queries. Shape is unchanged vs. the single-query path —
// backward compatible.
type SearchOutput struct {
	Markdown string `json:"markdown" jsonschema:"markdown-formatted ranked results with per-entry H3 sections (concatenated across batch queries if any)"`
	Count    int    `json:"count" jsonschema:"total number of results returned across all queries after ranking and limit"`
}

func (s *Server) handleSearch(ctx context.Context, req *mcpsdk.CallToolRequest, in SearchInput) (*mcpsdk.CallToolResult, SearchOutput, error) {
	// Exactly-one-of validation: callers must supply either query or
	// queries, not both and not neither. Enforced at runtime because
	// the SDK's reflection-generated schema can't express a oneOf
	// constraint against struct fields cleanly.
	queries, err := collectSearchQueries(in)
	if err != nil {
		return nil, SearchOutput{}, fmt.Errorf("mm_search: %w", err)
	}

	// Build the common Query template once — filters and limit apply
	// uniformly to every query in the batch.
	base := search.Query{
		Project:        in.Project,
		Tags:           in.Tags,
		IncludePending: in.IncludePending,
		Limit:          in.Limit,
	}
	for _, scope := range in.Scopes {
		base.Scopes = append(base.Scopes, format.Scope(scope))
	}
	for _, kind := range in.Kinds {
		base.Kinds = append(base.Kinds, format.Kind(kind))
	}

	var (
		out       SearchOutput
		mdBuilder []string
	)
	for _, qt := range queries {
		q := base
		q.QueryText = qt
		results, err := s.opts.Searcher.Search(q)
		if err != nil {
			return nil, SearchOutput{}, fmt.Errorf("mm_search %q: %w", qt, err)
		}
		mdBuilder = append(mdBuilder, search.FormatResultsMarkdown(qt, results, in.Expand))
		out.Count += len(results)
	}
	out.Markdown = joinMarkdownBlocks(mdBuilder)
	return nil, out, nil
}

// collectSearchQueries normalizes the input into a slice of query
// strings to run. Enforces the exactly-one-of rule: either Query
// is non-empty OR Queries contains at least one non-empty string,
// but not both.
func collectSearchQueries(in SearchInput) ([]string, error) {
	singleSet := in.Query != ""
	batch := trimNonEmpty(in.Queries)
	batchSet := len(batch) > 0

	switch {
	case singleSet && batchSet:
		return nil, fmt.Errorf("provide either query or queries, not both")
	case singleSet:
		return []string{in.Query}, nil
	case batchSet:
		return batch, nil
	default:
		return nil, fmt.Errorf("empty query text")
	}
}

// trimNonEmpty returns a copy of in with empty/whitespace-only
// entries removed. Used so a caller passing `queries: ["foo", ""]`
// doesn't trigger the search package's empty-query error on the
// second element.
func trimNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// joinMarkdownBlocks concatenates per-query result blocks with a
// blank line between them. A single-query caller gets the exact
// same bytes as before (no leading/trailing separator).
func joinMarkdownBlocks(blocks []string) string {
	switch len(blocks) {
	case 0:
		return ""
	case 1:
		return blocks[0]
	}
	// Multi-block: join with a blank line, and ensure each block
	// ends in exactly one newline so the separator lands cleanly.
	var out string
	for i, b := range blocks {
		if i > 0 {
			out += "\n"
		}
		out += b
	}
	return out
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
	resolvedPath, err := s.opts.Store.CloseLoop(in.PendingPath, in.Resolution)
	if err != nil {
		return nil, CloseLoopOutput{}, err
	}
	return nil, CloseLoopOutput{ResolvedPath: resolvedPath}, nil
}
