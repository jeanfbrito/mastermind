---
date: "2026-04-10"
project: mastermind
tags:
  - search
  - short-circuit
  - architecture
  - performance
topic: Two-pass searcher structure enables clean short-circuit on body I/O
kind: pattern
scope: project-shared
category: search
confidence: high
---

## Pattern

When a search pipeline has a cheap metadata path and an expensive body-load path, don't interleave them in a single loop. Structure as two passes:

- **Pass 1**: walk all filtered candidates, score on metadata only, collect results. No body reads. Entries that can't be classified without body content get deferred to pass 2 candidates.
- **Pass 2**: walk the deferred candidates, load body, score body. Append to results.

Between passes, sort the pass-1 results and check whether they satisfy a confidence threshold. If yes, skip pass 2 entirely — no body reads, no expensive I/O.

## Why

A single-loop searcher with inline body I/O makes short-circuiting hard. The decision "should I load this body or not" has to happen before you know whether pass 1 already had confident results. You end up either (a) always loading, defeating short-circuit, or (b) loading conditionally on per-entry state that's too local to see the global picture.

Two-pass structure moves the confidence check to a natural seam: after pass 1 is complete, before pass 2 starts. At that point you can see the full pass-1 result set and decide cleanly.

## How to apply

- Collect pass-1 results into a separate slice from the final results, or track which results came from pass 1 vs pass 2.
- The confidence check is simple: top-K pass-1 results all strong enough + at least one validated as useful.
- Pass 2 merges into the same result slice after; re-sort at the end.
- Lazy body load after final sort handles the case where top-N display still needs body text for rendering (even for entries that matched metadata-only).

Mastermind's `internal/search/search.go` (2026-04-10): `bodyNeeded := []bodyCandidate{}` is the pass-2 deferred list. `shouldShortCircuit(results, limit)` gates pass 2 execution. Tested via `shortCircuitCount` counter on `KeywordSearcher`.
