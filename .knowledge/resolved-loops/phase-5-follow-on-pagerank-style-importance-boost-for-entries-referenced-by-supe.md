---
date: "2026-04-10"
project: mastermind
tags:
  - phase-5
  - search
  - ranking
  - pagerank
  - relations
  - soulforge
  - blocked
topic: 'Phase 5+ follow-on: PageRank-style importance boost for entries referenced by supersedes/contradicts links'
kind: open-loop
scope: project-shared
category: search
confidence: high
accessed: 1
last_accessed: "2026-04-11"
---

## What's open
Once the `supersedes` / `contradicts` frontmatter schema lands (separate open-loop in `schema/`), add a link-based importance signal to `mm_search` ranking: entries referenced by many others (either superseded by newer decisions, or flagged as contradicting other entries — both count as "load-bearing enough for a link") rank higher.

Conceptually this is PageRank over the knowledge graph. At mastermind scale (hundreds to low thousands of entries) a single-pass incoming-link count is enough — no iterative eigenvector computation needed.

## Why it matters
Soulforge ranks files by PageRank importance in the repo graph because "which files matter most" is a better signal than "which files match your query most" for many tasks. Same logic applies to a long-lived knowledge corpus: an entry that 12 newer entries supersede is clearly a load-bearing decision, not dead weight — and today mastermind has no way to detect that.

## Why blocked
Depends on the relations schema in the `schema/` open-loop. No-op until that ships. When it does, this loop can be closed into it as a follow-on deliverable, OR kept open to track the ranking-side work separately.

## Next action (when unblocked)
- `internal/search/`: compute an incoming-link count per entry at search time (cheap — relations are explicit frontmatter fields, not discovered).
- Add a small weight in the ranking score (start with `+ 0.1 * log(1 + incoming_links)`; tune against real corpus once relations exist).
- Ensure the boost doesn't drown out the existing topic-dominance and access-frequency signals. Weight should be comparable, not dominant.

## Source
Second-pass survey of soulforge. PageRank repo ranking from soulforge `docs/repo-map.md`; conversation 2026-04-10.

## Resolution

Landed in internal/search/search.go: incoming-link boost (supersedes + contradicts), 1 + 0.1*ln(1+n), capped at +0.3, computed over scope-gathered refs. Coexists with the existing outgoing supersedes boost. 2 new tests in relations_test.go locking class invariant. DECISIONS.md entry 2026-04-10 has full rationale.
