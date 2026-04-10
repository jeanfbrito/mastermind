---
date: "2026-04-10"
project: mastermind
tags:
  - cli
  - json
  - scripting
  - ci
  - flags
topic: Add --json flag to mastermind CLI subcommands for scripting / CI
kind: open-loop
scope: project-shared
category: cli/json-output
confidence: high
---

## What
Add a `--json` flag to mastermind CLI subcommands so they emit structured JSON instead of human-readable output. Analog of soulforge's `sf --headless`. Target subcommands: `discover`, `session-start`, `post-compact`, `suggest`, `extract`, `extract-audit`. Exclude `mcp` and `session-close` (stub).

## Design map (from 2026-04-10 analysis)

### Flag parsing
All subcommands use ad-hoc manual `os.Args` scanning — no `flag.FlagSet`. Each iterates `os.Args[2:]` with a switch. Injection point is the same for all: add `case "--json": jsonOut = true` to each loop. `extract-audit` already does this at `extract_audit.go:140`.

### Natural injection point in dispatch
`cmd/mastermind/main.go:57` — `switch os.Args[1]` dispatches to `run*()` functions. The `--json` flag must be parsed inside each `run*()` since they all take `() error` signatures with no params.

### Output construction per subcommand
- **session-start**: `formatSessionStart()` at `main.go:~530` builds a `strings.Builder` markdown string; `fmt.Print(output)` at ~`main.go:315`. JSON shape: `{open_loops: [{topic, date, kind}], project_entries: [{topic, kind}], pending_count: int}`.
- **post-compact**: `formatPostCompact()` at `main.go:~560` same pattern. JSON shape: `{open_loops: [...], project_entries: [...]}`.
- **discover**: all output goes to `os.Stderr` (`main.go:810-822`); stdout is empty. JSON shape: `{entries_written: int, commits_analyzed: int, commits_skipped: int, packages_scanned: int}`.
- **extract**: all output goes to `os.Stderr` (`main.go:689, 705, 710`). JSON shape: `{entries_written: int, entries_failed: int}`.
- **suggest**: single `fmt.Printf` to stdout (`main.go:946-951`). JSON shape: `{topic: str, count: int, dir: str}` or `null`.
- **extract-audit**: `--json` flag ALREADY EXISTS at `extract_audit.go:140`. `writeAuditJSON()` at `extract_audit.go:~620` uses `json.NewEncoder(os.Stdout)` with indent. Shape: `{corpus, mode, transcripts: [...], totals: {...}}`.

### Shared helper candidate
A simple `printJSON(v interface{})` in `cmd/mastermind/main.go` (shared file) would work for all subcommands except `extract-audit` (which already has its own). All data is already in Go structs; just needs JSON tags. Could live near `formatSessionStart`.

### Prior art
`extract-audit` is the only subcommand with a working `--json` path. `writeAuditJSON()` at `extract_audit.go:~620` is the template to copy. The pattern: parse `--json` in the args loop, branch at the output site between `writeAuditTable()` and `writeAuditJSON()`.

## Status
Open — not started. Analysis complete 2026-04-10.

## Resolution

Shipped 2026-04-10 (duplicate of cli/add-json-flag-... open loop, closed together).
