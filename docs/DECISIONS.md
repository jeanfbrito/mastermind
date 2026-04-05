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

## TBD — project-personal sync strategy

**Status**: Open.

**Leaning**: a side git repo at `~/claude-personal-memory` that tracks `~/.claude/projects/*/memory/` across all projects, with a remote for cross-machine sync. Alternative: leave project-personal machine-local and use user-personal for anything that needs to cross machines.

**Next step**: decide after Phase 5. Either answer works; the decision is a personal preference, not a blocker.
