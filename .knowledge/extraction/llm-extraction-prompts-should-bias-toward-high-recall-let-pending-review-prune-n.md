---
date: "2026-04-10"
project: mastermind
tags:
  - extraction
  - llm
  - prompt
  - high-recall
  - pending
  - decision
topic: LLM extraction prompts should bias toward high recall — let pending review prune, not the prompt
kind: decision
scope: project-shared
category: extraction
confidence: high
---

## Decision
Mastermind's LLM extractor prompt is deliberately biased toward high recall. When the LLM is uncertain whether something is worth capturing, it should extract it. False positives are pruned cheaply by the `pending/` queue + `/mm-review` loop; missed lessons are unrecoverable.

## Why
Two independent benchmarks (MemPalace 96.6% raw vs. 84.2% lossy, shiba 50.2%, Mem0 49.0%) show extraction-based systems are lossy. Mastermind chose extraction anyway for token-budget and ADHD-consolidation reasons — but given that choice, the prompt must not add a SECOND lossy filter on top.

## What the prompt does now (commit 2ee4442)
- No "focus on LESSONS and DECISIONS" precision gate
- No upper bound on body length
- Explicit open-loop signal phrase list
- Explicit "all six kinds matter equally"
- Design-note comment explaining why Scope is NOT in the JSON schema (caller assigns it)
- Floor: "extract EVERYTHING worth remembering"

## What it does NOT do
- Does NOT ask the LLM to judge importance
- Does NOT cap the number of candidates
- Does NOT ask for compression or summarization in the body

## Guard rule for future edits
Any edit to `internal/extract/llm.go` that adds a word like "only", "most important", "concise", "key", "priority", or "focus" in the instruction section is a precision gate in disguise. Reject it unless it's provably necessary.
