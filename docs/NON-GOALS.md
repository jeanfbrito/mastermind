# Non-goals

This file exists because every "small, focused tool" project dies by scope creep. When a tempting feature comes up mid-build, read this file first.

## Hard non-goals (never)

### No server
mastermind runs as a local MCP process started by Claude Code. No daemon, no hosted service, no account, no "sign in." If you ever find yourself designing a server component, stop — you're building something else.

### No hub
Specifically, none of brv's hub integration. You disliked it, it's the reason this project exists. Don't accidentally reintroduce it under a different name.

### No vector store, no embeddings, no fine-tuning
FTS5 keyword search is enough at the corpus sizes we expect (thousands of entries over a career). If retrieval feels weak, the fix is **better tags and better "when this matters again" sections in entries**, not semantic search. Keep it boring.

Specifically: do not add embeddings "just for re-ranking." Do not add a vector column to FTS5. Do not fine-tune a model on the corpus. The corpus is for *humans* to read; retrieval just needs to surface the right file.

### No auto-writes without review
Every entry passes through `pending/`. Every promotion is a deliberate human action. No exceptions, ever. The moment you let the extractor write directly to the live store, the corpus starts to rot and you'll stop trusting it. This is the specific point where mastermind deliberately diverges from OpenViking (which auto-commits extracted memories) — see CONTINUITY.md and REFERENCE-NOTES.md for the rationale.

### No notifications, no reminders, no dashboards, no streaks, no badges
The default state of the tool is invisible. The only thing mastermind *ever* says unprompted is the session-start continuity injection, and that block is silent when there's nothing relevant. For the ADHD-shaped primary user, working memory is a non-renewable resource that the tool must not tax. Every notification, every badge, every "you haven't extracted in N days" message compounds into cognitive load and eventually into avoidance. The discipline of keeping the tool silent is the discipline of keeping it useful. See CONTINUITY.md for the full "silent unless needed" rule.

### No replacement for context-mode
mastermind is a **consumer** of context-mode, not a competitor. If context-mode adds a feature, celebrate it. If context-mode has a bug, fix it upstream. Do not reimplement FTS5 indexing, sandbox execution, or Bash interception inside mastermind.

**The boundary, stated explicitly**: context-mode handles *this-session raw tool output and unfamiliar content search*. mastermind handles *persistent curated conceptual knowledge across sessions and years*. context-mode's data lives in `/tmp` and resets on `/clear`; mastermind's data lives in a git repo and compounds forever. If you're searching something you generated five minutes ago, that's context-mode. If you're trying to remember what your 2026 self decided about auth flows, that's mastermind. No feature in mastermind should drift into context-mode's territory, and vice versa.

### No replacement for engram
mastermind is not a better engram. engram is an excellent general-purpose memory tool for any MCP-speaking agent, with a TUI, HTTP API, SQLite+FTS5 storage, and multi-agent support. Mastermind deliberately diverges on five dimensions (storage format, scope model, capture path, continuity layer, target audience) because those divergences serve the specific ADHD-shaped workflow of its primary user. If the divergences ever stop being load-bearing — if mastermind starts making choices that match engram's "generic agent memory tool" profile — the right response is to stop and use engram instead. Reinventing engram is a waste; diverging on purpose is the whole point.

### No code-understanding tool for unfamiliar repos
mastermind's value is in querying *your own* curated conceptual knowledge, not in searching unfamiliar codebases. For one-shot exploration of a repo you'll never touch again, raw tools (grep through context-mode, the Explore agent, opening files directly) beat any curated knowledge layer — the curation cost has no amortization path. Mastermind is worth it when you'll query the same knowledge ten times over months and years. It is not worth it for "I'm reading this external repo today."

### No saving code, file paths, or reconstructible facts
Entries capture insights, not artifacts. If a future-you could reconstruct the information from git log, docs, or a 5-minute web search, it doesn't belong in mastermind. The corpus is curated wisdom, not a dump.

### No per-project configuration sprawl
One format, one directory convention, one set of slash commands. If a user wants to "customize the extraction prompt per project" or "change the frontmatter schema for this repo" — no. Uniformity is what makes the corpus survive across decades and tools.

### No migration tooling for other systems
Don't import from Obsidian, Notion, Roam, Logseq, brv-hub, or anywhere else. If you want old notes in mastermind, copy them by hand — the manual work forces you to reformat and filter, and only the high-signal ones survive. That's a feature.

### No multi-user support
mastermind is personal (user-personal store) and team-within-a-repo (project-shared, via git). It is not a shared knowledge base across a company, not a wiki, not a CMS. If a team needs that, they need a different tool. Adding "user accounts" or "permissions" kills the tool.

### No web UI
The interface is: markdown files + git + MCP tool + slash commands. A web UI is a maintenance sink that doesn't solve any problem the filesystem doesn't already solve. If you want to browse entries in a GUI, use your editor.

### No encryption at rest
The `~/.mm/` git repo sits on your disk, pushed to a private remote you control. Full-disk encryption (FileVault, LUKS) handles the rest. Don't build per-file encryption, per-field encryption, or "secure mode." If an entry is sensitive enough to need encryption, it's sensitive enough to not write down.

### No publishing mastermind as a product
This tool is for you. It solves your problem. Open source it if you want, but don't design for users who aren't you. The moment you start caring about strangers' feature requests, the scope explodes.

## Soft non-goals (not now, maybe later, require explicit re-decision)

### Cross-session extraction across months
`/mm-extract --since 7d` is already in the roadmap. Scanning months of history and proposing candidates is plausible, but only after the main extraction flow is stable and you've lived with it for a season. Don't build it up front.

### Auto-promotion of high-confidence pending entries
Tempting and dangerous. If you ever find yourself "always accepting the high-confidence ones anyway," resist the urge to automate it. The review step is the consolidation — skipping it loses the value.

### Visual retrieval-trajectory (OpenViking's idea)
Showing *why* a result was returned, at what rank, from what source. Not needed at the corpus sizes we expect. Revisit only if retrieval starts feeling opaque.

### Tag suggestions / tag consistency tooling
Free-form tags will drift over years. A tool that suggests existing tags when you write an entry could help. Build only after tag drift becomes a real retrieval problem, not before.

### Public search index for open-source projects
Some `<repo>/.mm/` content could usefully be published (docs site, searchable on the web). Nice idea. Different project.

## Tests for any new feature proposal

Before adding *anything* to the roadmap, answer:

1. **Does this work on plain `~/.mm/` without the tool?** If the feature requires the tool to be running, it's suspect. The corpus must outlive the tool.
2. **Does this survive a tool rewrite?** If a future rewrite would break the feature's data, the feature is storing something in the wrong place.
3. **Is this solving a pain I've actually felt, or one I'm imagining?** Build from felt pain only.
4. **Would a user five years from now still want this?** If it solves a 2026-specific problem, it's a distraction.
5. **Can I delete this in a year if it turns out to be wrong?** If removal would be painful, the feature is a commitment you haven't earned yet.

If any answer is "no" or "not sure" — don't build it. Come back when you know.
