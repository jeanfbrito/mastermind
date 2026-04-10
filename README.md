# mastermind

> Persistent memory for AI coding agents — survives sessions, projects, and years.

mastermind gives your AI coding agent long-term memory. It captures lessons, decisions, patterns, and war stories from your sessions and surfaces them automatically when relevant — so you stop re-debugging the same bugs and re-making the same decisions.

Built as an [MCP](https://modelcontextprotocol.io) server with CLI hooks for [Claude Code](https://docs.anthropic.com/en/docs/claude-code). Single Go binary, zero runtime dependencies, plain markdown files synced with git.

## How it works

```
Session starts
  → mastermind injects relevant knowledge + open loops (SessionStart hook)

You work normally
  → agent reads a file → mastermind nudges: "3 entries about electron — consider mm_search"
  → agent discovers something worth keeping → calls mm_write

Context gets compressed
  → mastermind extracts lessons from the conversation (PreCompact hook)
  → extracted entries land in pending/ for review

Next session
  → knowledge compounds automatically
```

No commands to remember. No dashboards. The default state is invisible.

## Install

```bash
go install github.com/jeanfbrito/mastermind/cmd/mastermind@latest
```

Or build from source:

```bash
git clone https://github.com/jeanfbrito/mastermind.git
cd mastermind
make build && make install
```

Requires Go 1.25+.

## Setup

### 1. Register the MCP server

Add to `~/.claude/config.json`:

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

### 2. Configure the hooks

Add to `~/.claude/settings.json` under `"hooks"`:

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

### 3. Initialize the knowledge store

```bash
mkdir ~/.knowledge && cd ~/.knowledge && git init
```

Project-level stores (`<repo>/.knowledge/`) are created automatically on first use. Opt out with `MASTERMIND_NO_AUTO_INIT=1`.

## Knowledge scopes

mastermind organizes knowledge into three scopes, all searched automatically:

| Scope | Location | Synced via | Use case |
|---|---|---|---|
| **user-personal** | `~/.knowledge/` | personal git remote | General engineering lessons that apply across all projects |
| **project-shared** | `<repo>/.knowledge/` | committed to repo | Project-specific knowledge shared with the team |
| **project-personal** | `~/.claude/projects/<slug>/memory/` | personal git | Private notes about a specific project |

`mm_search` fans out to all three scopes and returns source-tagged, ranked results.

## MCP tools

| Tool | Description |
|------|-------------|
| `mm_search` | Search knowledge across all scopes. Called proactively by the agent at task start. |
| `mm_write` | Write an entry to the live store. The agent calls this when it discovers something worth preserving. |
| `mm_promote` | Promote a pending entry (from auto-extraction) to the live store after review. |
| `mm_close_loop` | Resolve an open loop. Archives the entry to `resolved-loops/` so it stops surfacing. |

## Hooks

| Hook | Trigger | Action |
|------|---------|--------|
| **SessionStart** | Session opens | Injects open loops and project-relevant knowledge into the agent's context |
| **PreCompact** | Before context compression | Extracts lessons from the conversation transcript into `pending/` |
| **PostToolUse** | After Read, Edit, or Write | Nudges the agent when relevant knowledge exists for the file being touched |

## Entry format

Every entry is a markdown file with YAML frontmatter:

```yaml
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
```

```markdown
## What happened
Used DOMPurify to sanitize markdown output. The <details> and <summary>
elements were silently stripped — they're not in DOMPurify's default allowlist.

## Fix
Add ALLOWED_TAGS: ['details', 'summary'] to the DOMPurify config.

## Rule
Always check DOMPurify's default allowlist when HTML elements disappear.
```

Entry kinds: `lesson` · `insight` · `war-story` · `decision` · `pattern` · `open-loop`

See [docs/FORMAT.md](docs/FORMAT.md) for the full schema.

## Optional LLM extraction

By default, knowledge extraction uses keyword/regex matching with zero external dependencies. For higher-quality extraction, enable LLM mode:

```bash
# Use Anthropic API (Haiku)
export MASTERMIND_EXTRACT_MODE=llm

# Or use a local model via Ollama
export MASTERMIND_EXTRACT_MODE=llm
export MASTERMIND_LLM_PROVIDER=ollama
```

## Configuration

| Environment variable | Default | Description |
|---------------------|---------|-------------|
| `MASTERMIND_NO_AUTO_INIT` | _(unset)_ | Set to `1` to disable automatic `.knowledge/` creation in git repos |
| `MASTERMIND_EXTRACT_MODE` | `keyword` | `keyword` for regex-based extraction, `llm` for model-powered extraction |
| `MASTERMIND_LLM_PROVIDER` | `anthropic` | `anthropic` or `ollama` |
| `MASTERMIND_LLM_MODEL` | _(auto)_ | Model identifier (defaults to Haiku for Anthropic, llama3.2 for Ollama) |
| `MASTERMIND_OLLAMA_URL` | `http://localhost:11434` | Ollama API endpoint |

## Design principles

- **Zero friction.** Capture and retrieval happen through hooks, not commands. The tool works on autopilot — nothing to remember, nothing to maintain.
- **Plain markdown + git.** No database. Files on disk are the store, git is the sync layer. A markdown file written today will still parse in 2034.
- **Zero runtime dependencies.** Single static Go binary. No Python, no Node, no Docker, no Postgres.
- **Scoped sharing.** Personal knowledge stays personal. Project knowledge lives in the repo, versioned and shareable. The boundary is explicit.

## Non-goals

- No server, hub, or cloud account
- No vector store or embeddings — keyword search scales to career-length corpora
- No code or file path storage — only insights, lessons, decisions, and patterns

## Documentation

| Doc | Contents |
|-----|----------|
| [CONTINUITY.md](docs/CONTINUITY.md) | Core behaviors: session-start injection, extraction, open-loop lifecycle |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Package layout, MCP tool surface, hook integration |
| [FORMAT.md](docs/FORMAT.md) | Entry schema — the long-term contract |
| [DECISIONS.md](docs/DECISIONS.md) | Architectural decision log |
| [ROADMAP.md](docs/ROADMAP.md) | Planned work |

## License

MIT
