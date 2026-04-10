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
accessed: 2
last_accessed: "2026-04-10"
---

## What's open
Four extractor improvements bundled because they all touch `internal/extract/`:

1. **Decision/discovery regex patterns** — port soulforge's WorkingStateManager patterns into the keyword backend: `"I'll use..."`, `"decided to..."`, `"because..."`, `"found that..."`, `"the issue was..."`. Zero LLM cost, measurable recall win.

2. **Specialized LLM sub-passes** — split the current omnibus extraction prompt into three narrow specialized calls modeled on shiba's `/extract/correction` + `/extract/summarize` + `/extract/preferences`. Concrete mastermind split: `detect-corrections` + `extract-decisions` + `extract-war-stories`. Higher precision at equal or lower cost (smaller outputs per call).

3. **LLM gap-fill short-circuit** — when the keyword backend already returned ≥N high-confidence candidates, skip the LLM pass entirely. Mirrors soulforge's "≥15 slots filled → skip gap-fill" rule. Cuts cost on rich sessions.

4. **High-recall audit** — two independent data points (shiba LongMemEval 50.2%, Mem0 49.0%) cluster at ~50% because extraction is lossy, while MemPalace raw-storage hits 96.6%. Mastermind deliberately chose extraction over verbatim storage (token budget, consolidation loop), but should audit the current extractor prompt for lossy over-summarization — when in doubt, extract MORE, let `/mm-review` and `pending/` prune. This is a prompt-engineering review, not a refactor.

## Why it matters
All four are Phase 3 polish with zero schema impact. Items 1 and 3 are near-zero-effort wins. Item 2 is a meaningful refactor but directly validated by shiba's production design. Item 4 is a half-hour prompt audit.

## Progress (2026-04-10)
- **Item 1 (regex patterns): DONE.** Landed in commit `ac0ccd0` (branch) + `c7322f7` (merge). Nine WorkingStateManager patterns ported into `internal/extract/keyword.go` with tiered confidence (`I'll use` / `the plan is` / `found that` etc. at medium; `because` / `going to` / `it seems` at low). Threaded a new `confidence` field through the pattern struct. "we should" deliberately skipped as open-loop territory. 18 new tests.
- **Item 4 (high-recall audit): DONE.** Landed in commit `2ee4442`. Removed the contradictory "do NOT extract trivial observations" clause, removed the 3-10 line body cap, added explicit open-loop signal phrase list, added "all six kinds matter equally", added design-note comment explaining why Scope is deliberately NOT in the LLM JSON schema. Audit agent proposed a scope-field addition which turned out to be wrong — the caller assigns scope in `cmd/mastermind/main.go:505-512`. Rejected that edit.
- **Item 2 (specialized LLM sub-passes): STILL OPEN.** Not yet split. Precision win gated on observing the LLM backend actually being used in anger (default is keyword mode).
- **Item 3 (gap-fill short-circuit): STILL OPEN.** Same gate as item 2 — only matters if LLM mode is the default.

Loop stays open for items 2 and 3. They're both gated on LLM-mode usage; revisit when MASTERMIND_EXTRACT_MODE=llm becomes the common path.

## Source
`docs/reference-notes/soulforge.md` §1, §5 items 1-2; `docs/reference-notes/shiba-memory.md` §3, §8 item 2.
