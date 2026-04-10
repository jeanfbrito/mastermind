---
date: "2026-04-10"
project: mastermind
tags:
  - store
  - promote
  - mcp
  - path-based
  - cross-project
topic: mastermind's mm_promote now derives scope root from pending path structure, not startup config
kind: decision
scope: project-shared
category: store
confidence: high
---

## Decision
`Store.Promote`, `Store.Reject`, and (as a fallback) `Store.CloseLoop` now derive the scope root from the pending path structure: `<root>/pending/<file>.md` → root = `<root>`. Previously they required the path to be under one of the scope roots configured at MCP server startup.

## Why
The MCP server's scope config is frozen at startup based on its cwd. If the agent works in a different project than where the server was launched (common with multiple Claude Code sessions or a server started from a parent directory), promotion would fail with "path not under any configured scope" even though the file structure was perfectly valid.

## Implementation
New helper `rootFromPendingPath(abs)`:
- Validates that the file's parent directory is named `pending`
- Returns the grandparent as the scope root
- Errors on root-level pending directories (no real parent)

## Consequence
`mm_promote` is now self-contained: the path tells it everything. Works across projects, across machines, across MCP server lifetimes. The price: less strict validation — if someone passes a path to `/tmp/foo/pending/bar.md`, it'll be promoted to `/tmp/foo/bar.md`. That's acceptable because the tool is local-only and the agent is explicit about paths.

## Related
- `FindProjectRoot` now stops at `$HOME` (separate fix, addresses the root cause of why paths weren't in configured scopes).
- `rootFromLivePath` is a similar helper for `CloseLoop` that walks up looking for `.knowledge`/`memory` ancestors.
