---
date: "2026-04-10"
project: mastermind
tags:
  - phase-3
  - extraction
  - regex
  - llm-backend
  - shiba-memory
  - soulforge
  - mempalace
topic: Phase 3 extractor polish — regex patterns, specialized LLM sub-passes, gap-fill short-circuit, high-recall audit
kind: open-loop
scope: project-shared
category: extraction
confidence: high
---

## What's open
Four extractor improvements bundled because they all touch `internal/extract/`:

1. **Decision/discovery regex patterns** — port soulforge's WorkingStateManager patterns into the keyword backend: `"I'll use..."`, `"decided to..."`, `"because..."`, `"found that..."`, `"the issue was..."`. Zero LLM cost, measurable recall win.

2. **Specialized LLM sub-passes** — split the current omnibus extraction prompt into three narrow specialized calls modeled on shiba's `/extract/correction` + `/extract/summarize` + `/extract/preferences`. Concrete mastermind split: `detect-corrections` + `extract-decisions` + `extract-war-stories`. Higher precision at equal or lower cost (smaller outputs per call).

3. **LLM gap-fill short-circuit** — when the keyword backend already returned ≥N high-confidence candidates, skip the LLM pass entirely. Mirrors soulforge's "≥15 slots filled → skip gap-fill" rule. Cuts cost on rich sessions.

4. **High-recall audit** — two independent data points (shiba LongMemEval 50.2%, Mem0 49.0%) cluster at ~50% because extraction is lossy, while MemPalace raw-storage hits 96.6%. Mastermind deliberately chose extraction over verbatim storage (token budget, consolidation loop), but should audit the current extractor prompt for lossy over-summarization — when in doubt, extract MORE, let `/mm-review` and `pending/` prune. This is a prompt-engineering review, not a refactor.

## Why it matters
All four are Phase 3 polish with zero schema impact. Items 1 and 3 are near-zero-effort wins. Item 2 is a meaningful refactor but directly validated by shiba's production design. Item 4 is a half-hour prompt audit.

## Source
`docs/reference-notes/soulforge.md` §1, §5 items 1-2; `docs/reference-notes/shiba-memory.md` §3, §8 item 2.
