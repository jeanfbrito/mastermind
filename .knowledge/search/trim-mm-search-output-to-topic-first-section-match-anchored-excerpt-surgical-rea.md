---
date: "2026-04-10"
project: mastermind
tags:
  - phase-3
  - mm-search
  - output
  - token-budget
  - session-start
  - soulforge
topic: Trim mm_search output to topic + first section + match-anchored excerpt (surgical reads analog)
kind: open-loop
scope: project-shared
category: search
confidence: high
---

## What's open
Today `mm_search` returns full entry bodies in its markdown response. For long entries (war-stories, multi-section insights, decision records) this floods the caller's context window — a real problem for session-start injection, which runs on every session under a tight token budget.

Trim to: `topic + first ## section + excerpt of the body anchored on the search match`. Provide full body via either (a) an explicit `expand: true` flag on the search call, or (b) the caller reading the file path directly (the path is already returned in every result).

## Why it matters
Soulforge's "surgical reads" extract exact symbols from 500-line files instead of dumping the full file. Same idea applied to memory: don't return the whole entry when the match is in a single paragraph. Session-start injection is the hot path — every token saved there compounds across every session, forever.

## Next action
- `internal/search/`: add an excerpt extractor that finds the match position in the body and returns ±3 lines around it, or the first `##` section if no specific match location (pure keyword hit).
- `internal/mcp/`: the default `mm_search` response uses the trimmed form; add an `expand: true` escape hatch that returns full bodies for callers that need them.
- Update session-start injection to use the trimmed default explicitly.
- Benchmark token impact on a real run: pick a session with ~5 open-loops + ~5 project entries and measure before/after.

## Constraints
- The file path in the result must stay — callers may need to Read the full file via native tools.
- Don't trim so aggressively that the match becomes uninterpretable out of its surrounding context. Prefer ±3 lines over ±0.
- No lossy summarization — excerpt only, never paraphrase.

## Source
Second-pass survey of soulforge. Surgical-reads pattern from soulforge `docs/architecture.md`; conversation 2026-04-10.
