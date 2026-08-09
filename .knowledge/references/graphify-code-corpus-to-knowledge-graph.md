---
date: "2026-04-20"
project: mastermind
tags:
  - reference
  - graphify
  - knowledge-graph
  - rationale-extraction
  - confidence-scores
  - god-nodes
  - mm-discover
  - mm-consolidate
topic: graphify — corpus-to-knowledge-graph skill with rationale + confidence
kind: insight
scope: project-shared
category: references
confidence: high
accessed: 2
last_accessed: "2026-04-21"
---

## What

graphify (https://github.com/safishamsi/graphify) is a Python skill that turns any folder of code/docs/papers/images/videos into a queryable knowledge graph. Cross-platform install (Claude Code, Codex, OpenCode, Cursor, Gemini CLI, OpenClaw, Factory Droid, Hermes, etc.).

Three-pass pipeline:
1. Deterministic AST pass — classes, functions, imports, call graphs, docstrings, rationale comments. No LLM.
2. Audio/video local transcription (faster-whisper, domain-aware prompt from corpus god nodes, cached).
3. Claude subagents in parallel over docs/papers/images/transcripts — concepts, relationships, design rationale.

Output: NetworkX graph → Leiden community detection → interactive HTML + `graph.json` + `GRAPH_REPORT.md` + plain-language audit report. Exposable as MCP server (`query_graph`, `get_node`, `get_neighbors`, `shortest_path`).

Claim: 71.5x fewer tokens per query vs raw files after graph is built.

## Relevant ideas for mastermind

### 1. Rationale extraction (actionable — feeds `mm_discover`)
graphify extracts `# WHY:`, `# NOTE:`, `# IMPORTANT:`, `# HACK:` comments as `rationale_for` nodes linked to the code they explain. Not just what the code does — why it was written that way. Mastermind's `mm_discover` currently mines git history + code shape; adding a rationale-comment extractor would surface the "why" that engineers leave as hints. Cheap pass (regex), high signal.

### 2. Confidence scores on INFERRED edges
Every INFERRED edge carries `confidence_score` (0.0–1.0). EXTRACTED edges are always 1.0. AMBIGUOUS edges are flagged for review. Mastermind's graph layer (supersedes/contradicts/PageRank) has no confidence field — relationships are binary. A confidence tag would let `mm_search` weight `contradicts` co-retrieval and PageRank-incoming boosts by certainty rather than count. Also maps cleanly onto pending/ triage: low-confidence auto-extracted edges land in pending, high-confidence go live.

### 3. God nodes → feeds `mm_consolidate` open loop
Highest-degree nodes = hub concepts everything connects through. Mastermind can compute this from the supersedes/contradicts graph + topic slugs. God-node detection is a natural trigger for `mm_consolidate` (open loop): when a topic has N live entries and sits as a god node, it's a candidate for synthesis. Skips the "manually notice there's duplication" step.

## What NOT to borrow

- Leiden community detection — overkill at mastermind corpus sizes (hundreds of entries, not tens of thousands of graph nodes). Topic slugs already cluster.
- PreToolUse hook that gates Glob/Grep — mastermind's L1 session-start injection + PostToolUse suggest already solves the "use the graph before grepping" problem, and does it without blocking.
- NetworkX + Python stack — mastermind is Go, markdown-first. A separate Python sidecar is the wrong shape.
- Interactive HTML graph viewer — pure consumer-facing UI, conflicts with "invisible by default" hard rule.
- Batch corpus compilation — mastermind is continuous session capture, not one-shot ingestion.
- Hyperedges (3+ node relationships) — interesting but overlaps with existing "Tensions as first-class knowledge kind" open loop; keep that loop as-is.

## Key divergence

graphify is batch: take a folder, produce a graph, query the graph. Mastermind is continuous: every session appends, entries graph themselves via supersedes/contradicts, search retrieves at prompt time. Both avoid embeddings — graphify uses topology (Leiden), mastermind uses tiered keyword + ACT-R + PageRank.

Same philosophical camp (graph-topology as similarity signal, no vector DB), different temporal model.

## Bottom line

Mine rationale extraction for `mm_discover`. Mine confidence scores for a future `contradicts`/`supersedes` weight. Mine god-node detection as a trigger heuristic for `mm_consolidate`. Ignore the Python stack, Leiden, HTML viewer, and PreToolUse gate.
