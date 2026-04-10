# Entry format (stable contract)

Every mastermind entry is a single markdown file with YAML frontmatter. **This format is the contract between present-you and future-you. It must stay stable for decades.** Tools are replaceable; the corpus is not.

## File location

- Working set (user-personal): `~/.knowledge/<category>/<slug>.md`
- Archive: `~/.knowledge/archive/<year>/<project>/<slug>.md`
- Project-shared: `<repo>/.knowledge/<category>/<slug>.md`
- Pending review: `<scope-root>/pending/<timestamp>-<slug>.md`

Category is a topic directory (1-2 path segments, e.g. `electron/ipc`, `go/modules`, `testing`). Derived from the entry's `category` frontmatter field, or from the first tag if `category` is absent. Entries are organized by subject, not by the project context they were captured in.

Filenames use kebab-case slugs. No spaces, no unicode, no dates in the filename (dates go in frontmatter).

## Frontmatter schema

```yaml
---
date: 2026-04-04              # ISO date, when captured
project: Rocket.Chat.Electron # free-form string, the project this came from
tags: [electron, ipc, macos]  # free-form, lowercase, plural OK
topic: "One-line summary"     # human headline, used in search previews
kind: lesson                  # enum: lesson | insight | war-story | decision | pattern | open-loop
scope: user-personal          # enum: user-personal | project-shared | project-personal
category: "electron/ipc"     # topic directory path (1-2 segments, optional, derived from first tag if omitted)
confidence: high              # enum: high | medium | low (optional, default: high)
---
```

### Field reference

- **date**: ISO 8601 date the entry was captured (not the event date — capture date is what matters for archiving).
- **project**: free-form string identifying the project or context. Used for archive scoping. Use `general` for cross-project entries in the user-personal store.
- **tags**: free-form lowercase strings. No controlled vocabulary — keeps it from rotting. Be generous; tags are how future-you will find things when the topic is forgotten.
- **topic**: one-line human summary. Shown in search results. Write it so you'd recognize the entry from this line alone.
- **kind**: exactly six values. More is bloat, fewer loses distinctions.
  - `lesson` — "I learned X the hard way because Y." Highest-value entries.
  - `insight` — "I realized X pattern solves Y class of problem." Cross-cutting.
  - `war-story` — "Spent N hours/days on X, root cause was Y, fix was Z." Specific enough to teach, general enough to transfer.
  - `decision` — "Chose X over Y because of constraint Z." The *why* is what you forget.
  - `pattern` — "When I see shape-X, I try approach Y first." Reusable heuristic.
  - `open-loop` — "I was about to do X but stopped. Resume when..." Unfinished work the user intended to return to. ADHD-specific kind; defaults to `scope: project-personal`; surfaced automatically at session start (see CONTINUITY.md); resolved via `mm_close_loop`. Like all entries, open-loops are never silently deleted — they stay until explicitly resolved or removed.
- **scope**: which of the three stores the entry belongs to. Exactly three values:
  - `user-personal` — lives at `~/.knowledge/lessons/<slug>.md`. Career-long, cross-project knowledge that follows you between machines.
  - `project-shared` — lives at `<repo>/.knowledge/<category>/<slug>.md`. Checked into the repo; shared with anyone who clones it.
  - `project-personal` — lives at `~/.claude/projects/<project>/memory/<category>/<slug>.md`. Machine-local notes about a specific project you don't want to share with collaborators; the default for `open-loop` entries.

  Optional in frontmatter for hand-placed files (the store can infer it from the directory the file lives in) but **required** when capturing via `mm_write`, because the tool has to decide which store root to target before the file exists on disk.
- **category**: topic directory path for organizing entries by subject. Max 2 path segments (e.g. `"electron/ipc"`, `"go/modules"`, `"testing"`). Classify by the SUBJECT of the lesson, not the context it was discovered in. Optional — if omitted, the store derives it from the first tag (or `"general"` if no tags). When the agent calls `mm_write`, it should always provide a category.
- **confidence**: how sure you are. `high` = battle-tested, `medium` = strong hunch, `low` = half-remembered intuition. Lets future-you weight old entries appropriately.
- **supersedes** (optional, added 2026-04-10): list of entry slugs this one explicitly replaces. Example: `supersedes: [old-decision-use-channel, older-lockfree-attempt]`. Human-populated only via `/mm-review` prompts; never auto-generated. At search time, each listed slug adds a within-class score multiplier (capped at 3 links so a single entry can surface preferentially without being gameable). Dangling slugs (a referenced entry no longer exists) are tolerated — they surface as broken links during review, never silently erased. Schema contract: additive extension, absent on pre-2026-04-10 entries, `omitempty` on write so the field never appears on entries that don't use it.
- **contradicts** (optional, added 2026-04-10): list of entry slugs this one conflicts with. Unlike `supersedes`, this field does NOT contribute to the search score. Instead, it triggers **co-retrieval**: when a search returns an entry with a non-empty `contradicts` list, the listed entries are pulled into the results alongside it with a `(contradicts "<topic>")` annotation — regardless of whether they match the query keyword. This is mastermind's "tensions surface, they don't hide" philosophy: a contradicting claim should appear adjacent to the claim it contradicts, not get demoted into invisibility. Human-populated only. See `docs/DECISIONS.md` 2026-04-10 entry for why contradicts is a co-retrieval signal rather than a score booster.

## Body structure

Every kind uses this structure. Sections can be empty but headings stay for consistency:

```markdown
# <topic>

## What happened
Concrete situation, in a paragraph. Enough for future-you to recognize the shape.

## Why
Root cause or underlying principle. The bit you'll want to recall.

## How I found it
The debugging path, including wrong turns. Wrong turns teach more than the fix.

## Lesson
One or two sentences. The takeaway, stated as a rule or heuristic.

## When this matters again
Pattern signature — what a future problem would look like that should pull this entry up.
```

The **When this matters again** section is critical. Future-you uses different vocabulary than present-you. This section bridges the gap: describe the *shape* of the problem, not the specifics. FTS5 will hit it on searches you can't predict today.

## Example

```markdown
---
date: 2024-03-14
project: Rocket.Chat.Electron
tags: [electron, ipc, macos, debugging, main-process]
topic: "macOS Electron IPC hangs when main process blocks on sync I/O"
kind: lesson
scope: user-personal
category: "electron/ipc"
confidence: high
---

# macOS Electron IPC hangs when main process blocks on sync I/O

## What happened
Shipped a feature that did synchronous file reads in the Electron main
process. Worked fine on Linux in CI. On macOS, the renderer hung for
several seconds whenever the feature ran.

## Why
macOS schedules the main thread differently than Linux. Long sync
operations in the main process don't yield, so IPC messages queue but
aren't drained until the main process returns to the event loop.

## How I found it
Tried async wrappers (wrong — still blocked). Tried worker threads
(wrong — IPC was the bottleneck, not the I/O). The tell was that the
Linux CI run passed every time and macOS hung every time. Platform-
divergent hangs in Electron almost always point at main-process blocking.

## Lesson
Never do sync I/O in the Electron main process. Not "try not to." Never.
If it has to happen, push it to a utility process.

## When this matters again
Any Electron app. Any "works on Linux, hangs on macOS" pattern. Any
renderer unresponsiveness that correlates with main-process activity.
```

## What NOT to put in entries

- Code snippets longer than a few lines. Git has the code.
- File paths. They change.
- Exact version numbers unless the lesson is version-specific.
- Names of coworkers (privacy, longevity).
- Anything reconstructible in under 5 minutes from docs or git log.
- Summaries of "what I did today." Only save when you learned something.

## Migration policy

Changing this format later costs: either migrate every old entry, or accept that old entries become less discoverable. **Decide the format once, live with it.** If a field turns out to be unnecessary, leave it — don't break backward compatibility. If a field is missing, add it as optional.

The one exception: the `kind` enum. If you discover a genuinely new kind of entry that doesn't fit the five above, add it — but only after living with the five for at least a year. New kinds after that year are an event, not a casual change.
