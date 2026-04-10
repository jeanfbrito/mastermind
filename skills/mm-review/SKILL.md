# /mm-review — Review Pending Knowledge Entries

Review auto-extracted entries from mastermind's pending queue. One entry at a time, five seconds each, no guilt.

## When to use

- After PreCompact extraction has generated pending candidates
- When session-start injection shows a pending count > 0
- When the user says `/mm-review`, "review pending", "check pending entries", etc.

## Workflow

### 1. Find all pending entries

Glob for pending entries across all three scope roots:

```
~/.knowledge/pending/*.md           (user-personal)
.knowledge/pending/*.md             (project-shared, from git root)
~/.claude/projects/*/memory/pending/*.md  (project-personal)
```

If no pending entries exist across any scope, say "No pending entries to review" and stop.

Otherwise, report the count: "Found N pending entries. Let's go through them."

### 2. Present one entry at a time

For each pending entry file:

1. **Read the file** to get its frontmatter and body
2. **Present it clearly** in this format:

```
**[1/N]** `scope` · `kind` · `project`

**Topic**: <topic from frontmatter>

<body content, full — these are short>

---
**promote** (p) · **skip** (s) · **reject** (r) · **edit** (e) · **quit** (q)
```

3. **Wait for user input** using AskUserQuestion. Accept:
   - **p / promote / yes / y / k** → Call `mm_promote` with `pending_path` set to the absolute file path. Report the live path. Move to next.
   - **s / skip / n / next** → Move to next entry without action.
   - **r / reject / x / delete** → Delete the file (`rm`). It's noise — gone forever. Move to next.
   - **e / edit** → Ask what to change. Edit the file (frontmatter or body). Re-present the entry for another decision.
   - **q / quit / done** → Stop reviewing. Report summary and exit.

### 3. After all entries (or quit)

Report a summary:

```
Review complete: N promoted, N skipped, N rejected (M remaining)
```

## Rules

- **One at a time.** Never show a list of all pending entries. Never ask "which one do you want to review?" Present the next one automatically.
- **No guilt.** Don't mention how old entries are, how long the queue has been waiting, or how many sessions have passed. The queue is patient.
- **No judgment on reject.** Rejecting an auto-extracted entry is normal — the extractor is imperfect. Don't ask "are you sure?"
- **Default order**: Process files in chronological order (oldest first — the timestamp prefix in filenames handles this naturally with glob sorting).
- **Promote calls mm_promote**, which moves the file from `pending/` to the live topic directory and strips the timestamp prefix. This is the only way entries leave pending for live.
- **After promoting, remind about git** (once, at the end): "Promoted entries in .knowledge/ — `git add .knowledge/` when ready to commit."
- **If mm_promote returns ErrEntryExists**: Tell the user a live entry with the same slug already exists. Offer to reject (duplicate) or edit (change topic to differentiate).
