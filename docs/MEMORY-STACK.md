# MEMORY-STACK.md — mastermind's L0-L3 memory model

## Why explicit tiering matters

Without a budget, every session pays for every entry in the corpus.
Open loops multiply, project knowledge accumulates, and `mm_search`
returns full bodies on every cold query. The result is a tool that
works well with 10 entries and poorly with 200. The L0-L3 model
makes the cost structure explicit so each layer can be tuned
independently and the always-loaded layers (L0, L1) stay bounded as
the corpus grows. Three independent memory systems (MemPalace,
shiba-memory, and mastermind) all converged on a similar tiered
design — that convergence is evidence the pattern is right, not
coincidence.

---

## Layer table

| Layer | What lives there | Soft budget | When loaded | Enforcement |
|-------|-----------------|-------------|-------------|-------------|
| L0 | Open-loop topics (session-start injection) | 500 tokens | Every session, automatically | None yet — documentation only |
| L1 | Project knowledge (session-start injection) | 2 000 tokens | Every session, automatically | None yet — documentation only |
| L2 | `mm_search` default response (trimmed excerpt) | ~200 tokens/result | On-demand, when agent calls mm_search | Enforced by `BodyExcerpt()` in `internal/search/excerpt.go` |
| L3 | Full entry body (expand:true or direct Read) | Unbounded | Explicit agent action | N/A — caller pays |

---

## Layer details

### L0 — Open loops (~500 tokens)

Open loops are in-progress work captured as `kind: open-loop` entries.
They appear first in the session-start injection block because forgetting
them is the most costly outcome. A corpus of 12 open loops at ~40 tokens
per one-liner sits at ~480 tokens — right at the ceiling. The ceiling
exists to force prioritization: old loops that are no longer active
should be closed with `mm_close_loop`, not allowed to accumulate
indefinitely.

**If the budget is exceeded today**: nothing happens — enforcement is
follow-on work. The session-start output will be longer than intended.
Mitigation: close stale loops regularly.

### L1 — Project knowledge (~2 000 tokens)

Project-shared and project-personal entries are injected after open
loops. The 2 000-token ceiling leaves room for ~5-8 topic lines plus
a first-section excerpt each. This is the "ambient awareness" layer:
the agent knows what the project has already figured out without having
to search.

**If the budget is exceeded today**: nothing happens. The session-start
output grows. Mitigation: keep project-shared entries to the most
load-bearing lessons; use user-personal scope for broad engineering
knowledge that doesn't need to be in every session.

### L2 — mm_search trimmed excerpt (~200 tokens/result)

The default `mm_search` response. Each result returns:

1. Topic + metadata header (always)
2. First `##` section (if body is long)
3. ±3 lines anchored on the search match (if a body match exists and
   differs from the first section)
4. Full body if body is under 800 characters — trimming a short body
   saves nothing

The `path` field in every result is the L2→L3 bridge: the agent can
pass it to the Read tool to get the full entry without re-searching.

**If a caller needs the full body**: use `expand: true` in the
`mm_search` call. This is an L3 call; the caller accepts the cost.

### L3 — Full entry content (unbounded)

Two paths to L3:

- `mm_search` with `expand: true` — returns full bodies for all results
- Read the `path` field from an L2 result directly

L3 is deliberate. It's the "I know exactly which entry I need and I
want all of it" operation. Use it for deep dives, not for broad sweeps.

---

## Mapping to MemPalace

MemPalace (see `docs/reference-notes/mempalace.md` §3) describes its
own L0-L3 stack:

| MemPalace | mastermind equivalent |
|-----------|----------------------|
| L0 "Identity" (~50 tokens, always) | L0 open-loops header |
| L1 "Critical facts" (~120 tokens, always) | L1 project knowledge |
| L2 "On-demand recall" | L2 mm_search default |
| L3 "Explicit deep dive" | L3 expand:true / Read |

The token budgets differ (MemPalace is more aggressive; mastermind
targets a larger working corpus), but the shape is the same. shiba-memory
(another reference system) independently arrived at a two-tier
"hot/cold" model that maps to L0+L1 vs L2+L3.

Three systems, same pattern. The tiering is the right abstraction.

---

## Non-goals for this task

This document establishes the model and documents the soft budgets.
It does NOT implement runtime enforcement of L0 or L1 budgets. That
is follow-on work:

- **L0/L1 enforcement**: the session-start subcommand should truncate
  or summarize when the injected content exceeds the soft budget.
  Likely implementation: count bytes, warn to stderr (silent-unless-
  needed: stderr is the agent's stderr, not the user's UI), and clip.
- **L2 per-result budget**: already enforced by `BodyExcerpt()`. The
  ~200-token budget is approximate (800-char threshold ÷ 4 chars/token).
  Tune the `shortBodyThreshold` constant in `internal/search/excerpt.go`
  if real-session data shows it's wrong.
