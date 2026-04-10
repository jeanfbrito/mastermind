// Package search provides the query layer across all three scopes.
//
// mastermind owns no persistent index. Every query reads EntryRef slices
// from internal/store, applies metadata filters in memory, then scores
// survivors through a tiered fallback chain and returns the top N.
//
// # Tiered fallback
//
// Results are sorted primarily by a tierClass enum (0–6); score is only
// a tiebreaker within a class. The tiers, from strictly strongest to
// weakest match:
//
//   - Class 0: exact phrase in topic (multi-word queries only)
//   - Class 1: exact phrase in a tag
//   - Class 2: exact phrase in body (requires body load)
//   - Class 3: all query tokens in topic
//   - Class 4: all query tokens across topic + tags
//   - Class 5: tokens matched in body (default keyword pipeline)
//   - Class 6: fuzzy gap-match against topic + tags (sahilm/fuzzy)
//
// A class-0 hit strictly dominates any class-6 hit regardless of access
// frequency, score magnitude, or recency. This is engram's "Rank = -1000
// sentinel" pattern translated into Go — class is lock-in-by-construction.
//
// Execution is three passes:
//  1. Pass 1 (metadata-only, no I/O) handles classes 0/1/3/4.
//  2. Pass 2 (body load) handles classes 2/5 — skipped entirely when
//     the short-circuit condition fires (top-K pass-1 results all in
//     class ≤ 4 AND at least one has access_count ≥ 3, borrowed from
//     shiba-memory's "earned confidence" gate).
//  3. Pass 3 (fuzzy fallback) handles class 6 — only if earlier passes
//     didn't fill the limit AND the query is ≥ 4 characters long
//     (engram's length-guard pattern; short queries drown precision).
//
// Within a class, the access-frequency tiebreaker uses ACT-R fast-mode
// base-level activation: ln(accessed+1) * 0.2, capped at +0.5. See
// DECISIONS.md (2026-04-10) for the full reference-repo sweep that
// informed the model. This is fast at realistic corpus sizes (sub-100ms
// for thousands of entries) with one direct dependency beyond stdlib
// (sahilm/fuzzy).
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
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"

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

	// Project is the name of the "current" project. Empty = project
	// boost disabled; every result gets a neutral 1.0× multiplier.
	//
	// When non-empty, Project acts as a soft ranking signal:
	//   - same-project entries: 1.3× score (promoted)
	//   - "general" / unset project on entry: 1.0× (neutral)
	//   - cross-project entries: 0.8× (demoted but NOT dropped)
	//
	// This is a within-class tiebreaker — class still strictly
	// dominates, so a cross-project class-0 hit always beats a
	// same-project class-5 hit. To restore the old hard-filter
	// behavior (drop cross-project results entirely), set
	// StrictProject = true. See DECISIONS.md 2026-04-10 entry on the
	// filter-to-multiplier refactor.
	Project string

	// StrictProject, when true, restores the hard-filter behavior on
	// Project — cross-project entries are dropped entirely before
	// scoring. Used by CLI callers with explicit "only this project"
	// intent (e.g., a future --project foo flag on `mastermind
	// discover`). The MCP tool surface always leaves this false:
	// agent callers want the most relevant results across projects,
	// just with a lean toward the current one.
	StrictProject bool

	// Tags requires every listed tag to be present (AND semantics). Empty = no tag filter.
	Tags []string

	// Limit caps the number of returned results. Zero means default (10).
	Limit int
}

// tierClass is the primary sort key for search results. Lower class
// values strictly dominate higher ones regardless of score: an exact-
// phrase topic hit (classExactTopic = 0) always outranks any fuzzy hit
// (classFuzzy = 6), even if the fuzzy hit has massive access-boost
// inflation. Score is only a tiebreaker within a class.
//
// The tier classes are borrowed in spirit from engram's Rank = -1000
// sentinel pattern (internal/store/store.go:1504-1512): instead of
// tuning additive boosts and hoping the weights line up, use a class
// enum so ordering is locked by construction. See DECISIONS.md for the
// reference-repo sweep that informed this model.
//
// Added in T2 of the tiered-fallback work — initially every result
// lands in classKeyword so behavior is unchanged; T3-T5 populate the
// other classes.
type tierClass int

const (
	classExactTopic tierClass = iota // 0: full query phrase found in topic
	classExactTag                    // 1: full query phrase found in a tag
	classExactBody                   // 2: full query phrase found in body text
	classTopicTokens                 // 3: all query tokens found in topic
	classMetaTokens                  // 4: all tokens across topic + tags
	classKeyword                     // 5: tokens matched in body (default)
	classFuzzy                       // 6: fuzzy topic/tag match fallback
)

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

	// Annotation is an optional tag rendered alongside the result
	// heading in the output markdown. Used by the contradicts
	// co-retrieval pass to mark entries pulled in via a
	// contradicts: [...] link on one of the top-K results.
	// Empty for normal keyword/fuzzy hits. See the Contradicts
	// field on format.Metadata and DECISIONS.md 2026-04-10.
	Annotation string

	// class is the tier bucket used as the primary sort key. Unexported
	// so it never crosses the MCP boundary — callers see only the
	// order of results, not which class produced them.
	class tierClass
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

	// shortCircuitCount is incremented every time a Search() call
	// skips the body-load pass because pass-1 (metadata-only) already
	// yielded enough high-confidence results. Used by tests to verify
	// the perf short-circuit fires at the right moments. Not thread-
	// safe by design — mastermind runs single-request from an MCP
	// stdio server, so there's no concurrent caller to race with.
	shortCircuitCount int
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

	// Exact-phrase detection: only meaningful for multi-word queries.
	// A single-word "phrase" is identical to the token, so we skip the
	// exact-phrase classes (0/1/2) and let single-word queries fall
	// through to token-level matching directly. Normalized once here to
	// avoid re-lowering per entry.
	var exactPhrase string
	if len(tokens) >= 2 {
		exactPhrase = strings.ToLower(strings.TrimSpace(q.QueryText))
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

	// Two-pass scoring pipeline.
	//
	// Pass 1 is metadata-only: no body reads. It handles classes
	// 0 (exact topic), 1 (exact tag), 3 (all tokens in topic),
	// 4 (all tokens across topic+tags). Any entry whose tokens are
	// fully satisfied by metadata goes into a pass-1 result directly.
	//
	// Pass 2 handles classes 2 (exact body phrase) and 5 (body
	// keyword match) — it loads the body for entries that pass 1
	// couldn't classify. Pass 2 is skipped entirely when the short-
	// circuit condition fires: top-K pass-1 results are all in
	// class ≤ 4 AND at least one has access_count ≥ 3 ("earned"
	// confidence, borrowed from shiba-memory's instinct-evolution
	// gate). The access gate prevents structural matches from
	// short-circuiting before the entry has proven useful.
	type bodyCandidate struct {
		ref          store.EntryRef
		topicHits    int // tokens found in topic
		metaHits     int // tokens found in topic or tags (topic wins)
		topicTagBase float64
	}

	results := make([]Result, 0, len(filtered))
	bodyNeeded := make([]bodyCandidate, 0, len(filtered))

	for _, r := range filtered {
		topicLower := strings.ToLower(r.Metadata.Topic)
		tagBlob := strings.ToLower(strings.Join(r.Metadata.Tags, " "))

		// Project boost: computed once per entry, applied as a
		// within-class multiplier on the final Score. See
		// projectMultiplier for the 1.3 / 1.0 / 0.8 semantics.
		projMult := projectMultiplier(q.Project, r.Metadata.Project)

		// Tier 0: exact phrase in topic.
		if exactPhrase != "" && strings.Contains(topicLower, exactPhrase) {
			results = append(results, Result{
				Ref:      r,
				Score:    (5.0 + accessBoost(r.Metadata.Accessed)) * projMult,
				Metadata: r.Metadata,
				class:    classExactTopic,
			})
			continue
		}
		// Tier 1: exact phrase in tags.
		if exactPhrase != "" && strings.Contains(tagBlob, exactPhrase) {
			results = append(results, Result{
				Ref:      r,
				Score:    (4.0 + accessBoost(r.Metadata.Accessed)) * projMult,
				Metadata: r.Metadata,
				class:    classExactTag,
			})
			continue
		}

		// Per-token classification: how many tokens hit topic vs.
		// (topic or tags). Topic wins on ties — a token in both is
		// counted in topicHits. This drives the class 3 vs. 4 split.
		topicHits, metaHits := 0, 0
		var topicTagScore float64
		for _, tok := range tokens {
			if tok == "" {
				continue
			}
			if strings.Contains(topicLower, tok) {
				topicHits++
				metaHits++
				topicTagScore += 2.0
				continue
			}
			if strings.Contains(tagBlob, tok) {
				metaHits++
				topicTagScore += 0.7
			}
		}

		nTokens := len(tokens)

		// Tier 3: every token found in topic. Metadata-only,
		// no body load needed. Highest keyword-class confidence.
		if topicHits == nTokens {
			results = append(results, Result{
				Ref:      r,
				Score:    (topicTagScore + accessBoost(r.Metadata.Accessed)) * projMult,
				Metadata: r.Metadata,
				class:    classTopicTokens,
			})
			continue
		}

		// Tier 4: every token found across topic + tags (but not
		// all in topic alone). Still no body read required.
		if metaHits == nTokens {
			results = append(results, Result{
				Ref:      r,
				Score:    (topicTagScore + accessBoost(r.Metadata.Accessed)) * projMult,
				Metadata: r.Metadata,
				class:    classMetaTokens,
			})
			continue
		}

		// Otherwise: we need the body to either complete keyword
		// coverage (class 5) or check for exact phrase (class 2).
		// Defer to pass 2 so short-circuit can skip it.
		bodyNeeded = append(bodyNeeded, bodyCandidate{
			ref:          r,
			topicHits:    topicHits,
			metaHits:     metaHits,
			topicTagBase: topicTagScore,
		})
	}

	// ── Pass 1 sort (so we can check short-circuit confidence) ──
	sortResultsByTier(results)

	// Short-circuit: if pass 1 already has high-confidence hits that
	// the user has accessed before, skip the expensive body-load pass.
	// Confidence rules:
	//   - top-K results (K = min(Limit, 3)) must all be class ≤ 4
	//   - at least one of those must have access_count ≥ 3 ("earned")
	if shouldShortCircuit(results, q.Limit) {
		k.shortCircuitCount++
	} else {
		// ── Pass 2: body load + class 2/5 scoring ──
		for _, c := range bodyNeeded {
			entry, err := k.Store.LoadRef(c.ref)
			if err != nil {
				// Malformed file; skip silently.
				continue
			}
			body := entry.Body
			bodyLower := strings.ToLower(body)
			projMult := projectMultiplier(q.Project, c.ref.Metadata.Project)

			// Tier 2: exact phrase in body.
			if exactPhrase != "" && strings.Contains(bodyLower, exactPhrase) {
				results = append(results, Result{
					Ref:      c.ref,
					Score:    (3.0 + accessBoost(c.ref.Metadata.Accessed)) * projMult,
					Body:     body,
					Metadata: c.ref.Metadata,
					class:    classExactBody,
				})
				continue
			}

			// Tier 5: body keyword scoring.
			bodyScore := scoreBody(body, tokens)
			score := c.topicTagBase + bodyScore
			if score <= 0 {
				continue
			}
			score += accessBoost(c.ref.Metadata.Accessed)
			results = append(results, Result{
				Ref:      c.ref,
				Score:    score * projMult,
				Body:     body,
				Metadata: c.ref.Metadata,
				class:    classKeyword,
			})
		}
		// Re-sort after pass-2 additions.
		sortResultsByTier(results)
	}

	// results are already sorted by sortResultsByTier (in pass 1 for
	// short-circuit, then re-sorted in pass 2 if body candidates were
	// processed). The comparator is: class ASC → score DESC →
	// date DESC → path ASC. See sortResultsByTier below.

	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}

	// ── Pass 3: fuzzy topic/tag fallback (class 6) ──
	//
	// Runs only when earlier tiers didn't fill the limit AND the query
	// is long enough to produce meaningful fuzzy hits. Engram's
	// internal/project/similar.go taught us the length guard: short
	// queries ("go", "io") fuzzy-match too many things and drown
	// precision. For mastermind, the cutoff is len(query) >= 4 —
	// enough for a realistic typo-corrected word ("hook", "extr").
	//
	// Only metadata is fuzzy-matched (topic + tags blob). Body fuzzy
	// is deliberately rejected: fuzzy on prose explodes false
	// positives, and mastermind's "bad working-memory day" failure
	// mode is misremembering the *topic*, not the body. See
	// DECISIONS.md for the rejection rationale.
	normalizedQuery := strings.ToLower(strings.TrimSpace(q.QueryText))
	if len(results) < limit && len(normalizedQuery) >= 4 {
		seen := make(map[string]bool, len(results))
		for _, r := range results {
			seen[r.Ref.Path] = true
		}
		haystack := make([]string, 0, len(filtered))
		refIndex := make([]store.EntryRef, 0, len(filtered))
		for _, r := range filtered {
			if seen[r.Path] {
				continue
			}
			blob := strings.ToLower(r.Metadata.Topic + " " + strings.Join(r.Metadata.Tags, " "))
			haystack = append(haystack, blob)
			refIndex = append(refIndex, r)
		}
		matches := fuzzy.Find(normalizedQuery, haystack)
		for _, m := range matches {
			if len(results) >= limit {
				break
			}
			ref := refIndex[m.Index]
			// Heavy discount: the class 6 sort-position already
			// guarantees fuzzy hits land below any class 0-5 match.
			// Within class 6, sahilm's score orders by match quality.
			// We cap at 0.5 so fuzzy scores never approach the
			// keyword-class range in downstream consumers.
			fuzzyScore := float64(m.Score) / 100.0
			if fuzzyScore > 0.5 {
				fuzzyScore = 0.5
			}
			projMult := projectMultiplier(q.Project, ref.Metadata.Project)
			results = append(results, Result{
				Ref:      ref,
				Score:    (fuzzyScore + accessBoost(ref.Metadata.Accessed)) * projMult,
				Metadata: ref.Metadata,
				class:    classFuzzy,
			})
		}
		sortResultsByTier(results)
	}

	// ── Supersedes boost ──
	//
	// Within-class score multiplier for entries that explicitly
	// supersede other entries. The count is capped at 3 to prevent
	// gaming. Applied BEFORE the limit trim so a boosted entry that
	// was previously just outside the top-N can surface into it.
	// Class still dominates — the boost cannot bridge a class gap
	// because it only scales Score, and the comparator checks class
	// first.
	//
	// Note: contradicts is deliberately NOT included in the boost
	// count. Contradicts triggers co-retrieval below instead, which
	// is a stronger signal: the contradicting entry is pulled into
	// the result list regardless of its own keyword score. See
	// DECISIONS.md 2026-04-10 supersedes/contradicts entry for the
	// rationale (shiba-memory treats contradicts as a score booster;
	// mastermind treats it as a co-retrieval signal because
	// "contradicts gets the same boost as supports" is wrong under
	// our 'knowledge is never silently overridden' philosophy).
	for i := range results {
		linked := len(results[i].Metadata.Supersedes)
		if linked > 3 {
			linked = 3
		}
		if linked > 0 {
			results[i].Score *= 1.0 + float64(linked)*0.2
		}
	}
	sortResultsByTier(results)

	if len(results) > limit {
		results = results[:limit]
	}

	// ── Contradicts co-retrieval ──
	//
	// For every top-K result with a non-empty Contradicts list,
	// look up the listed slugs and append them as additional
	// results with a "(contradicts <topic>)" annotation. These
	// appended entries bypass the limit — they're co-retrieved
	// because of the relationship, not because of their own
	// keyword match. Dedupe against entries already in results,
	// cap total appended at 3 to keep the output block bounded.
	results = appendContradictsCoRetrieval(results, filtered, k)

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

	// Project filtering is a soft ranking signal by default — see
	// projectMultiplier and DECISIONS.md 2026-04-10. The hard filter
	// only kicks in when StrictProject is explicitly set by the
	// caller (CLI subcommands with --project foo, not the MCP tool).
	if q.StrictProject && q.Project != "" && !strings.EqualFold(md.Project, q.Project) {
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
// entry has been returned by previous searches.
//
// The shape is ACT-R "fast mode" base-level activation, borrowed from
// shiba-memory's 007_actr_proper.sql: a log of the access count. This
// rewards even a single access meaningfully (one access already proves
// the entry was useful once) and saturates quickly (the difference
// between 100 and 1000 accesses is negligible). Concretely:
//
//	accessed=1  → 0.139
//	accessed=5  → 0.358
//	accessed=10 → 0.479
//	accessed=12 → 0.497 (approaches cap)
//	accessed=20 → 0.5 (hard cap)
//
// The 0.5 cap preserves the load-bearing ranking invariant: a single
// topic hit (2.0 per token) always dominates any combination of access
// boost + tag + body. See TestKeywordSearcherRankingFavorsTopicOverBody.
//
// Why log-shaped instead of linear (the previous formula was
// accessed * 0.05 capped at 0.5): linear reached the cap in exactly
// 10 steps and treated the first access the same as the tenth
// (+0.05 each). Log shape front-loads the reward — one access moves
// the entry noticeably, subsequent accesses move it less — which
// matches ACT-R's intuition that repeated activations of an already-
// familiar item contribute diminishingly. Shiba-memory proved out
// this formula shape in a similar memory-retrieval context.
func accessBoost(accessed int) float64 {
	if accessed <= 0 {
		return 0
	}
	boost := math.Log(float64(accessed)+1) * 0.2
	if boost > 0.5 {
		return 0.5
	}
	return boost
}

// projectMultiplier returns the within-class score multiplier for an
// entry based on its project relative to the query's current project.
// The three cases:
//
//   - queryProject is empty: project boost disabled, return 1.0.
//   - entryProject matches queryProject (case-insensitive): 1.3× —
//     same-project entries earn a noticeable promotion within their
//     tier class (a within-class tiebreaker, never a cross-class jump).
//   - entryProject is empty or "general": 1.0× — cross-project-by-
//     design entries stay neutral. "general" is mastermind's convention
//     for lessons that apply across any project (see mm_write docs).
//   - otherwise: 0.8× — a different project's lesson is demoted but
//     not dropped. A heavily-accessed cross-project entry can still
//     surface above a weaker same-project one, which is the whole
//     point of making this a multiplier instead of a hard filter.
//
// Borrowed from shiba-memory's 002_profiles_scoping.sql:129-133
// (1.3 / 1.0 / 0.8 weights, same semantics). The values were
// adopted verbatim because shiba-memory has validated them in a
// similar retrieval context and there's no a-priori reason to
// re-tune. Adjust if dogfooding shows cross-project noise.
//
// The multiplier applies to the FULL Score (match-score + access
// boost), so it scales proportionally. Class is not affected —
// this is purely a within-class ranking signal.
func projectMultiplier(queryProject, entryProject string) float64 {
	if queryProject == "" {
		return 1.0
	}
	if entryProject == "" || strings.EqualFold(entryProject, "general") {
		return 1.0
	}
	if strings.EqualFold(entryProject, queryProject) {
		return 1.3
	}
	return 0.8
}

// maxCoRetrievedContradicts caps how many contradicts co-retrieved
// entries can be appended to a single search result block. Keeps
// the output bounded when a single entry contradicts many others.
const maxCoRetrievedContradicts = 3

// appendContradictsCoRetrieval implements the contradicts
// co-retrieval pass. For every top-K result that has a non-empty
// Contradicts list, it looks up the listed slugs in the filtered
// corpus and appends them as additional Results with a
// "(contradicts <topic>)" Annotation. Appended entries bypass the
// limit — the co-retrieval relationship is the reason they surface,
// not their own keyword score.
//
// Dedupes against entries already in results so an entry that
// naturally matched the query AND is contradicted by another top
// result is not duplicated. Caps the total appended entries at
// maxCoRetrievedContradicts.
//
// Slug matching uses the filename without extension, which is the
// convention mastermind uses for entry identifiers (see
// internal/store for the slug generation). Dangling slugs (no
// matching file in the filtered corpus) are silently skipped,
// consistent with hard rule #7 — broken links surface for review,
// never silently erase.
func appendContradictsCoRetrieval(results []Result, filtered []store.EntryRef, k *KeywordSearcher) []Result {
	// Collect all contradicts slugs from the top-K results.
	seen := make(map[string]bool, len(results))
	for _, r := range results {
		seen[slugFromPath(r.Ref.Path)] = true
	}

	type pendingCo struct {
		slug       string
		annotation string
	}
	var pending []pendingCo
	pendingSet := make(map[string]bool)
	for _, r := range results {
		for _, slug := range r.Metadata.Contradicts {
			slug = strings.TrimSpace(slug)
			if slug == "" || seen[slug] || pendingSet[slug] {
				continue
			}
			pending = append(pending, pendingCo{
				slug:       slug,
				annotation: fmt.Sprintf("contradicts %q", r.Metadata.Topic),
			})
			pendingSet[slug] = true
			if len(pending) >= maxCoRetrievedContradicts {
				break
			}
		}
		if len(pending) >= maxCoRetrievedContradicts {
			break
		}
	}
	if len(pending) == 0 {
		return results
	}

	// Look up each pending slug in the filtered corpus. The filtered
	// slice is already scoped to the query's scope/kind filters, so
	// we respect the caller's intent — a contradicts target in a
	// filtered-out scope won't surface.
	for _, pc := range pending {
		for _, ref := range filtered {
			if slugFromPath(ref.Path) != pc.slug {
				continue
			}
			// Load the body lazily; co-retrieved entries need it for
			// presentation just like normal results.
			var body string
			if entry, err := k.Store.LoadRef(ref); err == nil {
				body = entry.Body
			}
			results = append(results, Result{
				Ref:        ref,
				Score:      0, // co-retrieved, not score-ranked
				Body:       body,
				Metadata:   ref.Metadata,
				Annotation: pc.annotation,
				class:      classKeyword, // arbitrary — sort pass doesn't re-order co-retrieved entries
			})
			break
		}
	}
	return results
}

// slugFromPath returns the entry slug from its full path — the
// filename with the .md extension stripped. Mastermind's store
// uses slugs as the stable identifier across file moves within
// a scope, so two entries with the same slug but different
// directories are considered the same logical entry.
func slugFromPath(path string) string {
	base := path
	// Strip directory.
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	// Strip .md extension.
	return strings.TrimSuffix(base, ".md")
}

// sortResultsByTier orders results by the tiered-fallback comparator:
// class ASC (strictly dominant) → score DESC → date DESC → path ASC
// (deterministic). Pulled out of Search() so pass 1 can check the
// short-circuit condition on a sorted slice before deciding whether
// pass 2 is needed.
func sortResultsByTier(results []Result) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].class != results[j].class {
			return results[i].class < results[j].class
		}
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Metadata.Date != results[j].Metadata.Date {
			return results[i].Metadata.Date > results[j].Metadata.Date
		}
		return results[i].Ref.Path < results[j].Ref.Path
	})
}

// shouldShortCircuit returns true when pass-1 metadata-only results
// already meet the confidence bar, so pass 2 (body loading) can be
// skipped entirely. Rules:
//
//  1. The top-K results (K = min(limit, 3)) must all be in class ≤ 4
//     (classExactTopic/ExactTag/TopicTokens/MetaTokens — all structural
//     metadata hits).
//  2. At least one of those must have access_count ≥ 3 ("earned"
//     confidence — borrowed from shiba-memory's instinct-evolution
//     gate at 003_instincts_tracking_gateway.sql:35,50-52).
//
// The access gate is the critical second condition. Without it, any
// structural match would short-circuit regardless of whether the
// entry has ever been useful — we'd risk burying genuinely better
// body matches behind stale topic hits. Requiring access_count ≥ 3
// means the short-circuit only fires when the user's own history
// has validated one of the pass-1 hits as actually useful.
//
// Results must be sorted before this is called.
func shouldShortCircuit(results []Result, limit int) bool {
	if limit <= 0 {
		limit = 10
	}
	k := limit
	if k > 3 {
		k = 3
	}
	if len(results) < k {
		return false
	}
	// Rule 1: top-K must all be class ≤ 4.
	for i := 0; i < k; i++ {
		if results[i].class > classMetaTokens {
			return false
		}
	}
	// Rule 2: at least one of those must be "earned" (access_count ≥ 3).
	for i := 0; i < k; i++ {
		if results[i].Metadata.Accessed >= 3 {
			return true
		}
	}
	return false
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
