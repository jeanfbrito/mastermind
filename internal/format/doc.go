// Package format defines the entry format: YAML frontmatter + markdown body.
//
// The schema is the long-term contract between present-you and future-you.
// It must stay backward-compatible. See docs/FORMAT.md for the full spec.
//
// Responsibilities:
//   - Parse frontmatter from a markdown file.
//   - Validate required fields (date, project, topic, kind).
//   - Serialize an Entry back to markdown for writing to pending/.
package format
