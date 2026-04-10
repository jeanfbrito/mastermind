---
date: "2026-04-10"
project: mastermind
tags:
  - search
  - ranking
  - open-loop
  - shiba-memory
  - scope
topic: Project-boost as ranking multiplier instead of hard scope filter
kind: open-loop
scope: project-shared
category: search/ranking
confidence: high
---

## What

Convert mastermind's scope filter into a soft ranking signal: 1.3× same-project, 1.0× user-personal/global, 0.8× cross-project. Applied multiplicatively to the final score in `internal/search/search.go`.

Today, `matchesMetadataFilters` treats `Project` as a hard filter — cross-project entries are dropped entirely from results. This prevents a highly-useful cross-project entry from ever surfacing, even when nothing project-local is relevant.

## Why

From the 2026-04-10 reference-repo sweep: shiba-memory's `002_profiles_scoping.sql:129-133` implements this as a multiplier, not a filter. The insight is that cross-project matches are *less* relevant but not *irrelevant* — a lesson from another project may be the only hit for a novel query. Converting scope from filter → ranking preserves that long-tail discoverability.

Bigger change than the T1-T7 tiered fallback work because it affects the metadata pre-filter path and changes the `Query.Project` contract. Worth its own DECISIONS.md entry and proposal.

## How to apply

1. Replace the `q.Project != "" && !strings.EqualFold(md.Project, q.Project)` hard-filter at `search.go:277-279` with a post-scoring multiplier.
2. Compute scope class per entry: `same-project` / `general` / `other-project`.
3. Multiply final score by 1.3 / 1.0 / 0.8 respectively.
4. Add a `Query.StrictProject bool` field for callers that genuinely need hard filtering (e.g., CLI `--project foo` when user wants only-foo).
5. Update test `TestKeywordSearcherFilterByProject` — behavior changes from "returns only rocket-chat results" to "ranks rocket-chat above mastermind results."
6. New DECISIONS.md entry explaining the shift.

Defer until after T1-T7 tiered fallback ships, so the tier-class sort is stable before layering project multipliers on top.

## Resolution

Shipped 2026-04-10. Query.Project is now a within-class score multiplier (1.3x same-project, 1.0x general, 0.8x cross-project) instead of a hard filter. Escape hatch: Query.StrictProject=true restores the old hard-filter behavior for CLI callers that need strict scoping. 5 new tests in internal/search lock in the soft-filter semantics, strict-filter escape hatch, multiplier matrix, within-class boundedness, and the updated matchesMetadataFilters behavior. Borrowed verbatim from shiba-memory's 002_profiles_scoping.sql weights. Full rationale in docs/DECISIONS.md 2026-04-10 entry.
