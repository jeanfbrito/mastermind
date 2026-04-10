package search

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FormatResultsMarkdown renders search results as human-readable
// markdown that is ALSO optimized for context-mode's automatic
// session-cache indexing.
//
// Why this shape matters: when the MCP server returns this markdown as
// the mm_search tool output, context-mode's standard tool-output
// indexer chunks it by markdown headings. Each result becomes its own
// searchable section in context-mode's session FTS5, tagged by source.
// Subsequent warm follow-ups within the same session can then find
// these sections without re-invoking mastermind — context-mode answers
// them directly.
//
// This is the entire integration between mastermind and context-mode:
// output shape. No coupling, no SDK calls, no fallback logic. It works
// because context-mode already does the right thing with
// heading-structured markdown from any MCP tool.
//
// Format:
//
//	## mm_search: "<query>" — N results
//
//	### [scope] <path-relative-slug> · <kind> · <date>
//	**topic**: <topic>
//	**tags**: tag1, tag2, tag3
//	**project**: <project>
//	**path**: /absolute/path/to/entry.md
//
//	<body — trimmed to topic+first-section+match-excerpt by default,
//	         full body when expand=true>
//
// A header per result is critical — it's what context-mode's chunker
// uses to separate sections.
//
// The expand flag controls body verbosity:
//   - false (default, L2): topic + first ## section + match-anchored
//     excerpt. Caller can Read the path for full content (L3).
//   - true: full body returned verbatim. Use for deep-dive queries
//     or when the caller knows it needs the complete entry.
func FormatResultsMarkdown(query string, results []Result, expand bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## mm_search: %q — %d result", query, len(results))
	if len(results) != 1 {
		b.WriteByte('s')
	}
	b.WriteString("\n\n")

	if len(results) == 0 {
		b.WriteString("_no matching entries in mastermind_\n")
		return b.String()
	}

	for _, r := range results {
		writeResultSection(&b, r, query, expand)
	}
	return b.String()
}

// writeResultSection renders a single result block. Each block starts
// with an H3 so context-mode's markdown chunker treats it as its own
// indexable section.
//
// Body verbosity is controlled by expand:
//   - false → BodyExcerpt (L2: topic+section+match window, ≤~200 tokens)
//   - true  → full body verbatim (L3: unbounded)
func writeResultSection(b *strings.Builder, r Result, query string, expand bool) {
	slug := filepath.Base(r.Ref.Path)
	slug = strings.TrimSuffix(slug, ".md")

	scopeLabel := string(r.Ref.Scope)
	if r.Ref.Pending {
		scopeLabel += ":pending"
	}

	fmt.Fprintf(b, "### [%s] %s · %s · %s\n", scopeLabel, slug, r.Metadata.Kind, r.Metadata.Date)
	fmt.Fprintf(b, "**topic**: %s\n", r.Metadata.Topic)
	if len(r.Metadata.Tags) > 0 {
		fmt.Fprintf(b, "**tags**: %s\n", strings.Join(r.Metadata.Tags, ", "))
	}
	if r.Metadata.Project != "" {
		fmt.Fprintf(b, "**project**: %s\n", r.Metadata.Project)
	}
	// Always include the path so callers can Read the full file (L3)
	// without needing to re-search.
	if r.Ref.Path != "" {
		fmt.Fprintf(b, "**path**: %s\n", r.Ref.Path)
	}
	b.WriteByte('\n')

	body := strings.TrimSpace(r.Body)
	if body != "" {
		var bodyText string
		if expand {
			bodyText = body
		} else {
			bodyText = BodyExcerpt(body, query)
		}
		b.WriteString(bodyText)
		b.WriteString("\n\n")
	}
}
