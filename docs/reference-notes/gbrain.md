# gbrain reference notes

**Repo**: https://github.com/garrytan/gbrain  
**Stars**: ~5k  
**Stack**: TypeScript, Postgres + pgvector (PGLite for local), Bun  
**Tagline**: "Garry's Opinionated OpenClaw/Hermes Agent Brain"  
**Added**: Post-Phase-3, discovered while building the Hermes memory provider.

gbrain is a personal knowledge graph built for agent operation (Hermes/OpenClaw).
It is the opposite of mastermind on storage (Postgres vs. markdown files) and scope
(personal CRM + knowledge graph vs. engineering second brain), but has two patterns
worth noting.

---

## What to borrow

### 1. Compiled truth + timeline entry shape

Every gbrain page splits at a `---` separator:

```markdown
---
type: concept
title: Do Things That Don't Scale
---

Paul Graham's argument that startups should do unscalable things early.
The key insight: the unscalable effort teaches you what users want.

---

- 2013-07-01: Published on paulgraham.com
- 2024-11-15: Referenced in batch W25 kickoff talk
```

**Above the separator**: *compiled truth* — the current best understanding of a topic.
Gets **rewritten** (not appended) when new evidence arrives.  
**Below the separator**: *timeline* — append-only evidence trail, never edited.

This is architecturally different from mastermind's current model where new knowledge
creates new entries linked via `supersedes`. gbrain instead maintains ONE canonical
page per topic and rewrites the compiled truth in place, preserving history in the
timeline.

**Why it's interesting for mastermind**: today, related entries on the same topic
accumulate as separate files, connected only by `supersedes` links. A user searching
for "auth middleware" gets 3 fragmented entries instead of one coherent current view.
The compiled truth pattern would let mastermind maintain a single entry per topic
where the body is always the current synthesis. The `supersedes` chain would collapse
into the timeline section.

**What would need to change**: FORMAT.md (add optional timeline section below `---`),
search (prefer compiled-truth entries for a topic), and `mm_write` (detect if a topic
already exists and offer rewrite vs. new entry). Non-trivial. Worth dogfooding current
shape first to see if fragmentation becomes a real pain point.

### 2. `maintain` skill — proactive brain health

gbrain ships a `maintain` skill that runs periodic health checks:

- Find contradictions between pages
- Find stale compiled truth (not updated in N months)
- Find orphan pages (no incoming links)
- Find dead links
- Find tag inconsistency

Mastermind has reactive contradiction detection (`contradicts` co-retrieval in
`mm_search`) but no proactive health surface. A `mastermind maintain` command — or
an `/mm-maintain` skill — that surfaces these on demand would be useful, especially
as the corpus grows. The contradiction check and stale-entry scan are the most
immediately valuable.

### 3. "Thin harness, fat skills" philosophy

gbrain's explicit design principle: no skill logic in the binary, skills are markdown
files that agents load. This is exactly what mastermind does with `/learn`,
`/mm-review`, `/mm-discover`. Good independent confirmation of the pattern.

---

## What to ignore

| gbrain | Reason to ignore |
|---|---|
| Postgres + pgvector | Mastermind hard rule #3: no persistent index |
| Vector embeddings | Requires API key; mastermind is local-first, keyword-first |
| 30 MCP tools | Mastermind hard rule #6: four tools, forever |
| CRM schema (people/companies/deals) | Different domain; mastermind is engineering knowledge |
| HNSW index, RRF fusion | Depends on vector infra we're not building |
| File storage migration (S3, Supabase) | Out of scope for current phases |

---

## Verdict

Not a behavioral peer (different audience, different storage, different scope).
Two patterns worth tracking as open loops:
1. Compiled truth + timeline as a candidate future entry shape (after dogfooding)
2. `mastermind maintain` as a Phase 5+ health-check command
