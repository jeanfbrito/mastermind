---
date: "2026-04-10"
project: mastermind
tags:
  - search
  - benchmark
  - token-savings
  - mm-search
  - excerpt
  - l2
topic: mm_search default-trim yields ~53% tokens saved per result on realistic entries
kind: insight
scope: project-shared
category: search
confidence: high
accessed: 2
last_accessed: "2026-04-11"
---

## The measurement
Benchmarked `mm_search`'s new default-trim output against full-body output on a realistic 1294-char war-story entry:

- Before (full body): ~323 tokens per result
- After (trimmed excerpt): ~151 tokens per result
- **Savings: 53% per result**

At 5 results per session-start `mm_search` call, that's ~860 tokens saved **per session**, compounding across every session forever. The savings scale directly with corpus size and entry length — longer entries save more.

## How the trim works
`internal/search/excerpt.go` `BodyExcerpt(body, query)` returns:
- Full body if under 800 chars (trimming is silly at that size)
- Topic + first `##` section + match-anchored ±3-line excerpt when there's a query match
- First `##` section when the match is on the topic line (no body match)
- Word-trim fallback when there's no `##` section structure

## Escape hatch
Callers that need full content use `expand: true` on the `mm_search` input (the L3 layer of the memory stack). The file path is always emitted in each result, so callers can also `Read` the file directly for unbounded L3 access.

## Why record this number
Future changes to mm_search (ranking tweaks, new filters, etc.) should preserve or improve this baseline. If a future refactor regresses below ~50% savings, something is wrong. Also useful when arguing that L0/L1 runtime enforcement is worth implementing — the L2 side proved budgets work.

## Source
Commit 038e028 + `docs/reference-notes/mempalace.md` §3 (the L0-L3 design that motivated the L2 budget).
