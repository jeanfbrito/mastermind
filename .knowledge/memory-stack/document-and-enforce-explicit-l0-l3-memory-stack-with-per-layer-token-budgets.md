---
date: "2026-04-10"
project: mastermind
tags:
  - phase-3
  - session-start
  - token-budget
  - memory-stack
  - mempalace
  - documentation
topic: Document and enforce explicit L0-L3 memory stack with per-layer token budgets
kind: open-loop
scope: project-shared
category: memory-stack
confidence: high
accessed: 2
last_accessed: "2026-04-11"
---

## What's open
Mastermind already has an implicit memory stack shaped exactly like MemPalace's L0-L3 design, but the layers are undocumented and there are no budget ceilings. Make them explicit, add soft budgets per layer, and tie the output-trimming work to the L2 ceiling.

**MemPalace's L0-L3 stack** (from `~/Github/mempalace` README → Memory Stack section):

| Layer | What | Size | When |
|---|---|---|---|
| L0 | Identity — who is this AI? | ~50 tokens | Always loaded |
| L1 | Critical facts — team, projects, preferences | ~120 tokens | Always loaded |
| L2 | Room recall — recent sessions, current project | On demand | When topic comes up |
| L3 | Deep search — semantic across all closets | On demand | When explicitly asked |

**Mastermind's current implicit shape**:

| Layer | Mastermind equivalent | Current state |
|---|---|---|
| L0 | Open-loops header in SessionStart injection | Exists, no size cap |
| L1 | Project knowledge entries in SessionStart injection | Exists, no size cap |
| L2 | `mm_search` results on demand | Exists, returns full bodies today |
| L3 | Reading `.knowledge/…` files directly via Read tool | Implicit, not documented |

## Why it matters
- **Prevents SessionStart injection creep** as the corpus grows. Without a budget, every new open-loop and project entry pays a tax on every session forever. Budget-aware design catches the creep before it becomes a real problem.
- **Gives the output-trimming open-loop (category `search`) a concrete target**: "L2 `mm_search` default responses stay under X tokens; use `expand: true` for L3."
- **Makes hot-path cost legible**. Every session pays L0+L1. Knowing the ceiling forces honest prioritization of what lives in the always-loaded layer.
- **Validates and protects a load-bearing design choice**. Three independent memory systems (MemPalace, shiba-memory, mastermind) all chose similar always-loaded-plus-on-demand tiering. Documenting it protects it against accidental drift.

## Next action (documentation first)
- Add a new section to `docs/ARCHITECTURE.md` (or a new `docs/MEMORY-STACK.md`) documenting the four layers, what lives in each, and the soft budgets.
- **Proposed soft budgets** (tune against real sessions):
  - **L0 — open-loops header**: 500 tokens. Current open-loop count is 12; at ~40 tokens per one-line entry that's 480. The header should stay a scannable summary, not a body dump.
  - **L1 — project knowledge injection**: 2000 tokens. Enough room for ~5-8 entry topics + first-section excerpts.
  - **L2 — `mm_search` result default**: 200 tokens per result (topic + first ## section + match excerpt). Escape hatch via `expand: true` moves to L3.
  - **L3 — full entry content**: unbounded (it's already a direct file read, cost is on the caller).
- Update `internal/mcp/serverInstructions.go` and the SessionStart subcommand to explain the stack to the agent.

## Next action (enforcement follow-on)
- `cmd/mastermind/session-start`: measure the tokens emitted for L0 + L1 on each run. Log a warning (to the silent-unless-needed log file) if the budget is exceeded. Do NOT truncate silently — surface the problem so the user can decide what to demote to L2.
- Pair with the `search` category open-loop "Trim mm_search output" — that loop IS the L2 budget enforcement, so closing one helps close the other.

## Why this deserves its own loop (vs. merging into the output-trimming loop)
The L0/L1 budgets are about **SessionStart injection**, not `mm_search`. Different code path, different enforcement point, different proposed numbers. Keeping them separate keeps the scope of each loop clear. The output-trimming loop is the L2 half of this same hierarchy; this loop is the L0+L1 half.

## Progress (2026-04-10)
- **Documentation: DONE.** Landed in commit `038e028` (branch) + `a253cde` (merge) as `docs/MEMORY-STACK.md`. All four layers documented with soft budgets (L0=500, L1=2000, L2=200/result, L3=unbounded). Mapped to MemPalace's original table. Added to CLAUDE.md "Read these in order" list as item 4.
- **L2 runtime enforcement: DONE.** Part of the same commit (the mm_search output-trim loop that closed alongside this one). `internal/search/excerpt.go` `BodyExcerpt` enforces the 200-tokens-per-result target by default, with `expand: true` as the L3 escape hatch. Benchmark: 53% token savings per result on a realistic war-story entry.
- **L0/L1 runtime enforcement: STILL OPEN.** No token measurement or warning log in `cmd/mastermind/session-start` yet. Documented-only for now. Enforcement is gated on observing actual SessionStart injection bloat — at the current corpus size (~35 live entries + 10 open-loops) the budget is comfortably under-spent. Revisit when the corpus passes ~100 entries or when a session-start dump visibly pushes past the soft budgets.

Loop stays open for the L0/L1 runtime enforcement half.

## Source
`docs/reference-notes/mempalace.md` §3. MemPalace's own memory stack lives in their README's "The Memory Stack" section. Conversation 2026-04-10.
