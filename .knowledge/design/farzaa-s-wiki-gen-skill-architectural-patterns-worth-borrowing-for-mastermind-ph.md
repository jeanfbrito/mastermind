---
date: "2026-04-06"
project: mastermind
tags:
  - design
  - reference
  - phase5
  - quality-audit
  - synthesis
  - subagents
topic: 'Farzaa''s wiki-gen skill: architectural patterns worth borrowing for mastermind Phase 5+'
kind: insight
scope: project-shared
confidence: high
accessed: 4
last_accessed: "2026-04-10"
---

## What I noticed
Farzaa's personal wiki skill (https://gist.github.com/farzaa/c35ac0cfbeb957788650e36aabea836d,
308 stars, April 2026) solves the adjacent problem of turning raw
personal data into a structured biographical knowledge base. Different
domain (life documentation vs engineering memory) but three patterns
transfer cleanly to mastermind's later phases.

## Why these matter

### 1. The 15-entry checkpoint audit
Every 15 absorbs, the wiki stops and re-reads the 3 most-updated
articles: is it narrative or chronological dump? Still useful or just
logged? Mastermind has no quality audit step beyond the one-at-a-time
promote flow. Phase 5+ could adopt a similar cadence: every N
promotions, re-read 3 random lessons from the working set and ask
whether they still hold, overlap with newer entries, or have evolved
into something richer than what was originally captured.

### 2. Parallel subagents for maintenance
/wiki cleanup spawns batches of 5 parallel agents, each auditing one
article for structure, staleness, broken links, and missing cross-
references. Mastermind's Phase 5 archive maintenance could borrow
this shape: parallel agents auditing entries for redundancy, evolved
understanding, or tag gaps — without flooding the main context.

### 3. The "breakdown" concept — emergent theme detection
/wiki breakdown scans for concrete nouns appearing across 3+ articles
without their own page, then creates stubs. The mastermind equivalent:
scan ~/.knowledge/lessons/ for tags appearing in 3+ entries that have no
"pattern" or "insight" entry synthesizing the recurring theme. This
is the "what have I learned about [electron] across all my war
stories?" question, answered mechanically. Genuinely new idea
mastermind doesn't have yet.

## What was explicitly rejected
- Persistent index (_index.md, _backlinks.json) — conflicts with
  hard rule #3 (no persistent index).
- Theme-driven synthesis/merging of entries into narrative articles —
  conflicts with atomic-entry-per-file design. Mastermind entries
  must survive a decade without the tool; if they are synthesized
  artifacts of an LLM pipeline they become unverifiable.

## When this matters again
Phase 5+ design (archive maintenance, working-set quality). Any
future feature proposal that sounds like "auto-organize" or
"auto-merge" — check whether farzaa's approach applies or whether
it conflicts with mastermind's atomicity constraint. The breakdown
concept specifically should be revisited when the working set
exceeds ~50 lessons and tag-based retrieval starts feeling noisy.
