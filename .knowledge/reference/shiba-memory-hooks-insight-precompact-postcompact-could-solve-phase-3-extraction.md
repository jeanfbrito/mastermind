---
date: "2026-04-09"
project: mastermind
tags:
  - shiba
  - hooks
  - pre-compact
  - extraction
  - phase-3
  - act-r
topic: Shiba Memory hooks insight — PreCompact/PostCompact could solve Phase 3 extraction
kind: insight
scope: project-shared
category: reference
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## Key ideas from Shiba Memory worth evaluating for mastermind

### 1. PreCompact/PostCompact hooks
Shiba hooks into Claude Code's context compression events. PreCompact fires BEFORE the context window gets compressed — this is the moment where conversation content is still available but about to be lost. This could be mastermind's extraction trigger instead of SessionClose.

**Why this matters**: SessionClose hooks may not have access to conversation content. PreCompact definitely does (it fires because content exists). This could be the Phase 3 answer.

### 2. PostToolUse hook for proactive mm_search
Shiba injects relevant context after tool calls, not just at session start. Mastermind could use a PostToolUse hook to trigger mm_search when the agent reads files or makes changes — surfacing relevant past lessons in real-time.

### 3. ACT-R access frequency scoring
Memories accessed more frequently score higher. Mastermind could track access counts in frontmatter (increment on each mm_search hit) without adding a database. This would make frequently-useful entries rank higher over time.

**Action**: Research Claude Code PreCompact/PostCompact hook availability before designing Phase 3.
