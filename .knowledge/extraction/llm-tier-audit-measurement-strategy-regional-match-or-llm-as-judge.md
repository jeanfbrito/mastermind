---
date: "2026-04-10"
project: mastermind
topic: LLM tier audit measurement strategy — regional match or LLM-as-judge
kind: open-loop
scope: project-shared
category: extraction
confidence: high
accessed: 3
last_accessed: "2026-04-11"
---

## What's open

The extract-audit harness (fb3e119) uses substring matching — a label matches an extraction when the label's key_phrase is a case-insensitive substring of the extraction's topic+body. This works cleanly for the keyword tier (which emits verbatim context windows from matched signal lines) but collapses against the LLM tier, because local and frontier LLMs both paraphrase. On the 17 llm-tier labels in testdata/audit/corpus.json, Gemopus-4-26B scored 1/17 under substring matching despite producing ~60-70% qualitatively correct extractions (verified via `--dump-extractions` eyeball review).

Two candidate fixes, neither worth building until gap-fill short-circuit work (Phase 5+) actually needs a number:

1. **Regional matching** — for each label, find its character offset in the normalized prose. For each LLM extraction, find the offset of its `source_quote` (the verbatim anchor the prompt forces). Match when both offsets are within N chars (~300-500). Deterministic, ~30 LoC, no extra API calls. Degrades gracefully when the model skips source_quote. Tradeoff: measures "same region of transcript" which is a proxy for "same idea", not the thing itself — two unrelated lessons in adjacent paragraphs would spuriously match.

2. **LLM-as-judge** — for each (label, extraction) pair, a second LLM call asks "does this extraction describe the same knowledge as this label?" Cleanest semantic match. Tradeoffs: ~20x more API calls per audit run, needs a judge model distinct from the extractor (otherwise circular), prompt engineering for the judge, and budget discipline so the audit doesn't become slow or expensive.

## Why defer

The LLM tier is opt-in (`MASTERMIND_EXTRACT_MODE=llm`), runs through `pending/` + `/mm-review`, and never auto-promotes to the live store without human review. A bad LLM extraction costs one review click; a measurement number would drive zero production decisions today. The keyword tier is the production default and already has a clean baseline from the audit harness.

The question "does the LLM tier work well enough?" is currently answered by `--dump-extractions` qualitative inspection in ~30 seconds. That's sufficient until Phase 5+ gap-fill short-circuit needs to decide programmatically whether keyword has left a gap that warrants an LLM call — at that point an automated measurement becomes load-bearing.

## When to pick this up

- When the gap-fill short-circuit feature lands (Phase 5+ per ROADMAP.md)
- When specialized LLM sub-passes are in-flight and need regression gating
- When corpus-based LLM prompt iteration needs a numeric baseline

## Related

- docs/ROADMAP.md Phase 5+ (gap-fill short-circuit, sub-passes)
- extract-audit harness (cmd/mastermind/extract_audit.go)
- Substring matching limitation notes in auditOneTranscript
- LLM hardening commit 3de7200 (source_quote field already present in extraction schema, will be the anchor for regional matching)
