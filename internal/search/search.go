// Package search provides the query layer across all three scopes.
//
// mastermind owns no persistent index. Every query reads EntryRef slices
// from internal/store, applies metadata filters in memory, then loads
// bodies on demand for the top candidates and scores them with a simple
// keyword match. This is fast at realistic corpus sizes (sub-100ms for
// thousands of entries on modern hardware) and has zero dependencies
// beyond the Go standard library.
//
// The division of labor with context-mode is deliberate and important:
//
//   - mastermind answers the cold query. It goes to disk, ranks, returns.
//   - context-mode automatically indexes mastermind's returned output
//     into its session FTS5 cache (because that's what context-mode does
//     with every MCP tool's output). Subsequent warm follow-ups within
//     the same session get answered by context-mode without mastermind
//     being re-invoked.
//
// This means mastermind and context-mode stack for free, with zero
// coupling. mastermind never calls context-mode. context-mode doesn't
// know mastermind is special. The synergy is automatic. See the
// DECISIONS.md entry on this division and REFERENCE-NOTES.md appendix
// for the full rationale.
//
// Responsibilities of this package:
//   - Define the Searcher interface and the Query / Result types.
//   - Provide a default stdlib-only keyword-match implementation.
//   - Format results as human-readable markdown (which is also
//     context-mode-indexable for warm follow-ups).
//
// Non-responsibilities:
//   - FTS5 or any other external index. Not ours.
//   - Delegation to context-mode. Not ours.
//   - Persistent caches. If listing ever slows down, the cache lives
//     in internal/store as an in-memory mtime-keyed map.
package search

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// Query describes a search request. All fields are optional except
// QueryText: an empty query returns zero results (we don't guess intent).
type Query struct {
	// QueryText is the free-form search string. Tokenized on whitespace
	// and matched case-insensitively against topic, tags, and body.
	QueryText string

	// Scopes limits the search to specific scope(s). Empty = search all
	// configured scopes.
	Scopes []format.Scope

	// IncludePending, when true, searches pending/ directories in
	// addition to live directories. Defaults to false — pending entries
	// are candidates, not corpus, and shouldn't pollute normal queries.
	IncludePending bool

	// Kinds filters by entry kind. Empty = any kind.
	Kinds []format.Kind

	// Project filters by the project field in frontmatter. Empty = any.
	Project string

	// Tags requires every listed tag to be present (AND semantics). Empty = no tag filter.
	Tags []string

	// Limit caps the number of returned results. Zero means default (10).
	Limit int
}

// Result is a single ranked entry returned by a search.
//
// The body is the full file body — we return it because the typical
// caller (mm_search MCP handler) immediately formats a markdown section
// that includes the body excerpt. Loading on demand is cheap at our
// sizes and avoids a second round-trip.
type Result struct {
	Ref      store.EntryRef // pointer back to the source file
	Score    float64        // ranking score, higher = better match
	Body     string         // the full markdown body (for presentation)
	Metadata format.Metadata
}

// Searcher is the query interface. Implementations are interchangeable
// via dependency injection; the default is KeywordSearcher.
//
// Keeping this as an interface (rather than a concrete type) means we
// can swap the backend in Phase 6 or beyond without touching any caller.
// Today we have one implementation. That's fine — the indirection
// costs nothing and the contract is clear.
type Searcher interface {
	Search(q Query) ([]Result, error)
}

// ─── KeywordSearcher — the default implementation ───────────────────────

// KeywordSearcher does stdlib-only keyword matching against entries
// obtained from an underlying store. This is the Phase 1 default and is
// expected to remain sufficient for career-long corpus sizes (thousands
// of entries, megabytes of text).
type KeywordSearcher struct {
	Store *store.Store
}

// NewKeywordSearcher constructs a KeywordSearcher backed by s.
func NewKeywordSearcher(s *store.Store) *KeywordSearcher {
	return &KeywordSearcher{Store: s}
}

// Search runs the query. The pipeline:
//
//  1. Determine which scopes to search.
//  2. Gather EntryRef slices from each scope (live, and pending if
//     requested). ListLive/ListPending already parse frontmatter.
//  3. Apply metadata filters (kind, project, tags) to drop non-matches
//     without reading bodies.
//  4. Score the survivors by keyword match against topic + tags first.
//     If the query text isn't satisfied by metadata alone, load the
//     body and score against it too.
//  5. Sort by score descending, apply Limit, return.
//
// Zero-match queries return an empty slice, not an error. Bad inputs
// (empty query text) return an error so the caller can distinguish
// "no results" from "nothing to search for".
func (k *KeywordSearcher) Search(q Query) ([]Result, error) {
	if strings.TrimSpace(q.QueryText) == "" {
		return nil, fmt.Errorf("search: empty query text")
	}
	if k.Store == nil {
		return nil, fmt.Errorf("search: nil store")
	}

	tokens := tokenize(q.QueryText)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("search: query yielded no tokens")
	}

	scopes := q.Scopes
	if len(scopes) == 0 {
		scopes = []format.Scope{
			format.ScopeUserPersonal,
			format.ScopeProjectShared,
			format.ScopeProjectPersonal,
		}
	}

	var refs []store.EntryRef
	for _, scope := range scopes {
		liveRefs, err := k.Store.ListLive(scope)
		if err != nil {
			return nil, fmt.Errorf("search: list live %s: %w", scope, err)
		}
		refs = append(refs, liveRefs...)

		if q.IncludePending {
			pendingRefs, err := k.Store.ListPending(scope)
			if err != nil {
				return nil, fmt.Errorf("search: list pending %s: %w", scope, err)
			}
			refs = append(refs, pendingRefs...)
		}
	}

	// Metadata-only pre-filter. Cheap: no body reads.
	filtered := refs[:0]
	for _, r := range refs {
		if !matchesMetadataFilters(r, q) {
			continue
		}
		filtered = append(filtered, r)
	}

	// Score each survivor. First pass: topic + tags (no body needed).
	// If that's unsatisfied, load body and rescore.
	results := make([]Result, 0, len(filtered))
	for _, r := range filtered {
		topicTagScore := scoreTopicAndTags(r.Metadata, tokens)

		var body string
		bodyScore := 0.0
		if topicTagScore < float64(len(tokens)) {
			// Not every token was found in metadata. Load body.
			entry, err := k.Store.LoadRef(r)
			if err != nil {
				// Malformed file; skip silently. The list layer
				// already handles most of this.
				continue
			}
			body = entry.Body
			bodyScore = scoreBody(body, tokens)
		}

		score := topicTagScore + bodyScore
		if score <= 0 {
			continue
		}
		// Access frequency boost: frequently useful entries rank
		// slightly higher. Capped at +0.5 so it's a tiebreaker,
		// never overrides topic relevance (2.0 per token).
		score += accessBoost(r.Metadata.Accessed)
		results = append(results, Result{
			Ref:      r,
			Score:    score,
			Body:     body,
			Metadata: r.Metadata,
		})
	}

	// Rank: higher score first; tiebreaker is date descending (newer
	// entries win ties), then path ascending (deterministic).
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Metadata.Date != results[j].Metadata.Date {
			return results[i].Metadata.Date > results[j].Metadata.Date
		}
		return results[i].Ref.Path < results[j].Ref.Path
	})

	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	if len(results) > limit {
		results = results[:limit]
	}

	// For results that matched on topic/tags alone, we still want the
	// body in the returned slice so the caller can format a usable
	// snippet. Load lazily now (cheap — only for top-N that survived).
	for i := range results {
		if results[i].Body == "" {
			entry, err := k.Store.LoadRef(results[i].Ref)
			if err == nil {
				results[i].Body = entry.Body
			}
		}
	}

	// Track access counts for returned results. Best-effort —
	// errors are silently discarded by IncrementAccess.
	now := time.Now()
	for _, r := range results {
		k.Store.IncrementAccess(r.Ref.Path, now)
	}

	return results, nil
}

// ─── filtering ──────────────────────────────────────────────────────────

// matchesMetadataFilters returns true if the ref passes the kind,
// project, and tags filters. Empty filters always pass.
func matchesMetadataFilters(ref store.EntryRef, q Query) bool {
	md := ref.Metadata

	if len(q.Kinds) > 0 {
		found := false
		for _, k := range q.Kinds {
			if md.Kind == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if q.Project != "" && !strings.EqualFold(md.Project, q.Project) {
		return false
	}

	if len(q.Tags) > 0 {
		// AND semantics: every requested tag must be present.
		for _, want := range q.Tags {
			wantLower := strings.ToLower(want)
			has := false
			for _, got := range md.Tags {
				if strings.ToLower(got) == wantLower {
					has = true
					break
				}
			}
			if !has {
				return false
			}
		}
	}

	return true
}

// ─── scoring ────────────────────────────────────────────────────────────

// scoreTopicAndTags returns a metadata-only score for how many query
// tokens appear in the topic or tags.
//
// Weighting: topic hit = 2.0, tag-only hit = 0.7. The 2.0 : 0.7 ratio
// (plus body scoring topping out around 0.75 per token) ensures a topic
// hit dominates any combination of tag + body hits on the same token.
// This is the load-bearing ranking invariant: if a user searches for
// "macos" and one entry has it in the topic while another only has it
// in tags + body, the topic hit wins. Tests lock this in — see
// TestKeywordSearcherRankingFavorsTopicOverBody.
func scoreTopicAndTags(md format.Metadata, tokens []string) float64 {
	topic := strings.ToLower(md.Topic)
	tagBlob := strings.ToLower(strings.Join(md.Tags, " "))

	var score float64
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		hit := false
		if strings.Contains(topic, tok) {
			score += 2.0 // topic hit dominates
			hit = true
		}
		if !hit && strings.Contains(tagBlob, tok) {
			score += 0.7
		}
	}
	return score
}

// scoreBody adds a body-match contribution for each token. Body hits are
// worth less than topic hits because the body contains a lot of text
// that isn't load-bearing. The per-token weight is 0.3, capped at 1.0
// per token (diminishing returns on repeated hits).
func scoreBody(body string, tokens []string) float64 {
	if body == "" {
		return 0
	}
	lower := strings.ToLower(body)
	var score float64
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		count := strings.Count(lower, tok)
		if count == 0 {
			continue
		}
		// Log-ish diminishing returns: 1 hit = 0.3, 2 = 0.45, 5 = 0.6, 10+ ~= 0.75
		contribution := 0.3
		for i := 1; i < count && contribution < 0.75; i++ {
			contribution += 0.15 / float64(i)
		}
		score += contribution
	}
	return score
}

// accessBoost returns a small score bonus based on how many times an
// entry has been returned by previous searches. The bonus uses
// diminishing returns: 1 access = 0.05, 10 = 0.5 (cap). This ensures
// frequently useful entries float up slightly without ever overriding
// topic relevance (2.0 per token).
func accessBoost(accessed int) float64 {
	if accessed <= 0 {
		return 0
	}
	boost := float64(accessed) * 0.05
	if boost > 0.5 {
		return 0.5
	}
	return boost
}

// tokenize splits a query string into lowercase tokens. Tokens are
// separated by any non-letter, non-digit ASCII character. Tokens of
// length 1 are dropped (too noisy). Empty result means the query has
// no searchable content.
func tokenize(s string) []string {
	lower := strings.ToLower(s)
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 2 {
			tokens = append(tokens, cur.String())
		}
		cur.Reset()
	}
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
