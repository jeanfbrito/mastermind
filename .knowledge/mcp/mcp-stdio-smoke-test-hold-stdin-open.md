---
date: "2026-04-04"
project: mastermind
tags:
  - mcp
  - stdio
  - testing
  - smoke-test
  - go-sdk
topic: MCP stdio smoke tests must hold stdin open past response flush
kind: pattern
scope: project-shared
confidence: high
accessed: 2
last_accessed: "2026-04-10"
---

# MCP stdio smoke tests must hold stdin open past response flush

## What happened
While validating Phase 2 end-to-end, piped four JSON-RPC messages
(`initialize`, `notifications/initialized`, `tools/list`, `tools/call`)
into `mastermind mcp` via a shell heredoc. The server logged
`server is closing: EOF` on stderr and produced zero bytes of stdout —
no initialize response, no tools list, no search result. The requests
were syntactically valid and matched working request shapes.

## Why
The go-sdk's `StdioTransport` treats stdin EOF as a shutdown signal
and exits the run loop immediately. With a heredoc, the kernel closes
stdin the instant the last line is delivered, and the server can race
to tear down before its response-writing goroutines flush to stdout.
The requests were received and processed; the responses were discarded
on the way out because the transport had already torn down its writer.

## How I found it
First attempt: `cat <<EOF | mastermind mcp` → zero stdout, stderr
complained about EOF. Tried adding `2>&1` to confirm nothing was
hiding on stderr — nothing there. Added a two-second grace period:
`( cat <<EOF; sleep 2 ) | mastermind mcp`. All four responses landed
cleanly on the second attempt. The fix is in the test driver, not in
the server code — closing stdin too fast is the caller's bug.

## Lesson
Any shell-level MCP smoke test against mastermind's stdio server MUST
keep stdin open long enough for the response goroutines to flush
after the last request. The reliable shell pattern is
`( requests; sleep 2 ) | mastermind mcp`. For Go-native tests, drive
the server with `exec.Cmd` and close stdin only after reading every
expected response line. Never rely on the heredoc race.

## When this matters again
Any new shell-driven smoke test against the MCP server. Any CI job
that scripts `mastermind mcp` from bash or make. Any debugging
session where the server "runs" but no output ever appears — first
check is always whether the caller is closing stdin before the
server has flushed, not whether the request JSON is malformed.
