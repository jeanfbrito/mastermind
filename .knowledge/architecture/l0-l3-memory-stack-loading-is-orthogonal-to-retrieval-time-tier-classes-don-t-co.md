---
date: "2026-04-10"
project: mastermind
tags:
  - memory-stack
  - search
  - ranking
  - mempalace
  - architecture
topic: L0-L3 memory stack (loading) is orthogonal to retrieval-time tier classes — don't conflate axes
kind: insight
scope: project-shared
category: architecture
confidence: high
accessed: 1
last_accessed: "2026-04-11"
---

## Insight

Mastermind has two independent "tiered" concepts that sound similar but operate on completely different axes. Don't conflate them in documentation, design discussions, or code comments.

**L0-L3 memory stack** (loading convention): what's held in the agent's context at different phases.
- L0 — always loaded, tiny (open-loops header, ~500 tokens)
- L1 — always loaded, small (project knowledge, ~2000 tokens)
- L2 — on-demand, bounded per call (`mm_search` trimmed excerpts, ~200 tokens/result)
- L3 — on-demand, unbounded (direct file reads)

See `docs/MEMORY-STACK.md` for the enforcement model. The agent decides which layers to load based on the current phase of work.

**tierClass enum** (retrieval-time scoring): how a single `mm_search` call ranks its returned results.
- Class 0-2 — exact phrase matches (topic / tag / body)
- Class 3-4 — all tokens in metadata
- Class 5 — body keyword
- Class 6 — fuzzy fallback

See `internal/search/search.go` and `docs/DECISIONS.md` (2026-04-10). The searcher assigns every result a class and sorts by it.

## Why this matters

Both concepts came from mempalace (L0-L3 is borrowed directly; tier classes are independently invented but mempalace's retrieval pipeline was one of three mined before the design pass). Reading mempalace's docs you might assume L0-L3 IS the retrieval mechanism — it's not. Mempalace's L0-L3 is also a loading convention; its actual retrieval is a single ChromaDB ANN call with zero tiering.

Conflating these in code comments or docs would produce confusion later: "is this 'tier' the loading layer or the sort class?" Keep them explicitly separate.

## How to apply

- When writing mastermind docs, always qualify "tier": say "L0-L3 layer" or "tierClass 0-6", never bare "tier 0".
- When reading prior art from other memory systems, check whether their "tier" is a *loading* concept or a *retrieval* concept. Mempalace: loading. Mastermind's class enum: retrieval. Shiba-memory's ACT-R score: neither (it's a continuous signal).
