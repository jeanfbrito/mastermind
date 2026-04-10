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

Glob for pending entries across all three scope roots:

```
~/.knowledge/pending/*.md
.knowledge/pending/*.md
~/.claude/projects/*/memory/pending/*.md
```

If empty, say "No pending entries to review" and stop.

Otherwise report: "Found N pending entries. Verifying against sources..."

### 2. Verify each entry (do NOT ask the human yet)

For each pending entry, **you** (the current session model) do the verification:

1. **Read the entry file** — get frontmatter, body, and `## Source` section
2. **Read the source material**:
   - If Source has short commit hashes (e.g., `abc1234`): run `git show <hash>` to see the actual diff
   - If Source has file paths: Read those files to see the actual code
3. **Verify the claim**:
   - Does the commit/file actually contain what the entry claims?
   - Is the claim non-trivial (not "this function does X" — we want lessons, not restatements)?
   - Is there an actionable takeaway, not just an observation?
   - Is it genuinely new knowledge, or a paraphrase of something already in the live store (check with `mm_search`)?
4. **Classify into one of three buckets**:

**VERIFIED** — promote automatically. Criteria:
- Source clearly supports the claim
- Non-trivial and actionable
- Not a duplicate of existing live knowledge
- Topic, kind, tags all make sense

**REJECTED** — delete automatically. Criteria:
- Source contradicts the claim (hallucination)
- Trivial restatement of what the code obviously does
- Source commit/file doesn't exist or isn't readable
- Paraphrase of an existing live entry
- Verbose or generic without a real takeaway

**AMBIGUOUS** — escalate to human. Criteria:
- Claim is partially correct but wording is off
- Source is tangentially related but not conclusive
- Judgment call on whether it's worth keeping
- You're not confident either way

### 3. Apply VERIFIED and REJECTED automatically

- **VERIFIED**: call `mm_promote` with the absolute pending path. Report briefly: `✓ <topic>`
- **REJECTED**: delete the file. Report briefly: `✗ <topic> — <one-line reason>`

Do not ask the human about these. You verified them against the source — that's the quality gate.

### 4. Present AMBIGUOUS entries to the human one at a time

For each ambiguous entry, present it in this format and use AskUserQuestion:

```
**[1/K ambiguous]** `scope` · `kind` · `project` · tags: [tag1, tag2]

**Topic**: <topic from frontmatter>

<full body content>

**Source**: <commit hashes or file paths>

**Why ambiguous**: <your one-line reasoning>

**Your read**: <your honest opinion — promote, reject, or judgment call>
```

Then wait for:
- **p / promote** → call `mm_promote`
- **r / reject** → delete the file
- **e / edit** → ask what to change, edit, re-present
- **s / skip** → leave in pending, move on
- **q / quit** → stop, report summary

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

- **Verify before triaging.** Never promote an entry without reading its `## Source`. The whole point of this skill is to catch Haiku's hallucinations.
- **Read actual sources.** For commits, use `git show`. For files, use Read. Don't guess.
- **Auto-promote verified, auto-reject hallucinations.** The human's time is for edge cases, not rubber-stamping.
- **When in doubt, escalate.** Better to ask than to wrongly promote or reject.
- **No guilt.** Don't mention how old entries are or how many were rejected.
- **Efficient presentation.** For ambiguous entries, give the human the entry AND your analysis in one block. Don't make them ask "what's wrong with this one?"
- **If `mm_promote` returns `ErrEntryExists`**: a live entry with the same slug already exists. Classify as REJECTED (duplicate) and delete.
- **Fall back to manual mode** if an entry has no `## Source` section: treat it as AMBIGUOUS (can't verify against unknown source).
