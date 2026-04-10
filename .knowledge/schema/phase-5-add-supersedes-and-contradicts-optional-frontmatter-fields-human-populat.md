---
date: "2026-04-10"
project: mastermind
tags:
  - phase-5
  - schema
  - format
  - relations
  - review
  - shiba-memory
  - soulforge
  - mempalace
topic: 'Phase 5+: add supersedes and contradicts optional frontmatter fields, human-populated via /mm-review prompts'
kind: open-loop
scope: project-shared
category: schema
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## What's open
Add two new optional frontmatter fields to FORMAT.md:
- `supersedes: [slug1, slug2]` — this entry explicitly replaces older ones (without deleting them).
- `contradicts: [slug1, slug2]` — flags a tension for surfacing at query time.

FORMAT.md is immutable for existing fields, but the contract allows additive extensions. This is schema-compatible.

## Why it matters
Mastermind's hard rule #7 is "knowledge is never silently deleted." `supersedes` is the explicit-replacement version of that rule — a new decision overrides an old one without losing history. `contradicts` surfaces conflicts instead of burying them. Both capabilities are load-bearing for long-corpus quality (target: run this tool for 10+ years, win the 2034 bug).

Three independent references converged on this design: shiba-memory's six relation types (`related, supports, contradicts, supersedes, caused_by, derived_from`), soulforge/MemPalace's temporal invalidation pattern (`kg.invalidate()` instead of delete), and the prior project-shared shiba-memory insight entry from 2026-04-09.

## Next action
- `docs/FORMAT.md`: add the two fields as optional with validation rules (slugs must resolve to existing entries in the same scope).
- `internal/format/`: parse + validate + marshal the new fields.
- `/mm-review` skill: when a candidate's body meaningfully overlaps an existing live entry (string/topic overlap — NOT embedding similarity), prompt the human to mark the supersede/contradict relation. No automatic detection.
- `internal/search/`: when mm_search returns entry A, boost entries that supersede A in ranking and surface entries that contradict A alongside (not instead).

## Critical constraint — do NOT auto-populate
Shiba's own README admits their automatic contradiction detection is "embedding dissimilarity as a proxy" and is shallow. Mastermind stays human-populated first. The review step IS the learning step — auto-population would bypass consolidation, which defeats the whole pending/ loop.

## Source
`docs/reference-notes/soulforge.md` §2, §5 item 3; `docs/reference-notes/shiba-memory.md` §2, §8 item 4.

## 2026-04-10 — Mining pass update

Dug into the actual shiba-memory and mempalace implementations during the second mining pass. Confirmations and concrete deltas:

**Confirmed (adopt verbatim)**: shiba-memory's `memory_links` schema at `~/Github/shiba-memory/schema/001_init.sql:63` uses exactly the two relation types mastermind wants (`supersedes`, `contradicts`) plus four others — but the other four are rarely populated. Only `related` fires automatically (via pgvector embedding similarity), and mastermind has no embeddings, so the manual-only policy maps cleanly onto mastermind's human-populated-first rule.

**Concrete boost formula** (shiba `001_init.sql:239-243`, repeated in `007_actr_proper.sql:121`):

```go
// For each result: scan the frontmatter supersedes/contradicts slugs
// and boost proportionally. Applied as a within-class multiplier
// after the project-boost multiplier — class still dominates.
linkedCount := len(entry.Supersedes) + len(entry.Contradicts)
if linkedCount > 3 {
    linkedCount = 3 // cap to prevent gaming
}
score *= 1.0 + float64(linkedCount)*0.2
```

Default link strength is 1.0 (not stored — binary presence). One link: 1.2×. Two links: 1.4×. Three links (cap): 1.6×. Still within-class; still cannot bridge a class gap.

**Critical divergence from shiba**: shiba's boost is relation-type-agnostic — `contradicts` gets the same multiplier as `supports`. For mastermind, `contradicts` should NOT be a score booster; it should be a **co-retrieval signal** ("when entry A appears in results, also show entries that contradict A, alongside not instead"). This is a better fit for mastermind's "knowledge is never silently overridden" philosophy. Implementation: when a top result has a `contradicts: [X]` list, include X in the output with a `(contradicts #N)` annotation regardless of its own keyword score.

**Directionality**: shiba stores links directed (source_id → target_id) but the search boost treats them undirected (`source_id OR target_id`). For mastermind, keep directed storage (entry A's frontmatter lists entries A supersedes, not vice versa) and undirected boost (both A and the entries A supersedes get the boost). Simpler than tracking back-references.

**Deletion note**: shiba uses `ON DELETE CASCADE` to auto-prune links. Mastermind's hard rule #7 says knowledge is never silently deleted, so cascade is wrong. If an entry is somehow deleted (shouldn't happen), leave referencing slugs dangling — that surfaces a broken link for human review rather than silently erasing the relationship.

**Mempalace palace_graph non-borrow**: `traverse()` and `find_tunnels()` rebuild the graph from ChromaDB metadata on every call. Works for small corpora via vector store metadata; would require walking every `.md` frontmatter for mastermind, which breaks the "no persistent index" assumption at scale. Defer a navigation tool until corpus size forces the question.

**Updated next action**:
1. `docs/FORMAT.md` — additive extension: document `supersedes: [slug]` and `contradicts: [slug]` as optional string arrays. Validation is best-effort (dangling slugs log a warning, don't fail parse).
2. `internal/format/` — parse + marshal the two slice fields. No validation changes (slugs are opaque strings).
3. `internal/search/` — apply the 10-line boost formula in the scoring loop. `contradicts` does NOT contribute to the boost; it triggers the co-retrieval annotation instead.
4. `internal/search/` — co-retrieval: after top-K is assembled, pull in entries listed in the top-K's `contradicts` fields (capped at +3 entries) with a `(contradicts <topic>)` tag in the output markdown.
5. `/mm-review` skill — when a candidate's body text contains phrases like "replaces", "supersedes", "contradicts" OR when topic-string overlap with a live entry exceeds a threshold, prompt the human to populate the relation. No automatic population.

References: `docs/reference-notes/shiba-memory.md` §2, mining report 2026-04-10 (Agent a06eeb60 — knowledge-graph relations), shiba-memory `schema/001_init.sql:63`, `007_actr_proper.sql:121`, `cli/src/commands/link.ts:9-24`.
