---
date: "2026-04-20"
project: mastermind
tags:
  - open-loop
  - mm-discover
  - graphify
  - rationale
  - code-mining
topic: Rationale-comment extractor for mm_discover (from graphify)
kind: open-loop
scope: project-shared
category: discover/rationale
confidence: high
---

## What

Extend `mm_discover` to mine rationale comments from source files — `# WHY:`, `# NOTE:`, `# IMPORTANT:`, `# HACK:`, `# TODO:`, `# FIXME:`, plus language-appropriate variants (`// WHY:`, `/* WHY: */`). Each hit becomes a candidate entry linking the comment text to the surrounding function/class as the "anchor."

## Why

Borrowed from graphify — treats these comments as `rationale_for` nodes linked to the code they explain. The "why" engineers leave as hints is exactly what mastermind wants: durable, non-obvious, written by someone with context. `mm_discover` currently mines git history + code shape; rationale comments are a free, high-signal third source.

## Design sketch

- Regex pass across tracked files (respect `.gitignore`). Cheap — no LLM required for the initial hit detection.
- For each hit: capture the comment body + N lines of context (function signature / class header).
- LLM pass (optional, tier-gated like existing discover) classifies as {insight, decision, war-story} and writes a pending entry with `confidence=medium` and `category: discover/rationale`.
- De-dup against existing entries via topic-slug fuzzy match so re-runs don't flood pending.

## Open questions

- Where to draw the line between "rationale worth capturing" and noise (`# TODO: fix later`)? Probably a length/verb heuristic + LLM gate.
- Should resolved comments (removed in later commits) still be captured from git history, or only from HEAD?

## Source

graphify — `rationale_for` node extraction. See `.knowledge/references/graphify-code-corpus-to-knowledge-graph.md`.
