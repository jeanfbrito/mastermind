---
date: "2026-04-10"
project: mastermind
tags:
  - search
  - ranking
  - open-loop
  - shiba-memory
  - act-r
  - recency
topic: Proper-mode ACT-R with per-access timestamp array for recency-aware scoring
kind: open-loop
scope: project-shared
category: search/ranking
confidence: high
accessed: 3
last_accessed: "2026-04-10"
---

## What

Replace mastermind's count-only ACT-R fast mode (`1 + ln(accessed+1) * 0.2`, landed 2026-04-10 in `internal/search/search.go`) with canonical ACT-R base-level activation:

```
B_i = ln(Σ age_j^(-0.5))
```

where `age_j` is the elapsed seconds since the j-th access. Requires storing a timestamp array in frontmatter (e.g. `access_log: [2026-04-01T12:00:00Z, 2026-04-03T09:15:00Z, …]`) instead of just a single `accessed` integer.

## Why

Fast mode is count-only — it can't distinguish "accessed 10 times last year" from "accessed 10 times last week." Proper mode captures both frequency and recency in one formula: recent accesses contribute more because `age^(-0.5)` shrinks as age grows. Reference: shiba-memory `007_actr_proper.sql:9-29`, which validates the 0.5 decay parameter as the standard ACT-R default.

## How to apply

ONLY if dogfooding reveals that count-only boosting promotes stale entries that were heavily accessed long ago but are no longer useful. Symptoms to watch for:
- Top search result is an entry I stopped using months ago
- "Newly discovered gold" entries get buried under old-favorite noise
- Access-boost cap saturates on everything useful, destroying tiebreaker utility

If those symptoms appear:
1. Extend `format.Metadata` with `AccessLog []time.Time` field in frontmatter (FORMAT.md is immutable for *existing* fields — adding an optional new field is allowed, confirm via DECISIONS.md entry first).
2. On each access in `Store.IncrementAccess`, append now() to the array (cap length at e.g. 50 entries to prevent unbounded growth).
3. Replace `accessBoost(accessed int)` with `actrProperBoost(log []time.Time, now time.Time)` implementing `B_i = ln(Σ age_j^(-0.5))`.
4. Multiply result by `0.1`, cap at 0.3 (multiplicative) OR cap at 0.5 (additive) to preserve topic-dominance invariant.
5. Keep fast mode behind an env var for A/B comparison during rollout.

Phase 4+ work. Do NOT touch until fast mode shows recency errors in real use.
