# mastermind

> *Persistent memory for AI coding agents — survives sessions, projects, and years.*

mastermind is a personal engineering knowledge base that gives your AI agent long-term memory. It captures lessons, decisions, patterns, and war stories from your coding sessions and surfaces them automatically when they're relevant — so you never re-debug the same bug or re-make the same decision.

Built as an [MCP](https://modelcontextprotocol.io) server + CLI hooks for [Claude Code](https://docs.anthropic.com/en/docs/claude-code). Single Go binary, zero runtime dependencies, plain markdown storage synced with git.

## How it works

```
Session starts
  → mastermind injects relevant knowledge + open loops (SessionStart hook)

You work normally
  → agent reads a file, mastermind nudges: "3 entries about electron — consider mm_search"
  → agent finds something worth keeping, calls mm_write

Context gets compressed
  → mastermind auto-extracts lessons from the transcript (PreCompact hook)
  → extracted entries land in pending/ for your review

Next session
  → cycle repeats, knowledge compounds
```

No commands to remember. No dashboards. No streaks. The default state is invisible.

## Install

```bash
# From source
go install github.com/jeanfbrito/mastermind/cmd/mastermind@latest

# Or build locally
git clone https://github.com/jeanfbrito/mastermind.git
cd mastermind
make build
make install    # copies to ~/.local/bin/
```

Requires Go 1.25+.

## Setup

### 1. Add the MCP server

Add to your Claude Code MCP config (`~/.claude/config.json`):

```json
{
  "mcpServers": {
    "mastermind": {
      "type": "stdio",
      "command": "mastermind"
    }
  }
}
```

### 2. Add the hooks

Add to `~/.claude/settings.json` inside the `"hooks"` object:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "mastermind session-start" }]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "mastermind extract --from-hook" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Read|Edit|Write",
        "hooks": [{ "type": "command", "command": "mastermind suggest --from-hook" }]
      }
    ]
  }
}
```

### 3. Initialize your knowledge store

```bash
mkdir ~/.knowledge
cd ~/.knowledge && git init
```

Project-level stores (`.knowledge/` in a repo) are created automatically on first use. Opt out with `MASTERMIND_NO_AUTO_INIT=1`.

## The three scopes

| Scope | Location | Synced via | Contains |
|---|---|---|---|
| **user-personal** | `~/.knowledge/` | git remote | Cross-project lessons, general patterns |
| **project-shared** | `<repo>/.knowledge/` | repo git | Project-specific knowledge, shareable with team |
| **project-personal** | `~/.claude/projects/<slug>/memory/` | personal git | Private notes about a project |

All three are searched automatically by `mm_search`. Results are tagged by scope.

## MCP tools

| Tool | What it does |
|------|-------------|
| `mm_search` | Search knowledge across all scopes. Agents should call this proactively. |
| `mm_write` | Save an entry to the live store. Use when you discover something worth keeping. |
| `mm_promote` | Move a pending entry (from auto-extraction) to the live store after review. |
| `mm_close_loop` | Mark an open-loop as resolved. Moves to `resolved-loops/`, stops appearing at session start. |

## Entry format

Every entry is a markdown file with YAML frontmatter:

```markdown
---
date: 2026-04-09
project: my-project
tags: [electron, webview, security]
topic: DOMPurify strips details/summary elements by default — must allowlist
kind: lesson
scope: project-shared
category: electron/security
confidence: high
---

## What happened
Used DOMPurify to sanitize markdown HTML output. The <details> and
<summary> elements were silently stripped because they're not in
DOMPurify's default allowlist.

## Fix
Add ALLOWED_TAGS: ['details', 'summary'] to the DOMPurify config.

## Rule
Always check DOMPurify's default allowlist when HTML elements
disappear after sanitization.
```

Six entry kinds: `lesson`, `insight`, `war-story`, `decision`, `pattern`, `open-loop`.

See [docs/FORMAT.md](docs/FORMAT.md) for the full schema — this is a long-term contract.

## Claude Code hooks

| Hook | When it fires | What mastermind does |
|------|--------------|---------------------|
| **SessionStart** | Session opens | Injects open loops + project knowledge into agent context |
| **PreCompact** | Before context compression | Extracts lessons from transcript → `pending/` |
| **PostToolUse** | After Read/Edit/Write | Nudges agent if knowledge exists about the file being touched |

## Optional LLM extraction

By default, extraction uses keyword/regex matching (zero dependencies). For smarter extraction, enable LLM mode:

```bash
# Anthropic API (uses Haiku, ~$0.001 per extraction)
export MASTERMIND_EXTRACT_MODE=llm

# Or use a local model via Ollama
export MASTERMIND_EXTRACT_MODE=llm
export MASTERMIND_LLM_PROVIDER=ollama
```

## Design philosophy

- **Invisible by default.** No notifications, no badges, no reminders. The tool works through hooks, not habits.
- **Plain markdown + git.** No database, no embeddings, no vector store. Files on disk are the database. Git is the sync layer. A markdown file written today will still parse in 2034.
- **Zero runtime dependencies.** Single static Go binary. No Python, no Node, no Docker, no Postgres. If the binary exists, it works.
- **ADHD-first design.** Every feature passes the test: "does this work on a bad working-memory day?" If it requires you to remember to use it, it's the wrong design.
- **Simplicity is a feature.** A small, focused tool surface beats a sprawling one.

## Non-goals

- No server, hub, or account
- No vector store or embeddings — keyword search is enough for career-scale corpora
- No saving code or file paths — only lessons, decisions, patterns, insights
- No multi-agent coordination — this is personal memory, not shared infrastructure

## Docs

- [docs/CONTINUITY.md](docs/CONTINUITY.md) — the five load-bearing behaviors
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — package layout and MCP tool surface
- [docs/FORMAT.md](docs/FORMAT.md) — the entry schema (long-term contract)
- [docs/DECISIONS.md](docs/DECISIONS.md) — why every architectural choice is what it is
- [docs/ROADMAP.md](docs/ROADMAP.md) — what's planned

## License

MIT
