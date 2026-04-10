---
date: "2026-04-09"
project: mastermind
tags:
  - reference
  - memory
  - competition
  - shiba
  - act-r
  - knowledge-graph
topic: Shiba Memory — self-hosted agent memory with ACT-R scoring, knowledge graph, and Claude Code hooks
kind: insight
scope: project-shared
category: reference
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## Reference
https://github.com/ryaboy25/shiba-memory

Self-hosted memory layer for AI agents. TypeScript CLI + Hono HTTP gateway, Postgres 16 + pgvector, Ollama embeddings.

## Architecture
- Hybrid search: pgvector cosine similarity (70%) + Postgres FTS (30%)
- ACT-R cognitive scoring: access frequency + recency with power-law decay
- Knowledge graph: 6 relation types (supports, contradicts, supersedes, etc.)
- Self-improving: low-confidence "instincts" auto-promote to "skills" via `shiba evolve`
- Tiered extraction: regex pattern matching (free) + LLM-based session summarization

## Claude Code integration
Hooks into SessionStart, PostToolUse, PreCompact, PostCompact — injects relevant context automatically. This is more hook points than mastermind currently uses.

## Benchmarks (self-reported)
- 50.2% LongMemEval (beats Mem0 49.0%, below Zep 63.8% which uses GPT-4o)
- 90.7% HaluMem false memory resistance
- 34ms average retrieval

## Where mastermind diverges (by design)
- No database dependency (files on disk, git sync for free)
- No embedding model dependency (keyword search, no Ollama)
- No cloud, no infrastructure — survives the 2034 bug test
- ADHD-focused: invisible by default, zero maintenance
