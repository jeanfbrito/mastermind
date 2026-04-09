---
date: "2026-04-09"
project: mastermind
tags:
  - session-close
  - hooks
  - phase-3
  - research
topic: Research whether Claude Code SessionClose hooks can access conversation content
kind: open-loop
scope: project-shared
category: mcp
confidence: high
---

Phase 3 session-close auto-extraction depends on this answer. If hooks can't access conversation content, need alternative design (agent dumps summary to temp file before exit, or PreToolUse interception). This research blocks the most important remaining feature.
