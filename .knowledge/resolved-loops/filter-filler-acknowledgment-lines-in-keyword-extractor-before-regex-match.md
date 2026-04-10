---
date: "2026-04-10"
project: mastermind
tags:
  - extraction
  - keyword
  - precision
  - soulforge
  - open-loop
topic: Filter filler acknowledgment lines in keyword extractor before regex match
kind: open-loop
scope: project-shared
category: extraction
confidence: high
---

## What's open

Add a pre-filter in `internal/extract/keyword.go` that skips sentences matching a filler-phrase regex before running the decision/discovery patterns:

```go
var fillerPattern = regexp.MustCompile(`(?i)^\s*(ok|sure|let me|i'll now|here's|looking at|alright|got it|understood)\b`)

// In the per-line extraction loop:
if fillerPattern.MatchString(line) {
    continue
}
```

## Why

Soulforge's `extractor.ts` `isSubstantive()` filter drops these acknowledgment openers before attempting decision/discovery regex (see `~/Github/soulforge/src/core/compaction/extractor.ts`, unlabeled sentence filter function). Mastermind's keyword extractor currently runs the full regex over every assistant line, producing false positives on lines like "Ok, let me look at the file" (which can trip the "let me" → decision heuristic if any such heuristic exists).

Zero-cost precision win. No behavior change for real decision/discovery lines. Mining report 2026-04-10 Agent a143a0da.

## Next action

1. Read `internal/extract/keyword.go` to confirm which pre-existing patterns could match filler lines.
2. Add the `fillerPattern` compile at package scope.
3. Skip-check at the top of the per-line loop before any regex match.
4. Add a test: line "Ok, let me look at this file" must produce zero extractions.
5. Verify via `extract-audit` on the labeled corpus that precision improves without hurting recall.

## Source

`docs/reference-notes/soulforge.md` extractor section; mining report 2026-04-10 (Agent a143a0da extraction patterns).

## Resolution

Shipped 2026-04-10. Added package-level fillerPattern regex and a precomputed skipLine []bool mask in KeywordExtractor.Extract() — the filler check runs O(lines) instead of O(lines × patterns). Anchored to start-of-line with a word boundary so it can't match inside legitimate content. 3 new tests in item_b_test.go cover the regex matrix, the integration case (filler lines produce zero entries), and the regression guard (real decision/fix/plan lines still extract).
