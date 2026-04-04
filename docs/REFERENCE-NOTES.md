# Reference notes — Phase 0 synthesis

Consolidated findings from the four Phase 0 explorations. Each source contributes one thing; together they answer "what do we build on, what do we avoid, what do we translate, what do we ignore."

Full per-source reports live in `docs/reference-notes/`:

- `rtk.md` — Rust hook-interceptor (NOT an MCP server). Style and release workflow only.
- `brv.md` — byterover-cli. Node format and behavior reference. Rewritten in Go, not forked.
- `openviking.md` — OpenViking extraction pipeline and prompt (verbatim captured).
- `go-mcp-landscape.md` — Go MCP SDK survey and real-world Go MCP server census.

## Unlocked decisions

1. **Go MCP SDK**: `github.com/modelcontextprotocol/go-sdk` v1.4.1.
2. **Primary implementation reference**: `Gentleman-Programming/engram` (cloned at `~/Github/engram`).
3. **rtk's role**: demoted to secondary reference for CLI style and release workflow. Not an MCP wiring reference — rtk is a hook interceptor, not a server.
4. **Node format**: validated against brv's actual on-disk format. Keep our FORMAT.md schema; brv's field names (name, tags, keywords) inform ours but we extend with `kind`, `confidence`, `project`, `date`, `topic` for the second-brain use case.
5. **Extraction**: implement OpenViking's two-phase pattern (fast return, async extraction) but **diverge on the review queue** — mastermind's pending/ flow is mandatory, OpenViking auto-commits. This is a deliberate difference driven by the trust and ADHD requirements.

## Per-source distilled findings

### 1. Go MCP SDK landscape — the load-bearing finding

**Pick `github.com/modelcontextprotocol/go-sdk` v1.4.1.**

- Official, Google co-maintained, past v1.0, stable semver.
- 88 commits since 2026-01-01; latest v1.4.1 shipped 2026-03-13; v1.5.0-pre.1 out 2026-03-31.
- Minimal deps: `google/jsonschema-go`, `golang-jwt/jwt/v5`, `oauth2`, `segmentio/encoding`, `uritemplate`. No web framework.
- Requires Go 1.25+. Not a problem — we're on 1.26.1.
- Full transport surface: **StdioTransport** (what Claude Code uses), streamable HTTP, SSE, CommandTransport.
- Generic type-safe API: `mcp.AddTool[Input, Output]` with struct tags for JSON Schema. No `map[string]any` fishing.
- Known users: GitHub's own `github-mcp-server` (28.5k stars), `containers/kubernetes-mcp-server` (1.4k stars, pinned at v1.4.1 stable).

**Runner-up `mark3labs/mcp-go`**: 8.5k stars, more shipping users, but pre-1.0 with 47 minor releases in 18 months. Breaking-change tax we don't want for a long-lived personal tool.

**Rejected `metoro-io/mcp-golang`**: unmaintained since Sept 2025, zero 2026 commits, drags gin + ~30 indirect deps. Disqualifying.

**Canonical tool registration pattern** (from the official SDK README):

```go
type Input struct {
    Name string `json:"name" jsonschema:"the name of the person to greet"`
}
type Output struct {
    Greeting string `json:"greeting" jsonschema:"the greeting"`
}

func SayHi(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Output, error) {
    return nil, Output{Greeting: "Hi " + input.Name}, nil
}

func main() {
    server := mcp.NewServer(&mcp.Implementation{Name: "mastermind", Version: version}, nil)
    mcp.AddTool(server, &mcp.Tool{Name: "mm_search", Description: "..."}, mmSearchHandler)
    if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
        log.Fatal(err)
    }
}
```

This is the shape `internal/mcp/` will take.

### 2. engram — the structural twin

**URL**: https://github.com/Gentleman-Programming/engram
**Local clone**: `~/Github/engram`
**Last commit**: 2026-03-30

engram is mastermind's closest living relative. Everything structural we need, it already solves:

- `cmd/engram/main.go` — entry point
- `internal/mcp/mcp.go` — single-file tool registration module
- `internal/store/` — storage layer (SQLite + FTS5)
- `internal/server/` — HTTP API (optional; mastermind can skip)
- `internal/tui/` — bubbletea TUI (mastermind skips)
- `internal/project/`, `setup/`, `sync/`, `version/` — smaller utility packages
- `.goreleaser.yaml` at repo root, CGO_ENABLED=0, darwin/linux/windows × amd64/arm64
- Homebrew tap at `Gentleman-Programming/homebrew-tap`
- `modernc.org/sqlite` (pure-Go SQLite — no CGO, cross-compiles cleanly)

**Caveat**: engram uses `mark3labs/mcp-go`, not the official SDK we're adopting. **Treat engram as the reference for project layout, storage structure, distribution pipeline, and CLI shape**. For the literal MCP tool-registration call sites, use the official SDK's `examples/` directory and `containers/kubernetes-mcp-server`. The translation between the two SDKs is mechanical — both use struct-tag JSON Schema input types.

**Interesting commit to read during Phase 1**: `perf(mcp): defer 4 rare tools to reduce session startup tokens` (2026-03-26). They've already thought about the mastermind-adjacent problem of MCP tool-surface token budget at session startup. Worth understanding before we register our own tool set.

**Key divergence from engram**: engram uses SQLite as the storage format. Mastermind uses plain markdown files on disk (with context-mode's FTS5 handling search over them). This is deliberate — see DECISIONS.md on the "plain markdown + frontmatter" choice. We steal engram's *layout and distribution*, not its storage model.

### 3. brv — node format validation and what to avoid

brv (`~/Github/byterover-cli`) is TypeScript/Node, uses oclif + React/Ink TUI, has a daemon architecture, 20+ LLM providers as direct deps. We're **not** forking it — see DECISIONS.md.

**Validation**: brv's on-disk node format matches what we designed in FORMAT.md, with minor additions needed for the second-brain use case:

```markdown
---
name: "JWT Auth Pattern"
tags: ["auth", "security", "middleware"]
keywords: ["jwt", "token", "expiry", "24h"]
---

# JWT Auth Implementation

[Narrative content]
```

brv identifies nodes by **file path relative to `.brv/context-tree/`** — no explicit ID field. Mastermind adopts the same model (path = identity). We extend with: `date`, `project`, `topic`, `kind`, `confidence` (see FORMAT.md for the full schema).

brv's directory convention is `.brv/context-tree/{domain}/{category}/{node-name}.md`. Mastermind uses `.mm/nodes/{slug}.md` for project-shared (flatter — we don't need the two-level domain/category hierarchy at our scale) and `~/.mm/lessons/{slug}.md` for user-personal. Simpler is better.

**Hub coupling analysis (critical)**: brv's hub is **tightly coupled architecturally but optional functionally**. Core query/curate works without the hub, but hub code is woven through transport handlers, config store, auth flow, keychain integration. There is **no flag to disable it**; it's just harmless if unused. This confirms the rewrite-don't-fork decision — cleanly extracting the non-hub core from the brv codebase would be surgery, not refactoring.

**Things to steal from brv's behavior**:

- Walk-up project detection (look for `.mm/config.json` upward from cwd).
- Fire-and-forget curate pattern (the write returns immediately, index updates async).
- Multi-tier retrieval strategy (cache → local search → LLM) — for Phase 1 we only need the local search tier.
- Markdown + YAML frontmatter as the storage format.
- Repo-relative project scope (`.brv/` lives at the repo root).

**Things to deliberately NOT reproduce**:

- Hub as a cross-cutting concern. Mastermind has no hub, ever. If extensibility is ever needed it goes in as a clean plugin boundary.
- Cloud sync woven into core. Mastermind's sync is `git push` — no tool code touches it.
- TUI coupled to business logic. Mastermind has no TUI in Phase 1-5.
- Daemon IPC in the critical path. Mastermind is a single Go binary, started on demand by Claude Code over stdio. No daemon.
- 20+ LLM providers as direct deps. Mastermind talks to Claude Code via MCP; the LLM is on the other side of the transport.
- Snapshot state (CoGit). Git already handles history.
- Agent-dependent search. Local retrieval (FTS5 via context-mode) is the default, always.

### 4. OpenViking — the extraction prompt and the divergences

OpenViking (`~/Github/OpenViking`) is a Rust + Python hybrid (the extraction path is Python). **Auto-extraction exists as real, production-grade code** — not just marketing.

**Two-phase commit pattern**: Phase 1 synchronously archives session messages and returns immediately. Phase 2 asynchronously runs the extraction pipeline in the background. Mastermind adopts the same split — the session-close hook returns in milliseconds and the extractor runs detached. This is the foundation of the "capture without willpower" design goal.

**The verbatim extraction prompt** (from `openviking/session/memory/session_extract_context_provider.py`):

```
You are a memory extraction agent. Your task is to analyze conversations and update memories.

## Workflow
1. Analyze the conversation and pre-fetched context
2. If you need more information, use the available tools (read/search)
3. When you have enough information, output ONLY a JSON object (no extra text before or after)

## Critical
- ONLY read and search tools are available - DO NOT use write tool
- Before editing ANY existing memory file, you MUST first read its complete content
- ONLY read URIs that are explicitly listed in ls tool results or returned by previous tool calls

## Target Output Language
All memory content MUST be written in {output_language}.

## URI Handling
The system automatically generates URIs based on memory_type and fields. Just provide correct memory_type and fields.

## Edit Overview Files
After writing new memories, you MUST also update the corresponding .overview.md file.
```

**Scope assignment via `memory_type`**: The LLM outputs a `memory_type` field per candidate (e.g., `"profile"`, `"events"`, `"cases"`), and Jinja2 templates expand `{{ user_space }}` / `{{ agent_space }}` variables to resolve the URI. Clean pattern — Go equivalent is `text/template`. Mastermind uses the same idea: the extractor proposes a *scope* (`user-personal` / `project-shared` / `project-personal`) and a *kind* (`lesson` / `insight` / `war-story` / `decision` / `pattern` / `open-loop`), and the store routes based on both.

**Input shape**: full transcript assembled with timestamps. Language auto-detected from conversation, with fallback to config. Session timestamp grounding is included in the prompt to prevent date off-by-one errors. Mastermind copies all three (full transcript, timestamp grounding, language detection).

**Output shape**: a `MemoryOperations` object with three maps — `write_uris` (new), `edit_uris` (search/replace on existing), `delete_uris` (removal). Mastermind adopts the three-operation model (write/edit/delete) but routes all of them through `pending/` first.

**Critical divergence — review queue**: OpenViking auto-commits extracted memories directly to storage. Telemetry tracks created/merged/deleted counts. **No user review step.** This is where mastermind deliberately parts ways: all writes go to `<scope>/pending/`, reviewed at next session start, never auto-committed. OpenViking's trust model is "the extractor is good enough to not need review"; mastermind's is "the corpus is too valuable to trust unreviewed writes, AND the user has ADHD, AND review is the consolidation step that makes the lesson stick." The divergence is intentional and load-bearing.

**Things worth translating**:

- The two-phase commit (fast archive, async extraction) — the essential pattern for zero-attention-cost capture.
- Conversation assembly with timestamps and language detection.
- `memory_type` as the scope router (adapted to mastermind's three scopes + five kinds).
- The `.overview.md` index pattern (one index file per scope root, kept up to date on each write) — useful for agents browsing without a query.
- Merge operators for updating existing entries (search/replace blocks for edits).
- Telemetry on extraction phases (timing, candidate count, accepted count).

**Things to NOT translate**:

- The ReAct loop (LLM iteratively calls read/search tools before generating operations). Overkill for our scale. Single-shot extraction with the full transcript as input is enough.
- Distributed locks. Multi-tenant concern; not ours.
- The skill/tool memory subsystem (TOOLS and SKILLS categories). Entangled with VikingBot.
- Dynamic schema generation. Mastermind uses static Go structs.
- Auto-commit. See the divergence above.

### 5. rtk — style notes only (NOT an MCP reference)

**Critical correction from Phase 0**: rtk is **NOT an MCP server**. It's a hook-based CLI interceptor that installs into Claude Code, Cursor, Cline, Windsurf, etc. via agent-native hook mechanisms. It rewrites commands (`git status` → `rtk git status`) through a `BeforeTool` hook, not through MCP stdio.

**What this means**: rtk's role as "implementation blueprint" is downgraded. It's still useful for:

- **Rust → Go CLI structure translation**: single binary, clap-style subcommand organization, ecosystem-oriented module layout (`src/cmds/git/`, `src/cmds/rust/`, etc.).
- **Release workflow**: cross-compilation for 5 targets via GitHub Actions, release tag triggers, tar.gz archives, checksums. Mastermind uses goreleaser (which is Go-native and cleaner), but rtk's matrix informs the target list.
- **Error handling patterns**: anyhow + context chaining, which maps to Go's wrapped errors (`fmt.Errorf("...: %w", err)`).
- **Atomic file writes**: tempfile + rename, critical for not corrupting the corpus on crash. Mastermind adopts this for every write to the store.
- **Filtering pipeline**: rtk composes output filters in layers; mastermind doesn't need it now but the pattern is useful.
- **Global flags** (verbose, quiet, etc.): simple discipline, worth copying.

**What rtk is NOT useful for**:

- MCP server wiring. rtk doesn't have one.
- Tool registration. rtk doesn't register tools — it rewrites commands.
- Hook installation machinery. Mastermind isn't a hook interceptor.
- The 100+ ecosystem command handlers. Mastermind's tool surface is 3-5 tools forever.
- SQLite metrics database (47 KB in `src/core/tracking.rs`). If mastermind ever needs telemetry it's a JSON log file.

**Short version**: read rtk for style, not substance. engram is the substantive reference.

## Consolidated "things to NOT copy" list

Across all four sources, the things to explicitly avoid:

1. **Hub / cloud sync / server** (brv, OpenViking) — mastermind is local-only, sync is git.
2. **Auto-commit of extracted memories** (OpenViking) — pending/ queue is mandatory.
3. **ReAct loops in the extractor** (OpenViking) — single-shot extraction is enough at our scale.
4. **Dynamic schema generation** (OpenViking) — static Go structs.
5. **Distributed locks** (OpenViking) — single-user, single-machine.
6. **TUI coupled to business logic** (brv) — no TUI in Phase 1-5.
7. **Daemon IPC hop** (brv) — single binary, direct storage access.
8. **20+ LLM provider dependencies** (brv) — LLM lives on the other side of MCP.
9. **CoGit snapshots** (brv) — git handles history.
10. **Hook installation machinery** (rtk) — mastermind is MCP server, not hook interceptor.
11. **100+ subcommand handlers** (rtk) — 3-5 MCP tools, forever.
12. **Skill/tool memory subsystem** (OpenViking) — entangled with agent framework.
13. **Hook integrity verification / SHA-256 trust gates** (rtk) — not a security boundary for us.
14. **SQLite metrics database** (rtk) — JSON log file if we need telemetry at all.

## Consolidated "things to translate" list

1. **Project layout** (engram): `cmd/mastermind/main.go` + `internal/{mcp,store,format,search}`.
2. **Distribution** (engram): `.goreleaser.yaml`, darwin/linux/windows × amd64/arm64, Homebrew tap.
3. **MCP wiring** (official SDK examples + kubernetes-mcp-server): `mcp.NewServer` + `mcp.AddTool[In, Out]` with struct tags.
4. **Tool registration convention** (engram's `internal/mcp/mcp.go`): single file, one function per tool.
5. **Node format** (brv, validated): markdown + YAML frontmatter, path = identity, stored in `.mm/nodes/` (project) or `~/.mm/lessons/` (user).
6. **Two-phase commit for extraction** (OpenViking): fast archive + async extraction.
7. **Transcript-with-timestamps prompt construction** (OpenViking).
8. **Scope assignment via a schema field** (OpenViking `memory_type` → mastermind `scope` + `kind`).
9. **`.overview.md` index files per scope root** (OpenViking): auto-updated on write, agent-browsable without a query.
10. **Walk-up project detection** (brv): find nearest `.mm/` ancestor from cwd.
11. **Fire-and-forget curate UX** (brv): the write returns immediately, indexing is async.
12. **Atomic file writes** (rtk): tempfile + rename for every store write.
13. **Error wrapping with context** (rtk → Go's `fmt.Errorf("...: %w", err)`).
14. **Cross-compilation matrix** (rtk): darwin/linux/windows × amd64/arm64 (engram's target list).

## Phase 1 start: engram-first

Phase 1 begins with reading engram's `internal/mcp/mcp.go`, `internal/store/`, and `cmd/engram/main.go` in order, then translating the structural patterns to mastermind's layout. For the MCP tool registration calls specifically, use the official SDK's README example (reproduced above) as the canonical form, not engram's mark3labs calls.

brv and OpenViking are consulted for behavioral questions during Phase 2-4, not Phase 1. rtk is consulted for error handling and atomic-write patterns when those come up.

`docs/reference-notes/` is the appendix — this file is the index. When Phase 1 hits a question not answered here, search the appendix via context-mode rather than re-reading the files wholesale.

## Appendix: what mastermind is NOT (specifically, what engram IS)

engram is the closest relative we found, and it's close enough that "why not just use engram?" is a legitimate question. The answer is that mastermind is deliberately different on five dimensions, and each difference is load-bearing for mastermind's primary user. **None of these differences are improvements or criticisms of engram** — engram is aimed at a different problem, which it solves well. Mastermind chooses otherwise because its problem is different.

| Dimension | engram | mastermind | Why the difference matters |
|---|---|---|---|
| **Storage format** | SQLite DB (`~/.engram/engram.db`) | Plain markdown files + YAML frontmatter | Longevity: if mastermind dies in 2034, the corpus is still `cat`able. SQLite is fine today, but plain markdown outlives every tool. |
| **Scope model** | Flat, single DB | Three scopes (user-personal, project-shared, project-personal) + archive tier | Career-long compounding requires separating "mine forever" from "this team, this repo" from "private scratch for now." One flat pool loses those distinctions. |
| **Capture** | Agent-initiated MCP writes | Automatic session-close extraction → mandatory pending/ review | ADHD constraint: capture cannot depend on remembering to trigger it. Review cannot be optional or the corpus rots. |
| **Continuity layer** | Not a concept — tools exist, agents call them when they remember | Automatic session-start injection (open-loops, lessons, pending count) | ADHD constraint: the tool must surface what's relevant *without being asked*. "Remember to query it" is not a viable UX. |
| **Target audience** | Any AI agent, general public, multi-UI (CLI+TUI+HTTP+MCP) | One user, one workflow, minimal surface | The differentiator is exactly that mastermind refuses to scale to a general audience. Refusing is the feature. |

**The takeaway for Phase 1 decisions**: when we're unsure whether to follow engram's pattern, the answer is "follow engram for layout, distribution, and Go idioms; diverge deliberately on storage, scope, capture, and continuity." Those four divergences are the entire reason mastermind exists as a separate tool.

If at any point during development we catch ourselves thinking "maybe we should just do it the engram way here," the right response is to re-read this section. Either we have a real reason to diverge (one of the five dimensions above) and the difference is load-bearing, or we don't and we're reinventing engram. Reinventing engram is a waste of our time and worse than just using engram. Diverging deliberately on the five dimensions is the whole point.
