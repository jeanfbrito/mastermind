---
date: "2026-04-12"
project: mastermind
tags:
  - maintain
  - health
  - gbrain
  - phase-5
topic: mastermind maintain — proactive brain health command (from gbrain)
kind: open-loop
scope: project-shared
category: mastermind/cli
confidence: medium
accessed: 1
last_accessed: "2026-04-21"
---

## What

gbrain ships a `maintain` skill that runs periodic health checks on the knowledge base: contradictions between entries, stale content, orphan pages, dead links, tag inconsistency.

Mastermind has *reactive* contradiction detection (`contradicts` co-retrieval in `mm_search`) but no *proactive* health surface.

## What a `mastermind maintain` command would do

Priority order (most to least valuable):

1. **Contradiction scan** — find all entries with `contradicts:` links, surface them grouped by topic so the user can resolve or promote one as the winner
2. **Stale open-loops** — open-loops older than N months with no activity; flag for close or acknowledge
3. **Supersedes chains** — entries with 3+ supersedes hops on the same topic; candidate for compiled-truth collapse (see related open-loop)
4. **Orphan entries** — entries in project-shared with no `project:` tag and no `supersedes`/`contradicts` links; likely mis-scoped
5. **Pending queue age** — pending entries older than 30 days; surface for review or auto-promote if policy allows

## Shape

```
mastermind maintain [--json] [--cwd DIR]
```

Output: markdown report with one section per health dimension. Silent if nothing to report (honors silent-unless-needed rule).

Could also be exposed as a `/mm-maintain` skill that runs the command and presents results for interactive triage.

## When to act

Phase 5+ — after the corpus has grown enough that health drift is noticeable. Likely after 200+ live entries.

**See**: `docs/reference-notes/gbrain.md` for the gbrain maintain skill description.
