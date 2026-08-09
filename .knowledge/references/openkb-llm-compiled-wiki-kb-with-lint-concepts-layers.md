---
date: "2026-04-19"
project: mastermind
tags:
  - reference
  - openkb
  - lint
  - knowledge-base
  - concepts-layer
  - mastermind-maintain
topic: OpenKB — LLM-compiled wiki KB with lint + concepts layers
kind: insight
scope: project-shared
category: references
confidence: high
---

## What

OpenKB (https://github.com/VectifyAI/OpenKB) is a Python CLI that ingests raw documents (PDF, Word, MD, etc.) and compiles them into a structured markdown wiki using LLMs. No vector DB — uses PageIndex (hierarchical tree index) for reasoning-based retrieval.

Wiki layout:
```
wiki/
├── index.md          KB overview
├── log.md            operations timeline
├── AGENTS.md         wiki schema (LLM instructions)
├── sources/          full-text conversions
├── summaries/        per-document summaries
├── concepts/         cross-document synthesis ← the good stuff
├── explorations/     saved query results
└── reports/          lint reports
```

## Relevant ideas for mastermind

### 1. `lint` subcommand (most actionable)
`openkb lint` runs structural + knowledge health checks: finds contradictions, gaps, orphaned entries, stale content. This maps almost directly to the open loop **"mastermind maintain — proactive brain health command"**. Mastermind's version would check: entries that contradict each other (contradicts graph), orphaned entries (no incoming links, no supersedes), stale open-loops (> N days old), L0/L1 budget drift.

### 2. `concepts/` layer
Cross-document synthesis — LLM reads multiple entries and writes a unified concept page. Relates to open loop **"compiled truth + timeline as candidate future entry shape"**. A mastermind equivalent could be `mm_consolidate`: takes N entries on the same topic + writes a synthesized insight. Different from `mm_promote` (pending→live) — this is live→live synthesis.

### 3. AGENTS.md in the wiki
Schema document that tells the LLM what the wiki structure means and how to navigate it. Mastermind's equivalent is the MCP tool descriptions + `serverInstructions`. Not much to borrow but the pattern is sound.

### 4. Watch mode
`openkb watch` auto-compiles when files land in `raw/`. Mastermind's hook model (PostToolUse, Stop) is the equivalent and is already more appropriate for the session-memory use case.

## What NOT to borrow

- Document ingestion (PDF, Word, etc.) — different domain from session memory
- LLM-heavy compilation at write time — mastermind keeps LLM optional
- Python stack — mastermind is Go
- PageIndex — overkill for mastermind's entry sizes (entries are short, not full PDFs)

## Key divergence

OpenKB is document→knowledge (batch ingestion). Mastermind is session→insight (continuous capture). The architectures are orthogonal despite both being markdown-first, no-vector-DB tools.

## Bottom line

Mine `lint` for mastermind maintain. Mine `concepts/` for compiled-truth entry shape. Ignore everything else.
