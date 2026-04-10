# Architecture

mastermind is a small MCP server + a handful of slash commands. There is no daemon, no database (FTS5 aside), no server to deploy. Storage is plain markdown in directories you control.

## Implementation language

**Go.** Single static binary, distributed as a GitHub release artifact (and optionally a Homebrew tap). No runtime installation on target machines — no Node, no Python, no `venv`. Install is one command; running on a new machine is one copy. `go.sum` is committed so builds are reproducible across years.

See `DECISIONS.md` for the full rationale. Short version: mastermind's value depends on the tool still running in a decade, and Go's combination of a 13-year-running backward compatibility promise, static binaries, and cross-compilation directly addresses that requirement. The stdlib covers almost everything mastermind needs, so the dependency graph stays tiny. `~/Github/rtk` remains a conceptual blueprint for the tool's shape, even though it's Rust — patterns translate.

**Consequence for the architecture**: mastermind is a single binary exposing an MCP server over stdio (the standard Claude Code transport). Slash commands invoke it via MCP tool calls. There is no separate daemon, no background service, no auxiliary process — one binary, one process per Claude Code session, started on demand.

**Expected dependency profile**: minimal. Target total: fewer than 10 direct dependencies.
- Go stdlib for filesystem, glob, JSON, `os/exec` (git), goroutines (parallel indexing).
- One MCP SDK (likely the official `github.com/modelcontextprotocol/go-sdk` or similar — confirm in Phase 0).
- One YAML frontmatter parser (`gopkg.in/yaml.v3` + a tiny splitter, or `github.com/adrg/frontmatter`).
- One markdown parser only if body sections need structured access (`github.com/yuin/goldmark`) — skip if plain text is enough.
- No ORM, no framework, no DI container, no code generator. Plain Go.

**Scaffold state (2026-04-04)**: verified building on Go 1.26.1 darwin/arm64. `go.mod` pins `go 1.22` as the minimum. `make build` produces `bin/mastermind` with a version string injected via `-ldflags`. The `internal/` layout is in place with `store`, `format`, `search`, `mcp` as empty packages (doc.go only) — each with a clear responsibility comment. Zero dependencies so far; the first real dependency will be the MCP SDK in Phase 0, followed by a YAML frontmatter parser in Phase 1.

**Directory layout**:
```
mastermind/
├── cmd/mastermind/main.go      # entry point, MCP server bootstrap
├── internal/
│   ├── format/                 # frontmatter parse/validate/serialize
│   ├── store/                  # three-scope markdown storage + pending/ invariant
│   ├── search/                 # fan-out query via context-mode FTS5 (+ grep fallback)
│   └── mcp/                    # MCP tool registration; only place the SDK is imported
├── docs/                       # design spec (this file and siblings)
├── Makefile                    # build / run / test / vet / fmt / tidy / install
└── go.mod                      # module github.com/jeanfbrito/mastermind
```

## The three stores

| Scope | Path | Sync | Visible to |
|---|---|---|---|
| **project-shared** | `<repo>/.knowledge/` | git (part of the repo) | anyone who clones the repo |
| **project-personal** | `~/.claude/projects/<repo>/memory/` | personal git repo | only you |
| **user-personal** | `~/.knowledge/` | personal git repo with remote | only you, across projects and years |

Two of the three already exist physically:
- Claude Code's auto-memory dir already acts as project-personal.
- Your existing `~/.claude/lessons.md` is the seed of user-personal (mastermind reads it in place initially, no migration).

The only **new** store on disk is project-shared (`<repo>/.knowledge/`).

## Directory layout

### user-personal (`~/.knowledge/`)

```
~/.knowledge/
├── FORMAT.md                    # the one immutable contract
├── lessons/                     # working set (always searched)
│   ├── electron-ipc-macos.md
│   ├── retry-budget-heuristic.md
│   └── ...
├── archive/                     # searched only with include_archive=true
│   ├── 2024/
│   │   └── RocketChatElectron/
│   │       ├── obscure-bug-1.md
│   │       └── ...
│   └── 2025/
│       └── SomeProject/
│           └── ...
├── pending/                     # extraction candidates awaiting review
│   └── 2026-04-04-auth-flow.md
└── .git/                        # sync via git remote
```

### project-shared (`<repo>/.knowledge/`)

```
<repo>/.knowledge/
├── nodes/                       # curated team knowledge
│   ├── auth-architecture.md
│   ├── build-pipeline.md
│   └── ...
└── pending/                     # extraction candidates for this project
    └── 2026-04-04-deployment-gotcha.md
```

No archive tier in project-shared. Projects end; the whole `.knowledge/` dir goes with the repo.

### project-personal (existing Claude auto-memory)

```
~/.claude/projects/<repo>/memory/
├── MEMORY.md                    # Claude's existing index
├── lessons.md                   # existing
└── ...                          # existing files, read in place
```

mastermind does not move or restructure this. It reads it and indexes it.

## MCP tools exposed

Minimal surface. The SDK is `github.com/modelcontextprotocol/go-sdk` v1.4.1 (see DECISIONS.md) with stdio transport. All tools use the generic `mcp.AddTool[Input, Output]` registration pattern with struct-tag JSON Schema.

1. **`mm_search(query, scopes?, include_archive?)`** — primary read. Fans out to all three scopes (or the subset specified), queries via context-mode's FTS5 with source labels, returns ranked results with source tags. Defaults: all scopes, archive excluded.
2. **`mm_write(content, scope, kind)`** — programmatic write (used by extraction and curate paths). Writes to `<scope>/pending/`, never directly to the live store.
3. **`mm_promote(pending_path, target_scope)`** — move a pending entry into the live store after review.
4. **`mm_close_loop(loop_id, resolution)`** — mark an open-loop as resolved. Agent calls this when the user indicates a loop is done ("ok I finished that auth refactor"). Moves the entry to `<scope>/resolved-loops/` for history and removes it from session-start injection.

Four tools total, forever. Adding a fifth requires a DECISIONS.md entry with a justification.

## CLI subcommands (non-MCP)

These subcommands are **not** MCP tools — they are CLI commands invoked by Claude Code hooks, outside the MCP protocol. They read/write the same store but run as short-lived subprocesses, not tool calls inside a running session.

1. **`mastermind session-start --cwd <dir>`** — invoked by a Claude Code SessionStart hook. Walks up from `--cwd` to find the nearest `.knowledge/` (if any), queries all three scopes, assembles the continuity-injection block (open-loops, relevant lessons, pending count), and writes it to stdout for Claude Code to inject as system context. Must return in <200ms. If slow, returns nothing silently. See CONTINUITY.md.
2. **`mastermind post-compact [--cwd <dir>]`** — invoked by a Claude Code PostCompact hook. Fires after context compaction, when the agent has just lost most of its working memory. Re-injects a curated project-scoped slice (open loops from project-shared and project-personal only; top project knowledge entries) so the next turn starts oriented. Scope is narrower than session-start: user-personal open loops and the pending count are excluded — this is about re-hydrating the current project, not the full session picture. Reads hook JSON from stdin if present (cwd field), falls back to --cwd flag or os.Getwd(). Silent if nothing to surface.
3. **`mastermind session-close --transcript <path>`** — invoked by a Claude Code session-close hook. Phase 1 (sync): validates and archives the transcript to `~/.knowledge/sessions/<timestamp>-<session-id>/`, forks a detached Phase 2 subprocess, returns immediately (<100ms target). Phase 2 (detached): loads the archived transcript, calls the extraction LLM, writes candidates to `<scope>/pending/`, logs telemetry. See EXTRACTION.md.

These subcommands are the load-bearing mechanism for the continuity layer. They convert mastermind from "a memory tool you use" into "a memory layer that runs silently." See CONTINUITY.md for why this distinction matters for the primary user.

## Slash commands (thin wrappers)

Slash commands live in Claude Code configuration, not in mastermind's binary. Each one is a wrapper that invokes an MCP tool or CLI subcommand with the right arguments.

- `/mm-search <query>` — thin wrapper around `mm_search`.
- `/mm-review` — starts the pending/ review flow (one entry at a time, keyboard-driven). See CONTINUITY.md for rules.
- `/mm-curate <text>` — manual one-shot entry creation. Prompts for scope and kind, builds frontmatter, writes via `mm_write`.
- `/mm-extract` — fallback manual extraction. Same pipeline as session-close, triggered explicitly. See EXTRACTION.md for why this is secondary.
- `/mm-archive <project>` — project transition. Finds all entries with matching project frontmatter, proposes cross-project promotion, moves non-promoted entries to `~/.knowledge/archive/<year>/<project>/`.
- `/mm-init` — warmup for a new project. Explores the codebase and seeds `<repo>/.knowledge/nodes/` with initial curated knowledge.

## Claude Code hook integration

Mastermind depends on three Claude Code hooks being registered in the user's Claude Code config (`~/.claude/settings.json` or equivalent). The installation instructions live in the README and are a **one-time** setup cost — after that, the hooks run automatically, forever, with no further user action required. This is the load-bearing automation that eliminates the "remember to trigger the tool" failure mode.

**SessionStart hook** (runs when Claude Code opens a session in a directory):
```
mastermind session-start --cwd "$PWD"
```
Output (stdout) is injected as system context before the first user turn. Silent if no `.knowledge/` is found or all context sections are empty.

**PreCompact hook** (runs before Claude Code compresses the conversation context):
```
mastermind extract --from-hook
```
Reads hook JSON (transcript_path, cwd) from stdin. Extracts knowledge candidates into `<scope>/pending/`. Returns immediately. User sees nothing.

**PostCompact hook** (runs after Claude Code compresses the conversation context):
```
mastermind post-compact
```
Reads hook JSON (cwd) from stdin if present. Re-injects project-scoped open loops and knowledge entries into the compressed context so the agent stays oriented. Output (stdout) is injected as system context. Silent if no project knowledge is found.

**session-close hook** (not yet implemented — see Phase 3b in ROADMAP.md):
```
mastermind session-close --transcript "$CLAUDE_TRANSCRIPT_PATH"
```
Returns immediately, detaches Phase 2. User sees nothing.

If Claude Code's hook API surface doesn't support these exact lifecycle events, we fall back to the nearest approximations (e.g., a wrapper script that runs mastermind before/after `claude` is invoked). The goal state — automatic fire at session boundaries — is non-negotiable; the implementation mechanism can vary by platform.

**Installation check**: `mastermind doctor` (future addition, not Phase 1) verifies the hooks are registered and working. Runs on demand; never nags.

## Retrieval flow

`mm_search` runs a stdlib-only keyword matcher with a tiered fallback chain. Implementation in `internal/search/search.go`; full rationale in DECISIONS.md (2026-04-10 entry).

1. Agent calls `mm_search("hook extraction")`.
2. mastermind reads entry refs from all three stores (or the configured subset). Frontmatter is parsed by `internal/store` during listing; bodies are not loaded yet.
3. A metadata-only filter drops any ref that fails the kind/project/tags filters (cheap — no body reads).
4. Each surviving ref is run through the tiered scoring pipeline. Results are sorted by tier class first, score second.

### Tier classes (primary sort key)

| Class | Name | Match criterion |
|---|---|---|
| 0 | `classExactTopic` | full query phrase appears in topic (multi-word queries only) |
| 1 | `classExactTag` | full query phrase appears in a tag |
| 2 | `classExactBody` | full query phrase appears in body (pass 2) |
| 3 | `classTopicTokens` | all query tokens found in topic |
| 4 | `classMetaTokens` | all query tokens found across topic + tags |
| 5 | `classKeyword` | tokens matched in body (pass 2, default keyword pipeline) |
| 6 | `classFuzzy` | sahilm/fuzzy gap-match against topic + tags (pass 3 fallback) |

Sort order: `class ASC → score DESC → date DESC → path ASC`. A class-0 hit strictly dominates any class-6 hit regardless of access frequency, recency, or score magnitude. This is the "engram `Rank = -1000` sentinel pattern" — class is lock-in-by-construction, not a tuning parameter.

### Within-class ranking

Inside a single class, `score` breaks ties. Score is the additive sum of:
- Topic-token weight (2.0 per matched token)
- Tag-token weight (0.7 per matched token, topic wins on duplicate hits)
- Body-keyword weight (0.3 per hit, log-diminishing up to ~0.75 per token, pass 2 only)
- ACT-R fast-mode access boost: `ln(accessed+1) * 0.2`, additive, capped at +0.5 (borrowed from shiba-memory)

The 0.5 access-boost cap preserves the load-bearing invariant that a single topic hit (2.0) strictly dominates any combination of tag + body + access boost within the same class.

### Three-pass execution

- **Pass 1** (metadata-only, no body I/O): handles classes 0, 1, 3, 4. Every ref is tested against topic and tag strings only.
- **Pass 2** (body load): handles classes 2 and 5. Runs only on refs that pass 1 could not classify into a meta-only class. **Skipped entirely** when the short-circuit condition fires: top-K pass-1 results (K = min(Limit, 3)) are all in class ≤ 4 AND at least one has `access_count ≥ 3`. The access gate (borrowed from shiba-memory's `003_instincts_tracking_gateway.sql`) prevents structural matches from short-circuiting before the entry has proven useful.
- **Pass 3** (fuzzy fallback): handles class 6. Runs only when earlier passes returned fewer results than the limit AND the normalized query is ≥ 4 characters (engram's length-guard pattern — short queries drown precision). Uses `sahilm/fuzzy.Find` against a per-entry `topic + " " + tags` blob. Body fuzzy is deliberately rejected; see DECISIONS.md.

After all passes, results are sorted one final time and truncated to `Query.Limit` (default 10). Bodies for top-N results are lazily loaded for presentation (so class 3/4 metadata-only hits can still render body excerpts via `BodyExcerpt`).

No custom retrieval logic beyond stdlib and `sahilm/fuzzy`. No persistent index — every query re-reads the corpus from disk via `internal/store`, which holds an ephemeral mtime-keyed in-memory cache. This is fast (sub-100ms) at realistic corpus sizes (thousands of entries, megabytes of text).

**Relationship to context-mode**: mastermind does NOT index into context-mode. The synergy is passive — context-mode automatically indexes any MCP tool's output as session content, so `mm_search` results are re-findable within the session via context-mode without mastermind being involved. See hard rule #4 in CLAUDE.md.

## Indexing flow

None. mastermind owns no persistent index and builds nothing at startup.

Every query walks the filesystem fresh. `internal/store` caches parsed `EntryRef` slices keyed by directory mtime — if a directory hasn't changed since the last listing, the cached slice is reused. The cache is ephemeral in-memory only; nothing touches disk beyond the markdown files themselves.

See CLAUDE.md hard rule #3: "No persistent index. Files on disk are the database."

## Writes: always through pending/

All writes — manual curation, extraction, programmatic — land in `<scope>/pending/` first. The only way an entry reaches the active store is via `mm_promote` (or manually moving the file).

This single rule prevents the main failure mode: junk entries polluting the corpus. Review is mandatory and happens via normal file-system tools (git diff, your editor, `mv`, `rm`).

## Sync story

| Store | How it syncs |
|---|---|
| project-shared | git, part of the repo, zero new infrastructure |
| user-personal | `~/.knowledge/` is itself a git repo with a remote (private GitHub repo, Gitea, whatever). `git pull` on the other machine. Optionally: pre-session hook auto-pulls. |
| project-personal | separate personal git repo that tracks `~/.claude/projects/*/memory/`, or accept it as machine-local |

No daemon, no Syncthing, no S3, no cloud service. Git is the only sync mechanism. If a machine is offline, mastermind still works against the local copy.

## Dependency on context-mode

mastermind **uses** context-mode; it does not replace or extend it.
- context-mode provides FTS5 indexing and search.
- mastermind indexes its files into context-mode at startup and queries via `ctx_search`.
- If context-mode disappears, mastermind degrades to a grep-based fallback. The corpus (markdown files) remains fully intact and readable with any tool.

This dependency is deliberate and asymmetric: context-mode owns the search layer, mastermind owns the knowledge layer. No overlap, no competition.

## What mastermind is not

- Not a server. A local MCP process, started by Claude Code, owns nothing durable.
- Not a database. FTS5 is a cache of markdown content; deleting it rebuilds from files.
- Not a vector store. Keyword search is sufficient at this scale.
- Not a replacement for brv or OpenViking. It steals ideas from both, no code from either.
- Not an auto-extractor. Extraction only runs when you explicitly call `/mm-extract`.
