// Package search provides the query layer across all three scopes.
//
// mastermind does not own an index. It indexes its markdown files into
// context-mode's FTS5 on startup with scope-labeled sources, then queries
// via context-mode's ctx_search tool and merges the results.
//
// Source labels used when indexing:
//   - mm:user                          (~/.mm/lessons/)
//   - mm:user-archive                  (~/.mm/archive/, only when include_archive=true)
//   - mm:project-shared:<repo>         (<repo>/.mm/nodes/)
//   - mm:project-personal:<repo>       (Claude auto-memory dir)
//
// A grep-based fallback exists for environments without context-mode so the
// tool degrades gracefully — the corpus (plain markdown) is always readable.
package search
