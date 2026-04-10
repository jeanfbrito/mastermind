---
date: "2026-04-10"
project: mastermind
tags:
  - phase-5
  - hooks
  - stop
  - open-loops
  - shiba-memory
  - experiment
  - adhd
topic: 'Phase 5+ experiment: Stop-hook auto-captures low-confidence open-loops when response ends without resolution'
kind: open-loop
scope: project-shared
category: hooks
confidence: high
---

## What's open
Shiba-memory ships a Stop hook that fires when Claude Code's response finishes. Their version updates session records and cleans episodes. An experiment for mastermind: use the Stop hook to auto-create low-confidence open-loop entries when a response ends without explicit resolution ("done", "fixed", "shipped", "closed", etc.).

## Why it matters
Today mastermind's open-loops are only created via explicit `mm_write` — requires the user or agent to remember. An automated Stop-hook trigger would populate open-loops without any user action, which is the holy grail for ADHD-friendly capture: zero willpower cost, silent, works on bad-memory days. High false-positive tolerance is acceptable — low confidence, high recall.

## Why parked at Phase 5+
Needs three supporting pieces that don't exist yet:
1. A **confidence threshold** so low-confidence auto-captures rank below user-created open-loops at session-start injection (so the injection doesn't drown in noise).
2. An **auto-close mechanism** for open-loops that turn out to be false positives. Likely benefits from landing after the `supersedes`/`contradicts` schema so newer entries can automatically close older loops they resolve.
3. A **resolution-phrase detector** (regex first, LLM if insufficient) so the hook only fires when the response genuinely ended without resolution.

## Open question
How does the Stop hook know what the unresolved task is? It sees the tail of the conversation but not the full working state. Probably needs to pair with the PostToolUse-based episodic capture (another shiba idea) so there's enough state to summarize.

## Source
`docs/reference-notes/shiba-memory.md` §1 (Stop row in hook table) and §8 item 6.
