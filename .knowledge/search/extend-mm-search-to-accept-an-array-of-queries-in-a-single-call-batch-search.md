---
date: "2026-04-10"
project: mastermind
tags:
  - phase-4
  - mcp
  - mm-search
  - batch
  - soulforge
  - tool-surface
topic: Extend mm_search to accept an array of queries in a single call (batch search)
kind: open-loop
scope: project-shared
category: search
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## What's open
`mm_search` takes a single query string today. Skills that run multiple searches (`/mm-discover`, `/mm-review`, `/dream`) make N round-trips where they could make one. Extend the tool's schema to accept `queries: []string` (the singular `query` stays for backward compat), returning results grouped per query.

## Why it matters
Round-trips dominate cost for multi-search skills. Hard rule #6 says "four MCP tools forever" — it says nothing about their schemas. A schema extension is fair game. Soulforge's `read` tool batches parallel + surgical for the exact same reason.

## Next action
- `internal/mcp/`: extend the `mm_search` input struct with an optional `queries []string` alongside the existing `query` string. Exactly one of the two must be set (enforce in the tool handler).
- `internal/search/`: add a `SearchBatch(queries []string)` that dedupes repeat candidates across queries and returns a `map[query][]Result`.
- Output format: group by query in the markdown response with a per-query summary header.
- Update the `mm_search` description in `internal/mcp/serverInstructions.go` so the agent knows batch is supported.

## Constraints
Keep the single-query path free of overhead. Do NOT hide the batch API behind an env var — it's a clean schema extension, not a feature flag.

## Source
Second-pass survey of soulforge (`~/Github/soulforge`) against mastermind's tool surface. Soulforge `docs/compound-tools.md`; conversation 2026-04-10.
