---
date: "2026-04-19"
project: mastermind
tags:
  - synthesis
  - consolidate
  - concepts
  - mcp-tool
  - compiled-truth
topic: mm_consolidate — synthesize N live entries on same topic into unified insight
kind: open-loop
scope: project-shared
category: mcp/tools
confidence: high
---

## What

Add `mm_consolidate` MCP tool (or CLI subcommand) that takes N live entries on the same topic and produces a single synthesized insight entry, inspired by OpenKB's `concepts/` layer (cross-document synthesis).

## Behavior

1. User (or agent) identifies a cluster of related entries (by tag, topic prefix, or explicit list of paths)
2. Tool feeds all entries to LLM with a synthesis prompt
3. LLM produces a unified `insight` or `decision` entry — the "compiled truth"
4. Original entries get a `supersedes:` back-reference on the new entry (or are demoted to `archive/`)
5. New entry lands in live store as the authoritative version

## Distinction from existing tools

- `mm_promote`: pending→live (review gate, no synthesis)
- `mm_write`: single new entry (no synthesis)
- `mm_consolidate`: live→live synthesis (many→one)

## Relation to other open loops

- "Compiled truth + timeline as candidate future entry shape" — this implements the compiled-truth half
- Proper-mode ACT-R — consolidated entries should reset access timestamp, not inherit oldest entry's

## Reference

OpenKB `concepts/` directory: cross-document synthesis where LLM reads multiple entries and writes unified concept pages.
See mastermind reference note: `.knowledge/references/openkb-llm-compiled-wiki-kb-with-lint-concepts-layers.md`

## Why

As the KB grows, the same insight gets captured multiple times across sessions from different angles. Consolidation turns fragmented signal into durable, authoritative knowledge.

## Requires

- LLM backend (optional, same pattern as existing `MASTERMIND_EXTRACT_MODE=llm`)
- Needs a DECISIONS.md entry before implementation (fifth MCP tool requires explicit justification per hard rule #6)
