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
