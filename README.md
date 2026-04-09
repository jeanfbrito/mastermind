# mastermind

> *The ADHD cure for agents that you always dreamed for yourself.*

A personal engineering second brain: repo-local shared knowledge + cross-project personal memory + lifelong archive, backed by plain markdown and git.

## Who this is for

The author, who has ADHD and builds software for a living. That's it. The primary design constraint is: **the tool must work on a day when working memory is at its worst, not a day when it's at its best.** Every feature passes through that test.

Specifically, this means the tool captures context automatically at session close (no slash command to remember), surfaces relevant knowledge automatically at session start (no query to remember to run), treats "I was about to do X but got pulled into a meeting" as a first-class memory type (open-loops), and never has a dashboard, a streak counter, or a notification. The default state of the tool is invisible.

If you're neurotypical and want a generic AI-agent memory tool with multi-agent support, a TUI, and an HTTP API, use [engram](https://github.com/Gentleman-Programming/engram) — it's excellent and aimed at exactly that. mastermind is smaller, more opinionated, and built for one person's workflow. If it works for others as a side effect, bonus.

## What success looks like

Not users. Not stars. Not a Discord. The win condition is: **in 2034, you hit a weird bug, you ask your agent, your agent finds a lesson your 2026 self wrote, and ten minutes later you've shipped the fix instead of losing three days.** Everything in the design serves that outcome. See [docs/CONTINUITY.md](docs/CONTINUITY.md) for the full spec of the behaviors that make it possible.

Inspired by [byterover-cli](https://github.com/campfirein/byterover-cli) (substrate, node model, warmup) and [OpenViking](https://github.com/volcengine/OpenViking) (auto-extraction, scope unification as ideas only — no code, no server).

**Reference sources (local clones):**
- `~/Github/byterover-cli` — **substrate model** (brv): what features to keep, what node format to adopt, what hub to strip.
- `~/Github/OpenViking` — **pattern source**: end-of-session extraction and warmup ideas only. No code reuse.
- `~/Github/rtk` — **conceptual blueprint** (Rust, not Go): proves the shape — a single-binary MCP-adjacent tool that ships and stays working for years. Patterns translate to Go; no direct code reuse.

## Language

Go. See [docs/DECISIONS.md](docs/DECISIONS.md) for the full rationale. Short version: mastermind is designed to outlive Node/Python environment rot. A static Go binary still runs in 2034 without any runtime installation. Go's standard library covers almost everything mastermind needs, iteration is fast, and the Go 1 compatibility promise has held since 2012 — directly addressing the longevity requirement. rtk (Rust) is a conceptual blueprint for the shape of the tool; patterns get translated to Go, not copied as code.

## Why

- **brv** is mostly right, but knowledge lives at machine level with a hub component; it should live in the repo, shared via git, with no hub.
- **OpenViking** has two good ideas (end-of-session extraction, user-wide memory) but ships a Rust server you don't want to depend on.
- **context-mode** is perfect as-is for in-session token protection; mastermind uses it (FTS5 indexing, ctx_search) and does not replace it.

What's missing today: a place to keep the hard-won lessons, war stories, decisions and patterns from every project you work on — for a whole career — in a format that outlives any tool.

## The three stores

| Scope | Location | Sync | Visibility |
|---|---|---|---|
| **project-shared** | `<repo>/.knowledge/` | git (checked in) | team |
| **project-personal** | `~/.claude/projects/<repo>/memory/` (existing) | personal git repo | only you |
| **user-personal** | `~/.knowledge/` | personal git repo with remote | only you, across all projects and years |

One query layer fans out to all three, source-tagged results, ranked union.

## The two tiers (user-personal only)

- **Working set** (`~/.knowledge/lessons/`): current, always searched.
- **Archive** (`~/.knowledge/archive/<year>/<project>/`): old, searched only with `include_archive=true`.

Archive is triggered manually via `/mm-archive <project>` when you leave a project. Cross-project lessons get promoted to the general working set before the rest is archived.

## Capture

Capture happens automatically via Claude Code hooks — no commands to remember:

- **PreCompact hook**: before context compression, mastermind reads the transcript and extracts lessons/decisions/patterns into `<scope>/pending/` for review.
- **`/mm-extract` skill**: manual fallback — run at session end to capture anything the hook missed.
- **`mm_write` MCP tool**: agent writes knowledge directly to the live store when you ask it to save something mid-session.

Auto-extracted entries land in `pending/`. User-initiated writes (`mm_write`) go straight to the live store — the user IS the review.

## Retrieval

Retrieval is also automatic:

- **SessionStart hook**: injects open loops + project knowledge into the agent's context at session start.
- **PostToolUse hook**: when the agent reads/edits a file, mastermind nudges it if relevant knowledge exists ("mastermind has 4 entries about electron — consider mm_search").
- **`mm_search` MCP tool**: keyword search across all three scopes, ranked by topic relevance + access frequency. Agents call this proactively.
- **Access frequency scoring**: entries returned by mm_search track access counts — frequently useful entries rank higher over time.

## Format

Every entry is a markdown file with frontmatter. See [docs/FORMAT.md](docs/FORMAT.md) — this is the most important file in the project and must stay stable.

## MCP tools

Four tools, forever. Adding a fifth requires a DECISIONS.md entry.

| Tool | Purpose |
|------|---------|
| `mm_search` | Search knowledge across all scopes |
| `mm_write` | Write an entry to the live store (user-initiated) |
| `mm_promote` | Move a pending entry to the live store (after review) |
| `mm_close_loop` | Resolve an open-loop → archived to `resolved-loops/` |

## Non-goals

- No server. No hub. No account.
- No vector store, no embeddings, no fine-tuning. Keyword search over plain markdown is enough.
- No replacement for context-mode. mastermind stacks with it automatically (context-mode indexes mm_search output into its FTS5 cache for warm follow-ups).
- No saving code, file paths, or reconstructible information. Only insights, lessons, decisions, patterns.

## Optional intelligence

By default, mastermind is zero-dependency (keyword extraction, stdlib Go). Optionally, set `MASTERMIND_EXTRACT_MODE=llm` to use a language model for smarter extraction:

- **Anthropic API** (default): uses Haiku via your existing `ANTHROPIC_API_KEY`. ~$0.001 per extraction.
- **Ollama** (local): set `MASTERMIND_LLM_PROVIDER=ollama` for fully local extraction with no API calls.

## Status

All four MCP tools functional. Four Claude Code hooks wired (SessionStart, PreCompact, PostToolUse, MCP server). See [docs/](docs/) for the full specification.
