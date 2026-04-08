---
date: "2026-04-08"
project: mastermind
tags:
  - session-close
  - extraction
  - hooks
  - phase-3
topic: 'Phase 3: session-close auto-extraction hook needed to make knowledge growth automatic'
kind: open-loop
scope: project-shared
category: mcp
confidence: high
---

## Status
Session-start hook is implemented and working (surfaces open loops + project knowledge). But knowledge only grows when the agent or user explicitly calls mm_write. This defeats the ADHD design — requiring the user to remember "/mm-extract" at session end is the same failure mode.

## What's needed
A SessionClose hook that:
1. Has access to the conversation content (or a summary of it)
2. Extracts lessons, decisions, patterns, war-stories automatically
3. Writes them to pending/ (not live — user wasn't present to review)
4. Runs without user action

## Open question
Claude Code SessionClose hooks may not have access to conversation content. Need to research what context is available to a session-close hook subprocess. If no conversation access, may need the agent to dump a summary to a temp file before the hook runs.
