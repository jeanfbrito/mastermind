---
date: "2026-04-10"
project: mastermind
tags:
  - store
  - findprojectroot
  - scope-collision
  - user-personal
  - project-shared
  - dogfooding
topic: FindProjectRoot walked to $HOME and collided ProjectSharedRoot with UserPersonalRoot
kind: war-story
scope: project-shared
category: store
confidence: high
accessed: 2
last_accessed: "2026-04-10"
---

## What happened
`buildSessionConfig` called `FindProjectRoot(cwd)` which walked upward looking for any `.knowledge/` directory. Since `~/.knowledge/` exists (the user-personal store), any cwd under `$HOME` walked up and matched it, returning `$HOME` as the "project root." `ProjectSharedRoot` was then set to `~/.knowledge/` — the exact same path as `UserPersonalRoot`. Writes with `scope=project-shared` silently wrote into `~/.knowledge/`, polluting the global store with 23 Rocket.Chat.Electron-specific lessons.

## Root cause
Auto-detection that walks up the filesystem has no notion of "outer boundary." Without an explicit stop, `FindProjectRoot` would match the first `.knowledge/` it found — even the user-personal store at `$HOME`.

## Fix
Added a `$HOME` guard in `FindProjectRoot`: stop walking when `abs == home`. A `.knowledge/` at `$HOME` is user-personal by definition, never a project root. Two tests lock this down (`TestFindProjectRootStopsAtHome`, `TestFindProjectRootFindsProjectBelowHome`).

## Defense in depth
Also made `Store.Promote`/`Reject` derive the scope root from the pending path structure itself (`<root>/pending/<file>.md`), so even if scope detection fails, the operation still works on the right directory.
