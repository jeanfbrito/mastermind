# mastermind

A personal engineering second brain: repo-local shared knowledge + cross-project personal memory + lifelong archive, backed by plain markdown and git.

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
| **project-shared** | `<repo>/.mm/` | git (checked in) | team |
| **project-personal** | `~/.claude/projects/<repo>/memory/` (existing) | personal git repo | only you |
| **user-personal** | `~/.mm/` | personal git repo with remote | only you, across all projects and years |

One query layer fans out to all three, source-tagged results, ranked union.

## The two tiers (user-personal only)

- **Working set** (`~/.mm/lessons/`): current, always searched.
- **Archive** (`~/.mm/archive/<year>/<project>/`): old, searched only with `include_archive=true`.

Archive is triggered manually via `/mm-archive <project>` when you leave a project. Cross-project lessons get promoted to the general working set before the rest is archived.

## Capture

Capture is the hard problem. mastermind solves it via explicit `/mm-extract` at session end:

1. You run `/mm-extract` after a session where you learned something.
2. Tool reads the transcript, proposes candidate entries in the mastermind format, and suggests a scope for each.
3. Candidates land in `<scope>/pending/`.
4. You review with git diff, edit, accept, reject.
5. Commit.

No auto-writes. No background extraction. Review is the consolidation.

## Format

Every entry is a markdown file with frontmatter. See [docs/FORMAT.md](docs/FORMAT.md) — this is the most important file in the project and must stay stable.

## Non-goals

- No server. No hub. No account.
- No vector store, no embeddings, no fine-tuning. FTS5 over plain markdown is enough.
- No replacement for context-mode. mastermind is a consumer of context-mode's index.
- No saving code, file paths, or reconstructible information. Only insights, lessons, decisions, patterns.
- No automatic writes without review.

## Status

Design phase. See [docs/](docs/) for the full specification.
