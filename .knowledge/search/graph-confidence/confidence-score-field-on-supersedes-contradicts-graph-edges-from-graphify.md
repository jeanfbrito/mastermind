---
date: "2026-04-20"
project: mastermind
tags:
  - open-loop
  - graph
  - confidence
  - search
  - contradicts
  - supersedes
  - graphify
topic: Confidence score field on supersedes/contradicts graph edges (from graphify)
kind: open-loop
scope: project-shared
category: search/graph-confidence
confidence: high
---

## What

Add a `confidence` field (0.0–1.0 or {high, medium, low}) to graph edges — `supersedes`, `contradicts`, and any future relations. Edges written by the user via `mm_write` default to 1.0 / high. Auto-extracted edges from pending/ carry the extractor's confidence. `mm_search` weights PageRank-incoming boost and contradicts co-retrieval by edge confidence.

## Why

Borrowed from graphify — every INFERRED edge has `confidence_score`; EXTRACTED edges are 1.0; AMBIGUOUS edges get flagged. Mastermind's graph is currently binary: an edge either exists or doesn't, so a noisy LLM-extracted `contradicts` edge contributes the same retrieval weight as a hand-curated one. This inflates false positives in contradiction co-retrieval and skews PageRank.

## Design sketch

- FORMAT.md: add optional `confidence:` per edge target (schema change — needs DECISIONS.md entry, FORMAT.md is a long-term contract).
- Parser: default missing `confidence` to `high` for backward compat.
- Ranking: multiply PageRank-incoming boost and contradicts co-retrieval weight by edge confidence.
- `/mm-review`: surface low-confidence pending edges as a batch for fast accept/reject.

## Open questions

- Scalar (0.0–1.0) or enum (high/medium/low)? Enum is more markdown-friendly; scalar is more expressive. Likely enum to match existing entry-level `confidence` field.
- Does this justify touching FORMAT.md? Probably yes — schema change but additive and optional.

## Source

graphify — EXTRACTED / INFERRED / AMBIGUOUS edge tags with numeric confidence. See `.knowledge/references/graphify-code-corpus-to-knowledge-graph.md`.
