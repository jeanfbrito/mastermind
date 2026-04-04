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
//
//	<body excerpt, first ~500 chars>
//
// A header per result is critical — it's what context-mode's chunker
// uses to separate sections.
func FormatResultsMarkdown(query string, results []Result) string {
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
		writeResultSection(&b, r)
	}
	return b.String()
}

// writeResultSection renders a single result block. Each block starts
// with an H3 so context-mode's markdown chunker treats it as its own
// indexable section.
func writeResultSection(b *strings.Builder, r Result) {
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
	b.WriteByte('\n')

	body := strings.TrimSpace(r.Body)
	if body != "" {
		b.WriteString(excerpt(body, 500))
		b.WriteString("\n\n")
	}
}

// excerpt returns the first N characters of body, broken at a word
// boundary if possible. If body is shorter than n, returns it verbatim.
// Appends an ellipsis only if truncation actually happened.
func excerpt(body string, n int) string {
	body = strings.TrimSpace(body)
	if len(body) <= n {
		return body
	}
	// Truncate at n, then back up to the last whitespace to avoid
	// cutting a word in half.
	cut := body[:n]
	if idx := strings.LastIndexAny(cut, " \t\n"); idx > n/2 {
		cut = cut[:idx]
	}
	return strings.TrimRight(cut, " \t\n") + "…"
}
