---
date: "2026-04-09"
project: mastermind
tags:
  - git
  - knowledge-store
  - decision
topic: Decide .knowledge/ git strategy — commit, gitignore, or partial ignore
kind: open-loop
scope: project-shared
category: store
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

Auto-init now creates .knowledge/ in every git repo. Need to decide: should it be committed (shared team knowledge)? Gitignored (personal)? Partially ignored (.personal/ and resolved-loops/ ignored, rest committed)? Needs a DECISIONS.md entry.

## Resolution

Decided: commit live entries + resolved-loops, gitignore pending/. Auto-init now creates .knowledge/.gitignore. DECISIONS.md entry added 2026-04-09.
