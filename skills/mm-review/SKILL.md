# /mm-review — Verify and Triage Pending Knowledge Entries

Review auto-extracted entries from mastermind's pending queue. The current session model (Opus/Sonnet) verifies Haiku-generated candidates against their source commits and files, auto-promotes what's clean, auto-rejects hallucinations, and escalates only ambiguous cases to the human.

## The architecture

```
Haiku scan (cheap, broad)
  → 50 candidates in pending/
  → Opus verify (uses existing session, no extra cost)
  → 30 verified (auto-promote) + 15 rejected (auto-delete) + 5 ambiguous (human)
  → live/
```

The heavy lifting (reading every commit, every file) was already done by Haiku during `/mm-discover`. Your job is the quality gate: read each candidate plus its specific `## Source`, verify the claim, and triage.

## When to use

- After `/mm-discover` has populated pending/ with candidates
- When PreCompact extraction has generated entries
- When session-start injection shows a pending count > 0
- When the user says `/mm-review`, "review pending", "check pending entries", etc.

## Workflow

### 1. Find all pending entries

First, determine the **project root** — the git toplevel of the current working directory:

```bash
git rev-parse --show-toplevel
```

If this fails (not in a git repo), use the current working directory.

Then glob for pending entries across all three scope roots using **absolute paths**. Relative globs like `.knowledge/pending/*.md` can fail to resolve depending on where the glob tool is anchored — always use absolute paths:

```
$HOME/.knowledge/pending/*.md                      (user-personal)
<project_root>/.knowledge/pending/*.md             (project-shared)
$HOME/.claude/projects/*/memory/pending/*.md       (project-personal)
```

**Do NOT skip the project-shared glob if the project root lookup fails** — fall back to globbing `.knowledge/pending/*.md` from the current working directory, and if that also returns nothing, try walking up from cwd looking for any `.knowledge/pending/` directory (bounded to 4 levels).

If empty across ALL three scopes after trying all fallbacks, say "No pending entries to review" and stop.

Otherwise report: "Found N pending entries. Verifying against sources..."

### 2. Verify entries ONE AT A TIME using subagents

**CRITICAL: Do NOT load all entries into your main context.** With 27+ pending entries, reading each one plus its source commits/files would blow the context window before you finish. Instead, spawn one verification subagent per entry. The subagent does the reading and returns a small verdict; the main agent just acts on verdicts.

For each pending entry path, in order:

1. **Spawn a verification subagent** with this pattern:

```
Agent({
  model: "sonnet",  // or inherit — any capable model, NOT haiku (haiku generated this)
  description: "Verify pending entry <slug>",
  prompt: "Verify this pending knowledge entry against its source.
    
    Entry path: <absolute path to pending .md file>
    
    Steps:
    1. Read the entry file to get the frontmatter, body, and ## Source section
    2. If ## Source has short commit hashes (e.g., abc1234): run `git show <hash>` for each
    3. If ## Source has file paths: Read each file
    4. If ## Source is missing or unparseable: verdict is AMBIGUOUS
    
    Judge the entry against these criteria:
    - Does the source actually support the claim? (hallucinations → REJECTED)
    - Is the claim non-trivial and actionable? (trivial restatements → REJECTED)
    - Is it genuinely new? Call mm_search with the topic to check for duplicates in the live store (paraphrases → REJECTED)
    - Do topic, kind, tags all make sense?
    
    Return EXACTLY this format (single line, no prose):
    VERDICT=<VERIFIED|REJECTED|AMBIGUOUS>|TOPIC=<entry topic>|REASON=<one-line reason>
    
    Examples:
    VERDICT=VERIFIED|TOPIC=Use sync.Map for concurrent access|REASON=commit abc1234 diff shows the race fix exactly
    VERDICT=REJECTED|TOPIC=Package store has types|REASON=trivial restatement, no actionable takeaway
    VERDICT=AMBIGUOUS|TOPIC=Avoid nil checks in handlers|REASON=source commit matches but claim is partially correct"
})
```

2. **Parse the subagent's response** to extract the verdict, topic, and reason.

3. **Act immediately** based on the verdict (see step 3).

4. **Move to the next entry.** Do NOT keep the previous entry's content in your context. The subagent's small verdict line is all you need.

**Why subagents**: each verification is isolated. Your main context stays small (just paths, verdicts, counters). 27 entries × full diff reads would be tens of thousands of tokens; 27 one-line verdicts is ~1K tokens.

**Why not Haiku for verification**: Haiku generated the candidates via `/mm-discover`. Using Haiku to verify its own output defeats the quality gate. Use Sonnet or inherit the session's model.

### 3. Act immediately on each verdict (do not batch)

For each verdict the subagent returns, take action **right away** before spawning the next subagent. Do NOT collect verdicts and batch-process at the end — act, then move on.

- **VERIFIED**: call `mm_promote` with the pending path. Print one line: `✓ <topic>`. Move to next entry.
- **REJECTED**: delete the file. Print one line: `✗ <topic> — <reason>`. Move to next entry.
- **AMBIGUOUS**: append `{path, topic, reason}` to an in-memory `ambiguous` list. Print one line: `? <topic> — <reason>`. Move to next entry. Do NOT read the file content at this stage.

**Keep a running counter**: `verified: N, rejected: M, ambiguous: K, remaining: R`. Print progress every 5 entries so the user sees movement: `[10/27] ✓3 ✗5 ?2`.

**Context discipline**: after acting on a verdict, the only things that should remain in your main context from that entry are its counter increment and — if ambiguous — its path, topic, and one-line reason. The full entry body and source material stay in the subagent where they belong.

### 4. Present AMBIGUOUS entries to the human one at a time

Only reach this step after ALL pending entries have been verified. By now your main context should be mostly empty — just the `ambiguous` list (paths + one-line reasons) and the counters.

For each path in the ambiguous list, one at a time:

1. **Read the entry file NOW** (not before) to get the frontmatter and body
2. **Present it** in this format, then use AskUserQuestion:

```
**[1/K ambiguous]** `scope` · `kind` · `project` · tags: [tag1, tag2]

**Topic**: <topic from frontmatter>

<full body content>

**Source**: <commit hashes or file paths>

**Why ambiguous**: <reason from subagent verdict>

**Your read**: <your honest opinion — promote, reject, or judgment call>
```

3. **Act** on the user's response:
   - **p / promote** → call `mm_promote`, move on
   - **r / reject** → delete the file, move on
   - **e / edit** → ask what to change, edit, re-present for decision
   - **s / skip** → leave in pending, move on
   - **q / quit** → stop, jump to summary

4. **Forget the entry** after acting. Move to the next ambiguous path.

### 5. Report summary

```
Review complete:
  ✓ Verified and promoted: N
  ✗ Rejected (hallucinations/duplicates): M
  ? Human-reviewed: K
  ⊙ Skipped: S

Promoted entries in .knowledge/ — `git add .knowledge/` when ready to commit.
```

If any VERIFIED or REJECTED decisions were close calls, briefly mention them so the human can spot-check if they want.

## Rules

- **One entry at a time, always.** Never load multiple entries into your main context at once. Spawn one verification subagent per entry, act on its verdict, then move on. Your main context should hold counters and the ambiguous list — nothing more.
- **Act immediately, don't batch.** When a verdict comes back, call `mm_promote` or delete right away. Don't collect verdicts and process them all at the end — that defeats the point of keeping context clean.
- **Subagents do the reading.** The subagent reads the entry file, the source commits/files, and checks mm_search for duplicates. The main agent never reads these itself during the verification phase.
- **Verify before triaging.** The subagent must read `## Source` to classify. An entry without Source → AMBIGUOUS, never auto-promote.
- **Don't use Haiku for verification.** Haiku generated the candidates. Use Sonnet or inherit the session's model as the verifier.
- **Auto-promote verified, auto-reject hallucinations.** The human's time is for edge cases, not rubber-stamping.
- **When in doubt, escalate.** Better to mark AMBIGUOUS than to wrongly promote or reject.
- **No guilt.** Don't mention how old entries are or how many were rejected.
- **Efficient presentation.** For ambiguous entries, give the human the entry AND the subagent's reason in one block. Don't make them ask "what's wrong with this one?"
- **If `mm_promote` returns `ErrEntryExists`**: a live entry with the same slug already exists. Classify as REJECTED (duplicate) and delete.
- **Progress feedback every 5 entries.** Print `[N/M] ✓X ✗Y ?Z` so the user sees movement on long runs.
