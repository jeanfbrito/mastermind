# shiba-memory — reference notes

**URL**: https://github.com/ryaboy25/shiba-memory
**Local clone**: `~/Github/shiba-memory`
**Language**: TypeScript (CLI + Hono HTTP gateway) + Python (benchmarks)
**Backend**: PostgreSQL 16 + pgvector + Ollama embeddings
**Role for mastermind**: closest living memory-layer peer by feature surface. **NOT a storage reference** (Postgres + vectors vs. our markdown-on-disk) but a strong **behavioral reference** for hook coverage, relation types, tiered extraction, and ACT-R scoring.

Shiba is "the memory tool engram would be if engram were focused on agent memory instead of general knowledge management." It ships a full Claude Code hook suite, a 17-endpoint HTTP gateway, six relation types, and published LongMemEval benchmarks. Most of its *implementation* is irrelevant to mastermind — the Postgres/pgvector/HTTP stack is specifically what mastermind's design rejects. Its *behaviors and vocabulary* are highly relevant.

A prior project-shared entry (`shiba-memory-self-hosted-agent-memory-with-act-r-scoring-knowledge-graph-and-cla`, 2026-04-09) and a hooks insight (`shiba-memory-hooks-insight-precompact-postcompact-could-solve-phase-3-extraction`, 2026-04-09) already live in the live store. This file is the structural appendix that the prior insights pointed at.

---

## 1. The full Claude Code hook suite — the load-bearing finding

Shiba ships **five** hooks. Mastermind today has two (SessionStart + PreCompact). The delta is the most actionable insight in the repo.

| Hook | When it fires | Shiba's action | Mastermind's current coverage |
|---|---|---|---|
| **SessionStart** | New session begins | Recall relevant project memories, preferences, feedback, skills → inject into context | Yes — `internal/mcp/session_start.go` equivalent via the CLI subcommand |
| **PostToolUse** | After `Edit\|Write\|Bash` | Capture significant actions as episodic memories (7-day TTL) | Partial — we have "PostToolUse suggest" that surfaces related entries on file touch, but we don't *write* episodic memories |
| **Stop** | Response finishes | Update session record, clean up old episodes | No |
| **PreCompact** | Before context compression | Snapshot current decisions and files before they are lost | Yes — extraction pipeline |
| **PostCompact** | After context compression | Re-inject key project context and user feedback into the compressed context | **No** |

**The interesting one: PostCompact.** After Claude Code compresses the context, the summary message replaces most of the conversation. At that moment the agent has *just lost* everything that wasn't in the compaction summary. Shiba re-injects a curated slice of memory *into* the compacted context so the next turn starts with the right project knowledge already present.

For mastermind this is the missing half of the extraction story. PreCompact captures what was learned *this session*; PostCompact re-hydrates what was learned *in previous sessions* so the post-compression agent doesn't wake up blind. Mastermind's SessionStart injection does this at session boundaries — PostCompact does it at compaction boundaries, which are more frequent in long sessions.

**Translation candidate (Phase 3 polish or Phase 4)**: add a PostCompact hook that runs the same injection logic as SessionStart, scoped to the current project only. Should be cheap — it's the same code path. Requires the hook to run on a stream that is *not* the transcript (Claude Code's PostCompact hook fires after the compaction has already landed).

**Other translation candidate (Stop hook)**: cleaner trigger for open-loop detection than waiting for PreCompact. When a response finishes without the user saying "done", shiba marks it as an open episode. Mastermind's open-loops are currently only user-created via `mm_write`. An automated "Stop without resolution" signal would populate open-loops without any user action — strong ADHD-friendly signal, weak precision (lots of false positives). Park as a Phase 5+ experiment with a confidence threshold.

---

## 2. Six relation types — the contradiction/supersedes answer

Shiba's knowledge graph stores a **flat adjacency list** between memories with six relation types:

```
related, supports, contradicts, supersedes, caused_by, derived_from
```

Each link has a strength weight 0-1 that boosts relevance in search (`graph_boost = 1 + sum(link_strengths) * 0.2`).

**Important caveat from shiba's own README**: *"`auto_link_memory` only creates `related` links via embedding similarity. Contradiction detection uses embedding dissimilarity as a proxy."* The taxonomy is rich but the automation is shallow. This is consistent with where mastermind should sit: **schema-ready for the richer relations, but don't over-invest in automatic detection that's mostly wrong.**

**Mastermind translation (confirms what soulforge.md already proposed)**:

- Add **two** optional frontmatter fields in a Phase 5+ schema extension: `supersedes: [slug1, slug2]` and `contradicts: [slug1, slug2]`. The other four shiba relations (`related`, `supports`, `caused_by`, `derived_from`) are either already implicit (topic overlap = related) or too speculative to bake in.
- **Human-populated first, agent-assisted later**. The review UI (`/mm-review`) prompts the human to mark a supersede/contradict link when an extracted candidate overlaps meaningfully with an existing entry. No embedding-similarity heuristic — just string/topic overlap as the trigger.
- **Search boost follows naturally**: when mm_search returns entry A, an entry that supersedes A gets promoted; an entry that contradicts A gets surfaced alongside. Both are cheap to implement once the frontmatter field exists.
- FORMAT.md is immutable for existing fields, but **adding new optional fields is allowed by the contract** — this is a compatible extension.

Same conclusion as the soulforge notes, but shiba gives a concrete naming / surface design to copy.

---

## 3. Tiered extraction — validates the keyword-first + LLM-optional design

Shiba exposes two extraction tiers via its HTTP gateway:

- **Tier 1 (free)**: `POST /extract/patterns` — regex-based fact extraction. Zero LLM cost.
- **Tier 2 (LLM)**: `POST /extract/correction`, `/extract/summarize`, `/extract/preferences` — three specialized LLM passes, each with a narrow output shape.

This is **exactly the shape of mastermind's `internal/extract/` package** (keyword backend free, Haiku/Ollama backend optional, gated by `MASTERMIND_EXTRACT_MODE`). Shiba validates the design.

**What's worth stealing from shiba's version**:

- **Split the LLM backend into specialized sub-passes instead of one big extraction prompt**. Shiba doesn't run "extract all lessons" — it runs "detect corrections", "summarize session", "infer preferences" as three narrow calls. Each call is smaller, cheaper, and higher-precision than one omnibus call. Today mastermind's LLM backend runs a single extraction prompt over the full transcript. Splitting it into "detect corrections" (user said "no, not that") + "extract decisions" (I'll use X because Y) + "extract war stories" (what failed and why) would improve precision without raising cost.
- **Vocabulary**: "Tier 1 (free) / Tier 2 (LLM)" is cleaner than "keyword backend / LLM backend". Worth adopting in docs.

---

## 4. ACT-R scoring — the upgrade path for our access-frequency feature

Mastermind already has access-frequency scoring (per CLAUDE.md status: "entries returned by mm_search track access counts, frequently useful entries rank higher"). Shiba has two scoring modes for the same concept:

- **Fast mode (default)**: `1 + ln(access_count + 1) * 0.1` — log-of-count, captures *frequency* only.
- **Proper mode**: `1 + B_i * 0.1` where `B_i = ln(Σ t_j^(-0.5))` — real ACT-R base-level activation using the list of individual access timestamps with power-law decay. Captures both *frequency and recency* — recently accessed memories get a stronger boost than equally-frequent but older accesses.

**Mastermind's current mode is fast** (implicitly — we track count, not per-access timestamps). The proper mode requires storing a JSONB array of timestamps per entry and scanning it at query time.

**Translation decision**: not yet. The proper-mode upgrade would require a frontmatter schema change (new `access_timestamps: []` field) and is only worth it if the fast mode is demonstrably missing things. Park as a Phase 4+ possibility. Noted for future reference because it's a clean upgrade path *if we ever observe a ranking problem attributable to recency being ignored*.

---

## 5. `reflect consolidate` — the checklist for mastermind's `/dream` skill

Shiba has a `reflect consolidate` CLI command that runs "full brain maintenance":

- Merge duplicates
- Detect contradictions
- Decay confidence of old unused memories
- Auto-link via embedding similarity
- Generate cross-project insights

Mastermind already has a `/dream` skill (per the skill list: "/dream — Memory Consolidation"). Whatever that skill does today, the shiba operations list is a concrete checklist to measure it against:

- Does `/dream` merge duplicates? Probably yes, via review prompts.
- Does it detect contradictions? Not currently (see §2 above).
- Does it decay old entries? No — mastermind's hard rule is "pending is patient, nothing is silently deleted." Decay is off the table; the equivalent would be *de-prioritizing* in search ranking, not removing.
- Does it auto-link? No.
- Does it generate cross-project insights? Unclear without reading the skill spec.

**Action**: next time `/dream` is touched, audit it against this checklist and decide which operations belong. Not a new PR on its own.

---

## 6. Benchmarks — the second data point

Shiba publishes LongMemEval: **50.2%** (500 questions, oracle split). For comparison, their own table:

| System | LongMemEval |
|---|---|
| Shiba | 50.2% |
| Mem0 | 49.0% |
| Zep | 63.8% |
| MemPalace (raw mode) | 96.6% |
| MemPalace (AAAK) | 84.2% |

These numbers are not directly comparable — different splits, different judges, different retrieval assumptions. But two observations:

1. **Extraction-based systems cluster around 50%**, while raw-verbatim storage jumps to 96%. Same finding as the MemPalace counterpoint recorded in soulforge.md. Two independent data points now support the same conclusion: **extraction is lossy and shouldn't be expected to ace benchmarks that reward verbatim recall**.
2. **Mastermind shouldn't try to win LongMemEval.** Our goal is "surface the right lesson on a bad working-memory day", not "recall every word of every session." Recording the finding so future Jean doesn't burn a weekend trying to benchmark mastermind against raw-storage systems.

---

## 7. Things to explicitly NOT copy

- **PostgreSQL + pgvector backend.** Mastermind is single-binary, plain files, zero infra. This is the single biggest architectural divergence and it's load-bearing — the whole point of markdown-on-disk is longevity and inspectability without a running database.
- **Vector embeddings / Ollama dependency.** Mastermind uses context-mode's FTS5 for search. No embedding model to install, no model version to track over time.
- **Hono HTTP gateway on port 18789.** Mastermind is MCP-only. Rule #4 in hard rules: no HTTP framework.
- **Instinct → skill auto-promotion.** Shiba auto-promotes low-confidence observations to skills at `>0.7` confidence + 3+ accesses. This is exactly the "silent writes to the live store" pattern that mastermind's pending/ flow exists to prevent. Mastermind's review step is load-bearing because (a) the corpus is too valuable to trust unreviewed writes and (b) reviewing IS the learning step for ADHD consolidation. Auto-promotion bypasses both.
- **Multi-user / multi-agent isolation.** Single-user tool. Don't add tenancy.
- **17 SQL functions.** See "no Postgres" above. All of mastermind's search is plain Go over markdown; we don't have a query language in the store layer.
- **Webhook subscriptions.** No push model. MCP-only.
- **Halfvec / HNSW index.** No vector store; no need.
- **Daemon mode / background consolidation service.** Mastermind's binary is started on-demand by Claude Code. No daemon, no persistent process.

---

## 8. Things to translate (consolidated)

Ranked by effort/value:

1. **PostCompact hook** → re-inject project knowledge after Claude Code compaction, using the existing SessionStart injection code path. Phase 3 polish. Cheapest win with the highest day-to-day impact.
2. **Split the LLM extraction backend into specialized sub-passes** ("detect corrections", "extract decisions", "extract war-stories") instead of one omnibus extraction prompt. Phase 3 polish. Precision win at equal or lower cost.
3. **Adopt the "Tier 1 / Tier 2" vocabulary** in extraction docs. Zero effort, cleaner mental model.
4. **Add `supersedes:` and `contradicts:` optional frontmatter fields**, human-populated via `/mm-review` prompts. Phase 5+. Confirms the same proposal from soulforge.md.
5. **Audit `/dream` skill against shiba's `reflect consolidate` checklist** (merge dupes, detect contradictions, auto-link) — document what we do, what we don't, and what we deliberately won't. Next time `/dream` is touched.
6. **Stop-hook open-loop capture** — when a response ends without resolution, auto-create a low-confidence open-loop candidate. Phase 5+ experiment with high false-positive tolerance.
7. **Proper-mode ACT-R scoring** (per-access timestamps, power-law decay) as an upgrade from the current count-only mode. Phase 4+, only if we observe a ranking problem attributable to missing recency.

---

## 9. Where shiba and mastermind agree (validation)

Surprising amount of design overlap. Both tools independently landed on:

- **Project scoping** with same-project boost (shiba: 1.3x; mastermind: topic-dominant ranking).
- **Access-frequency in the search score** (shiba: ACT-R; mastermind: access_count × weight).
- **Tiered extraction** — free pass + optional LLM pass.
- **Pre-compaction capture trigger** via Claude Code hooks.
- **Markdown/structured-text as the human-readable surface** (shiba renders memories as markdown, even though storage is Postgres).
- **Session-start context injection** as the default continuity mechanism.
- **Consolidation as a separate phase** from capture (shiba: `reflect consolidate`; mastermind: `/dream`).

When two independently-designed tools land on the same patterns, the patterns are probably load-bearing. This is the strongest external validation of mastermind's Phase 2-3 direction so far.

**The divergences are also load-bearing**: plain files vs. Postgres, MCP-only vs. HTTP gateway, review-gated vs. auto-promoted, one user vs. multi-tenant. These are the axes on which mastermind deliberately refuses to look like shiba, and each refusal ties back to the ADHD/longevity/trust requirements. See CLAUDE.md hard rules 1-8.
