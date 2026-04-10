---
date: "2026-04-10"
project: mastermind
tags:
  - extraction
  - llm
  - prompt
  - openviking
  - temporal
  - open-loop
topic: Inject session timestamp into LLM extraction prompt for temporal grounding
kind: open-loop
scope: project-shared
category: extraction
confidence: high
---

## What's open

Prepend a session-time header to the LLM extraction prompt in `internal/extract/llm.go` so the model can date-stamp open-loops and events correctly:

```go
// In callAnthropic/callOpenAI/callOllama, before appending the transcript:
header := fmt.Sprintf("Session time: %s\n\n", time.Now().Format("2006-01-02 (Monday)"))
prompt := header + existingPrompt + transcript
```

## Why

OpenViking injects `**Session Time:** 2026-04-10 14:30 (Thursday)` as the first line of the user message in its end-of-session extraction prompt (see `~/Github/openviking/session/memory/session_extract_context_provider.py` line ~120, captured in `docs/reference-notes/openviking.md`). Without this, the LLM sees relative temporal references like "tomorrow", "next sprint", "by end of month" with no anchor — extracted open-loops and event entries end up with the wrong `date` field or an invented one.

Three characters of Go template; measurable quality win for any extraction that produces date-sensitive entries. Cost: zero tokens of meaningful budget (~15 chars).

## Next action

1. Read `internal/extract/llm.go` to find the prompt assembly point (likely a `const extractionPrompt` + per-provider `call*` functions).
2. Add a `sessionTimeHeader()` helper that returns today's date in a consistent format.
3. Prepend to the transcript in each provider call, OR inject once in the shared `Extract()` entry point.
4. Update the prompt description to mention that the header exists, so future prompt edits don't strip it.
5. Test via `extract-audit` on the labeled corpus — verify date accuracy on open-loop entries improves.

## Source

`docs/reference-notes/openviking.md`, OpenViking `session/memory/session_extract_context_provider.py:~120`, mining report 2026-04-10 (Agent a143a0da extraction patterns).

## Resolution

Shipped 2026-04-10. LLMExtractor.Extract now prepends "Session time: YYYY-MM-DD (Weekday)\n\n" to the transcript before the provider switch, so all three providers (Anthropic, Ollama, OpenAI) pick it up automatically. Wall-clock source is indirected via a sessionNow package var for deterministic testing. 2 new tests verify format and real-clock behavior.
