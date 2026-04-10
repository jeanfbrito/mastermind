# /mm-extract — Extract Session Knowledge to Mastermind

Trigger: `/mm-extract`

When the user invokes `/mm-extract`, review the ENTIRE conversation and extract every piece of knowledge worth preserving into mastermind via `mm_write`.

## What to extract

Scan for ALL of these:

- **Bugs fixed**: What broke, why, how it was fixed. Kind: `war-story` or `lesson`
- **Non-obvious discoveries**: Things that weren't obvious until found. Kind: `lesson` or `insight`
- **Patterns discovered**: Reusable approaches that worked. Kind: `pattern`
- **Architectural decisions**: Choices made and WHY. Kind: `decision`
- **Unfinished work**: Things started but not completed, or "we should do X later." Kind: `open-loop`
- **Failed approaches**: What was tried and why it didn't work (so future sessions don't repeat it). Kind: `war-story`

## How to classify scope

- **user-personal**: General engineering lessons that apply across any project
- **project-shared**: Lessons specific to the current project's codebase, architecture, or tooling
- **project-personal**: Personal notes about this project (rare — prefer project-shared)

## How to classify category

Classify by the SUBJECT of the lesson, not the context you were in:
- Level 1: primary technology or domain (e.g., `go`, `electron`, `react`, `mcp`)
- Level 2: optional sub-topic (e.g., `go/modules`, `electron/ipc`)

## Rules

- Call `mm_write` once per distinct lesson. Do NOT bundle multiple lessons into one entry.
- Topic should be a clear one-line summary — future you searching for this needs to find it.
- Body should include: what happened, why it matters, and the actionable takeaway.
- Keep each entry body under 15 lines. Dense and useful, not verbose.
- Set `project` to the current project name (from git) for project-specific entries, or `general` for cross-project lessons.
- Err on the side of capturing MORE. It's cheaper to have an extra entry than to lose a lesson.
- Check `mm_search` first for related entries to avoid exact duplicates. Closely related but distinct lessons are fine as separate entries.
- After all writes, report a summary: how many entries written, titles, and scopes.
