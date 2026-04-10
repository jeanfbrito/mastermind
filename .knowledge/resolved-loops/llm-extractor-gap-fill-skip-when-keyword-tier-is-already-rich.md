---
date: "2026-04-10"
project: mastermind
tags:
  - extraction
  - llm
  - optimization
  - soulforge
  - cost
  - open-loop
topic: LLM extractor gap-fill skip when keyword tier is already rich
kind: open-loop
scope: project-shared
category: extraction
confidence: high
---

## What's open

Reshape `internal/extract/llm.go` `LLMExtractor.Extract()` so the LLM call is skipped entirely when the keyword tier already produced sufficient results:

```go
func (l *LLMExtractor) Extract(transcript string, existingTopics []string) ([]format.Entry, error) {
    // Always run the keyword tier first (it's local, free).
    kw, err := l.keyword.Extract(transcript, existingTopics)
    if err != nil {
        return nil, err
    }
    // If keyword tier is already rich, skip the LLM entirely.
    if len(kw) >= l.gapFillThreshold {
        return kw, nil
    }
    // Otherwise run the LLM with keyword results as seeds, returning
    // combined results.
    return l.callLLM(transcript, kw, existingTopics)
}
```

Default `gapFillThreshold = 5` (tunable).

## Why

Soulforge's `buildV2Summary()` skips the LLM gap-fill when `slotCount() >= 15` — structured state is already rich enough that an LLM call would mostly re-extract what the rule-based tier already found. The same principle applies to mastermind: high-signal sessions often produce 5+ keyword hits, and the LLM tier then repeats or paraphrases them at API cost. Gap-fill skip makes the LLM free for high-signal sessions (zero API calls) while preserving its value on thin transcripts.

Today mastermind's extractor dispatcher is binary: `Mode == "keyword"` or `Mode == "llm"`. If LLM is set, the LLM always runs. This is wasteful.

## Next action

1. Audit `internal/extract/extractor.go` and `llm.go` to see if `LLMExtractor` already holds a `KeywordExtractor` (may need composition refactor).
2. Add `gapFillThreshold int` to `Config` (default 5, configurable).
3. Always run keyword tier first inside LLMExtractor.Extract.
4. Skip LLM if `len(kw) >= threshold`.
5. Otherwise run LLM with keyword results passed as "already extracted, don't duplicate" seeds.
6. New extract-audit test: a high-signal transcript (5+ clear signals) runs the keyword tier only.

Depends on the isSubstantive filler filter open loop landing first — otherwise the keyword tier overcounts filler lines and the gap-fill threshold fires spuriously.

## Source

`~/Github/soulforge/src/core/compaction/summarize.ts` `buildV2Summary` gap-fill threshold; mining report 2026-04-10 (Agent a143a0da extraction patterns); reference-notes/soulforge.md.

## Resolution

Shipped 2026-04-10. LLMExtractor.Extract always runs the keyword tier first; if it returns >= Config.GapFillThreshold entries (default 5), the LLM call is skipped and keyword entries are returned directly. Fallback path on LLM error also reuses the cached keyword results instead of re-running. No composition refactor needed — LLMExtractor already held a KeywordExtractor instance for fallback. 2 new tests lock in the skip behavior (unreachable endpoint + guaranteed connection refused proves no LLM call is made) and the threshold=0 bypass (preserves pre-refactor behavior).
