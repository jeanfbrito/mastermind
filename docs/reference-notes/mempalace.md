# mempalace — reference notes

**URL**: https://github.com/milla-jovovich/mempalace
**Local clone**: `~/Github/mempalace`
**Language**: Python
**Backend**: ChromaDB (vector) + SQLite (knowledge graph)
**Stars**: ~38k
**License**: see repo
**Role for mastermind**: **philosophical counterpoint** + L0-L3 memory stack design + hook-block extraction pattern. **NOT a storage or wiring reference** — Python, ChromaDB, fundamentally different extraction philosophy.

MemPalace is the "raw verbatim storage" end of the memory-tool design space. Soulforge integrates with it as an upstream memory MCP server, and prior notes in `docs/reference-notes/soulforge.md` §2-3 touched on MemPalace for temporal KG triples and the 96.6% LongMemEval finding. This file is the full treatment on its own terms.

MemPalace matters to mastermind precisely because it is the **opposite philosophy**: keep everything verbatim, let semantic search find it, never extract or summarize. Reading it on its own terms keeps mastermind's extraction tradeoff honest.

---

## 1. What MemPalace is

> "The highest-scoring AI memory system ever benchmarked."
> — README, 2026-04-07 note from Milla & Ben

MemPalace stores conversations verbatim in ChromaDB and organizes them via the "memory palace" metaphor borrowed from classical rhetoric: ancient Greek orators memorized entire speeches by placing ideas in rooms of an imaginary building; to recall, they walked the building. MemPalace applies the same principle to agent memory:

- **Wings** — per person, per project, per specialist agent
- **Halls** — per memory type (facts, events, advice, etc.)
- **Rooms** — per specific idea or topic
- **Drawers** — the raw filed content inside a room

No AI decides what's worth remembering. The AI decides *where to file* it, and the structure provides a navigable index instead of a flat blob.

**Core components**:

- `mempalace/mcp_server.py` — MCP server with 19 tools (see §4).
- `mempalace/knowledge_graph.py` — temporal entity-relationship graph (SQLite).
- `mempalace/palace_graph.py` — room navigation graph.
- `mempalace/dialect.py` — AAAK compression (experimental, regresses the benchmark).
- `mempalace/miner.py` + `convo_miner.py` — ingest files or conversation transcripts in bulk.
- `mempalace/searcher.py` — semantic search (ChromaDB + filters).
- `hooks/mempal_save_hook.sh` + `mempal_precompact_hook.sh` — auto-save hooks for Claude Code / Codex CLI.

---

## 2. The philosophical counterpoint — "store everything, let search find it"

From MemPalace's README:

> Other memory systems try to fix this by letting AI decide what's worth remembering. It extracts "user prefers Postgres" and throws away the conversation where you explained *why*. MemPalace takes a different approach: **store everything, then make it findable.**

And directly on the benchmark:

> The 96.6% LongMemEval result comes from this raw mode. We don't burn an LLM to decide what's "worth remembering" — we keep everything and let semantic search find it.

**MemPalace's own honest caveat** (in the "A Note from Milla & Ben" section): their AAAK lossy compression dialect regresses the benchmark from 96.6% to 84.2%. **Their own lossy layer loses 12.4 points.** Extraction is lossy. They are transparent about this.

**Mastermind's position on the same tradeoff** (already recorded in `soulforge.md` §3, reinforced here):

- Mastermind deliberately chooses extraction over verbatim storage because:
  1. **Token budget**: session-start injection runs on every session. Verbatim storage means injecting raw transcripts — impossible inside mastermind's budget.
  2. **Consolidation loop**: the `/mm-review` step IS the learning step for the ADHD user. Reviewing a pending candidate is *how the lesson sticks*. Raw storage removes that loop entirely.
  3. **Different goal**: LongMemEval measures verbatim recall. Mastermind's goal is "surface the right lesson on a bad working-memory day." Those are different optimization targets.
- **But** the finding has a load-bearing implication for mastermind's extraction prompt: **bias toward high recall, not lossy summarization**. When the extractor is unsure, extract *more*, let `pending/` and `/mm-review` prune. Already captured in the extractor open-loop (category `extraction`).

**This counterpoint is the primary reason MemPalace deserves its own reference entry.** Not as a translation target, but as the opposite pole on the design space — rereading it periodically keeps mastermind from drifting toward "extract everything, store little" just because extraction happens to be mastermind's current pipeline.

---

## 3. The L0-L3 memory stack — the one design idea worth translating

From MemPalace's README "The Memory Stack" section:

| Layer | What | Size | When |
|---|---|---|---|
| **L0** | Identity — who is this AI? | ~50 tokens | Always loaded |
| **L1** | Critical facts — team, projects, preferences | ~120 tokens (AAAK) | Always loaded |
| **L2** | Room recall — recent sessions, current project | On demand | When topic comes up |
| **L3** | Deep search — semantic across all closets | On demand | When explicitly asked |

> Your AI wakes up with L0 + L1 (~170 tokens) and knows your world. Searches only fire when needed.

**Mastermind already has this shape implicitly**:

| Layer | Mastermind equivalent | Current state |
|---|---|---|
| L0 | Open-loops header in SessionStart injection | Exists, no size cap documented |
| L1 | Project knowledge entries in SessionStart injection | Exists, no size cap documented |
| L2 | `mm_search` results on demand | Exists, returns full bodies today |
| L3 | Reading `.knowledge/…` files directly via Read tool | Implicit, no explicit escape-hatch pattern |

**What's missing is the explicit tiering with documented budgets.** Making the layers explicit would:

- **Cap L0+L1 at a hard token ceiling** (proposal: 300-500 tokens for L0, 1500-2000 tokens for L1). Prevents session-start injection creep, which is a real risk as the corpus grows.
- **Give the output-trimming open-loop a concrete target**: "L2 (mm_search) responses stay under X tokens per result by default, use `expand: true` to get full content = L3."
- **Make the hot-path cost legible**. Every session pays the L0+L1 cost; knowing the ceiling forces discipline on what lives there.
- **Clarify the "load vs fetch" distinction** — today mastermind's SessionStart injection can grow without a budget alarm; an explicit layer ceiling surfaces the problem before it becomes one.

This is the single actionable idea from MemPalace worth an open-loop. Zero code to land the documentation; a small follow-on to enforce budgets programmatically in the session-start subcommand.

---

## 4. 19 MCP tools — and why mastermind stays at 4

MemPalace exposes 19 MCP tools across five categories:

- **Palace read**: `mempalace_status`, `mempalace_list_wings`, `mempalace_list_rooms`, `mempalace_get_taxonomy`, `mempalace_search`, `mempalace_check_duplicate`, `mempalace_get_aaak_spec`
- **Palace write**: `mempalace_add_drawer`, `mempalace_delete_drawer`
- **Knowledge graph**: `mempalace_kg_query`, `mempalace_kg_add`, `mempalace_kg_invalidate`, `mempalace_kg_timeline`, `mempalace_kg_stats`
- **Navigation**: `mempalace_traverse`, `mempalace_find_tunnels`, `mempalace_graph_stats`
- **Agent diary**: `mempalace_diary_write`, `mempalace_diary_read`

**Mastermind's hard rule #6 says four MCP tools, forever.** This is the concrete justification: every MCP tool is a token in the system prompt the agent must keep in working memory. MemPalace's 19 tools pay a cost on every turn whether or not they are used. That cost is acceptable for a general-audience tool (MemPalace targets any AI harness); it is *wrong* for mastermind's single-user ADHD target where cognitive surface area is the constraint.

MemPalace's tool list is also a reminder of how easy it is to let a tool surface sprawl. Every time mastermind is tempted to add a fifth tool, re-read this section.

**No translation**. Mastermind's four tools cover the same functional ground as MemPalace's 19:

| MemPalace operations | Mastermind equivalent |
|---|---|
| Palace read (search, list, check-dup) | `mm_search` |
| Palace write (add drawer) | `mm_write` |
| KG add/query/invalidate/timeline | Deferred to Phase 5+ relations schema (not new tools) |
| Navigation (traverse, tunnels) | Topic-dominant ranking in `mm_search` |
| Agent diary read/write | `kind: open-loop` + `mm_close_loop` |
| Delete drawer | **Intentionally absent** — hard rule #7 ("knowledge is never silently deleted") |

Mastermind's smaller tool surface is not a gap; it's the design.

---

## 5. The hook-block extraction pattern — novel, probably wrong for mastermind

MemPalace ships two hooks: `mempal_save_hook.sh` (Stop hook, auto-save every N exchanges) and `mempal_precompact_hook.sh` (PreCompact, always fires). Both use a **fundamentally different pattern** from mastermind's current extractor:

**The hook does NOT extract.** Instead, it returns a Claude Code hook response with `decision: block` and a `reason` field containing an instruction to the AI:

```json
{
  "decision": "block",
  "reason": "AUTO-SAVE checkpoint. Save key topics, decisions, quotes, and code from this session to your memory system. Organize into appropriate categories. Use verbatim quotes where possible. Continue conversation after saving."
}
```

Claude Code surfaces the `reason` as a system message. The agent sees it, saves memory via MCP tool calls in the next turn, then tries to stop again. On that second Stop, `stop_hook_active=true` is set in the hook input, the hook lets it through, and the conversation resumes. Infinite-loop prevention is built into the protocol.

The AI does the classification. The hook's job is *scheduling*, not extraction.

**Technical details worth noting**:

- `SAVE_INTERVAL=15` — every 15 human messages (configurable). Counted from the JSONL transcript at `transcript_path`, supplied by Claude Code on stdin.
- State file at `~/.mempalace/hook_state/<session_id>.count` tracks "exchanges since last save" per session.
- The PreCompact hook has no counting — compaction always warrants a save.
- Optional background `mempalace mine <MEMPAL_DIR>` call for bulk ingest if the user wants auto-ingest on top of the AI-driven save.
- Shell-safe argv passing (explicit `sys.argv` rather than shell interpolation) to avoid injection via crafted transcript paths.
- Installed via `.claude/settings.local.json` (the `.local.json` variant, not the shared `settings.json`).

**Why this pattern is interesting for mastermind**:

- **Mastermind's current PreCompact hook** runs its own extraction over the transcript (regex backend, optional LLM backend). The MemPalace pattern would be: "do nothing yourself, instead block the agent and tell it to call `mm_write` for each lesson before compaction."
- **Pros**: agent has full context; zero LLM call from the hook; reuses the agent's own reasoning budget; no transcript parsing in the hook.
- **Cons for mastermind specifically**:
  1. **Intrusive UX**. Blocking the agent interrupts the user's flow. Mastermind's design bias is "invisible until needed." Blocking every 15 messages violates that directly. (The PreCompact variant is less bad — it fires when context is about to be lost anyway.)
  2. **Agent cooperation is not guaranteed**. Agents under cognitive load skip instructions (see the live entry `agent-proactivity-requires-mechanical-enforcement-not-just-mcp-instructions`, 2026-04-08 — mastermind already learned this lesson). Mastermind's auto-extraction was designed *precisely* to not depend on agent cooperation.
  3. **Duplicates existing work**. Mastermind's regex + LLM extractor already runs at PreCompact. Replacing it with a block-and-instruct pattern regresses capability.
- **Where it *might* fit as a safety net**: PreCompact as a last-ditch verification layer. Current flow runs extractor → writes to `pending/`. Adding a block-and-instruct "please verify these pending candidates capture the lessons you actually learned" as a second pass before compaction *might* improve quality, at the cost of interrupting the user at every compaction boundary. Probably not worth it — but worth parking as a thought experiment if extraction quality becomes a concern.

**Verdict**: fundamentally incompatible with mastermind's "silent, automatic, no cooperation required" design. Do not adopt the block-and-instruct pattern. Do continue to study it as an example of what an alternative extraction path looks like.

---

## 6. The temporal knowledge graph — already covered, re-validated

From `mempalace/knowledge_graph.py`: temporal entity-relationship triples with `valid_from` timestamps and explicit invalidation. Facts have validity windows. When something stops being true, invalidate it (don't delete it); historical queries still return the old state.

```python
kg.add_triple("Kai", "works_on", "Orion", valid_from="2025-06-01")
kg.invalidate("Kai", "works_on", "Orion", ended="2026-03-01")
# Current queries skip the invalidated triple; historical queries still return it.
```

Already captured in `soulforge.md` §2 as the conceptual backing for the `supersedes:` + `contradicts:` frontmatter open-loop. MemPalace's version is SQLite-backed (Zep/Graphiti is Neo4j); mastermind's version is markdown-frontmatter-backed. Same concept, three storage layers, all converging on "don't delete, invalidate with a timestamp."

**Confirms mastermind hard rule #7 ("knowledge is never silently deleted") from a third independent source**, and confirms the `supersedes:` / `contradicts:` schema design choice.

One detail worth remembering: MemPalace's README explicitly says contradiction detection is **"experimental, not yet wired into KG"**. Even the system that cares most about it admits auto-detection is hard. Mastermind's human-populated-first stance on relations is vindicated.

---

## 7. Conversation mining — the bulk-ingest path mastermind lacks

MemPalace ships `mempalace mine <dir>` and `mempalace mine <dir> --mode convos` for bulk-ingesting files or conversation transcripts into the palace. Useful for bootstrapping memory from an existing archive of notes, old sessions, or exported transcripts.

**Mastermind has no equivalent**. `mastermind discover` is the closest — it mines git history + codebase for knowledge using Haiku — but it doesn't ingest arbitrary file dumps or pre-existing conversation archives.

**Should mastermind grow this?** Probably not as a top-level feature. The dogfooding path is the primary capture mechanism (SessionStart + PreCompact + mm_write). Bulk import is a one-time bootstrap need for new users, and mastermind's current onboarding is "start using it, let the corpus grow organically." The longer the corpus grows organically, the less a bulk-import matters.

**Park as a maybe-future CLI subcommand** if users ever ask for it. Not worth an open-loop. Documented here so future-me remembers the pattern exists.

---

## 8. Things to NOT copy from MemPalace

- **ChromaDB / vector storage.** Mastermind uses context-mode FTS5 over markdown. No embedding model dependency, no vector index to rebuild, no Python runtime.
- **Raw verbatim storage as the primary persistence mode.** The whole point of mastermind's pending/ + review pipeline is lossy-but-consolidated capture. Going verbatim would collapse the learning loop.
- **19 MCP tools.** See §4. Hard rule #6.
- **Delete operations (`mempalace_delete_drawer`).** Hard rule #7.
- **AAAK compression dialect.** Their own benchmark shows it regresses vs raw. Mastermind's markdown + YAML frontmatter is the 2034 contract.
- **Per-specialist agent diaries (`~/.mempalace/agents/reviewer.json`, etc.).** Violates the "one user, one workflow" target audience.
- **Hook-block + instruct extraction pattern.** See §5. Violates "silent until needed" and duplicates existing mastermind work.
- **Python runtime, ChromaDB docker dependency, HTTP gateway, webhooks.** None of these fit the single-binary Go model.

---

## 9. Things to translate

Exactly one:

**L0-L3 memory stack with explicit per-layer token budgets.** Document the implicit tiering mastermind already has; add soft budget ceilings for L0 (open-loops header) and L1 (project-shared session-start injection); design L2 (`mm_search` default response) for a token-budget-friendly excerpt (ties into the existing output-trimming open-loop); leave L3 (direct file reads) as the explicit escape hatch. Documentation-first; enforcement as a small follow-on.

New open-loop under category `memory-stack`. See `mm_search` for the entry.

---

## 10. Things MemPalace validates about mastermind

Surprisingly specific alignments, each from a different independent design choice:

- **Markdown/structured text as the human-readable surface**, even when storage is a database (MemPalace stores drawers as text content inside ChromaDB).
- **Knowledge-graph invalidation instead of deletion** (MemPalace KG's `invalidate()` + mastermind hard rule #7).
- **Per-turn context cost is a first-class constraint** (MemPalace's L0+L1 ~170 token always-loaded layer + mastermind's tight session-start budget).
- **Agent does the classification at write time** (MemPalace's hook-block "reason" tells the AI to classify; mastermind's `mm_write` requires scope + kind + category from the caller). Same philosophy, opposite mechanism.
- **Contradiction detection is hard; don't over-automate** (MemPalace admits theirs is experimental; mastermind stays human-populated).
- **A palace/room/drawer taxonomy maps cleanly onto scope/kind/topic**. Same shape, different vocabulary — the structure is load-bearing in any long-lived memory tool.

When three independently-built memory systems (shiba-memory, MemPalace, mastermind) converge on the same structural choices, those choices are probably load-bearing and worth protecting against drift.
