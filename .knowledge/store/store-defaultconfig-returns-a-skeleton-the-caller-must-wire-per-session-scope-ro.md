---
date: "2026-04-05"
project: mastermind
tags:
  - store
  - config
  - scopes
  - initialization
  - dogfooding
topic: store.DefaultConfig returns a skeleton — the caller must wire per-session scope roots before use
kind: pattern
scope: project-shared
confidence: high
---

## What happened
Shipped Phase 1 with store.DefaultConfig() populating UserPersonalRoot
and explicitly leaving ProjectSharedRoot and ProjectPersonalRoot empty,
documented as "caller must set via FindProjectRoot". runMCPServer in
cmd/mastermind/main.go then called DefaultConfig + store.New without
ever calling FindProjectRoot or computing the project-personal path.
The MCP server's view of the world was user-personal only. Any
mm_write with scope=project-shared returned "invalid or unconfigured
scope" and the scope was structurally unreachable from an agent.

## Why
DefaultConfig is deliberately a skeleton, not a complete config —
the per-session fields need runtime context (cwd for project-shared,
project name for project-personal) that the store package can't
compute for itself without pulling in the project package. The
comment said so. The caller never did the work.

## How I found it
Phase 2 dogfooding. Attempted to capture a lesson about MCP stdio
smoke testing via mm_write with scope=project-shared from inside
~/Github/mastermind. Got the config error. Traced DefaultConfig →
saw the empty field → read runMCPServer → found the wire-up was
missing. Fixed in commit 73e6804 by adding a 7-line detection block
after DefaultConfig.

## Lesson
Whenever a Config constructor documents "caller must set field X
before use," the caller that actually uses the config in production
(main.go here) MUST have a matching test OR a runtime assertion
that the field is set before any code path that depends on it
runs. Comments on the struct field are not enforcement; the caller's
init path has to do the wiring and it's easy to forget during a
refactor. Every per-session scope root belongs in runMCPServer's
setup block, next to DefaultConfig, in a form that's impossible to
miss when reading the function top-to-bottom.

## When this matters again
Any future scope added to the store (e.g., team-shared, session-local).
Any refactor of runMCPServer. Any new CLI subcommand that constructs
a store. Any bug report of the form "mm_write with scope=X returns
invalid or unconfigured scope" — check the init path first, not the
store logic.
