---
date: "2026-04-05"
project: mastermind
tags:
  - phase2
  - phase3
  - store
  - config
  - project-personal
  - design-gap
topic: Wire ProjectPersonalRoot in runMCPServer — needs design call on path naming
kind: open-loop
scope: project-shared
confidence: high
---

## What was I about to do
Wire ProjectPersonalRoot in cmd/mastermind/main.go:runMCPServer the
same way I wired ProjectSharedRoot in commit 73e6804, so mm_write
with scope=project-personal can actually land entries on disk
instead of returning "invalid or unconfigured scope".

## Why I stopped
Project-personal lives at ~/.claude/projects/<project>/memory/, but
there are two incompatible naming conventions for <project>:

  1. Claude Code's own auto-memory dir uses dash-encoded absolute
     cwd paths, e.g.
     ~/.claude/projects/-Users-jean-Github-mastermind/memory/
  2. internal/project.Detect returns a normalized slug like
     "mastermind" from git remote → git root → cwd basename.

These do not agree. Whichever one I pick becomes hard to unwind
once entries exist under that path — mastermind would be reading
from a directory Claude Code doesn't populate, or writing to a
directory the rest of the project-personal workflow never looks at.

This is a design question, not an implementation question, and
it belongs to Jean, not to me.

## Resume when
Jean has picked one of:
  (a) Match Claude Code's dash-encoding so project-personal shares
      a directory with Claude Code's auto-memory — strongest
      continuity story, worst ergonomics for humans inspecting
      the path.
  (b) Use project.Detect's slug and keep project-personal separate
      from Claude Code's auto-memory — cleanest path, but now
      there are two "per-project" stores on disk that need to
      stay in sync mentally.
  (c) Something else (e.g., a symlink, a config override).

Once picked, the fix is 4–6 lines in runMCPServer, modelled on
the project-shared block at cmd/mastermind/main.go:93-103.

## Related
- commit 73e6804 (the project-shared half of this same wiring gap)
- docs/DECISIONS.md "TBD — project-personal sync strategy"
  (adjacent but not identical decision; may resolve together)
- .mm/nodes/store-defaultconfig-returns-a-skeleton-the-caller-must-wire-per-session-scope-ro.md
  (the pattern lesson about why this kind of gap exists)
