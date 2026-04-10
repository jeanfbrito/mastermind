---
date: "2026-04-10"
project: mastermind
tags:
  - hooks
  - stop
  - telemetry
  - shiba-memory
  - open-loop
topic: Minimal Stop hook that logs session usage telemetry to .knowledge/logs/sessions.jsonl
kind: open-loop
scope: project-shared
category: hooks
confidence: high
---

## What's open

Add a `mastermind stop` subcommand that reads Claude Code Stop-hook JSON from stdin and appends a single JSONL line to `~/.knowledge/logs/sessions.jsonl`:

```json
{"timestamp":"2026-04-10T14:30:00Z","session_id":"...","stop_reason":"end_turn","message_count":12,"input_tokens":8423,"output_tokens":1205}
```

Registered in settings.json as:

```json
"Stop": [{
  "matcher": "",
  "hooks": [{"type": "command", "command": "mastermind stop", "timeout": 5}]
}]
```

## Why

This is the reduced-scope deliverable that falls out of the 2026-04-10 mining-update findings on the bigger Stop-hook open loop. The original "auto-capture unresolved open-loops from response text" idea is blocked: Claude Code's Stop hook stdin contains only metadata (`session_id, stop_reason, message_count, input_tokens, output_tokens`) — no response body. Without the assistant's last message, there is nothing to parse for "resolution phrases".

What IS tractable is the part of shiba-memory's Stop hook that doesn't need the response text: turn-count-gated session telemetry logging. Shiba does this too (`~/Github/shiba-memory/cli/src/hooks/stop.ts:~30`). It gives mastermind its first usage-telemetry surface — "how often do I use Claude Code", "which sessions burned the most tokens", etc. — without touching extraction.

Much smaller than the original open loop. Isolated. Ships in one commit.

## Next action

1. Add `case "stop":` dispatch in `cmd/mastermind/main.go:58-109`.
2. Implement `runStop()` that: reads JSON from stdin, parses `{session_id, stop_reason, message_count, input_tokens, output_tokens}`, appends one line to `~/.knowledge/logs/sessions.jsonl` (create dir if missing), exits silently.
3. Respect the `MASTERMIND_NO_AUTO_INIT` env var — if set, don't create `.knowledge/logs/`.
4. Turn-count gate: if `message_count < 4`, still log but add a `"short":true` flag so future analysis can filter out short sessions.
5. Add a test that feeds a sample Stop JSON into stdin and verifies the JSONL line lands correctly.
6. Update help text + CONTINUITY.md to list the fifth hook.

## Non-goal

- NO LLM calls (hard 5s timeout)
- NO extraction (blocked on data availability)
- NO session-resumption logic (log-only)
- NO auto-capture of open-loops (original idea, now blocked)

## Source

`~/Github/shiba-memory/cli/src/hooks/stop.ts`; mining report 2026-04-10 (Agent a50b8964 hook patterns); the parent open loop `phase-5-experiment-stop-hook-auto-captures-low-confidence-open-loops...` documents why the original framing doesn't work.
