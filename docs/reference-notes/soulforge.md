# soulforge — reference notes

**URL**: https://github.com/proxysoul/soulforge
**Stars**: ~358
**Language**: TypeScript
**License**: BSL 1.1
**Role for mastermind**: Phase 3 extraction comparison + Phase 5+ contradiction detection idea source. **NOT a wiring reference** — TypeScript, agent-harness architecture, different problem.

SoulForge is a codebase-aware CLI with embedded Neovim, live PageRank repo graph, multi-agent dispatch. Most of that is irrelevant to mastermind (we are a memory store, not a code-intelligence harness). Two subsystems are worth studying:

1. **Compaction v2** — incremental structured extraction during conversation, with optional tiny LLM gap-fill. Directly comparable to mastermind's PreCompact extractor.
2. **MemPalace integration** — how an agent harness persists working state into an external memory MCP server at compaction time. Mastermind sits on the other side of that boundary and should understand what the upstream wants.

---

## 1. Compaction v2 — WorkingStateManager

**Source**: `docs/compaction.md` in the soulforge repo.

The WSM extracts structured state **as the conversation happens**, not in a batch at compaction time. Slot categories:

```
task, plan, files, decisions, failures, discoveries,
environment, toolResults, userRequirements, assistantNotes
```

**Deterministic extractors (zero LLM cost)**:

- `extractFromToolCall()` — tool call arguments feed `files`, `toolResults`
- `extractFromToolResult()` — error results feed `failures`
- `extractFromUserMessage()` — first user message sets `task`

**Regex extractors (zero LLM cost)**:

- **Decisions**: `"I'll use..."`, `"decided to..."`, `"because..."`
- **Discoveries**: `"found that..."`, `"the issue was..."`

**Compaction flow**:

1. WSM already has structured state (built incrementally).
2. `buildV2Summary()` serializes WSM into markdown.
3. **Gap-fill threshold**: if ≥15 slots are populated across all categories, the LLM gap-fill pass is **skipped entirely**. Otherwise a 2K-token LLM call sees the structured state + 4k chars of older messages and outputs only what's missing.
4. Summary message + ack message + N recent messages are kept; everything else is replaced.

**Comparison to mastermind's Phase 3 extractor** (`internal/extract/`):

| Dimension | soulforge WSM | mastermind Phase 3 |
|---|---|---|
| When extraction runs | Continuously, on every tool call + message | Once at PreCompact, over the full transcript |
| Cost model | Free during conversation; optional 2K LLM gap-fill | Full LLM pass (Haiku/Ollama) or keyword-only |
| Input shape | Incrementally built structured state | Full raw transcript |
| Lives inside | The agent harness itself | External Go binary over MCP |
| Output | Compaction summary (ephemeral, replaces old messages) | Candidate entries → `pending/` → live store (persistent) |

**What mastermind can't copy directly**: mastermind is a separate binary reached via MCP stdio. It has no visibility into per-tool-call state *as it happens* — Claude Code doesn't stream tool results to MCP servers. The WSM pattern requires living inside the harness. Mastermind's PreCompact hook gets a snapshot, not a stream.

**What mastermind can steal**:

1. **Regex patterns for decisions/discoveries** — directly translatable into the keyword backend in `internal/extract/`. Today the keyword extractor is mostly keyword-based; adding the phrase patterns ("I'll use", "decided to", "because", "found that", "the issue was") would improve recall at zero cost. Strong candidate for a Phase 3 polish PR.
2. **The slot taxonomy** — `task, plan, files, decisions, failures, discoveries, environment, requirements, notes`. Mastermind currently has `kind: lesson|insight|war-story|decision|pattern|open-loop`. The soulforge taxonomy is more fine-grained on the "what went wrong" and "what got found" axes. Not a schema change (FORMAT.md is immutable), but useful as an extractor-internal categorization before mapping back to mastermind's kinds.
3. **The gap-fill threshold idea** — skip the LLM when rule-based extraction already filled enough slots. Mastermind today always runs the LLM backend when `MASTERMIND_EXTRACT_MODE=llm`. Adding a "skip LLM when keyword extractor already returned ≥N candidates" short-circuit would cut costs.
4. **"Extract incrementally, compact instantly"** as a future Phase 3 v2 direction — *if* mastermind ever grows a PostToolUse hook that stores partial state in `~/.knowledge/scratch/`, a PreCompact run could just serialize that instead of re-reading the transcript. Speculative; parks the idea.

---

## 2. MemPalace integration (soulforge's upstream memory)

**Source**: `docs/compaction.md#mempalace-integration` in soulforge; README at https://github.com/milla-jovovich/mempalace.

When a MemPalace MCP server is connected, soulforge's compaction v2 automatically persists working state before resetting. Three outputs at zero extra cost:

1. **Drawer** — full serialized summary filed as a palace drawer. Wing = project name, room = `compaction`.
2. **Knowledge graph** — decisions, discoveries, and failures filed as **temporal entity-relationship triples** with `valid_from` timestamps. Contradiction detection flags conflicting decisions across compactions.
3. **Agent diary** — compact AAAK-format entry summarising task, key decisions, files touched.

**Relevance to mastermind**:

- Mastermind IS a memory MCP server. This is the shape of one potential upstream integration. We don't need to copy soulforge's side; we should recognize the pattern so that if any agent harness ever wants to hand us "decisions + discoveries + failures + files" as a compaction event, our MCP surface already accommodates it via `mm_write` (scope + kind + body). No API changes needed.
- The **"wing = project, room = compaction"** routing already maps cleanly onto mastermind's `project-shared/nodes/<slug>.md` scope. Validation of our scope model.

**MemPalace's knowledge graph model — the interesting idea**:

- Facts are **triples with validity windows**: `add_triple("Kai", "works_on", "Orion", valid_from="2025-06-01")`.
- When something stops being true: `invalidate("Kai", "works_on", "Orion", ended="2026-03-01")`. The triple is not deleted — historical queries still return it.
- Timeline query: `kg.timeline("Orion")` → chronological story of the entity.
- **Contradiction detection**: when a new decision contradicts an old one, flag it.

**Why this matters for mastermind**:

- Mastermind's hard rule is "knowledge is never silently deleted" (CLAUDE.md rule #7). MemPalace's "invalidate, don't delete" is the same principle applied to KG triples. Validates the intuition.
- **Contradiction detection is a feature mastermind lacks and arguably wants**. When an extracted candidate says "we use Postgres now" and an old entry says "we use SQLite," the review UI should flag the tension. Today mastermind's `/mm-review` verifies candidates against their ## Source commits, but doesn't cross-check against existing entries for contradictions. Open-loop candidate for Phase 5+.
- The full KG/triple model is **not** a translation candidate — mastermind is plain markdown, not a graph DB. But the concept of "this entry supersedes that one" as an explicit relation (cf. shiba-memory's `supersedes` relation type in the mm_search result) is a schema-compatible addition: it would live in frontmatter as `supersedes: [slug1, slug2]`.

---

## 3. MemPalace benchmark finding — the counterpoint

From MemPalace README (April 2026): **raw verbatim storage scores 96.6% on LongMemEval**; the AAAK lossy-compression extraction layer regresses to 84.2%. Their claim: "We don't burn an LLM to decide what's worth remembering — we keep everything and let semantic search find it."

**This is a direct counterpoint to mastermind's extraction-first design.** Worth taking seriously, not dismissing.

**Mastermind's rebuttal (which should be recorded in DECISIONS.md if it isn't already)**:

- LongMemEval measures **recall of conversation content**. Mastermind's goal is NOT "recall every word of every session." It's "surface the right lesson at the right moment for an ADHD user on a bad working-memory day."
- Raw storage trades LLM cost for context-window cost at query time. For mastermind's session-start injection (runs on every session, not on demand), the token budget is tight — we can't inject 50 raw transcripts and hope semantic search finds the relevant one.
- Extraction is also the **consolidation step** that makes lessons stick. The `/mm-review` UX is load-bearing for ADHD — reviewing a pending candidate IS learning it. Raw storage removes that loop.
- That said, MemPalace's finding suggests mastermind's extraction prompt needs to be **high-recall, not lossy**: when in doubt, extract more, prune in review. This aligns with the existing "pending/ is patient" policy.

**Takeaway**: don't flip to verbatim storage, but audit the extraction prompt for lossy over-summarization. If the extractor is dropping context that would have helped future-Jean, that's a bug, not a feature.

---

## 4. Things from soulforge to explicitly NOT copy

- **Live PageRank Soul Map / repo graph.** Not our problem. Mastermind is memory, not code intelligence. context-mode + the agent's own code navigation handle this.
- **Multi-agent dispatch / AgentBus.** We are single-user, single-binary. No fleet.
- **Per-tab models, cross-tab coordination.** Mastermind has no UI. Claude Code is the UI.
- **Task router / model selection.** The agent on the other side of MCP picks its own model; mastermind doesn't care.
- **19 providers / provider abstraction.** Our LLM call (in `internal/discover/`, optional `internal/extract/` LLM backend) hits one provider at a time via a small adapter. We don't need a provider framework.
- **Specialist agent diaries (per-reviewer, per-architect, etc.).** Violates the "one person, one workflow" target audience. Mastermind's `kind` field is the closest we'll get.
- **AAAK compression dialect.** Mastermind's markdown + YAML format is the contract for 2034. Compressed dialects are exactly the kind of drift FORMAT.md exists to prevent.

---

## 5. Things to translate

1. **Decision/discovery regex patterns** → `internal/extract/` keyword backend. Low effort, measurable recall win.
2. **Gap-fill threshold short-circuit** → `internal/extract/` LLM backend. If keyword extractor returns ≥N high-confidence candidates, skip the LLM pass.
3. **Contradiction detection at review time** → Phase 5+ feature for `/mm-review`. When a new candidate's body contradicts an existing entry (string overlap + conflict heuristics, LLM for the ambiguous cases), flag it in the review UI.
4. **Explicit `supersedes` relation in frontmatter** → FORMAT.md is immutable for the existing fields, but *adding* a new optional field is allowed. Track as a Phase 5+ schema extension. Lets entries explicitly replace older ones without deleting them.

---

## 6. Open questions parked for later

- Should mastermind ship an official MemPalace-shaped "compaction drawer" kind, i.e. a kind specifically for "full compaction summary captured by agent harness X"? Or does that collapse into the existing `insight` kind? Probably the latter, but worth re-checking if a second harness starts writing to mastermind.
- Is there a "good day" scenario where the verbatim-storage-plus-semantic-search model would serve mastermind better than extraction? Probably not for session-start injection (token budget), but maybe for `mm_search` on complex multi-session threads. Park.
