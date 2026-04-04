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

## Phase 1 — the substrate (1-2 days)

The minimum viable mastermind. Everything else is built on top.

Starting point: the Go scaffold (`cmd/mastermind`, `internal/{format,store,search,mcp}`) is already in place and building. Phase 1 fills in the empty packages in the order that maximizes verifiability.

- [ ] `internal/format`: parse YAML frontmatter + markdown body, validate required fields (`date`, `project`, `topic`, `kind`), serialize back to markdown. No dependencies beyond stdlib + one YAML crate. Tested in isolation against fixture files.
- [ ] `internal/store`: locate store roots (`~/.mm/`, `<repo>/.mm/`, Claude auto-memory), glob markdown entries, read/write via `format`, enforce the pending/ invariant (all writes land in `pending/` first). Tested against a tmp dir.
- [ ] `internal/search`: fan-out query against context-mode's FTS5 using the source-label convention (`mm:user`, `mm:project-shared:<repo>`, etc.). Fallback grep path for environments without context-mode.
- [ ] `internal/mcp`: wire the MCP server over stdio and register `mm_search`, `mm_write`, `mm_promote`. This is the only package that imports the SDK.
- [ ] `~/.mm/` initialized as a real git repo with a remote. Seed with 3-5 hand-written entries.
- [ ] `/mm-search <query>` slash command (Claude Code wrapper) for manual testing.
- [ ] Dogfood for a day. Query the seed entries. Confirm retrieval works and feels right.

**Exit criteria**: you can search `~/.mm/` from Claude Code via `mm_search`, results are correct and source-tagged, and the format hasn't revealed any obvious flaws.

## Phase 2 — project-shared store (1 day)

- [ ] Convention for `<repo>/.mm/` directory layout.
- [ ] mastermind auto-detects `.mm/` in the current working directory and includes it as a scope.
- [ ] `/mm-init` slash command that explores the current repo with parallel subagents and seeds `<repo>/.mm/nodes/`. Reuse brv-init's approach, point at the new location.
- [ ] Test in one project (probably Rocket.Chat.Electron). Confirm team-shared entries are committed to git and survive a fresh clone.

**Exit criteria**: one real repo has a populated `.mm/nodes/` dir, entries are in git, `mm_search` returns hits from it.

## Phase 3 — capture (2 days)

This is the piece that makes or breaks the whole project. Spend the prompt-engineering time here.

- [ ] `/mm-curate <text>` — manual entry writer. Prompts for scope and kind. Writes to `pending/`.
- [ ] `/mm-extract` — session-end extraction.
  - Reads transcript.
  - Runs the extraction prompt (versioned at `prompts/extract.md`).
  - Writes candidates to appropriate `pending/` dirs with proposed scope/kind/confidence.
  - Prints summary with file paths.
- [ ] Dogfood for a week. Run `/mm-extract` at the end of every non-trivial session. Review candidates. Tune the prompt until the signal-to-noise ratio is high enough to trust.
- [ ] `mm_promote(pending_path, target_scope)` helper for the review step.

**Exit criteria**: a week of dogfooding produces entries you genuinely value and don't feel tempted to reject wholesale.

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
