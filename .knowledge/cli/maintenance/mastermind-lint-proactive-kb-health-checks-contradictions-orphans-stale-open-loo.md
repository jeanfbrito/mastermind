---
date: "2026-04-19"
project: mastermind
tags:
  - lint
  - maintenance
  - health-check
  - orphans
  - contradictions
  - open-loops
topic: mastermind lint — proactive KB health checks (contradictions, orphans, stale open-loops, budget drift)
kind: open-loop
scope: project-shared
category: cli/maintenance
confidence: high
accessed: 1
last_accessed: "2026-04-21"
---

## What

Add `mastermind lint` (or `mastermind maintain`) subcommand that runs structural health checks across the live knowledge store, inspired by OpenKB's `lint` subcommand.

## Checks to implement

1. **Contradictions** — scan `contradicts:` frontmatter links; report pairs where both entries are live and no resolution exists
2. **Orphans** — entries with no incoming links (not referenced by any `supersedes:`, `contradicts:`, or body wikilink); may indicate drift or dead knowledge
3. **Stale open-loops** — open-loop kind entries older than N days (configurable, default 90); surfaces loops that were captured but never resolved
4. **L0/L1 budget drift** — count open-loops and project-knowledge entries; warn when approaching the soft token budgets defined in MEMORY-STACK.md
5. **Broken supersedes chains** — `supersedes:` pointing to entries that no longer exist

## Output

- Human-readable report to stdout (one section per check)
- Exit code 0 if clean, 1 if issues found (CI-friendly)
- Optional `--json` flag for machine-readable output

## Reference

OpenKB `openkb lint` → `wiki/reports/` directory. Their checks: contradictions, gaps, orphans, stale content.
See mastermind reference note: `.knowledge/references/openkb-llm-compiled-wiki-kb-with-lint-concepts-layers.md`

## Why

Knowledge rots. Open loops accumulate. Contradictions go unresolved. A periodic lint pass surfaces drift without requiring the user to remember to look.
