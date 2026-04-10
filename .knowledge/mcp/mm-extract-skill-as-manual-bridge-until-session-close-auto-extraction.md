---
date: "2026-04-08"
project: mastermind
tags:
  - skills
  - extraction
  - workflow
  - bridge-pattern
topic: /mm-extract skill as manual bridge until session-close auto-extraction
kind: pattern
scope: project-shared
category: mcp
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## Pattern
Created `/mm-extract` as a Claude Code skill that reviews the entire conversation and calls mm_write for each extractable lesson. This is the manual version of what Phase 3's session-close hook will automate.

## Why it matters
The skill reduces the cognitive cost from "craft a prompt explaining what to extract" to "type /mm-extract". Still requires remembering to run it, but it's one command instead of a paragraph.

## Location
`~/.claude/skills/mm-extract/SKILL.md`

## Upgrade path
When Phase 3 session-close hook is built, /mm-extract becomes the "manual re-extraction" tool for mid-session captures or re-processing old sessions. It doesn't get replaced — it gains a sibling.
