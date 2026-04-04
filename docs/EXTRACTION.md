# Capture & extraction

Capture is the hard problem. Retrieval is easy; people fail at second brains because they never write the entries. mastermind's answer is **explicit, reviewable, session-end extraction** plus **cheap manual curation**.

## Principles

1. **Never auto-write.** Every entry passes through `pending/` and a human review step. No exceptions.
2. **Extraction is explicit.** You run `/mm-extract` when you think a session taught you something. The tool never fires on its own.
3. **Review is the consolidation.** Rereading a proposed entry while the session is fresh is when the lesson actually lands in your head. Skipping review defeats the whole tool.
4. **Propose, don't decide.** The extractor proposes scope, kind, tags — all overrideable. You are the editor.
5. **Conservative by default.** The prompt biases toward missing a lesson over inventing one. Extracted junk is worse than extracted nothing.

## The extraction flow

1. You finish a session. You sense you learned something — a bug, a decision, a new mental model.
2. You run `/mm-extract`.
3. mastermind reads the session transcript.
4. For each candidate lesson, it writes a markdown file to the appropriate `pending/` directory with full frontmatter including a *proposed* scope and kind.
5. Tool prints:
   ```
   Proposed 3 entries:
     [user-personal] pending/2026-04-04-sync-io-electron.md  (lesson, confidence: high)
     [project-shared] .mm/pending/2026-04-04-retry-logic.md (decision, confidence: medium)
     [user-personal] pending/2026-04-04-debugging-approach.md (pattern, confidence: medium)
   Review and accept/reject with git and your editor.
   ```
6. You review. Accept = move out of `pending/` into the live store and commit. Reject = delete. Edit = edit first, then move.
7. Done.

## The extraction prompt (sketch)

The extractor sends the session transcript to the model with roughly this shape:

> You are reviewing a work session transcript to identify lessons worth preserving in a personal engineering journal. For each candidate, write a markdown entry in the mastermind format (see FORMAT.md). Be conservative — prefer missing a fact over inventing one. Only extract:
>
> - **Lessons**: the user learned something the hard way that would save future time.
> - **Insights**: a new mental model or cross-cutting principle.
> - **War stories**: a specific debugging journey with a clear root cause.
> - **Decisions**: a choice with a documented rationale that's likely to be forgotten.
> - **Patterns**: a reusable heuristic demonstrated in this session.
>
> For each candidate, also propose:
>
> - **scope**: `user-personal` if the lesson applies beyond this one project, `project-shared` if it's specific to this codebase and teammates would benefit, `project-personal` if it's private scratch for this project.
> - **kind**: one of `lesson | insight | war-story | decision | pattern`.
> - **confidence**: `high` if battle-tested in this session, `medium` if strong hunch, `low` if half-remembered.
>
> Do NOT extract:
>
> - Summaries of what was done. Only what was learned.
> - Code snippets. Only the *why*.
> - Facts that could be looked up in docs in under 5 minutes.
> - Transient task state or TODOs.
>
> Return one markdown file per candidate, with full frontmatter and body sections.

The prompt is versioned and lives at `prompts/extract.md` in the mastermind repo. Changes are commits.

## Scope proposal heuristics (baked into the prompt)

- Mentions a specific repo/codebase/product name → likely `project-shared` or `project-personal`.
- Mentions a general engineering principle, platform, or language feature → likely `user-personal`.
- Private, personal, or sensitive content → `project-personal` (the agent should flag these explicitly).
- When in doubt → `project-shared` (the safest default for project work; private stuff is opt-in).

The agent's proposal is always overrideable in review.

## Manual curation

Sometimes you want to record something without running a full extraction. The slash command:

```
/mm-curate "Always verify library APIs with .d.ts before assuming"
```

Prompts for scope and kind (or infers from tags), writes to `<scope>/pending/`, and tells you to review. Same review flow as extraction.

## Cross-session extraction

Sometimes you forget to extract and realize a week later there was something worth keeping. A secondary command scans recent Claude Code session history and proposes candidates from sessions you didn't explicitly extract:

```
/mm-extract --since 7d
```

Scope: Claude Code transcript history (already on disk). Output: same `pending/` flow. Use sparingly — the main path is `/mm-extract` at session end, because the memory is freshest then.

## Review discipline

The review step is where the whole system succeeds or fails. Rules:

1. **Never blanket-accept pending entries.** Each one gets read and evaluated.
2. **Edit for clarity.** If the extracted entry is wordy or vague, fix it before promoting. The edit is part of the consolidation.
3. **When in doubt, reject.** An empty corpus is better than a noisy one. You can always re-extract later.
4. **Commit promoted entries individually or in small groups.** Each commit is a checkpoint; git history becomes a log of what you learned over time.
5. **Review within ~24 hours.** Pending entries older than that are probably stale in your head and harder to evaluate. Reject or promote; don't let them rot in `pending/`.

## What the extractor should NEVER do

- Write directly to `lessons/`, `nodes/`, or `archive/`. Always `pending/`.
- Invent facts not present in the transcript.
- Merge multiple unrelated ideas into one entry.
- Extract more than ~5 candidates per session — high-signal only.
- Make decisions that change the live corpus without user confirmation.
