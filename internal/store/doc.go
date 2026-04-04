// Package store implements the three-scope markdown-backed knowledge store.
//
// Scopes:
//   - user-personal     (~/.mm/)
//   - project-shared    (<repo>/.mm/)
//   - project-personal  (~/.claude/projects/<repo>/memory/)
//
// Responsibilities:
//   - Locate store roots on disk.
//   - Glob and read markdown entries with YAML frontmatter.
//   - Write candidate entries to <scope>/pending/ (never directly to live).
//   - Promote pending entries to the live store on explicit request.
//
// Non-responsibilities:
//   - Search/indexing — that lives in the search package and delegates to
//     context-mode's FTS5.
//   - Sync — git handles it, mastermind does not.
package store
