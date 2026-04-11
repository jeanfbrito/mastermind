---
date: "2026-04-10"
project: mastermind
tags:
  - phase-5
  - hooks
  - stop
  - open-loops
  - shiba-memory
  - experiment
  - adhd
topic: 'Phase 5+ experiment: Stop-hook auto-captures low-confidence open-loops when response ends without resolution'
kind: open-loop
scope: project-shared
category: hooks
confidence: high
accessed: 2
last_accessed: "2026-04-11"
---

## What's open
Shiba-memory ships a Stop hook that fires when Claude Code's response finishes. Their version updates session records and cleans episodes. An experiment for mastermind: use the Stop hook to auto-create low-confidence open-loop entries when a response ends without explicit resolution ("done", "fixed", "shipped", "closed", etc.).

## Why it matters
Today mastermind's open-loops are only created via explicit `mm_write` — requires the user or agent to remember. An automated Stop-hook trigger would populate open-loops without any user action, which is the holy grail for ADHD-friendly capture: zero willpower cost, silent, works on bad-memory days. High false-positive tolerance is acceptable — low confidence, high recall.

## Why parked at Phase 5+
Needs three supporting pieces that don't exist yet:
1. A **confidence threshold** so low-confidence auto-captures rank below user-created open-loops at session-start injection (so the injection doesn't drown in noise).
2. An **auto-close mechanism** for open-loops that turn out to be false positives. Likely benefits from landing after the `supersedes`/`contradicts` schema so newer entries can automatically close older loops they resolve.
3. A **resolution-phrase detector** (regex first, LLM if insufficient) so the hook only fires when the response genuinely ended without resolution.

## Open question
How does the Stop hook know what the unresolved task is? It sees the tail of the conversation but not the full working state. Probably needs to pair with the PostToolUse-based episodic capture (another shiba idea) so there's enough state to summarize.

## Source
`docs/reference-notes/shiba-memory.md` §1 (Stop row in hook table) and §8 item 6.

## 2026-04-10 — Mining pass update (critical constraint found)

Dug into shiba-memory's actual Stop hook implementation (`~/Github/shiba-memory/cli/src/hooks/stop.ts`). Two important findings, one of them a genuine blocker for the original design.

**BLOCKER — Stop hook has no access to response text**. The stdin schema shiba receives is `{session_id, stop_reason, message_count, input_tokens, output_tokens}`. That's it. No assistant message, no tool output, no conversation tail. Shiba's own Stop hook does NOT attempt "unresolved-task detection" because it can't — the data simply isn't available from Claude Code's Stop hook surface. Whatever "resolution-phrase detector" this open loop originally envisioned is impossible under Claude Code's current hook contract.

This kills the original "auto-capture low-confidence open-loops when response ends without resolution" framing. Without the response text, there is nothing to parse for resolution phrases. Revisit only if a future Claude Code version exposes the response body to Stop hooks.

**Still tractable — turn-count-gated lightweight checks**. Shiba's Stop hook does four cheap things that DON'T require response text:
1. Token accounting: append session tokens to a log. Mastermind equivalent: append to `~/.knowledge/logs/sessions.jsonl` (one line per session). Useful for the "how much did I use mastermind this week" question.
2. Turn-count gate: skip everything if `message_count < 4` — short clarification turns don't warrant extraction work.
3. Cheap consolidation: regenerate any stale session-cache files. Mastermind has no equivalent today.
4. Episode pruning: drop old low-value entries. Mastermind deliberately doesn't prune (hard rule #7).

Of those four, only #1 and #2 are load-bearing for mastermind. A minimal Stop hook could: log session counts to `~/.knowledge/logs/sessions.jsonl`, skip the rest if turn count < 4. Zero LLM, zero file reads, <10ms.

**Shiba's timing caveat**: `timeout: 5` in settings.json means the hook must complete within 5 seconds or Claude Code kills it. Shiba's LLM-based Stop extraction frequently hits that ceiling in loaded systems. Mastermind's Go binary doing pure file I/O would not — budget remains forgiving.

**PostCompact cross-check**: mastermind already has a `post-compact` subcommand (`cmd/mastermind/main.go:479`). Shiba's PostCompact annotates its injected output with `reason="post-compact"` to distinguish it from SessionStart injection. Mastermind's `formatPostCompact` uses the distinct "## mastermind (post-compact)" header — already distinguishable. No change needed.

**Updated next action**:
1. Accept the blocker: "auto-capture unresolved open-loops from response text" is not possible under current Claude Code Stop hook contract. Document this in NON-GOALS.md under "Things that sound possible but aren't".
2. **Still worth building**: a minimal Stop hook subcommand that logs session counts to `~/.knowledge/logs/sessions.jsonl` and exits. Gives mastermind its first usage-telemetry surface without touching the extraction path. Much smaller scope than the original loop — separate open loop worth filing.
3. Revisit the auto-capture framing only if Claude Code exposes response body to Stop hooks in a future version, OR if mastermind adopts an inline PostToolUse capture pattern (shiba does this too — PostToolUse gets `tool_output` and can observe the assistant's running state).

References: `~/Github/shiba-memory/cli/src/hooks/stop.ts`, `common.ts` (StopEvent interface), mining report 2026-04-10 (Agent a50b8964 — Stop/compaction hooks).
