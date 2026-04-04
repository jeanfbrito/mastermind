// Package mcp wires mastermind's functionality into the Model Context Protocol.
//
// It exposes three tools over stdio:
//
//   - mm_search(query, scopes?, include_archive?) — fan-out query across scopes.
//   - mm_write(content, scope, kind)              — write to <scope>/pending/.
//   - mm_promote(pending_path, target_scope)      — move pending → live.
//
// The concrete MCP SDK is chosen in Phase 0 (see docs/REFERENCE-NOTES.md
// once it exists). This package is the only place that imports the SDK, so
// it can be swapped without touching store/format/search.
package mcp
