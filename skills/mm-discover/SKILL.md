# /mm-discover — Autonomous Knowledge Discovery

Mine the codebase and git history for knowledge that should be in mastermind but isn't. Uses Haiku subagents for all heavy lifting to minimize token cost.

## When to use

- First time setting up mastermind on an existing project (seed initial knowledge)
- Periodically to catch knowledge gaps
- After major refactors or architecture changes
- When the user says `/mm-discover`, "discover knowledge", "mine the codebase", "seed knowledge", etc.

## Modes and depth

Parse the user's argument to select mode and depth:

- `/mm-discover` or `/mm-discover all` — run both git and codebase modes (default depth: 100 commits)
- `/mm-discover git` — git history mining only
- `/mm-discover codebase` — codebase scan only
- `/mm-discover git --depth 500` — scan deeper into history

**Order: always newest first.** Recent knowledge is most valuable. A fix from last week matters more than a commit from 2021.

## Workflow

### 1. Determine scan range (main agent — cheap)

The scan range is derived from **existing entries**, not a cursor file. The entries themselves are the state.

```bash
# Project structure
find . -name '*.go' -not -path './vendor/*' | head -100
# or for other languages, adjust the glob
```

**For git mode**, determine what's already been discovered using two bash commands (zero LLM cost):

```bash
# Step A: Extract all commit hashes already in discovered entries.
# Every discovered entry has a "## Source" section with hashes like "abc1234".
# One grep pulls them all — no need to Read individual files.
KNOWN_HASHES=$(grep -rh '## Source' -A5 .knowledge/ ~/.knowledge/ 2>/dev/null \
  | grep -oE '\b[0-9a-f]{7}\b' | sort -u)

# Step B: Get the last N commits (default 100).
ALL_COMMITS=$(git log --oneline -100)

# Step C: Filter — remove commits whose hash appears in KNOWN_HASHES.
# The remaining lines are the WORK LIST — only these go to Haiku.
```

Filter the commits: for each line in ALL_COMMITS, check if its short hash appears in KNOWN_HASHES. Only commits NOT in KNOWN_HASHES make the work list.

**Example — second run, 5 new commits since last time:**
- `git log --oneline -100` returns 100 commits
- `KNOWN_HASHES` contains 95 hashes from existing entries
- Work list: 5 commits → only these go to Haiku (~$0.002)

**Example — first run on a 5000-commit repo:**
- `git log --oneline -100` returns the 100 most recent commits
- `KNOWN_HASHES` is empty (no entries yet)
- Work list: 100 commits → sent to Haiku in batches of 25 (~$0.02)
- User wants more? `/mm-discover git --depth 500`

**Example — no new commits since last run:**
- 100 commits listed, all 100 in KNOWN_HASHES
- Work list: empty → **no Haiku subagents spawned** → zero cost

**Why no cursor file**: the entries ARE the cursor. This is self-correcting — if you reject an entry in `/mm-review`, its source commits lose their "already discovered" marker and get re-analyzed next run (a feature, not a bug). Works across machines because entries sync via git.

Also call `mm_search` with a broad query like the project name to see what topics are already captured.

### 2. Spawn Haiku subagents for analysis

**CRITICAL: All exploration agents MUST use `model: "haiku"`** to keep costs near zero. The main agent only orchestrates — never read source files or analyze diffs yourself.

#### Git mode: mine commit history

If the work list has more than 25 commits, **batch into chunks of 25** and spawn one Haiku subagent per chunk (max 4 concurrent). Otherwise, one subagent handles everything.

Each subagent gets the specific commit hashes to analyze:

```
Agent({
  model: "haiku",
  description: "Mine git history batch N for knowledge",
  prompt: "You are analyzing git history for a project called <PROJECT>.
    
    Analyze ONLY these commits (newest first):
    <COMMIT_LIST — one 'hash message' per line, max 25>
    
    For any that look like bug fixes, architectural decisions, or
    non-trivial changes, read the full diff with: git show <hash>
    
    Focus on commits with messages containing: fix, refactor, revert,
    decision, change, migrate, replace, remove, add (new features).
    Skip: typo fixes, version bumps, dependency updates, formatting.
    
    For each interesting finding, output a JSON object on its own line:
    {\"topic\": \"one-line summary\", \"kind\": \"lesson|decision|pattern|war-story|insight\", \"body\": \"3-5 lines: what happened, why, takeaway\", \"tags\": [\"tag1\", \"tag2\"], \"category\": \"topic-dir\", \"source\": \"abc1234 — commit message summary\"}
    
    IMPORTANT: The \"source\" field MUST include the short commit hash(es)
    the knowledge came from. This is the provenance trail AND the
    mechanism for incremental runs. Without it, the same commits get
    re-analyzed every time.
    
    Output ONLY the JSON lines, nothing else. Maximum 15 entries per batch.
    Skip anything trivial — only extract what would save future-you time."
})
```

#### Codebase mode: scan for patterns and conventions

Spawn **one Haiku subagent per major package/directory** (max 5 concurrent). Each gets:

```
Agent({
  model: "haiku",
  description: "Analyze <package> for knowledge",
  prompt: "You are analyzing the <PACKAGE> package in a Go project called <PROJECT>.
    
    Read the key source files in <PACKAGE_PATH>/.
    
    Extract non-obvious knowledge that a developer working on this code
    should know. Focus on:
    - Conventions and patterns used (not obvious from reading one file)
    - Gotchas and edge cases
    - Design decisions baked into the code structure
    - Invariants that aren't documented in comments
    
    Do NOT extract:
    - What the code does (that's what reading it is for)
    - Obvious things (function names, struct fields)
    - Anything already in comments or docs
    
    For each finding, output a JSON object on its own line:
    {\"topic\": \"one-line summary\", \"kind\": \"pattern|lesson|insight|decision\", \"body\": \"3-5 lines explaining the non-obvious part\", \"tags\": [\"tag1\", \"tag2\"], \"category\": \"topic-dir\", \"source\": \"internal/store/store.go, internal/store/config.go\"}
    
    IMPORTANT: The \"source\" field MUST include the file path(s) where
    the knowledge was observed. This is the provenance trail.
    
    Output ONLY the JSON lines, nothing else. Maximum 5 entries per package.
    Quality over quantity — only things worth remembering in 6 months."
})
```

### 3. Collect and deduplicate (main agent)

**Before processing candidates**, build a dedup set from ALL existing knowledge:

1. **Live entries**: Call `mm_search` with `include_pending: true` and a broad query (project name) to get existing topics
2. **Pending entries**: Glob for `**/pending/*.md` across all scope roots. Read each file's frontmatter to extract its topic. This catches entries from previous `/mm-discover` runs that haven't been reviewed yet.

Collect all existing topics (live + pending) into a set for substring matching.

Then for each candidate from subagents:

1. **Check the dedup set** — if the candidate topic is a substring of an existing topic (or vice versa), skip it
2. **Check other candidates** — if two candidates overlap, keep the better one
3. **Validate**: topic must be non-empty, kind must be valid, body must be non-empty
4. **Add accepted candidates to the dedup set** so later candidates in the same batch don't duplicate them

### 4. Write entries to pending/

For each surviving candidate, create a file in the appropriate pending directory using the **Write tool**:

**Path**: `<scope-root>/pending/YYYYMMDD-HHMMSS-<slug>.md`

Where:
- `<scope-root>` is `.knowledge` for project-shared entries (most discovery output), or `~/.knowledge` for general engineering lessons
- `YYYYMMDD-HHMMSS` is the current UTC timestamp (e.g., `20260409-143022`)
- `<slug>` is the topic lowercased, non-alphanumeric replaced with dashes, max 80 chars

**File content**:
```yaml
---
date: YYYY-MM-DD
project: <project-name>
tags: [tag1, tag2]
topic: "<topic from JSON>"
kind: <kind from JSON>
scope: project-shared
category: <category from JSON>
confidence: medium
---

<body from JSON>

## Source
<source from JSON — commit hash(es) or file path(s)>
```

The `## Source` section is **mandatory** for discovered entries. It serves two purposes:
1. **Provenance**: trace an entry back to the code or commit that produced it
2. **Incremental cursor**: commit hashes in Source sections tell future runs which commits have already been analyzed

**IMPORTANT**: Use the Write tool to create each file. Do NOT use mm_write (that goes to live store, bypassing review).

### 5. Report summary

```
Discovery complete: N entries written to pending/

Git history: X entries from Y commits analyzed (Z commits skipped — already discovered)
Codebase:    A entries from B packages scanned
Duplicates:  C skipped (matched existing knowledge)

Run /mm-review to promote the good ones.
```

## Rules

- **Haiku subagents for ALL reading and analysis.** The main agent only orchestrates, deduplicates, and writes files. This keeps cost near zero.
- **Everything goes to pending/, never live.** Discovery is auto-generated — the user reviews with /mm-review.
- **Newest first.** Always process commits in reverse chronological order. Recent knowledge is most valuable.
- **Quality over quantity.** 10 good entries beat 50 mediocre ones. The extraction prompt emphasizes "worth remembering in 6 months."
- **Max 15 entries per git batch, max 5 per package from codebase.** Hard caps prevent runaway output.
- **Batch large commit ranges.** More than 25 commits → split into chunks of 25, one Haiku subagent per chunk, max 4 concurrent.
- **project-shared scope for most entries.** Unless an entry is clearly a general engineering lesson (not project-specific), use project-shared.
- **Don't discover what's documented.** If the project has good docs/README/comments, skip those areas. Discovery finds the UNdocumented knowledge.
- **Fully incremental, no cursor file.** The entries ARE the state. Commit hashes in `## Source` sections tell future runs what's been analyzed. Rejecting an entry in /mm-review makes those commits eligible for re-analysis — self-correcting by design.
