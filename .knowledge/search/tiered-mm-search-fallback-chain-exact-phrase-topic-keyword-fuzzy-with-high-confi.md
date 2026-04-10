---
date: "2026-04-10"
project: mastermind
tags:
  - phase-4
  - search
  - fallback
  - tiered
  - short-circuit
  - soulforge
topic: Tiered mm_search fallback chain (exact phrase → topic → keyword → fuzzy) with high-confidence short-circuit
kind: open-loop
scope: project-shared
category: search
confidence: high
---

## What's open
Soulforge's 4-tier code-intelligence fallback (LSP → ts-morph → tree-sitter → regex) degrades gracefully when the precise tool fails. Mastermind's `mm_search` is single-tier today: context-mode FTS5 + topic-dominant ranking + access frequency, all combined in one pass.

A tiered version: **exact-phrase match → topic overlap → keyword match → fuzzy match**. If tier N returns ≥K high-confidence hits, stop and return them. Else widen the net to tier N+1.

## Why it matters
Exact-phrase hits are much more relevant than keyword hits but much rarer. Today they're mixed together in ranking and keyword noise can drown them when the corpus is large. Short-circuiting at the tier with confident hits is the same pattern as the extractor's gap-fill short-circuit (already in the `extraction` open-loop) — apply it to search too.

## Next action — logging first, refactor later
- `internal/search/`: add a `tier` field to each result and log which tier produced the hit. NO behavior change yet.
- Dogfood for a few weeks with logging on; verify the assumption — are exact-phrase hits actually getting ranked below keyword hits in practice?
- Only if the logging confirms the problem: refactor the ranking pipeline into explicit tiers with a `MinConfidentHits` threshold per tier and the short-circuit.

## Why parked
Possibly unnecessary — the current combined ranker may already be good enough at mastermind's current corpus size (~35 entries). The logging step is the gate. If exact-phrase hits aren't actually getting buried, SKIP the refactor entirely and close this loop.

## Source
Second-pass survey of soulforge. 4-tier fallback pattern from soulforge `docs/architecture.md`; conversation 2026-04-10.
