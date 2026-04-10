---
date: "2026-04-10"
project: mastermind
tags:
  - dream
  - consolidation
  - audit
  - shiba-memory
  - skill
topic: Audit /dream skill against shiba's `reflect consolidate` checklist
kind: open-loop
scope: project-shared
category: consolidation
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## What's open
Next time `/dream` is touched, audit it against shiba-memory's `reflect consolidate` operation list and document what mastermind's version deliberately does and refuses to do.

Shiba's checklist (from their CLI docs):
- Merge duplicates
- Detect contradictions
- Decay confidence of old unused memories
- Auto-link via embedding similarity
- Generate cross-project insights

## Expected mastermind positions (to confirm)
- **Merge duplicates**: YES — probably already covered via review prompts. Confirm and document.
- **Detect contradictions**: NO (not yet) — deferred until the `supersedes`/`contradicts` schema lands. Separate open-loop.
- **Decay old entries**: NO — violates hard rule #7 ("knowledge is never silently deleted"). The mastermind equivalent is de-prioritization in search ranking, not removal. Document this refusal explicitly so future-Jean doesn't accidentally add decay under pressure.
- **Auto-link via embedding similarity**: NO — no embedding store, and relations are human-populated. Document.
- **Cross-project insights**: UNCLEAR — needs audit. If `/dream` generates cross-project synthesis, what's the trigger and where does the output go?

## Deliverable
A clearer `/dream` skill spec that says what it does and what it deliberately refuses to do, with links back to the hard rules in CLAUDE.md that justify each refusal. Documentation task, not a code change (unless the audit surfaces gaps).

## Source
`docs/reference-notes/shiba-memory.md` §5, §8 item 5.

## Resolution

Audited 2026-04-10. Edited `~/.claude/skills/dream/SKILL.md` with four changes: (1) added explicit Scope section clarifying /dream operates on Claude Code auto-memory only, NOT on mastermind's .knowledge/ store or other independent project stores; (2) softened step 3 "Remove outdated items" to "Remove stale INDEX ENTRIES" (MEMORY.md is an index, removing a line never destroys content); (3) tightened step 6 "Clean up" to forbid deletion of non-empty files (merge or keep, don't drop); (4) added a "What /dream deliberately does NOT do" section documenting five refusals (no decay, no auto-link, no auto contradiction detection, no cross-project synthesis, no access-frequency decay) with rationale per item. Confirmed the current skill was already mostly correct — the audit surfaced clarifications and absences, not bugs. Documentation-only change, no code.
