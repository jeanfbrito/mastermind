---
date: "2026-04-10"
project: mastermind
tags:
  - design
  - hard-rule
  - knowledge-graph
  - mempalace
  - shiba-memory
  - invariant
  - validation
topic: 'Three independent memory systems converged on "invalidate, don''t delete" — validates mastermind hard rule #7'
kind: insight
scope: project-shared
category: design
confidence: high
accessed: 1
last_accessed: "2026-04-11"
---

## The convergence
Three independently-designed memory systems all landed on "don't delete knowledge, invalidate it":

- **MemPalace** (Python, ChromaDB): `kg.invalidate("Kai", "works_on", "Orion", ended="2026-03-01")`. Current queries skip the triple; historical queries still return it.
- **shiba-memory** (TypeScript, Postgres+pgvector): six relation types including `supersedes` and `contradicts`. Links are add-only; nothing is deleted.
- **mastermind** (Go, markdown): hard rule #7 — *"Pending entries are kept indefinitely. Knowledge is never silently deleted."* The `supersedes`/`contradicts` schema is an open-loop for Phase 5+.

## Why this matters
When three independently-built systems solving the same general problem converge on the same invariant, the invariant is load-bearing. Any future pressure to add a "delete old entries" feature to mastermind should be met with: "but three systems that solved this differently all ended up at the same rule."

## The underlying insight
Memory evolution is non-monotonic: what's true now may have been true differently before. Deleting the old state destroys the historical record. Invalidating marks it as "not current" without losing the timeline. In a corpus meant to run for 10+ years, the historical record is often more valuable than the current snapshot.

## Guard rule
If someone proposes a `delete` operation on the live store, re-read this entry. The answer is "mark it superseded and move on," not "remove it."

## Source
`docs/reference-notes/mempalace.md` §6, `docs/reference-notes/shiba-memory.md` §2, mastermind `CLAUDE.md` hard rule #7. Conversation 2026-04-10.
