---
date: "2026-04-10"
project: mastermind
tags:
  - phase-4
  - config
  - task-router
  - model-selection
  - soulforge
  - env-vars
  - trigger-based
topic: 'Trigger-based: consolidate model/provider selection into ~/.knowledge/config.json + project override'
kind: open-loop
scope: project-shared
category: config
confidence: high
accessed: 1
last_accessed: "2026-04-10"
---

## What's open
Today mastermind's model/provider selection is scattered across environment variables: `MASTERMIND_EXTRACT_MODE`, `discover` subcommand provider flags, possibly others. Soulforge consolidates the equivalent into `taskRouter.spark = sonnet-4-6`, `taskRouter.compact = gemini-2.0-flash`, etc., in a single config file with global-plus-project layering.

Proposed mastermind equivalent:
```json
{
  "taskRouter": {
    "extract": "anthropic/claude-haiku-4-5",
    "discover": "anthropic/claude-haiku-4-5",
    "review": "anthropic/claude-sonnet-4-6"
  }
}
```
Global: `~/.knowledge/config.json`. Project override: `.knowledge/config.json`. Env vars stay valid as the outermost override for scripts.

Precedence (outer wins): env > project config > global config > hardcoded default.

## Why it matters
Env vars are fine for occasional use but painful for long-running users with multiple projects on multiple models. A config file is more discoverable and survives shell restarts.

## Tension with "invisible until needed"
Mastermind's default UX bias is to add zero config surface. This open-loop is **trigger-based**: only act on it if the current env-var setup causes real friction 2+ times in real dogfooding sessions. Do NOT implement pre-emptively.

## Next action (only when triggered)
- `internal/config/` (new package): layered config loader with the precedence above.
- Thread the resolved values through `internal/extract/` and `internal/discover/`.
- Document the schema in README.

## Close condition
Close this loop if 6 months pass with zero config-friction reports in the dogfooding log. The current env-var approach is probably fine at mastermind's scale.

## Source
Second-pass survey of soulforge. Config layering from soulforge `docs/provider-options.md`; conversation 2026-04-10.
