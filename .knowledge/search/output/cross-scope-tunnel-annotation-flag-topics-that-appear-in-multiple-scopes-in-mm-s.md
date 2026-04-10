---
date: "2026-04-10"
project: mastermind
tags:
  - search
  - output
  - formatting
  - mempalace
  - cosmetic
  - open-loop
topic: Cross-scope "tunnel" annotation — flag topics that appear in multiple scopes in mm_search output
kind: open-loop
scope: project-shared
category: search/output
confidence: high
---

## What's open

When `mm_search` returns results spanning multiple scopes, annotate entries whose topic string appears across more than one scope with a `[cross-scope]` tag in the output markdown. Purely cosmetic — helps the reader recognize "this lesson applies everywhere, not just one project".

```markdown
### [user-personal] Go module tidy quirks · lesson · 2026-04-05 [cross-scope: also in project-shared]
```

## Why

Mempalace's `palace_graph.find_tunnels()` (`~/Github/mempalace/palace_graph.py:161`) defines a "tunnel" as a room name (topic) that appears in two or more wings. These are the conceptual bridges between domains — topics worth flagging because they cut across context. Mempalace uses them for navigation; mastermind can use them as a retrieval-time annotation.

Zero infrastructure: mastermind already groups results by scope in the output. One extra pass over result topics, `map[string][]scope`, flag duplicates. ~15 lines in `internal/search/format.go` (the markdown formatter).

## Next action

1. Read `internal/search/format.go` to find where per-result markdown is assembled.
2. After all results are collected but before formatting: build `topicToScopes := map[string]map[string]bool{}` over the results.
3. For each result, if `len(topicToScopes[result.Topic]) > 1`, append `[cross-scope: also in %s]` to the result header where `%s` is the other scope(s) excluding the current one.
4. No change to the sort or class ordering — purely cosmetic.
5. Test: three entries with topic "Go modules" in user-personal + project-shared + project-personal all get the annotation.

## Non-goal

No traversal tool, no separate MCP call, no graph structure. The "tunnel" concept in mempalace requires a graph walk because their storage is opaque vector metadata; mastermind's filesystem makes topic-name matching trivial at query time.

## Source

`~/Github/mempalace/palace_graph.py:161` `find_tunnels`; mining report 2026-04-10 (Agent a06eeb60 knowledge-graph relations).
