---
date: "2026-08-09"
project: mastermind
tags:
  - reference
  - openwolf
  - hooks
  - token-budget
  - multi-agent
  - transcript-usage
topic: openwolf — seven-hook context-discipline suite with token ledger + symbol hints
kind: insight
scope: project-shared
category: references
confidence: high
---

## What

openwolf (https://github.com/cytostack/openwolf, local clone `~/Github/openwolf`, TypeScript) is a context-management and scaffolding system for AI coding agents. It installs a `.wolf/` directory per project holding markdown (memory.md, cerebrum.md, STATUS.md, anatomy.md) plus JSON stores (anatomy-index.json, token-ledger.json, buglog.json), and wires **seven lifecycle hooks** (SessionStart, PreToolUse:Read, PreToolUse:Write, PostToolUse:Read, PostToolUse:Write, PreCompact, Stop) into up to six agents (Claude Code, Codex, OpenCode full; Cursor, Gemini CLI, Antigravity beta/context-only). Its center of gravity is **token discipline**, not memory: per-file descriptions + token estimates ("anatomy"), symbol-level line-range hints so agents read slices instead of whole files, repeated-read warnings, and a token ledger comparing estimates against real usage parsed from transcripts.

**NOT a design reference for mastermind's memory model** — it is mostly a philosophical contrast, like mempalace but on the discipline axis. Its anatomy/symbol-hint layer is context-mode/GitNexus territory (a domain mastermind explicitly stays out of), and several of its behaviors are mastermind anti-patterns.

## Confirms mastermind's design (independently converged)

- **Post-compaction re-injection**: openwolf's PreCompact snapshot → SessionStart(trigger=compact) digest re-inject is the same shape as mastermind's `post-compact` subcommand (cmd/mastermind/main.go). Already covered.
- **STATUS.md "next phase" as top-priority resume context** ≈ open-loops as first-class kind injected first.
- **Per-hook token budget on the session digest** (digestBudget, default 1500) ≈ mastermind's L0/L1 soft budgets — except openwolf's greedy `tryAdd` **silently skips** oversized sections, which is worse than mastermind's warn-without-truncation. Keep ours.

## Anti-patterns (per NON-GOALS / hard rules — do not copy)

- **Stop-hook nagging**: `checkSemanticSummaries` prints "ACTION REQUIRED: append a one-line summary…" when ≥2 files changed but no memory.md entry exists. That is a guilt machine — the exact opposite of guilt-free review. Mastermind auto-extracts instead of nagging the user to write summaries by hand.
- **Dashboard + daemon + cron + AISuggestions panel**: violates "invisible by default."
- **Anatomy / symbol-extractor / pre-read hints / repeated-read tracking**: solves context-window discipline, not memory continuity. Jean already has context-mode + GitNexus for that; folding it into mastermind would be scope creep.

## Worth taking (small, ranked)

### 1. Real token usage from transcripts (best idea)
openwolf's stop hook accepts `transcript_path` in the hook input JSON, parses the harness transcript, and records **real** input/output/cache tokens next to its estimates (`readTranscriptUsage()` in src/hooks/shared.ts; attached as `real_usage` on the session entry). Mastermind's L0/L1 soft-budget warnings rest entirely on a chars-per-token estimate. Teaching `mastermind stop` (which already exists as a telemetry subcommand) to parse `transcript_path` and log estimated-vs-real ratio would ground-truth the budget heuristic with zero user-visible surface — evidence for the "budget drift" check in the planned `mastermind lint`.

### 2. Install/wiring health check → fold into `mastermind lint`
scripts/openwolf-check.mjs is a zero-dep diagnostic: which agents are wired, hook generation version (detects stale hook scripts after upgrades), staleness (mtime) of state files. The mastermind-lint open loop (from OpenKB) covers *corpus* health; this adds the *wiring* dimension: are the hooks actually registered in `.claude/settings.json`, does the installed binary version match the hook commands, when did extraction last actually run. Cheap, and diagnoses the silent-failure mode where hooks quietly stop firing.

### 3. Multi-agent adapter pattern (file away for later)
openwolf keeps hook scripts provider-agnostic and uses thin per-agent installers (src/agents/codex.ts `buildCodexHooks`) plus a per-agent budget map in config.json (`context.budgets[agent]`). If mastermind ever supports Codex/OpenCode, this is the shape: same subcommands, per-agent install adapters. Not now.

### 4. Kind-aware salience in PostToolUse suggest (marginal)
openwolf's pre-write hook scans its "Do-Not-Repeat" section for matches against the pending edit and warns. Mastermind's PostToolUse suggest already surfaces the most relevant entry on file touch; the delta would be weighting gotcha/mistake-kind entries higher when the tool is Edit/Write vs Read. Only if dogfooding shows suggest missing important warnings.

### Concurrency footnote
openwolf serializes concurrent hook writes to anatomy-index.json with a lock (anatomy-lock.ts, skip-on-contention). Mastermind's `access.json` sidecar can race between the MCP server and hook processes — atomic writes mean worst case is a lost access increment, which is harmless at current scale. No action; noted in case access telemetry ever matters more.
