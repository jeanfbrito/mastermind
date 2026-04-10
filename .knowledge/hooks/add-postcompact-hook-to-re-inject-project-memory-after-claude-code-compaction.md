---
date: "2026-04-10"
project: mastermind
tags:
  - phase-3
  - hooks
  - postcompact
  - shiba-memory
  - context-injection
topic: Add PostCompact hook to re-inject project memory after Claude Code compaction
kind: open-loop
scope: project-shared
category: hooks
confidence: high
---

## What's open
Mastermind ships SessionStart + PreCompact hooks but not PostCompact. After Claude Code compacts a long session, most of the conversation is replaced by the compaction summary and the post-compression agent loses the project context it had at SessionStart. PostCompact is the missing half of the continuity story.

## Why it matters
PostCompact fires at every compaction boundary inside long sessions — more frequent than SessionStart. Re-injecting a curated slice of project memory at that moment keeps the agent oriented for the rest of the session without the user having to notice. This is the single highest day-to-day impact item from the shiba-memory reference.

## Next action
Add a `post-compact` subcommand to `cmd/mastermind/main.go` that runs the same curated-injection logic as SessionStart, scoped to the current project only. Same code path as SessionStart — tiny amount of new plumbing. Register the hook in the auto-init template so new `.knowledge/` installs get it automatically.

## Source
`docs/reference-notes/shiba-memory.md` §1 (hook table) and §8 item 1. Shiba ships all five Claude Code hooks (SessionStart + PostToolUse + Stop + PreCompact + PostCompact); mastermind ships only two.
