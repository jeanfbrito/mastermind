# Archive tier

The archive solves the long-timeline problem: lessons from years ago shouldn't clutter day-to-day retrieval, but they must remain retrievable when the right problem comes up in 2029.

## Working set vs archive

Only the **user-personal** store has an archive tier. Project stores don't need one — when a project ends, the whole `.mm/` directory retires with the repo.

```
~/.mm/
├── lessons/          ← working set: always searched
└── archive/<year>/<project>/  ← searched only with include_archive=true
```

## Why two tiers

- **Working set stays small and fast.** Everyday queries hit only the current, high-value entries. No noise from old projects you'll never touch again.
- **Archive stays searchable.** Nothing is ever deleted. `mm_search(query, include_archive=true)` searches everything.
- **Matches human memory.** Recent stuff comes up cheaply; old stuff takes a deliberate effort to recall. Same model, same shape.

## When archiving happens

**Manual, project-triggered, explicit.** No background process. No time-based auto-archive. No LRU.

You run:

```
/mm-archive Rocket.Chat.Electron
```

when you're done with a project — leaving a job, shipping a final release, shelving a side project.

## What the archive command does

1. **Identifies entries to archive.** Every entry in `~/.mm/lessons/` with `project: Rocket.Chat.Electron` in its frontmatter.
2. **Proposes cross-project promotion.** For each entry, the extractor asks: "Is this lesson specific to this project, or does it apply to any Electron app / any debugging / any distributed system?" Cross-project candidates are flagged.
3. **You review the promotion list.** For each flagged entry, accept (rewrite with `project: general` and keep in the working set) or reject (archive with the rest).
4. **Moves non-promoted entries** into `~/.mm/archive/<current-year>/<project>/`.
5. **Commits the move** in the `~/.mm/` git repo — archiving is a discrete, reviewable event.

The promotion step is the important one. Many lessons start as "I learned this about Project X" but are really "I learned this about the platform / the class of problem." Promoting them keeps the working set full of cross-cutting wisdom instead of project-specific trivia.

## What gets archived

- Any entry with `project: <archived-project>` in frontmatter.
- Pending entries for that project (archive them without promotion — they weren't reviewed, so they're low-trust).

## What stays in the working set after archive

- Entries promoted to `project: general`.
- Entries for other projects (untouched).
- The `FORMAT.md` file and any mastermind-internal docs.

## Retrieval across tiers

Default query: working set only.

```
mm_search("electron ipc weird")
```

Deep query: include archive.

```
mm_search("electron ipc weird", include_archive=true)
```

Results are source-tagged so you can tell where hits came from:

```
[mm:user]         lessons/electron-ipc-macos.md
[mm:user-archive] archive/2024/RocketChatElectron/obscure-macos-bug.md
```

The agent should include the archive automatically when the working set returns no strong matches, and explicitly surface that it's reaching into old projects.

## Manual browsing

The archive is plain markdown under a predictable path. Any tool works:

```
ls ~/.mm/archive/2024/RocketChatElectron/
grep -rl "ipc" ~/.mm/archive/
open ~/.mm/archive/2024/
```

This matters. The tool is replaceable; the corpus must survive any tool's death. If mastermind stops existing tomorrow, everything you wrote is still plain markdown in a git repo you own.

## What NOT to archive

- **Never auto-archive by age alone.** Old lessons from 5 years ago can still be active working-set knowledge. Archiving is a *project-transition event*, not a calendar event.
- **Never delete.** Archive is permanent. Storage is free; regret is not.
- **Never compress or consolidate old entries.** Don't try to "summarize 2024's learnings into one file." You'll lose the specific details that make old entries valuable. Keep them atomic.

## Un-archiving

Rare, but it happens: you return to a project, or an old lesson is suddenly relevant again. Just `mv` the file back to `~/.mm/lessons/`. It's files. No migration, no re-indexing, no metadata to update. The next FTS5 index rebuild picks it up.

## The long timeline

After 5 years, `~/.mm/archive/` might hold a few thousand entries across dozens of projects. Total size: megabytes. FTS5 search over it: still milliseconds. Git repo size: still trivial.

This is the design target. The archive should comfortably hold a 30-year career's worth of lessons without ever needing a database migration, a format change, or a tool rewrite. Plain markdown + git + FTS5 is enough. Keep it boring.
