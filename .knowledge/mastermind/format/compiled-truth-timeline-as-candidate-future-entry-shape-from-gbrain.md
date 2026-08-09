---
date: "2026-04-12"
project: mastermind
tags:
  - entry-shape
  - gbrain
  - compiled-truth
  - future-design
topic: Compiled truth + timeline as candidate future entry shape (from gbrain)
kind: open-loop
scope: project-shared
category: mastermind/format
confidence: medium
---

## What

gbrain (garrytan/gbrain) uses a "compiled truth + timeline" pattern per topic: one canonical entry split by `---`. Above: current best understanding, **rewritten** when new evidence arrives. Below: append-only evidence trail, never edited.

## Why it matters for mastermind

Today, related knowledge on the same topic accumulates as separate entries linked via `supersedes`. A user searching "auth middleware" gets 3 fragmented entries instead of one coherent current view. The compiled truth pattern collapses the `supersedes` chain into a single living entry with history preserved in a timeline section.

## What would need to change

- `FORMAT.md`: add optional `## Timeline` section (append-only, below a `---` separator)
- `mm_write`: detect if a topic already exists and offer rewrite-compiled-truth vs. new-entry
- Search: prefer compiled-truth entries when topic matches exactly
- `mm_close_loop`-like command for "update compiled truth on existing entry"

## When to act

After dogfooding current shape long enough to feel the fragmentation pain. If `supersedes` chains regularly reach 3+ entries on the same topic, that's the signal. Don't design around a hypothetical pain point — let the corpus speak first.

**See**: `docs/reference-notes/gbrain.md` for full analysis.
