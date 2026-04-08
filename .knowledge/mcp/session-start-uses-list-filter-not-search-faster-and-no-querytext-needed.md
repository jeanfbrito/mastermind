---
date: "2026-04-08"
project: mastermind
tags:
  - session-start
  - search
  - performance
  - architecture
topic: session-start uses list+filter not search — faster and no QueryText needed
kind: decision
scope: project-shared
category: mcp
confidence: high
---

## Decision
The session-start subcommand collects entries via ListLive + metadata filter (Kind == open-loop, Project == detected name) rather than using the KeywordSearcher.

## Why
- No meaningful query text at session start — we want ALL open loops and ALL project entries, not keyword matches
- ListLive + filter is O(entries) with metadata only (no body loading) — completes in single-digit ms
- Search requires QueryText (empty query returns error by design) and does scoring work that adds no value here
- Keeps session-start independent of search internals
