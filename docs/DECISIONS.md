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

## TBD — project-personal sync strategy

**Status**: Open.

**Leaning**: a side git repo at `~/claude-personal-memory` that tracks `~/.claude/projects/*/memory/` across all projects, with a remote for cross-machine sync. Alternative: leave project-personal machine-local and use user-personal for anything that needs to cross machines.

**Next step**: decide after Phase 5. Either answer works; the decision is a personal preference, not a blocker.
