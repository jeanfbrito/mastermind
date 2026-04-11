# Decisions log

Append-only log of architectural decisions. Each entry: date, decision, rationale, alternatives considered. When a decision is reversed, write a new entry reversing it — never rewrite history.

---

## 2026-04-04 — Project kickoff

**Decision**: Build mastermind as a personal engineering second brain with three scopes (user-personal, project-shared, project-personal), a two-tier user-personal store (working set + archive), and explicit reviewable extraction.

**Rationale**: Derived from a long conversation comparing brv, OpenViking, and context-mode. See `README.md` for the summary. Short version: brv is 60% right but has a hub we don't want and is machine-scoped instead of repo-scoped; OpenViking has two good ideas (end-of-session extraction, user-wide memory) wrapped in a Rust server we don't want; context-mode is perfect as-is and will be a dependency, not a competitor.

**Alternatives considered**:
- *Use brv as-is*: rejected — hub, machine-level config, no user-scope, no archive tier.
- *Fork brv*: still open, pending Phase 0 source review.
- *Use OpenViking*: rejected — server dependency, skills overlap with Claude Code, over-engineered for the corpus size.
- *Run all three together*: rejected — redundancy, no semantic dedupe possible, three places to query for the same facts.

---

## 2026-04-04 — Storage format: plain markdown + frontmatter

**Decision**: Every entry is a `.md` file with YAML frontmatter. See `FORMAT.md` for the schema.

**Rationale**: The corpus must outlive any tool. Plain markdown is readable with `cat`, searchable with `grep`, diffable with git, editable with any tool ever made. No database migration in 2040 when SQLite changes its format. Frontmatter gives structured fields for scoping and retrieval without sacrificing human readability.

**Alternatives considered**:
- *SQLite as primary store*: rejected — opaque, tool-dependent, harder to back up, scary on long timelines.
- *JSON files*: rejected — worse for humans to read and edit, no heading structure.
- *Plain text without frontmatter*: rejected — loses scope/kind/project metadata that we need for archiving and fan-out.

---

## 2026-04-04 — Retrieval via context-mode FTS5, not a custom index

**Decision**: mastermind indexes its markdown files into context-mode's FTS5 at startup and queries via `ctx_search`. No separate database.

**Rationale**: context-mode already ships FTS5, already has a query tool, already handles the index lifecycle. Reimplementing this would be pure duplication. The dependency is deliberate and asymmetric — mastermind owns knowledge, context-mode owns search.

**Alternatives considered**:
- *Custom SQLite FTS5 index*: rejected — duplicates context-mode, adds maintenance burden, no benefit.
- *grep-only*: rejected — works as a fallback, but FTS5 ranking is meaningfully better for keyword queries.
- *Tantivy / Meilisearch / Elasticsearch*: rejected — dramatic overkill for thousands of markdown entries.

---

## 2026-04-04 — Writes always go through pending/

**Decision**: `mm_write` and all extraction paths write to `<scope>/pending/`. The only way an entry reaches the live store is via explicit promotion (`mm_promote` or manual `mv`).

**Rationale**: Auto-writes kill curated corpora. OpenViking's auto-extraction is valuable conceptually but dangerous operationally — without review, junk accumulates and trust erodes. A `pending/` staging area with a mandatory review step captures the value of automation (draft creation) without the cost (unreviewed writes). Review is also the consolidation — the cognitive step that makes the lesson stick.

**Alternatives considered**:
- *Direct writes with a confidence threshold*: rejected — any automatic writes, even high-confidence, compound into drift over years.
- *Write directly, revert later if wrong*: rejected — revert requires noticing, and you won't.
- *No extraction at all, manual only*: rejected — capture is the hard problem and extraction solves it at the moment the memory is freshest.

---

## 2026-04-04 — Archive is project-triggered and manual

**Decision**: Archiving happens only when the user runs `/mm-archive <project>`. No automatic archive by age or access time.

**Rationale**: A lesson from 2021 can still be load-bearing in 2026. Time-based archiving would drop active knowledge out of the working set. Project transitions (leaving a job, shipping a final release) are the real signal that a body of knowledge should retire — and those transitions are events the user knows about. Manual triggering with project scope matches the actual lifecycle.

**Alternatives considered**:
- *Auto-archive by age*: rejected — drops active knowledge, surprises the user.
- *Auto-archive by LRU*: rejected — requires tracking access, adds state, punishes lessons that are important but rarely needed.
- *No archive at all, flat store forever*: rejected — the working set bloats over decades and everyday queries get noisy.

---

## 2026-04-04 — Language: Go

**Decision**: Mastermind is written in Go. Single static binary, `go.sum` committed, distributed as a GitHub release artifact (optionally a Homebrew tap). No language runtime required on target machines.

**Rationale**: The load-bearing argument is **longevity, not speed**. mastermind is designed to hold insights for decades, and the tool must still run when you go back to query them years from now. Node and Python projects rot aggressively: `node_modules` from two years ago often fails to install, yanked wheels break `pip install`, native extensions stop building against current headers, and interpreter versions move faster than old code can track. This is a failure mode directly experienced by the user and is incompatible with a tool whose whole value proposition is "still useful in 10 years."

Go addresses this directly:
- Static binaries ship without any runtime installation.
- `go.sum` is reproducible; the Go module proxy provides strong immutability guarantees.
- The **Go 1 compatibility promise** has held since 2012: code written for Go 1.0 still compiles on Go 1.22 with near-zero changes. Thirteen years of stability, enforced as a project-level commitment.
- Cross-compilation is trivial: `GOOS=linux GOARCH=amd64 go build` works from any machine to any target.
- Standard library covers most of what mastermind needs (filesystem, glob, JSON, HTTP, goroutines, `os/exec`) — few external deps, smaller supply chain.
- Fast compilation (seconds, not minutes) keeps iteration snappy while tuning extraction prompts and slash-command flows.
- Smaller code surface than Rust for glue-heavy work — typically 30-40% less code for equivalent logic, which matters when you come back cold in a year.

**Why not Rust** (the close runner-up):
- Rust was considered seriously and initially chosen because `~/Github/rtk` is the reference implementation and matching its language would allow direct pattern copying.
- Rust is genuinely excellent and for many projects would be the right call.
- It was rejected here because mastermind is a glue-heavy, small-surface, long-lived CLI — exactly the shape Go was designed for. The Rust advantages (stronger compile-time guarantees, crates.io's versioning culture) don't outweigh Go's advantages for this specific profile: faster iteration, smaller codebase, broader stdlib coverage, and (arguably) an even stronger backward-compatibility track record.
- rtk's role shifts from "implementation blueprint" to "conceptual blueprint." Patterns get translated, not copied. This is a real cost but a bounded one — rtk's shape is simple enough to re-express in Go.

**Alternatives considered and rejected**:
- *TypeScript/Node*: rejected — directly incompatible with the longevity requirement due to environment rot.
- *Python*: rejected — same reason, worse packaging story.
- *Rust*: see above. Excellent second choice; specific factors tilted Go for this project.

**Reference implementation for the shape** (despite the language mismatch): `~/Github/rtk` — proves that a single-binary, MCP-adjacent, long-lived CLI tool is a viable form, and its architecture translates cleanly to Go.

---

## 2026-04-04 — Fork brv vs rewrite substrate: rewrite

**Decision**: Rewrite the substrate in Go. Do not fork byterover-cli.

**Rationale**: The language decision (Go, for longevity) settles this. byterover-cli is not Go, so "forking" would mean either (a) rewriting it in Go anyway, at which point it's not a fork, or (b) keeping it in its original language, at which point we inherit exactly the environment-rot problem the Go decision was meant to solve. Neither option preserves the value of forking.

brv's *ideas* (node format, per-project curation, warmup exploration, MCP query interface) are still the substrate model. Those get re-implemented in Rust from scratch, with the node format extended to mastermind's FORMAT.md schema. The brv source remains a reference for behavior, not a code ancestor.

**What this means concretely**:
- Read brv's source for design understanding, not code reuse.
- Diverge from brv's format where mastermind's FORMAT.md says so (scope field, kind enum, "when this matters again" section).
- No upstream merges to track.
- No hub code to rip out — it never enters the codebase in the first place.

---

## 2026-04-04 — Go MCP SDK: official modelcontextprotocol/go-sdk v1.4.1

**Decision**: Depend on `github.com/modelcontextprotocol/go-sdk` at v1.4.1 (stable) or later within the 1.x line. Pin to the latest stable in `go.sum`; upgrade deliberately when new stable tags land.

**Rationale**: Official, Anthropic + Google co-maintained, past v1.0 with stable semver, 88 commits since 2026-01-01, tagged stable v1.4.1 released 2026-03-13. Minimal dependency footprint (`google/jsonschema-go`, `golang-jwt/jwt/v5`, `oauth2`, `segmentio/encoding`, `uritemplate`) — no web framework. Full transport surface including `StdioTransport` (what Claude Code uses). Generic type-safe API (`mcp.AddTool[Input, Output]`) with struct-tag JSON Schema — compile-time guarantees, no `map[string]any` fishing. Known shipping users: `github/github-mcp-server` (28.5k stars, pins the SDK), `containers/kubernetes-mcp-server` (1.4k stars, pinned at stable v1.4.1). For a long-lived personal tool, API stability is worth more than community size.

**Runner-up considered**: `github.com/mark3labs/mcp-go`. Has more stars (8.5k) and more shipping users (grafana-mcp, terraform-mcp, engram, mcp-language-server). Loses because it's pre-1.0 with 47 minor releases in 18 months — the breaking-change tax on a tool we expect to maintain for years is higher than the community-size upside.

**Rejected**: `github.com/metoro-io/mcp-golang`. Unmaintained since September 2025, zero 2026 commits, drags gin + ~30 indirect deps. Disqualifying for a tool targeting minimal dependencies and long life.

**Consequence**: the canonical tool registration pattern mastermind uses is:

```go
type Input struct { ... }
type Output struct { ... }

func handler(ctx context.Context, req *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) { ... }

server := mcp.NewServer(&mcp.Implementation{Name: "mastermind", Version: version}, nil)
mcp.AddTool(server, &mcp.Tool{Name: "mm_search", Description: "..."}, handler)
server.Run(ctx, &mcp.StdioTransport{})
```

The Go version minimum (`go 1.25`) was a concern briefly but we're already on 1.26.1 so it's moot.

---

## 2026-04-04 — Implementation reference: Gentleman-Programming/engram

**Decision**: `Gentleman-Programming/engram` (https://github.com/Gentleman-Programming/engram) is the primary implementation reference for mastermind's project layout, storage module structure, distribution pipeline, and CLI shape. Cloned locally at `~/Github/engram`. rtk is demoted to secondary reference (style and release workflow only).

**Rationale**: engram is mastermind's structural twin — Go + MCP stdio + single-binary CLI + goreleaser + Homebrew tap + `cmd/<name>` + `internal/mcp/`, `internal/store/`, `internal/server/` layout + pure-Go SQLite. Same domain (memory for AI agents), same transport (MCP stdio), same distribution story (cross-platform binaries via goreleaser + Homebrew). Last commit 2026-03-30; actively maintained. Its `internal/mcp/mcp.go` is a one-file tool-registration module matching mastermind's Phase 1 scope exactly.

**Explicit divergences from engram** (see REFERENCE-NOTES.md appendix for the full matrix):

1. **Storage format**: engram uses SQLite with FTS5 inside the DB. Mastermind uses plain markdown files on disk with FTS5 via context-mode. The longevity argument (the corpus must be `cat`able in 2034 without the tool) is load-bearing and non-negotiable.
2. **Scope model**: engram is flat. Mastermind has three scopes (user-personal / project-shared / project-personal) plus an archive tier. Career-long compounding requires those distinctions.
3. **Capture path**: engram writes happen via agent-initiated MCP tool calls. Mastermind's primary capture path is automatic session-close extraction into a mandatory pending/ review queue. This is the ADHD-constraint answer; see CONTINUITY.md.
4. **Continuity layer**: engram doesn't have one explicitly. Mastermind's session-start injection (open-loops, lessons, pending count surfaced without being asked) is load-bearing for its primary user.
5. **Target audience**: engram is a general-purpose product shipping to anyone with an agent. Mastermind is built for one user; public release is a bonus if it happens.

**Caveat on MCP wiring**: engram uses `mark3labs/mcp-go`, not the official SDK mastermind adopted. Treat engram as the reference for **project layout, storage module structure, distribution pipeline, and CLI shape** — not for the literal MCP tool-registration call sites. For the registration API use the official SDK's README example and `containers/kubernetes-mcp-server` (which pins the stable v1.4.1). The translation between the two SDKs is mechanical.

**Phase 1 concrete consequence**: read engram's `cmd/engram/main.go`, `internal/mcp/mcp.go`, `internal/store/` (for module structure, not storage format), and `.goreleaser.yaml` before writing any Phase 1 code.

---

## 2026-04-04 — ADHD as the load-bearing design constraint

**Decision**: Mastermind's design is optimized for a user with ADHD. This is not an accessibility consideration or a nice-to-have; it is the primary constraint that shapes every feature decision. Any feature that requires neurotypical working memory to use is, for this user, a broken feature. Any default that assumes "you'll remember to check it" or "you'll trigger it at the right moment" is, for this user, a silent failure.

**Rationale**: The user's own words in the conversation that led to this project: *"just help me with memory stuff and dont needing to tell again and again to the agents and even remember things, my adhd dont helps a lot, so mastermind taking my personal world is all I need."* That sentence is the spec. Every design choice downstream either honors it or betrays it.

**Consequences baked into the design** (see CONTINUITY.md for the full specification):

1. **Capture is automatic, triggered by session-close hooks.** Never willpower-based. The extraction pipeline runs detached on every Claude Code session close, no user action required.
2. **Retrieval is automatic, triggered by session-start hooks.** Context is injected into every new session before the user types a character — open-loops, relevant lessons, pending count. The tool surfaces what's needed without being asked.
3. **Open-loops are a first-class entry kind.** The "I was about to do X but got pulled away" pattern is the most ADHD-specific failure mode, and it's the one mastermind must handle automatically and explicitly.
4. **Pending entries auto-expire after 7 days.** A review queue that accumulates guilt is worse than no tool. Silent deletion, no nag.
5. **Review is one-at-a-time, keyboard-driven, five seconds per entry.** Lists cause decision paralysis; single items don't.
6. **No notifications, no reminders, no badges, no streaks, no dashboards.** The default state of the tool is invisible. Presence is not a UX.
7. **Silent unless needed.** Session-start injection shows only sections with content. Zero pending? Don't mention pending. This is a rule, not a polish item.

**The test for any future feature**: does this work on a day when the user's working memory is at its worst, or does it require a good day to use? If it requires a good day, it's the wrong design. Full stop.

**What this decision explicitly rejects**:

- Any "gentle reminder" UX (reminders compound into guilt).
- Any progress gamification (streaks punish ADHD cognition).
- Any dashboard the user has to remember to check.
- Any capture path that starts with "run this command at session end."
- Any review flow that shows N entries at once.
- Any feature whose value depends on user consistency over long timelines.

**Acknowledged trade-off**: these design choices make mastermind less attractive to general users who might prefer a dashboard, notifications, or manual control. That's fine — mastermind is not built for general users. If someone else benefits from the tool as a side effect, great. If nobody else ever uses it, it has still succeeded if it does what it's built to do for its one user.

---

## 2026-04-04 — Go minimum bumped 1.22 → 1.25 (forced by MCP SDK)

**Decision**: `go.mod` declares `go 1.25`. The earlier plan targeted Go 1.22 to stay close to the oldest supported toolchain, but `github.com/modelcontextprotocol/go-sdk` v1.4.1 requires Go 1.25 and the dependency is non-negotiable.

**Rationale**: The SDK uses language features (generics ergonomics, `slices`/`maps` stdlib additions, `for range int`) introduced between 1.22 and 1.23-1.25. Pinning below the SDK's floor is not an option. Local toolchain is already Go 1.26.1, so this bump has zero practical cost; it only affects users building from source on an old Go, who can `go install` a newer toolchain in under a minute.

**Consequence**: the Go 1 compatibility promise still applies — code written against 1.25 will compile on every future 1.x. The longevity argument from the language decision is unchanged. Upgrading the SDK floor in the future will likely drag the Go minimum with it; that is acceptable and expected.

**Alternatives considered**:
- *Vendor the SDK and backport*: rejected — maintenance burden, defeats the point of using an upstream SDK.
- *Switch to mark3labs/mcp-go (lower Go floor)*: rejected — already rejected in the SDK decision on stability grounds; the Go floor is not a good enough reason to reopen that.

---

## 2026-04-04 — Phase 1 test baseline: 91 tests across 6 packages

**Decision**: The Phase 1 completion baseline is **91 tests passing across 6 Go packages** (`internal/format`, `internal/store`, `internal/project`, `internal/search`, `internal/mcp`, and the `cmd/mastermind` package which currently contributes 0). This is recorded here so any future commit that lands with fewer tests is a red flag unless the drop is explicit and justified.

**Rationale**: Phase 1 was the locked-format moment — frontmatter parser, three-scope store, topic-dominant search ranking, and MCP tool wiring. Those behaviors are load-bearing and the tests exist specifically to catch silent drift (ranking invariants, pending-only write enforcement, serverInstructions content). Losing tests without an explanation means something got weakened without anyone noticing.

**How to verify**: `make test` — the summary line should show all packages green and total count ≥ 91. `go test ./... -count=1 -v | grep -c '^=== RUN'` gives the exact current count.

**Consequence**: Phase 2 and beyond MUST only grow this number. If a refactor genuinely deletes a test (because the thing being tested stopped existing), note it in the commit message and update this entry.

---

## 2026-04-10 — Test baseline update: 181 tests across 8 packages

**Decision**: The current test baseline is **181 tests passing across 8 Go packages** (`internal/format`, `internal/store`, `internal/project`, `internal/search`, `internal/mcp`, `internal/extract`, `internal/discover`, and `cmd/mastermind`). This supersedes the Phase 1 baseline of 91 tests across 6 packages.

**What grew**:
- `internal/extract`: 28 tests (keyword extractor patterns, deriveTopic, isDuplicate, parseExtractionResponse, NewExtractor factory)
- `cmd/mastermind`: 16 tests for the suggest path (extractPathKeywords, countEntriesInDir, bestEntryTopic)
- `internal/discover`: 17 tests (parseResponse, collectKnownHashes, isDuplicate, findPackages)
- Other packages grew incrementally through Phases 2-3

**How to verify**: `make test` — all packages green and total count ≥ 181. `go test ./... -count=1 -v | grep -c '^=== RUN'` gives the exact count.

**Consequence**: Same rule as Phase 1 — this number MUST only grow. If a commit lands with fewer, something got silently broken.

---

## 2026-04-10 — Autonomous discovery pipeline with model-verified review

**Decision**: Add a two-stage autonomous knowledge discovery pipeline on top of the existing capture paths. Stage 1 is a cheap, broad scan using Haiku (or any OpenAI-compatible endpoint). Stage 2 is a precise, model-verified review that checks candidates against their source material.

**The pipeline**:
```
Haiku scan (cheap, broad)       → pending/ (50 candidates)
  ↓
Session model verify (free)     → verified / rejected / ambiguous
  ↓
Human (only ambiguous)          → final decisions
  ↓
live/
```

**What's new**:
1. **`/mm-discover` skill** + **`mastermind discover` CLI** — both invoke Haiku (Anthropic) or any OpenAI-compatible endpoint (Ollama, LM Studio, vLLM, Together.ai, Groq) to mine git history and/or source packages. Writes candidates to `pending/` with mandatory `## Source` sections (commit hashes for git, file paths for codebase).
2. **`/mm-review` redesigned as verify-first** — the current session model (Opus/Sonnet) reads each candidate's `## Source`, fetches the actual commit or file, verifies the claim matches the source, and triages into auto-promote / auto-reject / escalate-to-human.
3. **Entries as cursor, no state file** — `## Source` hashes in existing entries are the incremental cursor. Running discover again only processes new commits. Self-correcting: rejecting an entry in `/mm-review` makes its source commits eligible for re-analysis.

**Rationale**: Knowledge should grow even when the user isn't actively working. A small cheap model (Haiku) can scan broadly without burning the budget. A bigger model (the session's Opus) can verify the curated output precisely — catching hallucinations that a human skimming titles would miss. The human only sees edge cases. This respects both the cost constraint and the ADHD design constraint (no burden when working memory is bad).

**Why Haiku and not Opus for discovery**: cost at scale. Scanning 100 commits on Opus would be ~50x more expensive than on Haiku, and Haiku is perfectly capable of extracting straightforward lessons from diffs. The precision gap is closed by the verify step, which uses the session's existing Opus context (effectively free).

**Why the session model verifies and not Haiku**: Haiku hallucinates. A second pass by a more capable model against the actual source material catches those hallucinations. Critically, the verifier only reads the 50 curated candidates plus their specific sources — not the full codebase — so its context stays small even on a 5000-commit repo.

**What this reinforces**: the hard rule "auto-extracted entries go through pending/" still holds. `/mm-discover` writes to pending/, not live, because the LLM (not the user) authored the content. The verify step is the quality gate that makes auto-writes safe — hallucinations never reach live.

**Files**:
- `internal/discover/` — new Go package (llmclient.go, prompts.go, parse.go, discover.go, plus tests)
- `cmd/mastermind/main.go` — new `discover` subcommand
- `skills/mm-discover/SKILL.md` — Claude Code skill (Haiku subagents for orchestration)
- `skills/mm-review/SKILL.md` — rewritten as verify-first triage
- Env vars: `MASTERMIND_LLM_PROVIDER` (anthropic|openai), `MASTERMIND_LLM_BASE_URL`, `MASTERMIND_LLM_API_KEY`

**Alternatives considered**:
- *Write discover output directly to live*: rejected — auto-writes kill curated corpora (DECISIONS.md 2026-04-04). Haiku hallucinates; direct writes would pollute the store.
- *Use Ollama-only API format*: rejected — OpenAI-compatible `/v1/chat/completions` works with everything (Ollama, LM Studio, vLLM, hosted providers), no vendor lock-in.
- *Ask the human to review each candidate without verification*: rejected — human can't open every commit diff to verify Haiku's claims. The verification step is precisely the value a model adds over a human here.
- *Single-stage Opus scan*: rejected — 50x more expensive, no benefit from the two-tier funnel.

---

## 2026-04-06 — Reverse auto-expire: pending entries never lose knowledge

**Decision**: Pending entries are kept indefinitely by default. The original hard rule #7 ("auto-expire after 7 days, silently") is reversed. An optional `PendingAutoPromote` policy moves old candidates to the live store after a configurable duration (default 7 days) — but the default behavior (`PendingKeepForever`) never touches pending entries. Knowledge is never silently deleted.

**Rationale**: Auto-delete is the only irreversible failure mode in the system. It punishes exactly the ADHD pattern mastermind is designed for: a user who doesn't review for 10 days loses knowledge silently, and never even knows what was lost. The original rationale ("a guilt queue is worse than no tool") correctly identified the problem but chose the wrong fix. Deleting knowledge to prevent guilt is like burning a notebook to prevent clutter. The right fix is to make review trivially easy — the agent can assist with review and promotion, so the cost of reviewing is near-zero — and to accept that an old queue is not shameful, it's patient.

**What changed in code**:
- `store.PendingTTL` constant removed, replaced by `PendingPolicy` type (`PendingKeepForever` | `PendingAutoPromote`) and `Config.PendingBehavior` / `Config.AutoPromoteAfter` fields.
- `store.PruneStale()` removed, replaced by `store.AutoPromoteStale()` which promotes (not deletes) old entries when the auto-promote policy is active, and is a no-op under the default keep-forever policy.
- Entries that would collide with an existing live entry on auto-promote are silently skipped (stay in pending for manual review), never lost.

**Alternatives considered**:
- *Auto-delete with longer TTL (30 days)*: rejected — still irreversible, still silent, still punishes the wrong pattern. Duration is not the variable.
- *Auto-accept as the default*: rejected for now — keep-forever is safer as default. Auto-accept is available as an opt-in policy for users who want zero-maintenance.
- *Nag the user to review*: rejected — nagging is a guilt machine. ADHD constraint #6.

---

## 2026-04-06 — User-initiated writes bypass pending: the user IS the review

**Decision**: `mm_write` (the MCP tool called by the agent during a session) writes directly to the live store via `store.WriteLive()`, bypassing `pending/` entirely. The pending queue is reserved for auto-captured knowledge (session-close extraction, Phase 3) where the user wasn't consciously involved.

**Rationale**: When the user tells the agent to capture something, three things are already true: (1) the user is present, (2) the user can see what the agent is writing, (3) the user chose to create it. Forcing a second approve step via `mm_promote` is pointless ceremony — the user already reviewed the entry by being in the conversation that produced it. The pending gate exists to protect against unreviewed auto-writes (session-close extraction), not against writes the user explicitly requested.

**What changed in code**:
- `internal/mcp/tools.go`: `handleWrite` calls `store.WriteLive()` instead of `store.Write()`.
- `store.WriteLive()`: new method that writes directly to the live directory (same atomicity guarantees as `Write`). Returns `ErrEntryExists` on slug collision.
- `store.Write()` still exists and still writes to `pending/`. It will be used by the Phase 3 session-close extraction pipeline.
- `serverInstructions` updated: agents are told `mm_write` goes to live, and `mm_promote` is for reviewing auto-extracted pending candidates.

**What this reverses**: Hard rule #1, which previously said "No auto-writes to the live store. Every entry passes through pending/ and a human review step." The new rule: user-initiated writes go directly to live; only automatic extraction passes through pending.

**Alternatives considered**:
- *Keep mm_write → pending, but auto-promote immediately*: rejected — a Write+Promote round-trip that always succeeds is indistinguishable from a direct write, but with more moving parts. Just write to live.
- *Add a `direct: bool` parameter to mm_write*: rejected — the agent can't reliably distinguish "user asked me to save this" from "I'm saving this proactively." Since mm_write is always called in a session where the user is present, the simpler rule is: mm_write always goes to live.

---

## 2026-04-09 — .knowledge/ git strategy: commit live, ignore pending

**Decision**: Project `.knowledge/` directories are committed to git. Auto-init creates a `.knowledge/.gitignore` that excludes `pending/` (auto-extracted, pre-review entries). Everything else — topic directories with live entries, `resolved-loops/` — is committed.

**Rationale**: The `.knowledge/` directory in a project repo IS the project-shared scope. "Shared" means committed. The whole value proposition is that project lessons survive across clones, machines, and teammates. Gitignoring the entire directory would make "project-shared" a misnomer.

`pending/` is excluded because auto-extracted entries are personal workflow artifacts — they haven't been reviewed, may contain noise, and pollute git history with churn from every session-close extraction. Once promoted to live (via `mm_promote`), they move into topic directories and get committed naturally.

`resolved-loops/` is committed because resolved loops have historical value: they document what was investigated and concluded, which prevents re-opening the same question in six months.

**What auto-init creates**:
```
.knowledge/
.knowledge/.gitignore   # contains: pending/
```

**Consequence for users**: `git add .knowledge/` after promoting entries or closing loops. This is a conscious act, not an automatic one — consistent with git's "explicit staging" model. A future `/mm-review` skill could remind the user to commit after promotion.

**Alternatives considered**:
- *Gitignore everything*: rejected — defeats project-shared scope entirely. Entries only live on one machine.
- *Commit everything including pending/*: rejected — auto-extraction creates churn, pre-review entries may be low quality, and two developers' pending queues would conflict.
- *Partial ignore with .personal/*: considered but unnecessary — project-personal scope already lives in `~/.claude/projects/`, not in `.knowledge/`. There's no `.personal/` subdirectory to worry about.

---

## 2026-04-10 — Tiered `mm_search` fallback chain (seven classes)

**Decision**: `mm_search` now sorts results by a primary `tierClass` enum (0–6), with score only used as a within-class tiebreaker. Classes:

| Class | Name | Match criterion |
|---|---|---|
| 0 | `classExactTopic` | full query phrase appears in topic (multi-word queries only) |
| 1 | `classExactTag` | full query phrase appears in a tag |
| 2 | `classExactBody` | full query phrase appears in body |
| 3 | `classTopicTokens` | all query tokens found in topic |
| 4 | `classMetaTokens` | all query tokens found across topic + tags |
| 5 | `classKeyword` | tokens matched in body (default keyword pipeline) |
| 6 | `classFuzzy` | sahilm/fuzzy gap-match against topic + tags |

Sort order: `class ASC → score DESC → date DESC → path ASC`. A class-0 hit strictly dominates any class-6 hit regardless of access frequency, recency, or score magnitude. Within a class, ACT-R fast-mode access boosting (`ln(accessed+1) * 0.2`, additive, capped at +0.5) breaks ties toward frequently-useful entries.

**Short-circuit**: pass 2 (body loading) is skipped entirely when pass 1 (metadata-only) yields top-K results (K = min(Limit, 3)) all in class ≤ 4 AND at least one of them has `access_count ≥ 3`. The access gate is the load-bearing second condition — it prevents structural matches from short-circuiting before the entry has proven useful.

**Implementation**: `internal/search/search.go`. Added:
- `tierClass` enum and unexported `class` field on `Result` (never crosses the MCP boundary).
- `sortResultsByTier` + `shouldShortCircuit` helpers.
- `fuzzy.Find` from `github.com/sahilm/fuzzy` (third direct dep, zero-dep, ~300 LOC).
- `accessBoost` switched from linear (`accessed * 0.05`, cap 0.5) to log-shaped ACT-R fast mode (`ln(accessed+1) * 0.2`, cap 0.5). One access now produces a meaningful boost (0.139 vs. 0.05); saturation still occurs well below a topic hit.

**New direct dep**: `github.com/sahilm/fuzzy v0.1.1`. Selected for Sublime-style gap matching over pure Levenshtein — gap matching handles "I half-remember the topic" better than typo-only matching, which is the bad-working-memory-day failure mode mastermind is designed for. Zero-dep, MIT, small enough to keep alive for the 2034 bug.

**Length guard**: the class-6 fuzzy pass skips entirely when the normalized query is fewer than 4 characters. Borrowed from engram's `internal/project/similar.go` length-scaled Levenshtein — short queries produce too much noise in fuzzy matching. For mastermind's corpus, 4 is the threshold where fuzzy hits become useful rather than distracting.

**Rationale — why tiered, why now**:

The previous single-pass scoring was correct but fragile. Additive boosts (2.0 topic, 0.7 tag, 0.3 body, +0.5 access cap) worked because the constants happened to enforce the "topic dominates" invariant, but tuning any one value risked inverting the ordering. A class-first sort is lock-in-by-construction: no combination of scores or boosts can make a class-6 fuzzy hit beat a class-0 exact-topic hit.

Three reference repos were mined before the design was finalized (see reference-notes/):

1. **engram** (`internal/store/store.go:1504-1512`) — the `Rank = -1000` sentinel pattern. When engram detects a topic_key match it assigns a synthetic rank that pins the result to the top. Translated directly: mastermind uses class enums instead of sentinel values, but the principle is identical — "this match is a different class, not just a higher score."
2. **shiba-memory** (`schema/001_init.sql`, `007_actr_proper.sql`) — ACT-R fast-mode `1 + ln(access_count+1) * 0.1` capped at +30%. The formula shape was borrowed; the constants were adapted (0.2 and 0.5 cap) to preserve mastermind's pre-existing 0.5 access-boost budget. Also borrowed: the "earned confidence" gate from `003_instincts_tracking_gateway.sql:35,50-52` (`access_count >= 3`), which is the second condition on the pass-2 short-circuit.
3. **mempalace** (`searcher.py:34-50`) — validated mastermind's existing metadata-pre-filter pattern (filter before body I/O). No new code borrowed, but the convergent design is worth noting: two independent projects arrived at the same insight.

**Explicit non-borrow from mempalace**: the L0–L3 memory stack is a *loading convention*, not a retrieval cascade. The tiered fallback chain is retrieval-time scoring. These are orthogonal axes and must not be conflated — the L0–L3 model (see MEMORY-STACK.md) describes what's held in context at different phases of the conversation; the tier classes describe how a single `mm_search` call ranks its output.

**What was rejected**:

- **Body fuzzy matching**: running sahilm against entry bodies would explode false positives. The bad-day failure mode is misremembering a topic, not misremembering body prose. Body stays on stdlib keyword.
- **Multiplicative access boost**: shiba-memory uses `score *= (1 + ln(access+1)*0.1)`. Considered, but mastermind's additive model is more transparent when debugging why an entry ranked where it did. The invariant is easier to test additively.
- **Hand-rolled Levenshtein**: `agnivade/levenshtein` is ~100 LOC and zero-dep. But pure edit distance doesn't capture "gap match" — sahilm/fuzzy's Sublime-style matching is a strict superset of useful behavior for ~300 LOC.
- **Project-boost as ranking multiplier** (shiba-memory's 1.3× / 0.8× for same/cross-project): bigger change, requires converting `Query.Project` from a hard filter to a soft ranking signal. Deferred to an open-loop for Phase 5+.
- **Proper-mode ACT-R with timestamp array**: canonical `B_i = ln(Σ age_j^(-0.5))` formula would give mastermind recency-aware scoring, but requires storing a timestamp array per entry (frontmatter schema extension). Deferred to an open-loop for Phase 4+; will only be implemented if dogfooding reveals that count-only boost promotes stale-but-frequently-accessed entries.

**Consequence for FORMAT.md**: None. The tier-class work is entirely internal to `internal/search/` — no frontmatter changes, no schema extensions, no new MCP tool fields. `Result.class` is unexported so it never crosses the MCP boundary.

**Test coverage**: 45 tests in `internal/search/` (was 17 pre-tiered). New tests lock in:
- `TestAccessBoost` — ACT-R fast-mode formula, monotonic, saturating, topic-dominance preserved.
- `TestKeywordSearcherExactPhraseTiers` — classes 0/1/2 assignment.
- `TestKeywordSearcherClassDominatesAccessBoost` — 500-access entry still loses to class-0 hit.
- `TestKeywordSearcherKeywordClassSplit` — class 3/4/5 differentiation.
- `TestKeywordSearcherShortCircuitFires` / `ShortCircuitNeedsEarnedAccess` — both halves of the short-circuit gate.
- `TestKeywordSearcherFuzzyTypo` / `FuzzyGapMatch` / `FuzzyLengthGuard` / `FuzzyRanksBelowKeyword` / `FuzzyDedupes` — class 6 behavior.
- `TestKeywordSearcherStrictClassOrderingInvariant` — end-to-end contract: six entries, six classes, strictly monotonic class order in results.
- `TestKeywordSearcherWithinClassTiebreakByACTR` — within-class access-boost tiebreak.

---

## 2026-04-10 — Batch `mm_search` via `queries` array (schema extension)

**Decision**: Extend the `mm_search` MCP tool to accept `queries: []string` alongside the existing `query: string` field. Exactly one of the two must be provided — runtime validation, not a JSONSchema oneOf (the reflection-generated schema can't express it cleanly for our SDK, and LLM clients don't enforce oneOf anyway). Each query in the batch runs the full tiered fallback pipeline independently; filters and limit apply uniformly; per-query results are concatenated into the single `Markdown` output field separated by their own `## mm_search: "<query>"` H2 headings.

**Implementation**: `internal/mcp/tools.go`. Added `Queries` field to `SearchInput` (with `omitempty` so the reflection-generated schema makes both fields optional — required-ness is enforced at runtime instead). New helpers `collectSearchQueries`, `trimNonEmpty`, `joinMarkdownBlocks`. Empty strings inside a `queries` array are filtered (no error) so programmatically-built query lists can drop candidates without tripping the empty-query guard.

**Why not four new tools**: Hard rule #6 is "four MCP tools forever." Adding a dedicated `mm_search_batch` would violate the rule. A schema extension to an existing tool is idiomatic and stays within the rule — the rule constrains tool count, not input-shape evolution. When the tool's schema changes, old clients keep working because the new field is optional and the validation falls back to the old single-query path.

**Why concatenated markdown, not per-query output struct**: Simplicity. The markdown already carries per-query structure via the `## mm_search: "<query>"` headings, which context-mode's automatic session indexing can chunk for warm follow-ups just like it did before. Adding a `Queries []QueryResult` field to `SearchOutput` would duplicate structure the markdown already exposes, complicate the schema, and break existing scripts that parse `out.Count` as "total results returned" (which now sums across all queries — still correct semantically).

**Why not a `sf --headless`-style hard separator between blocks**: Two tradeoffs considered. Explicit separators (`---\n`, `\x00\x00`, etc.) would make machine-parsing easier but uglier for human reading. Preserving the existing H2-per-query structure achieves both — human-readable AND machine-parseable by splitting on `\n## mm_search: "`. Callers that need strict scripting should use the CLI with `--json` (Item 3) rather than parsing MCP output.

**Rejected alternatives**:
- **Per-query limit**: considered letting batch callers specify `limit` per query (e.g., `[{query: "a", limit: 3}, {query: "b", limit: 10}]`). Rejected — adds complexity, and the common case is "same N results for each angle." Callers who need heterogeneous limits can make two calls.
- **Concurrent per-query execution**: considered running batch queries in parallel goroutines. Rejected — the searcher already completes each query in sub-100ms on realistic corpora, and the `KeywordSearcher.shortCircuitCount` field is not thread-safe by design (documented as such). Sequential keeps the debug path simple.
- **Requiring `Queries` to always be used (deprecate `Query`)**: rejected — breaks every existing caller. Backward compatibility is the whole point of the optional-field design.

**Test coverage**: 4 new tests in `internal/mcp/mcp_test.go`:
- `TestHandleSearchBatchQueries` — multi-query hits land in separate H2 blocks, Count sums correctly.
- `TestHandleSearchBatchEmptyStringFiltered` — empty-string entries in a batch are dropped, not errors.
- `TestHandleSearchQueryAndQueriesBothFails` — mutual-exclusion enforced at runtime.
- `TestHandleSearchBothEmptyFails` — neither field set, or a batch of only empty strings, both error.

**Consequence for existing callers**: None. A caller passing `query: "foo"` gets byte-identical output as before — `joinMarkdownBlocks` short-circuits the single-block case with no added bytes.

---

## 2026-04-10 — Project filter becomes a soft ranking multiplier

**Decision**: `Query.Project` in `internal/search/search.go` is no longer a hard filter on the corpus. It now acts as a within-class score multiplier:

| Entry project | Multiplier |
|---|---|
| same as query project (case-insensitive) | **1.3×** |
| empty or `"general"` | **1.0×** (neutral) |
| any other project | **0.8×** (demoted, not dropped) |

A new `Query.StrictProject bool` field restores the pre-refactor hard-filter behavior for callers that truly want only-this-project results (CLI flags like a future `mastermind discover --project foo`). The MCP `mm_search` handler leaves `StrictProject` false — agent callers want the most relevant results across projects, just weighted toward the current one.

**Implementation**: `internal/search/search.go`. Added `projectMultiplier(queryProject, entryProject string) float64` near `accessBoost`. Each per-entry scoring site in the two-pass pipeline (and the fuzzy fallback) computes `projMult := projectMultiplier(q.Project, r.Metadata.Project)` once per entry and multiplies the final `Score` by it. The multiplier applies to the full score (match contribution + access boost), so it scales proportionally. Class is not affected — this stays within-class only, preserving the tier-class invariant.

**Why soft, not hard**: The hard filter dropped cross-project entries entirely. A user searching for "hook" in the mastermind repo never saw their own "hook" lesson from a different project, even if it was the only useful hit. The soft filter preserves cross-project discoverability while still privileging the current project as the most likely context.

Convergent validation from shiba-memory (`002_profiles_scoping.sql:129-133`): shiba implements this exact pattern with the same 1.3 / 1.0 / 0.8 weights. The values were adopted verbatim because shiba has validated them in a similar retrieval context and there's no a-priori reason to re-tune. Adjust if dogfooding surfaces cross-project noise.

**`general` as the neutral case**: mastermind's convention is that lessons with `project: general` apply across any project (see mm_write docs). The multiplier treats those as neutral (1.0×) regardless of the query's project — they should surface at their natural rank, not get demoted as "cross-project". Same treatment for entries with an empty project field (rare, but handled).

**Interaction with tier classes**: Critical invariant — the multiplier is strictly within-class. A cross-project class-3 hit (topic tokens) still beats a same-project class-5 hit (body keyword) because class dominates score in the sort comparator. Test `TestKeywordSearcherProjectBoostIsWithinClassOnly` locks this in.

**Interaction with access boost**: The multiplier scales the full score including the ACT-R access boost. This means a heavily-accessed cross-project entry can still surface above a fresh same-project one IF the underlying match signal is weak enough that access history is the deciding factor. That's the correct behavior — access proof > project proximity when the structural signal is marginal.

**What was rejected**:
- **Hard filter as default**: the old behavior. Dropped cross-project long-tail discoverability.
- **Configurable weights**: considered letting `mm_search` callers pass custom multipliers. Rejected — one more knob with no clear use case, and shiba-memory's defaults are already validated. Revisit if real-world dogfooding reveals the 1.3/0.8 split is wrong.
- **Project-boost as an additive score bump**: considered `score += 0.3` for same-project, `score -= 0.3` for cross-project. Rejected because additive bumps interact weirdly with tier-class bounds (a class-5 score is much smaller than class-3, so the same absolute bump has very different relative effects). Multiplicative scales proportionally and is easier to reason about.

**Test coverage** (5 new tests in `internal/search/search_test.go`):
- `TestProjectMultiplierCases` — all 7 matrix cases of the multiplier function.
- `TestKeywordSearcherProjectBoostRanksSameProjectFirst` — soft-filter surfaces both entries, same-project ranks first.
- `TestKeywordSearcherStrictProjectRestoresHardFilter` — `StrictProject: true` restores old behavior.
- `TestKeywordSearcherProjectBoostIsWithinClassOnly` — multiplier cannot bridge class gaps.
- Updates to `TestMatchesMetadataFilters` — project cases now test both soft (default) and strict paths.

**Consequence for existing callers**: The MCP `mm_search` handler's semantics change subtly. A caller passing `project: "mastermind"` previously saw ONLY mastermind entries; now they see a mastermind-biased ranking across all projects. This is the intended behavior. CLI callers that need hard filtering can set `StrictProject: true` — but the current CLI subcommands don't pass `Project` through `Query` at all (verified during the design pass), so no CLI is affected.

---

## 2026-04-10 — Extraction quality bundle (filler filter + session timestamp + gap-fill skip)

**Decision**: Three small improvements to `internal/extract/` land together as one logical change. Each was filed as a separate open loop after the 2026-04-10 reference-repo mining pass; they're bundled because they touch adjacent code and share ordering constraints.

### 1. Filler-line filter (keyword.go)

`keyword.go` now precomputes a `skipLine []bool` mask before the pattern sweep. Any line that opens with a filler phrase — `ok`, `sure`, `let me`, `i'll now`, `here's`, `looking at`, `alright`, `got it`, `understood`, `let's see`, `sounds good`, `on it` — is dropped from the per-pattern match loop. Borrowed from soulforge's `isSubstantive()` filter. The mask is hoisted outside the outer `patterns` loop so the filler regex runs O(lines) instead of O(lines × patterns).

Anchored to start-of-line (`^\s*`) with a word boundary so the filter can't match inside legitimate content. `I'll use Redis because it's faster` is NOT filler; `Let me use Redis because it's faster` IS filler because the opener carries no signal and the "because" decision match is spurious.

### 2. Session-timestamp header (llm.go)

LLM extraction now prepends `Session time: 2026-04-10 (Monday)\n\n` to the transcript before the provider switch (Anthropic/Ollama/OpenAI). Borrowed from OpenViking's `session/memory/session_extract_context_provider.py` extraction prompt structure. Purpose: ground relative temporal references — "tomorrow", "next sprint", "by end of month" — against today's date so extracted open-loops and events have accurate `date` fields.

Cost is ~15 characters. Wall-clock source is indirected via a `sessionNow` package var so tests can freeze time without patching `time.Now` globally.

### 3. LLM gap-fill skip (llm.go)

`LLMExtractor.Extract()` now always runs the keyword tier first (the LLMExtractor already held a `KeywordExtractor` instance for fallback purposes; the new behavior promotes it from fallback to first-pass). If the keyword tier returns at least `GapFillThreshold` entries (default 5, configurable in `Config`), the LLM call is skipped entirely and the keyword results are returned directly.

Borrowed from soulforge's `buildV2Summary()` threshold pattern (skip LLM gap-fill when slot count ≥ 15). Same principle: high-signal sessions often produce enough rule-based matches that paying API cost to re-extract the same signals is wasteful. The LLM tier becomes free on high-signal sessions and retains its value on thin transcripts.

Side benefit: the fallback path (LLM error, `Strict=false`) now returns the cached keyword entries from pass 1 instead of re-running keyword extraction. One less duplicated regex pass.

**Config**: new `Config.GapFillThreshold int`. Default `5` in `DefaultConfig()`. Zero disables the skip (preserves pre-2026-04-10 behavior). Not exposed to the CLI or config file yet — the default is conservative enough that tuning can wait for dogfooding signal.

**Ordering constraint**: filler filter must land before gap-fill skip. Without the filter, the keyword tier overcounts filler lines and the gap-fill threshold can fire on spurious matches. Landing them together in one commit avoids any intermediate state where the threshold is tripped by noise.

**Test coverage** (7 new tests in `internal/extract/item_b_test.go`):
- `TestFillerPattern_MatchesCommonOpeners` — regex matrix: 18 cases covering openers that should match + legitimate content that should not.
- `TestKeywordExtractor_SkipsFillerLines` — integration: `Let me use X because Y` produces zero entries (filler filter blocks the `because` decision pattern).
- `TestKeywordExtractor_FillerFilterDoesNotHurtRealContent` — regression guard: real decision/fix/plan lines still produce entries.
- `TestSessionTimestampHeader_Format` — deterministic format via frozen `sessionNow`.
- `TestSessionTimestampHeader_RealClockNotZero` — prefix/suffix sanity with the real clock.
- `TestLLMExtractor_GapFillSkipWhenKeywordRich` — LLM is never called when keyword tier returns ≥ 5 entries (verified by pointing at `http://127.0.0.1:1/v1` — a guaranteed connection-refused endpoint — and asserting no error).
- `TestLLMExtractor_GapFillThresholdZeroDisablesSkip` — threshold 0 means always call the LLM (then fall back to keyword on the unreachable endpoint under `Strict=false`).

**Verification**: full suite passes. `extract-audit` corpus was not run as part of this commit because the improvements are structural (filter/reorder/header) rather than new match patterns — no recall/precision shift expected on existing labeled signals. A separate audit pass should confirm this once the next corpus update lands.

**What was rejected**:
- **Filler filter as a per-pattern check** (inside the inner loop instead of hoisted): simpler but O(lines × patterns) regex evals vs. O(lines). The hoist is a free perf win.
- **Session timestamp as a system-prompt addition** (put it in `extractionPrompt` instead of prepending to transcript): the system prompt is static at compile time; injecting `time.Now()` would require a dynamic prompt builder and touch three `call*` functions. Prepending to the transcript is one line in `Extract()`.
- **Gap-fill skip with a separate `keyword_first` mode**: considered exposing this as a new `Mode` value. Rejected — it's not a different backend, it's an optimization on the existing LLM backend. The threshold config suffices.

---

## 2026-04-10 — Supersedes/contradicts frontmatter fields + search co-retrieval

**Decision**: Added two optional `[]string` fields to `format.Metadata` — `Supersedes` and `Contradicts` — as an additive schema extension. Both are human-populated only; mastermind does not auto-generate them. Their search-time behavior diverges by design:

- **`supersedes`**: contributes to within-class score multiplier. Each listed slug adds 0.2 to a multiplier capped at 3 links (1.0× → 1.6× max). Applied in a single post-sort pass in `internal/search/search.go` before the limit trim, then results are re-sorted. Within-class only — class still dominates, the boost cannot bridge a class gap.

- **`contradicts`**: does NOT contribute to the score at all. Instead, it triggers **co-retrieval**: after the top-K results are selected, any entry in that top-K with a non-empty `Contradicts` list has its listed slugs looked up in the filtered corpus and appended as additional Results with an `Annotation` field set to `contradicts "<topic>"`. Appended entries bypass the limit (they're co-retrieved because of the relationship, not because of keyword score). Capped at 3 total appended entries per query to keep the output bounded.

**Implementation**: `internal/format/entry.go` (struct fields), `internal/search/search.go` (boost pass + co-retrieval helper), `internal/search/format.go` (annotation rendering on the H3 heading), `docs/FORMAT.md` (additive field documentation).

**Critical divergence from shiba-memory**: shiba's `schema/007_actr_proper.sql:121` boost formula treats all six relation types identically (`* (1 + SUM(link_strengths) * 0.2)`) — `contradicts` gets the same score bump as `supports`. Mastermind rejects that semantics because it violates hard rule #7 ("knowledge is never silently overridden"): boosting a contradicting entry hides the tension behind score math instead of surfacing it. The co-retrieval pattern is stronger — the contradicting claim appears *alongside* the claim it contradicts, with an annotation that makes the relationship legible to the reader. Shiba validated the multiplier shape; mastermind applies it only to the monotonic-confidence signal (`supersedes`).

**Why slugs, not paths**: Paths are volatile — moving an entry between scopes or directories would break all references. Slugs (filename minus `.md`) are stable across file moves within a scope. The `slugFromPath` helper is intentionally naive (last path component, strip `.md`) because mastermind's store generates slugs from the topic at write time and doesn't rename them post-hoc.

**Dangling slug policy**: A slug in `supersedes` or `contradicts` that no longer resolves (the target entry was moved, renamed, or deleted) is silently skipped during lookup. No error, no log — consistent with the silent-unless-needed rule. Review surfaces dangling links via `/mm-review` as a visible broken reference, which is where human triage belongs. Cascading deletion (shiba's `ON DELETE CASCADE`) is explicitly rejected because it violates hard rule #7.

**What was deferred**:

- **`/mm-review` integration**: the skill is a markdown prompt file (`~/.claude/skills/mm-review.md`), not a Go subcommand. Populating supersedes/contradicts from the skill is a follow-up in a separate session. The frontmatter + search behavior land here; the review-side prompting can iterate independently without breaking anything.
- **PageRank-style importance boost** (separate open loop): depends on the relations schema existing. **Landed 2026-04-10 as a follow-on, see the next entry.**
- **Validation of slug targets**: considered failing parse on a slug that doesn't resolve. Rejected — would create a chicken-and-egg problem where writing a new entry that supersedes an entry you're about to delete crashes parse. Dangling-slug tolerance is the cheaper and more forgiving default.

**Test coverage**: 8 new tests across two files.

`internal/format/relations_test.go` (3 tests):
- `TestParseSupersedesAndContradicts` — YAML parse populates both fields.
- `TestParseWithoutRelationsFields` — legacy entries still parse with empty slices.
- `TestMarshalPreservesRelations` — round-trip preserves fields; empty slices emit nothing (omitempty).

`internal/search/relations_test.go` (5 tests):
- `TestSupersedesBoostRanksHigherWithinClass` — same-class entries, one with supersedes links, boosted entry ranks first.
- `TestSupersedesBoostCapsAtThreeLinks` — anti-gaming: 10-link entry gets the same boost as 3-link; natural date order wins when the cap equalizes.
- `TestContradictsCoRetrievalSurfacesTarget` — entry A with `contradicts: [B]` pulls B into results with an `Annotation`, even if B doesn't match the query keyword.
- `TestContradictsCoRetrievalDoesNotDoubleCount` — an entry that matches naturally AND is contradicted by another top result appears exactly once.
- `TestSlugFromPath` — helper unit test covering path-with-dir, path-with-ext, plain name, extensionless name, empty input.

**Consequence for callers**: `Result` gains a new exported `Annotation string` field. Existing callers that don't read the field are unaffected (zero value is empty string). MCP callers see the annotation rendered inline in the H3 heading of each result — `### [scope] slug · kind · date · (contradicts "topic")`.

---

## 2026-04-10 — PageRank-style incoming-link boost (follow-on to supersedes/contradicts)

**Decision**: Added a second within-class score multiplier in `internal/search/search.go` that rewards entries based on **incoming** links from the rest of the (scope-gathered) corpus. Counts both `supersedes` and `contradicts` edges as incoming links — anything that explicitly points at an entry is evidence the entry was load-bearing enough to be referenced. Formula: `multiplier *= 1 + 0.1 * ln(1 + incoming_count)`, capped at +0.3 (max 1.3×). Single-pass count; no iterative eigenvector computation. Applied alongside the existing outgoing-supersedes boost — both signals coexist because they answer different questions.

**Why two boosts, not one**:
- **Outgoing supersedes** (existing): "this entry replaces N older decisions" → rewards the *latest synthesis*. The current state-of-the-art entry in a chain.
- **Incoming supersedes/contradicts** (new): "N newer entries had to explicitly replace or argue with this entry" → rewards the *historical anchor*. The decision that mattered enough to be revisited multiple times.

Both are legitimate "important entry" signals at different points in the lifecycle, and under hard rule #7 (knowledge is never silently deleted) the historical anchor is still searchable and worth surfacing.

**Why contradicts IS counted in the incoming pass even though it's excluded from the outgoing pass**: incoming contradicts means "newer entries say I am wrong", which is exactly the load-bearing signal we want — the entry was important enough to argue with. The asymmetry (excluded from outgoing, included in incoming) is intentional, not an inconsistency: the outgoing pass needed to avoid the shiba-memory failure mode of treating `contradicts` and `supports` identically as positive evidence about the *citing* entry. The incoming pass is asking the opposite question — what does the link tell us about the *cited* entry — and the answer is "people kept revisiting it", which is positive regardless of edge polarity.

**Why ln(1 + n) instead of n**: linear scaling lets a heavily-cited entry runaway-dominate. Log shape front-loads the reward (the first few citations matter a lot, additional ones matter less) and bounds growth naturally. The `+0.3` cap is a belt-and-suspenders safeguard at ~20 incoming links — at career-long corpus sizes one entry might plausibly accumulate that many citations, and we want the cap to engage before it bridges class boundaries.

**Scope choice — `refs`, not `filtered` or whole-store**: Computed over the scope-gathered `refs` slice (post-scope, pre-kind/tag-filter), so kind/tag filters don't shift importance — narrowing by `kind: lesson` shouldn't change which decision is "load-bearing" in the user's universe. Computing against the entire store was rejected because cross-scope edges would inflate importance even when the user has explicitly excluded a scope. The chosen middle ground keeps importance stable as filters narrow but scope-aware as the user changes which slice of their knowledge matters.

**Inspiration**: soulforge's `docs/repo-map.md` ranks files in the repo graph by PageRank importance because "which files matter most" is a stronger signal than "which files match the query best" for many tasks. Same logic applies to a long-lived knowledge corpus. Mastermind doesn't need iterative eigenvectors at this scale — a single-pass count is sufficient and stays within the no-persistent-index constraint (hard rule #3).

**Class invariant preserved**: like every other within-class boost (supersedes, access frequency, project), the multiplier never bridges a class gap. A maximally-boosted body-only hit (class 5) will still lose to a topic-token hit (class 3) because `sortResultsByTier` checks class first and treats score as a within-class tiebreaker only. Locked in by `TestIncomingLinkBoostCannotBridgeClass`.

**Test coverage**: 2 new tests in `internal/search/relations_test.go`:
- `TestIncomingLinkBoostRanksReferencedHigher` — anchor entry with three pointing-at-it newer entries (2 supersedes + 1 contradicts) ranks above an otherwise identical competing entry. The pointing entries have unrelated topics so only the relations metadata moves the needle.
- `TestIncomingLinkBoostCannotBridgeClass` — body-only hit with 30 incoming links (saturates the cap) still loses to a class-3 topic-token hit.

Both passing. Total search-package test count climbs accordingly; full suite still green.

**What was deliberately NOT done**:
- **Iterative PageRank eigenvector**: rejected on the YAGNI principle. At hundreds-to-thousands of entries, single-pass counting is indistinguishable from full PageRank for ranking purposes and avoids the need for a persistent precomputed importance index (hard rule #3).
- **Edge weighting** (e.g., `supersedes` worth more than `contradicts` or vice versa): rejected because there's no a-priori reason to weight them differently as incoming evidence. Both mean "someone bothered to write a frontmatter pointer at this entry". If dogfooding shows asymmetry is needed, revisit.
- **Damping factor**: classical PageRank uses one to prevent rank sinks. Not applicable here because we're not iterating.

---

## TBD — project-personal sync strategy

**Status**: Open.

**Leaning**: a side git repo at `~/claude-personal-memory` that tracks `~/.claude/projects/*/memory/` across all projects, with a remote for cross-machine sync. Alternative: leave project-personal machine-local and use user-personal for anything that needs to cross machines.

**Next step**: decide after Phase 5. Either answer works; the decision is a personal preference, not a blocker.
