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

## TBD — project-personal sync strategy

**Status**: Open.

**Leaning**: a side git repo at `~/claude-personal-memory` that tracks `~/.claude/projects/*/memory/` across all projects, with a remote for cross-machine sync. Alternative: leave project-personal machine-local and use user-personal for anything that needs to cross machines.

**Next step**: decide after Phase 5. Either answer works; the decision is a personal preference, not a blocker.
