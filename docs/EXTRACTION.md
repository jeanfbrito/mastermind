# Capture & extraction

Capture is the hard problem. Retrieval is easy; second brains fail because the corpus never gets written. mastermind's answer is **automatic session-close extraction into a mandatory pending/ review queue**, with an opt-in manual fallback.

**Read CONTINUITY.md before this document.** That file explains *why* the capture path looks the way it does. This file explains *how*.

## Principles

1. **Never auto-write to the live store.** Every entry passes through `pending/` and a human review step. No exceptions. OpenViking auto-commits; mastermind deliberately does not. See CONTINUITY.md on why.
2. **Extraction is automatic, not willpower-based.** The trigger is a Claude Code session-close hook, not a slash command the user has to remember. The slash command exists as a fallback for ad-hoc extraction, not as the primary path.
3. **Two-phase commit** (pattern stolen from OpenViking). Phase 1 returns immediately (fast archive of the transcript path). Phase 2 runs the extraction pipeline detached. The user never waits for extraction.
4. **Review is the consolidation.** Rereading a proposed entry while it's fresh is when the lesson actually lands in your head. The review flow is one-at-a-time, keyboard-driven, five seconds per entry — see CONTINUITY.md for the rules.
5. **Propose, don't decide.** The extractor proposes scope, kind, tags. You are the editor. Everything is overrideable in review.
6. **Conservative by default.** The prompt biases toward missing a lesson over inventing one. Extracted junk erodes trust faster than missed signal. An empty `pending/` is better than a noisy one.

## The primary capture path: session-close hook

This is the default, and the only path most sessions will use.

### Trigger

A Claude Code session-close hook runs `mastermind session-close --transcript <path>`. The user does not invoke this directly, ever. It fires automatically when Claude Code closes, every time, without exception.

Setup is a one-time cost: adding a hook line to the user's Claude Code config (`~/.claude/settings.json` or similar — documented in README under "Installation"). After that, it runs forever, invisibly.

### Phase 1: fast archive

`mastermind session-close` receives the transcript path. It does three things and returns in <100ms:

1. Validate the transcript file exists and is readable.
2. Copy it to `~/.knowledge/sessions/<timestamp>-<session-id>/transcript.json` for later reference and auditability.
3. Fork a detached subprocess for Phase 2 and exit successfully.

The user's terminal gets control back instantly. Nothing visible happens. The session ended, that's it.

### Phase 2: the extraction pipeline (detached)

The forked subprocess runs the full extraction. Taking as long as it needs is fine because the user isn't waiting.

1. **Load the transcript** from the archived location.
2. **Assemble the conversation** with timestamps and language detection (pattern from OpenViking — see REFERENCE-NOTES.md).
3. **Send to an LLM** via the Claude API with the extraction prompt (see prompt sketch below). The LLM is called directly, not through Claude Code — we're outside the session now. Credentials come from `ANTHROPIC_API_KEY` or `~/.knowledge/config.json`.
4. **Parse the response** into a list of candidate entries. Each candidate has: `scope`, `kind`, `topic`, body sections, proposed `confidence`, and tags.
5. **Validate each candidate** against the FORMAT.md schema. Reject malformed ones silently (log to `~/.knowledge/logs/extraction.log`).
6. **Write each valid candidate** to the appropriate `<scope>/pending/` directory as a markdown file with full frontmatter.
7. **Log telemetry**: phase timing, candidate count, validation reject count, final written count. Written to `~/.knowledge/logs/extraction.log`, never to stdout.
8. **Exit.**

The user sees nothing. The next time they start a Claude Code session, the session-start hook (see CONTINUITY.md) surfaces "Pending review: N entries from the last M sessions" in the injected context. That's the only signal.

### The extraction prompt

Versioned at `prompts/extract.md` in the mastermind repo. Changes are commits. This is the single most important artifact in Phase 3 — prompt quality is the difference between a corpus that compounds and a corpus that rots.

Sketch (the real version will be iterated during Phase 3 dogfooding):

```
You are a memory extraction agent working on behalf of a software engineer
with ADHD. Your job: scan this work session transcript and identify things
worth preserving in their personal engineering journal.

## Be conservative
Prefer missing a fact over inventing one. A false positive costs the user a
review click; a false negative costs them the lesson forever. When in doubt,
don't extract.

## What to extract

For each extracted entry, output a JSON object with these fields:

- scope: "user-personal" | "project-shared" | "project-personal"
- kind: "lesson" | "insight" | "war-story" | "decision" | "pattern" | "open-loop"
- topic: one-line human summary
- tags: free-form lowercase strings
- confidence: "high" | "medium" | "low"
- body: { "what_happened": "...", "why": "...", "how_i_found_it": "...",
          "lesson": "...", "when_this_matters_again": "..." }

### lesson — "I learned X the hard way because Y"
The highest-value entries. Something concrete the user learned that would
save time if remembered later.

### insight — "I realized X pattern solves Y class of problem"
Cross-cutting mental models. Often cross-project.

### war-story — "Spent N hours on X, root cause was Y, fix was Z"
Specific debugging journeys with clear root causes. Scope: where they
happened.

### decision — "Chose X over Y because of Z"
Architectural or technical decisions with a documented rationale. The *why*
is what gets forgotten.

### pattern — "When I see shape-X, I try approach Y first"
Reusable heuristics demonstrated in this session.

### open-loop — "I was about to do X but stopped. Resume when..."
CRITICAL for this user. Scan for: "I'll come back to this tomorrow,"
"let me finish this later," "I should ask Alice about X," "after the
deploy I'll refactor Y," "remind me to...", or any point where the user
clearly intended to continue a thread but the conversation ended first.
Be generous here — false positives are cheap (they auto-expire in 30 days),
false negatives are expensive (work lost, re-done later).

Open-loops default to scope: "project-personal" unless the user explicitly
said it belongs somewhere else.

## Scope heuristics

- Mentions a specific codebase, product, or team concern → project-shared
  (if others on the team would benefit) or project-personal (if it's
  private-to-user).
- Mentions a general engineering principle, language feature, platform
  constraint → user-personal.
- Private, personal, or sensitive content → project-personal (flag it).
- When in doubt → project-shared. It's the safest default for project work.

## What NOT to extract

- Summaries of what was done. Only what was learned.
- Code snippets longer than a few lines. Only the *why*.
- Facts the user could look up in docs in under 5 minutes.
- Transient task state or TODOs (unless captured as open-loops per above).
- Things that would embarrass the user if they were in the corpus. Err
  toward discretion.

## Output format

Return a JSON array of extracted entries. If nothing is worth extracting,
return an empty array. Do NOT include any text outside the JSON.

## The session transcript

{timestamp}
{language}

{transcript}
```

Several things to note about the prompt:

- It explicitly names ADHD and the cost asymmetry around open-loops. The model should lean in on open-loop detection even at the cost of false positives.
- It defines six kinds (the five from FORMAT.md plus `open-loop`).
- It proposes scope per entry, but ties scope to heuristics the user can understand and override.
- It asks for JSON output (no markdown fences, no prose). This keeps parsing robust.
- Timestamp grounding is baked in (pattern from OpenViking — prevents date-off-by-one errors).

### Writing to pending/

Each valid candidate becomes a markdown file at `<scope>/pending/<timestamp>-<slug>.md`. The `<slug>` is kebab-case from the topic. The file looks exactly like a finished entry — same format, same frontmatter, same body structure — so review is just "read and decide." No format conversion during promote.

Example output file at `~/.knowledge/pending/2026-04-04-143022-macos-electron-ipc.md`:

```markdown
---
date: 2026-04-04
project: Rocket.Chat.Electron
tags: [electron, ipc, macos, debugging]
topic: "macOS Electron IPC hangs when main process blocks on sync I/O"
kind: lesson
confidence: high
---

# macOS Electron IPC hangs when main process blocks on sync I/O

## What happened
[...]
```

### Extraction failure modes and handling

- **Transcript unreadable**: log, skip Phase 2, user notices nothing.
- **LLM API down**: log, retry once with backoff, then give up. Leave the transcript in `~/.knowledge/sessions/` for manual re-processing.
- **Malformed LLM response**: log, write the raw response to `~/.knowledge/logs/failed-extractions/`, skip.
- **Candidate fails format validation**: log, skip that one candidate, keep the rest.
- **No candidates at all**: that's fine, not a failure. Some sessions legitimately have nothing to extract.

The rule: **never break the user's session or next session**. Extraction failures are silent and recoverable. A hard failure in Phase 2 must not corrupt `pending/` or prevent future sessions from running.

## The secondary capture path: `/mm-extract` manual slash command

Exists as a fallback for ad-hoc capture, not the primary path. Use cases:

- You had an insight in the middle of a session and want to capture it before it evaporates, without waiting for session-close.
- You want to re-run extraction on a past session (`mm-extract --session <id>`).
- You want to extract from a non-Claude-Code source (a scratch file, a voice memo transcript, etc.).

Invocation:

```
/mm-extract                 # extract from the current in-progress session
/mm-extract --session <id>  # re-run against a past session from ~/.knowledge/sessions/
/mm-extract --file <path>   # extract from an arbitrary markdown/text file
```

Same pipeline as the automatic path: candidates land in `pending/`, review flow is the same. Manual invocation is a convenience, not a privileged path.

## The tertiary capture path: `/mm-curate`

Manual one-shot entry creation. Use when you want to write down a lesson directly without running an extraction pass.

```
/mm-curate "When the Linux CI passes but macOS hangs, always look at main-process sync I/O first."
```

This prompts you for scope and kind, builds a full frontmatter block, writes to `<scope>/pending/`, and tells you to review. Same review flow. Think of it as "extract, but with a corpus of one sentence."

## Review discipline (cross-reference)

The full review flow is specified in CONTINUITY.md. Key rules (repeated here for Phase 3 implementers):

1. Pending entries **auto-expire after 7 days**. Silent deletion, no nag.
2. Review is **one entry at a time**, never a list. Keyboard-driven: `k` keep, `x` reject, `e` edit, `s` skip, `q` quit.
3. **Default-accept for open-loops**. Review still offers the choice but the default action is keep.
4. **No counters, no streaks, no gamification.**
5. **Silent when pending is empty.** Session-start injection shows nothing about review if nothing's there.

## What the extractor must NEVER do

- Write to `~/.knowledge/lessons/` or `.knowledge/nodes/` directly. Always `pending/`.
- Invent facts not present in the transcript.
- Merge multiple unrelated ideas into one entry. One entry = one idea.
- Extract more than ~8 candidates per session. High-signal only; if the session had 8 lessons it was probably a very good session, and if it had 15 the extractor is being noisy.
- Block the user's session in any way. Phase 1 must return in <100ms; Phase 2 runs detached.
- Make decisions that mutate the live store. The review queue is the only path.
- Surface anything to the user synchronously. All feedback goes through the next session-start injection.
