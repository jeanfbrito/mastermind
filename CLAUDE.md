# mastermind — project instructions for Claude Code

> *The ADHD cure for agents that you always dreamed for yourself.*

This file is the orientation pointer for a fresh Claude Code session opened in this repo. Read it first, then fan out into `docs/` as needed. It's deliberately short — the real design lives in `docs/`.

## Who this is built for

**Jean**, one engineer with ADHD. The tool is optimized for a day when working memory is at its worst, not a day when it's at its best. Every feature decision passes through that test. If a feature requires you to remember to use it, it's the wrong design. Full stop.

This is NOT a general-purpose memory tool. For that, use [engram](https://github.com/Gentleman-Programming/engram) — it's excellent and built for that audience.

## Status

**Phases 1-3 complete.** 302 tests passing across 9 Go packages, binary builds and runs. Actively dogfooding. For a current commit log use `git log --oneline -15`; this file doesn't try to stay in sync with HEAD.

## Read these in order before changing anything

1. **[docs/CONTINUITY.md](docs/CONTINUITY.md)** — THE most important doc. The five load-bearing behaviors (session-start injection, session-close extraction, open-loops as first-class kind, guilt-free review, silent-unless-needed). Any work that doesn't honor these is the wrong work.
2. **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — package layout, MCP tool surface, CLI subcommands, hook integration.
3. **[docs/DECISIONS.md](docs/DECISIONS.md)** — why every architectural choice is what it is. Append-only. Read before proposing to change any "why."
4. **[docs/MEMORY-STACK.md](docs/MEMORY-STACK.md)** — the L0-L3 memory model with per-layer token budgets. All four tiers have matching enforcement: L0/L1 soft warnings land in `formatSessionStart` (warn-to-stderr, no truncation); L2 is enforced by `BodyExcerpt()`; L3 is caller-pays by design.
5. **[docs/ROADMAP.md](docs/ROADMAP.md)** — Phase 0 through Phase 6. Phases 1-3 complete; **Phase 4 (archive tier)** is the next unblocked roadmap item. Distribution (`goreleaser` + Homebrew) is the other unblocked item and is deliberately deferred per user preference.
6. **[docs/FORMAT.md](docs/FORMAT.md)** — the entry schema. **This is a long-term contract.** Do not change it casually — existing entries will need to keep parsing in 2034.
7. **[docs/NON-GOALS.md](docs/NON-GOALS.md)** — things explicitly rejected. Read before proposing a new feature.
8. **[docs/EXTRACTION.md](docs/EXTRACTION.md)** — the capture pipeline spec (Phase 3).
9. **[docs/ARCHIVE.md](docs/ARCHIVE.md)** — working set vs lifelong archive.
10. **[docs/REFERENCE-NOTES.md](docs/REFERENCE-NOTES.md)** — Phase 0 synthesis of the four reference repos. Appendix lives in `docs/reference-notes/`.

## Hard rules (non-negotiable)

These are load-bearing. Violating any of them breaks the tool for its primary user.

1. **User-initiated writes go directly to the live store.** When the user tells the agent to capture something (`mm_write`), the entry goes straight to the live store — the user IS the review. `pending/` is reserved for auto-extracted entries from session-close (Phase 3), which the user wasn't present to review. `mm_promote` moves those pending candidates to live after review.
2. **No notifications, no reminders, no badges, no streaks, no dashboards.** The default state of the tool is invisible.
3. **No persistent index.** Files on disk are the database. Ephemeral in-memory caches only (see search + store packages).
4. **No delegation to context-mode from inside mastermind code.** context-mode and mastermind are two independent tools the agent can reach for separately. The synergy happens automatically because context-mode indexes mastermind's MCP output as session content. No MCP client inside mastermind.
5. **No replacement for engram.** Mastermind deliberately diverges on storage format, scope model, capture path, continuity layer, and target audience. If a design impulse would make mastermind look more like engram, that's a signal to stop and re-read REFERENCE-NOTES.md appendix.
6. **Four MCP tools, forever.** `mm_search`, `mm_write`, `mm_promote`, `mm_close_loop`. Adding a fifth requires a DECISIONS.md entry with explicit justification.
7. **Pending entries are kept indefinitely. Knowledge is never silently deleted.** Optionally, a configurable auto-promote policy moves old candidates to the live store after N days (default: off). The queue is patient — old entries are waiting for a good day, not accumulating shame.
8. **Session-close extraction is automatic (hook-driven). Session-start injection is automatic (hook-driven).** The user never has to remember to trigger capture or retrieval. Phase 3 implements the hooks.

## Reference repos (local clones)

- `~/Github/engram` — **primary implementation reference.** Same general shape: Go + MCP stdio + single-binary + goreleaser + Homebrew. Copy project layout, distribution pipeline, and Go idioms. Diverge on storage (markdown vs SQLite), scope model, capture path, continuity layer.
- `~/Github/byterover-cli` — substrate model (brv). Node format validated; hub deliberately not copied. TypeScript; patterns only, no code reuse.
- `~/Github/OpenViking` — end-of-session extraction prompt (verbatim captured in reference-notes/openviking.md) and scope-assignment pattern. Python/Rust; ideas only.
- `~/Github/rtk` — Rust hook-interceptor. **Not an MCP server**, so NOT a wiring reference. Style notes and release workflow only.
- `~/Github/soulforge` — TypeScript. Added post-Phase-0 for Phase 3 extraction comparison. **NOT a wiring reference.** Useful for its `WorkingStateManager` compaction-v2 regex extractors and for how it integrates with MemPalace as an upstream memory MCP server. Full notes in `docs/reference-notes/soulforge.md`.
- `~/Github/shiba-memory` — TypeScript + Postgres + pgvector. Added post-Phase-0 as the closest behavioral peer. **NOT a storage reference** (Postgres is specifically what we rejected). **IS** a behavioral reference for the full five-hook Claude Code suite (adds PostCompact + Stop beyond what mastermind has), six knowledge-graph relation types, tiered extraction (free + LLM), and ACT-R scoring. Full notes in `docs/reference-notes/shiba-memory.md`.
- `~/Github/mempalace` — Python + ChromaDB. Added post-Phase-0 as the **philosophical counterpoint** on the extraction axis: raw-verbatim storage with 96.6% LongMemEval, "store everything, let search find it." **NOT a translation target** (opposite philosophy, different stack). IS useful for its L0-L3 memory stack design with per-layer token budgets, the palace/wing/room taxonomy, and its hook-block extraction pattern (which mastermind deliberately rejects, documented for contrast). Full notes in `docs/reference-notes/mempalace.md`.

When Phase 1+ work needs to check "how does X work in reference Y," use context-mode's `ctx_batch_execute` against these repos — do NOT read them with the native Read tool (they're too large and will flood context).

## Language, SDK, and dependencies

- **Go 1.25+** (forced by the SDK minimum; we're on 1.26.1 locally).
- **MCP SDK**: `github.com/modelcontextprotocol/go-sdk v1.4.1` (official, Google co-maintained, past v1.0).
- **Frontmatter**: `gopkg.in/yaml.v3` (only other direct dep).
- **Module path**: `github.com/jeanfbrito/mastermind`.
- **No HTTP framework, no ORM, no DI container, no code generator.** Stdlib-first for everything except the MCP SDK and YAML parsing.

## Package layout

```
cmd/mastermind/main.go          entry point; version flag, subcommand dispatch, MCP server bootstrap
internal/config/                env var / config file loading (MASTERMIND_* knobs)
internal/format/                entry schema, frontmatter parse/validate/marshal
internal/store/                 three-scope markdown storage; pending invariant enforced at type level
internal/project/               project name detection (git remote → git root → cwd basename)
internal/search/                stdlib keyword search + tiered fallback + ACT-R access scoring + PageRank incoming-link boost
internal/mcp/                   MCP SDK wiring; the ONLY importer of modelcontextprotocol/go-sdk
internal/extract/               knowledge extraction from transcripts (keyword + optional LLM backends)
internal/discover/              autonomous discovery from git history + codebase (Haiku / OpenAI-compat)
docs/                           design spec (read CONTINUITY.md first)
```

**Package boundary rule:** only `internal/mcp` imports the MCP SDK. Everything else stays SDK-agnostic so the SDK can be swapped or upgraded without cascading changes.

## Build, test, run

```
make build        # produces bin/mastermind with ldflags version injection
make test         # go test ./...
make vet          # go vet ./...
make fmt          # gofmt -w .
make tidy         # go mod tidy
make install      # copies bin/mastermind to ~/.local/bin/ (verify first)
```

**Always run `make test` and `make vet` before committing.** 302 tests across 9 packages is the current baseline; if a commit lands with fewer, something got silently broken.

## Git discipline (this repo specifically)

- Don't commit or push without explicit user instruction. "Fix X" does NOT mean "commit X."
- Don't commit directly to `master`/`main` for new work — create a branch, test, open a PR. **Exception**: this repo is pre-release, solo, and currently working directly on `main` is fine until the first public release tag.
- Read operations (`git status`, `git diff`, `git log`) are always fine.
- Before any write operation, confirm the working tree state.

## Current status

**Phases 1-3 complete. Dogfooding in progress.**

What's working:
- All four MCP tools functional: `mm_search`, `mm_write`, `mm_promote`, `mm_close_loop`
- **SessionStart hook** auto-injects open loops + project knowledge, with soft L0/L1 token-budget warnings to stderr on overage (no truncation)
- **PreCompact hook** auto-extracts knowledge from transcripts before context compression
- **Stop hook** telemetry subcommand (`mastermind stop`)
- **Auto-init** creates `.knowledge/` with `.gitignore` (excludes `pending/`) in git repos on first use (opt out: `MASTERMIND_NO_AUTO_INIT=1`)
- **Tiered search ranking** — 7-class tiered fallback, ACT-R log-shaped access boost, project-boost multiplier, supersedes outgoing boost + PageRank-style incoming-link boost from the supersedes/contradicts graph
- **Contradicts co-retrieval** — `contradicts` targets are pulled into results with an annotation on the heading
- **Cross-scope "tunnel" annotation** — entries whose topic spans multiple scopes get a `[cross-scope: also in ...]` marker in mm_search output
- **LLM extraction** (optional) — set `MASTERMIND_EXTRACT_MODE=llm` for Haiku/Ollama-powered extraction
- **`/mm-extract`, `/mm-review`, `/mm-discover` skills** — manual extraction, verify-first triage, and Haiku-driven codebase mining
- **`mastermind discover` CLI** — standalone discovery, supports Anthropic + any OpenAI-compatible endpoint
- **PostToolUse suggest** — surfaces the most relevant entry's topic when you Read/Edit/Write a file, with per-file debounce

What's next (all unblocked items):
1. **Phase 4 (archive tier)** — `~/.knowledge/archive/<year>/<project>/` + `/mm-archive` CLI. ~1 day.
2. **Phase 5 (sync)** — mostly document the git-remote story in `docs/SYNC.md`. ~half day.
3. **goreleaser + Homebrew** — binary distribution. Deferred per user preference until other work catches up.

What's blocked on dogfooding evidence:
- Proper-mode ACT-R with per-access timestamp array — waiting for signs that the count-only fast mode promotes stale entries.
- LLM tier audit measurement strategy — needs a design decision (regional match vs LLM-as-judge) before any code.
- Stop-hook auto-capture of low-confidence open loops — Phase 5+ experiment, may produce noise.

## Known limitations (worth remembering)

- **`session-close` subcommand is still a stub.** PreCompact hook handles most of the extraction use case, but session-close could be useful for final cleanup.
- **Access tracking is synchronous** in search. Adds ~50ms for 10 results. Acceptable for current corpus sizes but worth monitoring.
- **L0/L1 budget warnings go to stderr only.** When a session-start block exceeds its soft budget, `formatSessionStart` warns to stderr (Claude Code's hook log channel). There is no persistent `~/.knowledge/logs/mastermind.log` file yet — if the stderr channel ever stops being the right place (e.g. a future hook runner that swallows it), revisit the logging design.

## When you get stuck

- If the docs contradict each other: DECISIONS.md wins over everything except FORMAT.md (the format contract is immutable). CONTINUITY.md wins over ARCHITECTURE.md when the two ever disagree about behavior.
- If a test fails after a change: read the failure message, fix honestly. Don't weaken the test to pass; don't rationalize. The ranking-invariant tests and serverInstructions tests are deliberately strict because they catch drift.
- If you're unsure whether a feature belongs: apply the "does this work on a bad working-memory day" test. If it requires a good day, it's the wrong design.

## One last thing

Take the corpus seriously. Keep the tool small. Win the 2034 bug.
