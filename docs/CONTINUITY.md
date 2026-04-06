# Continuity layer — the load-bearing feature

This document describes the part of mastermind that matters most to its primary user (and, therefore, matters most full stop). Everything else is supporting infrastructure.

## The requirement, stated honestly

mastermind's primary user has ADHD. That is not a footnote or an accessibility consideration — it is the **load-bearing design constraint**. Any feature that requires neurotypical working memory to use is, for this user, a feature that doesn't work.

Specifically:

1. **You cannot afford to re-explain project context to every agent, every session.** The working-memory tax of "what was I working on, what have I told this assistant before, what's the state of things" is exactly the tax a second brain has to eliminate. If mastermind doesn't do this, nothing else it does matters.
2. **You cannot rely on session-end awareness to trigger capture.** Neurotypical workflows assume "at the end of a session I'll remember to write down what I learned." ADHD workflows don't — attention has already moved on by the time the session ends, and the insight is gone. Any capture mechanism gated on willpower at session end will silently produce an empty corpus.
3. **You cannot rely on remembering to check the tool.** A retrieval tool that requires you to remember to query it is just another thing to forget. The tool must surface what's relevant *without being asked*.
4. **You cannot afford review-queue guilt.** A pending queue that fills up, sits unreviewed, and generates shame is worse than no tool at all. It adds to the cognitive load it was supposed to reduce.

These constraints lead to five behaviors that are **not optional features** — they are the core of what mastermind is for. If any one of them isn't working, the tool has failed its primary user.

## The five behaviors

### 1. Session-start context injection (automatic, every time)

**What**: Every time Claude Code opens in a directory with a `.mm/` or under a known project, mastermind automatically runs a query with the project as input and injects the most relevant context into the agent's starting prompt — *before the user has typed a character*.

**Why**: The "re-explain context to every agent" tax disappears. You open a new session, the agent already knows:

- What this project is and what it's for (project-shared nodes).
- Your preferences and patterns relevant to this project (user-personal, tag-filtered).
- What you were in the middle of (project-personal open-loops).
- What you extracted but haven't reviewed yet (pending count, not content).

**How**: A Claude Code session-start hook runs a command like `mastermind session-start --cwd $PWD` which:

1. Detects the project via walk-up from `$PWD` looking for `.mm/config.json` (brv pattern, adapted).
2. Fans out queries to all three scopes, weighted by project relevance.
3. Returns a compact markdown block — 500-1000 tokens max — that Claude Code injects as system context.
4. Returns fast — under 200ms. If it can't, it returns nothing and logs a warning. Slow injection is worse than no injection.

**What gets injected**:

```
## From your mastermind

**Currently open loops in this project** (3):
- Finish auth refactor — you stopped mid-way last Tuesday because of the merge conflict
- Ask Alice about the Friday deploy failures
- Verify the macOS sync I/O fix holds under load

**Relevant lessons** (top 5 by project + tag match):
- macOS Electron IPC hangs when main process blocks on sync I/O
- Never skip hooks — always investigate why they're failing
- ...

**Pending review** (7 entries from the last 3 sessions)
Run `/mm-review` when ready.
```

The format is deliberately dense and scannable. The agent reads it on its own and the user sees it if they glance, but neither *has to act* on it for the session to proceed normally. **Silent unless needed** — if all three sections are empty, inject nothing.

### 2. Session-close extraction (automatic, no remembering required)

**What**: At the end of every Claude Code session, mastermind automatically runs the extraction pipeline on the session transcript. Candidate entries land in `<scope>/pending/`. No user action required.

**Why**: Capture cannot depend on the user remembering to trigger it at session end. By the time a session is over, the ADHD brain has already released the context. The only window where capture has high signal is *while the session is still loaded*. That window closes the moment Claude Code closes. Therefore the trigger must be the system, not the user.

**How**: A Claude Code session-close hook runs `mastermind session-close --transcript <path>` which:

1. Reads the transcript from wherever Claude Code stores it.
2. Sends it to an LLM with the extraction prompt (adapted from OpenViking — see REFERENCE-NOTES.md).
3. Writes each candidate to the appropriate `<scope>/pending/` directory.
4. Returns immediately — the extraction runs detached (two-phase commit pattern from OpenViking).
5. Logs to `~/.mm/logs/extraction.log` for debugging; silent otherwise.

**What the user sees**: nothing, at close. The next time they open Claude Code in any project, the session-start injection mentions "You have N pending entries from the last M sessions." That's the only signal.

**What the extractor captures** (five kinds — see FORMAT.md):

- `lesson` — "I learned X the hard way because Y."
- `insight` — "I realized X pattern solves Y class of problem."
- `war-story` — "Spent N hours on X, root cause was Y, fix was Z."
- `decision` — "Chose X over Y because of Z."
- `pattern` — "When I see shape-X, I try approach Y first."
- `open-loop` — "I was about to do X but stopped. Resume when..." (see next section)

**Critical**: the extractor writes to `pending/`, not to the live store. Ever. See the review flow below.

### 3. Open-loops as a first-class entry kind

**What**: A sixth `kind` value alongside `lesson`, `insight`, `war-story`, `decision`, `pattern`. Represents unfinished work or loose ends the user wanted to remember but will drop without help.

**Why**: This is the single most ADHD-specific feature. The "I was about to do X but got pulled into a meeting" loops are exactly the things a neurotypical brain holds loosely in working memory and an ADHD brain drops on the floor the moment attention shifts. Losing these loops is the #1 friction in working on big projects over weeks. mastermind captures them automatically, surfaces them proactively at session start, and moves them out of `open-loop` when they're resolved.

**How**:

- The extractor has explicit instructions (in the prompt) to watch for "user said they'd do X later" / "user wanted to come back to Y" / "user was in the middle of Z" patterns and propose them as `kind: open-loop`.
- Open-loops always land in `project-personal` scope by default — they're private to you and relevant to the current project, not team-shared knowledge.
- Session-start injection **always** shows all current open-loops for the active project, sorted by capture date, newest first.
- Open-loops expire differently than lessons. Default: 30 days after capture, an open-loop that hasn't been marked done gets an "is this still relevant?" prompt at session start (once, not repeatedly). If ignored, it auto-archives.
- A new tool: `mm_close_loop(loop_id, resolution)` for the agent to mark loops done when the user says things like "okay, I finished that auth refactor." The agent calls it, mastermind moves the open-loop to a resolved state in `<scope>/resolved-loops/` for history, and it stops appearing in session-start injection.

**Prompt instruction to the extractor** (to be added to the extraction prompt):

> Also scan the conversation for **open loops** — things the user said they would do later, or was in the middle of when the conversation ended. Examples: "I'll come back to this tomorrow," "let me finish this later," "I should ask Alice about X," "after the deploy I'll refactor Y." Capture these as `kind: open-loop` with `scope: project-personal`. Be generous — a false positive is cheap (you delete it), a false negative is expensive (the user forgets and re-does work).

### 4. Review without guilt

**What**: A review flow designed so it cannot become a to-do list hell that the user avoids.

**Why**: ADHD + pending queues is a well-known failure mode. The queue fills up, guilt accumulates, the user stops opening the queue, and the whole system becomes a reminder of the thing they're avoiding. mastermind must prevent this by design, not by discipline.

**The rules**:

1. **Pending entries are kept indefinitely.** The queue is patient. Old entries are not shameful — they're waiting for a good day. The agent can help review them. Knowledge is never silently deleted. Optionally, a configurable auto-promote policy moves old candidates to the live store after N days (default: off). See DECISIONS.md "Reverse auto-expire" for the reasoning.
2. **User-initiated writes bypass pending entirely.** When the user tells the agent to capture something (via `mm_write`), the entry goes directly to the live store. The user IS the review — they're present, they can see what the agent is writing, they chose to create it. Pending exists only for auto-captured knowledge (session-close extraction) where the user wasn't consciously involved.
3. **One entry at a time, never a list.** When `/mm-review` runs, it shows exactly one candidate with three choices: keep, reject, edit. Lists trigger decision paralysis; single items don't.
4. **Keyboard-driven, five seconds per entry.** `k` to keep, `x` to reject, `e` to edit, `n` to skip to next. No mouse, no ceremony, no "are you sure?" dialogs.
5. **Default-accept for open-loops.** Open-loops are valuable if right and cheap to delete if wrong. The review flow auto-accepts them unless the user explicitly rejects.
6. **No counters, no streaks, no gamification.** Those are retention hooks for people with attention budgets. For ADHD they become another source of guilt when the streak breaks.
7. **Silent when the queue is empty.** Session-start injection shows pending count only when nonzero. If zero, it shows nothing about pending.
8. **No reminders. No notifications. No badges.** The tool is always silent unless the user asks.

**The review flow**:

```
$ /mm-review

[1/7] user-personal · lesson · confidence: high · captured 2d ago

## macOS Electron IPC hangs when main process blocks on sync I/O

## What happened
[...content...]

## Lesson
Never do sync I/O in the Electron main process.

[k]eep  [x] reject  [e]dit  [s]kip  [q]uit
>
```

No dashboard. No progress bar (it's in the header: `[1/7]`). No "are you sure?" No "you still have 6 more to review!" Just: one candidate, three choices, next.

### 5. Silent unless needed

**What**: The tool produces output only when there's something the user needs to see. Zero output otherwise.

**Why**: Working memory for ADHD is not a renewable resource that regenerates on a schedule; it's a deeply variable and often-low resource. Every notification, every badge, every "did you know mastermind has X new feature" is a working-memory tax. The default state of the tool must be *invisible*.

**Specific rules**:

- **No dashboards** that you have to remember to check.
- **No stats commands** by default. If they exist, they're opt-in and nobody nags you to look at them.
- **No notifications** of any kind (OS, terminal bell, etc.).
- **No reminders** to extract, review, archive, or capture.
- **No startup banner** when the binary runs. It runs and goes to work. If it needs to say something, it says it once, quietly.
- **No "X days since your last lesson"** anti-patterns. The tool is indifferent to frequency.
- **Session-start injection shows only non-empty sections.** Zero pending? Don't mention pending. Zero open-loops? Don't mention open-loops. Don't even announce "mastermind is active" — presence is its own signal.

The only thing mastermind *ever* says unprompted is the session-start injection block, and that block contains only material the user needs for the current session. Everything else is on-demand.

## The silent-unless-needed test

For every feature, every slash command, every piece of output mastermind produces, the question is:

> **Does this work on a day when the user's working memory is at its worst, or does it require a good day to use?**

If it requires a good day, it's the wrong design. Full stop.

This is stricter than "is this usable" — many tools are usable on good days and invisible-blockers on bad days. mastermind is built to be used *most* on bad days, because bad days are when the working-memory tax is highest and the value of an external brain is greatest.

## What this means for the code

Concretely, the five behaviors require these additions to the existing Phase 1-5 plan:

1. **A `session-start` subcommand** (not an MCP tool — a CLI command invoked by a Claude Code hook). Outputs the context injection block on stdout. Fast, <200ms.
2. **A `session-close` subcommand** (not an MCP tool — a CLI command invoked by a Claude Code hook). Reads the transcript path, forks the extraction pipeline, returns immediately.
3. **An extraction subsystem** that can run detached, invoke an LLM, and write candidates to `pending/`. Phase 3 territory.
4. **`kind: open-loop`** added to FORMAT.md as a sixth enum value.
5. **An `mm_close_loop` MCP tool** so the agent can resolve loops as the user works.
6. **A 7-day auto-expire on `pending/`** — a simple startup pass that deletes stale candidates. No nag, no log line.
7. **An `/mm-review` slash command** that implements the one-entry-at-a-time keyboard flow. Terminal UI, not a TUI framework — just raw stdin reads.
8. **Claude Code hook installation instructions** in README — how to wire session-start and session-close into the user's Claude Code config. This is a one-time setup cost.

These additions push Phase 3 (capture) from 2 days to 3-4 days of work. The increased scope is load-bearing — without it, mastermind is a generic knowledge base and the specific user it's built for cannot use it.

## Non-negotiables

Things that cannot be changed later without violating the continuity layer's purpose:

1. **Extraction is always automatic and always produces `pending/` entries. Never direct-write.**
2. **Review is always one-at-a-time, never a list.**
3. **Pending entries always auto-expire after 7 days. Never nag the user about them.**
4. **Session-start injection is always silent when there's nothing relevant.**
5. **Open-loops are always surfaced at session start without being asked for.**
6. **No notifications, no reminders, no badges, ever.**
7. **The default state of the tool is invisible.**

If any of these seven ever start to erode — "just this one notification, it's useful" — the whole design collapses for its primary user. The discipline to keep the tool silent is the discipline to keep it useful.
