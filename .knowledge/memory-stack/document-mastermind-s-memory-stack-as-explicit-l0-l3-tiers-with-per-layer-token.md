---
date: "2026-04-10"
project: mastermind
tags:
  - memory-stack
  - session-start
  - budget
  - design
  - mempalace
  - decision
topic: Document mastermind's memory stack as explicit L0-L3 tiers with per-layer token budgets
kind: decision
scope: project-shared
category: memory-stack
confidence: high
accessed: 3
last_accessed: "2026-04-11"
---

## Decision
Mastermind's implicit context-tiering is now documented as an explicit L0-L3 stack with soft budgets per layer (see `docs/MEMORY-STACK.md`, commit 038e028):

| Layer | What | Budget | When loaded |
|---|---|---|---|
| L0 | Open-loops header in SessionStart | 500 tokens | Always |
| L1 | Project knowledge in SessionStart | 2000 tokens | Always |
| L2 | `mm_search` default result excerpt | 200 tokens/result | On demand |
| L3 | Direct `.knowledge/…` file reads | unbounded | Explicit |

## Why
Borrowed from MemPalace's L0/L1/L2/L3 table (their README → Memory Stack). The shape was already implicit in mastermind but had no budgets and no enforcement. Making it explicit:
- Prevents SessionStart injection creep as the corpus grows
- Gives the mm_search trim a concrete target (the L2 budget)
- Makes hot-path cost legible (every session pays L0 + L1)
- Protects a load-bearing design choice against drift (three independent memory systems converged on this shape)

## Current enforcement
- L2 is enforced in code via `internal/search/excerpt.go` `BodyExcerpt`. Measured 53% token savings on a realistic war-story entry.
- L0 and L1 are documentation-only for now. Runtime enforcement (warn if SessionStart output exceeds budget) is an open-loop in `.knowledge/memory-stack/`.

## Guard rule
Adding a new piece of always-loaded context? Check which layer it lives in and whether it fits under the budget. If it doesn't, demote to on-demand (L2) rather than growing the budget silently.
