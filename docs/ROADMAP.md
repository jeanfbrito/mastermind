# Roadmap

Goal: ship the smallest thing that works, in the right order so you can bail cheaply if a piece turns out wrong.

## Phase 0 — study the references before coding (hours, not days)

Language and fork-vs-rewrite are already decided (see `DECISIONS.md`): Go, rewrite from scratch, no fork. The Go scaffold is already in place and verified building (Go 1.26.1, `make build` produces a working binary). Phase 0 is now purely about reading the reference repos and picking the MCP SDK.

1. **Study rtk's shape (Rust, translated to Go).** `~/Github/rtk`. Goals:
   - Project layout: how is the binary organized? What lives where?
   - MCP wiring: how does rtk register and handle tools? The transport and lifecycle model translates directly to Go.
   - Release workflow: how does rtk produce binaries and publish them? We want the same pipeline in Go (goreleaser is the obvious candidate).
   - Dependency philosophy: heavy or minimal? mastermind targets minimal — stdlib first.
2. **Skim brv's source for behavioral reference.** `~/Github/byterover-cli`. Not for code reuse, but to confirm the node format details, the warmup exploration flow, and the MCP tool surface shape. Fifteen minutes, no more.
3. **Skim OpenViking's extraction path.** `~/Github/OpenViking`. Only the end-of-session extraction logic. Confirm it's a prompt + review loop we can reimplement in a day.
4. **Pick the Go MCP SDK.** Primary candidate: `github.com/modelcontextprotocol/go-sdk` (official). Decision criteria: recent commits, used by real tools, supports stdio transport, ergonomic tool registration. Commit the choice to `docs/REFERENCE-NOTES.md`.

**Exit criteria for Phase 0**: a short `docs/REFERENCE-NOTES.md` with:
- One section per repo summarizing what we're taking and what we're leaving behind.
- The Go MCP SDK choice, with a one-line rationale.
- A one-paragraph sketch of mastermind's top-level directory layout (already scaffolded; doc confirms the intent).

Then move to Phase 1.

## Phase 1 — the substrate (2-3 days)

The minimum viable mastermind. Everything else is built on top.

Starting point: the Go scaffold (`cmd/mastermind`, `internal/{format,store,search,mcp}`) is already in place and building. Phase 1 fills in the empty packages in the order that maximizes verifiability.

**Engram-first step**: before writing any Phase 1 code, spend ~30 minutes reading these files from `~/Github/engram` with context-mode searches, not raw Read:

- `cmd/engram/main.go` — bootstrap flow, flag parsing, server startup.
- `internal/mcp/mcp.go` — how tools are registered in a single file.
- `internal/store/` — module layout (NOT storage format — engram uses SQLite, mastermind uses markdown).
- `.goreleaser.yaml` — distribution pipeline.
- The `perf(mcp): defer 4 rare tools...` commit (2026-03-26) — tool-surface token budget trick.

Then translate patterns to Go with the **official** SDK (`modelcontextprotocol/go-sdk` v1.4.1), not mark3labs. Engram's registration calls translate mechanically.

### Phase 1 tasks

- [ ] Add `github.com/modelcontextprotocol/go-sdk` to `go.mod`, pinned to v1.4.1 or latest stable in the 1.x line. Verify `go mod tidy` and `make build` still work.
- [ ] `internal/format`: parse YAML frontmatter + markdown body, validate required fields (`date`, `project`, `topic`, `kind`), serialize back to markdown. Dependencies: `gopkg.in/yaml.v3` (or `github.com/adrg/frontmatter` — pick during implementation). Tested in isolation against fixture files in `internal/format/testdata/`.
- [ ] `internal/store`: locate store roots (`~/.mm/` via $HOME, `<repo>/.mm/` via walk-up from cwd, Claude auto-memory dir), glob markdown entries, read/write via `format`, enforce the pending/ invariant (all writes land in `pending/` first). Auto-expire pending entries older than 7 days at startup. Tested against a `t.TempDir()` in unit tests.
- [ ] `internal/search`: fan-out query against context-mode's FTS5 using the source-label convention (`mm:user`, `mm:user-archive`, `mm:project-shared:<repo>`, `mm:project-personal:<repo>`). Fallback grep path for environments without context-mode. Returns source-tagged ranked results.
- [ ] `internal/mcp`: wire the MCP server over stdio using the official SDK. Register `mm_search`, `mm_write`, `mm_promote`, `mm_close_loop`. This is the only package that imports the SDK. One file (`mcp.go`) for the tool registrations, like engram.
- [ ] `cmd/mastermind/main.go`: bootstrap — parse args, dispatch to MCP server mode (default) or to CLI subcommands (`session-start`, `session-close`, which arrive in Phase 3).
- [ ] `~/.mm/` initialized as a real git repo with a remote (personal private repo). Seed with 3-5 hand-written entries in the FORMAT.md schema.
- [ ] `/mm-search <query>` slash command (Claude Code wrapper) for manual testing.
- [ ] Dogfood for a day. Query the seed entries. Confirm retrieval works and feels right.

**Exit criteria**: `mm_search` returns correct, source-tagged results from a populated `~/.mm/` against real queries, `mm_write` + `mm_promote` correctly land entries in pending/ and move them to the live store, and the format has survived its first encounter with real entries without revealing major schema flaws.

**Exit criteria**: you can search `~/.mm/` from Claude Code via `mm_search`, results are correct and source-tagged, and the format hasn't revealed any obvious flaws.

## Phase 2 — project-shared store (1 day)

- [ ] Convention for `<repo>/.mm/` directory layout.
- [ ] mastermind auto-detects `.mm/` in the current working directory and includes it as a scope.
- [ ] `/mm-init` slash command that explores the current repo with parallel subagents and seeds `<repo>/.mm/nodes/`. Reuse brv-init's approach, point at the new location.
- [ ] Test in one project (probably Rocket.Chat.Electron). Confirm team-shared entries are committed to git and survive a fresh clone.

**Exit criteria**: one real repo has a populated `.mm/nodes/` dir, entries are in git, `mm_search` returns hits from it.

## Phase 3 — capture & continuity (3-4 days)

This is the piece that makes or breaks the whole project. It is also the piece most reshaped by the ADHD-constraint decision. The primary capture path is **not** a slash command the user remembers to run — it is an automatic hook that fires on every Claude Code session close, with candidates landing in a mandatory pending/ review queue. See EXTRACTION.md and CONTINUITY.md for the full specification.

### Phase 3a — the extraction pipeline (1-2 days)

- [ ] `prompts/extract.md` — the extraction prompt, versioned. Starts from the sketch in EXTRACTION.md. Includes open-loop detection, scope heuristics, six-kind taxonomy, JSON output schema.
- [ ] LLM client: thin wrapper around the Claude API (direct HTTP, no SDK needed for Phase 3 — we're outside the MCP session when extraction runs). Reads `ANTHROPIC_API_KEY` from env or `~/.mm/config.json`.
- [ ] Transcript loader: reads Claude Code transcript files from wherever they're stored, assembles them into a conversation object with timestamps (pattern from OpenViking).
- [ ] Language detection: auto-detect conversation language, fall back to config (pattern from OpenViking).
- [ ] Extraction runner: single-shot (not ReAct — see REFERENCE-NOTES.md for why), parses JSON response, validates against FORMAT.md schema, writes each valid candidate to `<scope>/pending/`. Logs to `~/.mm/logs/extraction.log`.

### Phase 3b — session-close automation (1 day)

- [ ] `mastermind session-close --transcript <path>` subcommand: Phase 1 (sync) archives transcript, forks detached Phase 2, returns in <100ms.
- [ ] Claude Code hook wiring instructions in README. One-time setup cost, runs forever after.
- [ ] Detachment correctness: the Phase 2 subprocess must survive the parent exiting, must not hold the terminal, must not block shell prompt return.
- [ ] Failure-mode handling: unreadable transcript, LLM API down, malformed response, format validation failures — all silent, all logged, none break the user's next session.

### Phase 3c — session-start continuity layer (1 day)

- [ ] `mastermind session-start --cwd <dir>` subcommand: walks up to find `.mm/`, queries all three scopes, assembles the continuity-injection block (open-loops, relevant lessons, pending count), writes to stdout for Claude Code to inject. <200ms target.
- [ ] Claude Code hook wiring for session-start.
- [ ] Silent-unless-needed discipline: empty sections are omitted entirely. If all three sections (open-loops, lessons, pending) are empty, output nothing.
- [ ] `mm_close_loop` MCP tool: agents call this when the user resolves an open-loop during a session. Moves the entry to `<scope>/resolved-loops/`.

### Phase 3d — review flow (0.5 days)

- [ ] `/mm-review` slash command (Claude Code wrapper calling a `mastermind review` CLI subcommand).
- [ ] One-at-a-time, keyboard-driven: `k` keep, `x` reject, `e` edit, `s` skip, `q` quit. Five seconds per entry.
- [ ] Default-accept for open-loops.
- [ ] 7-day auto-expire is implemented in `internal/store` during Phase 1 (startup pass), not here — but verify it works end-to-end during Phase 3d dogfooding.

### Phase 3e — dogfooding and prompt tuning (1-2 weeks of calendar time, ~0.5 days of active work)

- [ ] Use mastermind for real work for 1-2 weeks.
- [ ] Let session-close extraction run on every session, automatically.
- [ ] Review pending/ at the start of every session.
- [ ] Tune `prompts/extract.md` until signal-to-noise is high enough to trust. Commit prompt changes frequently so the tuning history is captured.
- [ ] Watch for: missed open-loops (false negatives), extracted noise (false positives), wrong scope assignment, stale lessons surfaced at session start, the tool being too loud or too quiet.

**Exit criteria**: after two weeks of automatic session-close extraction and daily session-start injection, the user trusts the system enough to leave it running without supervision, and the pending/ queue produces entries they accept rather than reject wholesale. If dogfooding reveals the continuity layer is annoying or wrong, fix that *before* moving to Phase 4.

## Phase 4 — archive tier (1 day)

- [ ] `~/.mm/archive/<year>/<project>/` directory structure.
- [ ] `mm_search` honors `include_archive=true` (default: false).
- [ ] `/mm-archive <project>` command:
  - Finds all `lessons/*.md` with matching `project` frontmatter.
  - Proposes cross-project promotion candidates.
  - Moves non-promoted entries to `archive/<year>/<project>/`.
  - Commits the move.

**Exit criteria**: you can archive a test project, search without the archive, then search with `include_archive=true` and see the archived entries.

## Phase 5 — sync (0.5 days)

- [ ] Document the sync story per store in `docs/SYNC.md`.
- [ ] Set up `~/.mm/` git remote on both your laptop and the Xeon machine. Verify two-way sync.
- [ ] Optional: pre-session hook that runs `git pull` in `~/.mm/` before a session starts. Only add if you actually forget to pull manually.

**Exit criteria**: an entry captured on the Xeon machine is searchable on the laptop after one `git pull`.

## Phase 6 — polish (ongoing, never done)

- Extraction prompt tuning as you discover failure modes.
- Cross-session extraction (`/mm-extract --since 7d`).
- Better source-tag rendering in search results.
- Whatever friction you hit during real use.

**No feature goes into Phase 6 until Phases 1-5 are stable and have been dogfooded for at least two weeks.**

## What's explicitly NOT on the roadmap

- Web UI.
- Multi-user support.
- Embeddings / vector search / semantic dedupe.
- Plugin system.
- Publishing mastermind as a product.
- Importing from other tools (Notion, Obsidian, Roam).
- Export to other formats.
- Mobile app / sync client.
- Encryption at rest (git repo on your machine with a private remote is enough).
- Any form of AI training on the corpus.

If any of these feel tempting mid-build: stop and re-read `docs/NON-GOALS.md`. Scope discipline is the whole reason this project is actually finishable.

## Rough total estimate

~1-2 weeks of real work. Spread over whatever calendar time feels right. The corpus itself grows from day one and keeps growing for decades — the tooling grows around it.

## The single rule for building

**Build the format first, the storage second, the extraction third, the retrieval last. Dogfood each phase before starting the next.**

If Phase 1 reveals the format is wrong, fix it in Phase 1 — don't carry a wrong format into Phase 2. If Phase 3 reveals extraction produces junk, fix the prompt before building the archive tier on top of junk entries. Each phase exits only when you'd trust it unsupervised for a week.
