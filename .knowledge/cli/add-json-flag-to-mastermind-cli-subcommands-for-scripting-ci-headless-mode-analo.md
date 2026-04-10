---
date: "2026-04-10"
project: mastermind
tags:
  - phase-4
  - cli
  - json
  - headless
  - scripting
  - soulforge
topic: Add --json flag to mastermind CLI subcommands for scripting / CI (headless mode analog)
kind: open-loop
scope: project-shared
category: cli
confidence: high
---

## What's open
`mastermind discover`, `mastermind session-start`, and other CLI subcommands print human-readable output today. Soulforge's `sf --headless` produces JSON/JSONL for CI/CD consumption. Add a `--json` flag to each subcommand that emits a structured version of the same output.

## Why it matters
Low priority but zero design cost. Enables scripting mastermind inside shell pipelines, cron jobs, and eventual CI uses without parsing prose. Useful for `/mm-discover` when the skill wants structured candidate lists it can iterate over cleanly.

## Next action (when a concrete consumer appears)
- Audit each CLI subcommand for what output it produces today.
- Define a JSON schema per subcommand (keep flat — no nesting where a list will do).
- Add a global `--json` flag parsed in `main.go` and threaded through to each subcommand's printer.
- Document in README.

## Why parked at Phase 4
Not urgent. No one is scripting mastermind today. Land when there's a concrete driver — e.g., a shell script that wants to consume discover output, or a skill that needs structured session-start data. Don't add the surface speculatively.

## Source
Second-pass survey of soulforge. Headless mode from soulforge `docs/headless.md`; conversation 2026-04-10.
