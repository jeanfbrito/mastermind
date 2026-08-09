---
date: "2026-04-20"
project: mastermind
tags:
  - open-loop
  - mm-consolidate
  - graphify
  - god-nodes
  - graph-topology
topic: God-node detection as mm_consolidate trigger (from graphify)
kind: open-loop
scope: project-shared
category: consolidation/triggers
confidence: high
---

## What

Compute "god nodes" — highest-degree nodes in mastermind's entry graph (supersedes + contradicts + topic-slug clustering) — and surface them as candidate triggers for `mm_consolidate`. When a topic slug has N live entries AND sits as a god node (degree above some percentile), the lint/maintain pass flags it for synthesis.

## Why

Borrowed from graphify — god nodes identify hub concepts that everything connects through. In mastermind terms, these are topics where the user has accumulated enough scattered insights that a single synthesized entry would beat the scattered ones on retrieval. This skips the "manually notice you're duplicating yourself" step and gives `mm_consolidate` (already an open loop) a concrete heuristic for *when* to fire.

## Design sketch

- Periodic pass (driven by `mastermind lint` open loop) computes node degree across supersedes + contradicts + topic-slug collisions.
- God nodes = top K by degree OR above Pth percentile.
- If a god node has >= M live entries AND no recent consolidated entry covering it → emit a `mm_consolidate` suggestion in the lint report.
- User-initiated consolidation still writes to live; lint only suggests.

## Open questions

- Does mastermind's graph have enough edges to make degree meaningful, or do most entries still sit as isolates? Needs corpus measurement before investing.
- Interaction with `contradicts` edges — a topic with many contradictions is noisy, not a hub. Probably weight supersedes-in differently from contradicts.

## Depends on

- `mm_consolidate` open loop (synthesize N live entries on same topic)
- `mastermind lint` open loop (proactive KB health checks)

## Source

graphify — god-node detection via graph degree. See `.knowledge/references/graphify-code-corpus-to-knowledge-graph.md`.
