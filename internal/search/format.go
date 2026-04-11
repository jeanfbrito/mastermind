package search

import (
	"fmt"
	"path/filepath"
	"sort"
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

	// Build a topic → set of scopes map for the cross-scope "tunnel"
	// annotation. A topic that appears in more than one scope among
	// the returned results is a "tunnel" in mempalace's sense —
	// conceptual bridge that cuts across domains, worth flagging so
	// the reader recognizes "this lesson applies everywhere, not just
	// one project". Pending variants collapse into their base scope
	// (user-personal:pending ≡ user-personal) so an unreviewed
	// candidate in the same scope doesn't count as cross-scope.
	//
	// Topic keys are normalized (lowercased, trimmed) so trivial
	// formatting variance ("Go modules" vs "go modules") doesn't
	// prevent the match.
	topicScopes := make(map[string]map[string]bool, len(results))
	for _, r := range results {
		key := normalizeTopicKey(r.Metadata.Topic)
		if key == "" {
			continue
		}
		if topicScopes[key] == nil {
			topicScopes[key] = make(map[string]bool)
		}
		topicScopes[key][string(r.Ref.Scope)] = true
	}

	for _, r := range results {
		writeResultSection(&b, r, query, expand, topicScopes)
	}
	return b.String()
}

// normalizeTopicKey lowercases and trims a topic string for
// cross-scope equality matching. Empty after trimming → empty key,
// which the caller treats as "skip" (no annotation).
func normalizeTopicKey(topic string) string {
	return strings.ToLower(strings.TrimSpace(topic))
}

// crossScopeAnnotation returns the " [cross-scope: also in X, Y]"
// suffix for a result whose topic appears in more than one scope
// among the returned set, or empty string if the topic is
// scope-unique. The listed scopes exclude the result's own scope
// and are sorted for deterministic output.
//
// Inspired by mempalace's palace_graph.find_tunnels() (palace_graph.py:161),
// which identifies topics that span multiple wings as "tunnels" —
// conceptual bridges between domains. Mastermind needs none of the
// graph-traversal machinery because the flat markdown store makes
// topic-name matching a single-pass map lookup.
func crossScopeAnnotation(r Result, topicScopes map[string]map[string]bool) string {
	key := normalizeTopicKey(r.Metadata.Topic)
	if key == "" {
		return ""
	}
	scopes := topicScopes[key]
	if len(scopes) < 2 {
		return ""
	}
	current := string(r.Ref.Scope)
	others := make([]string, 0, len(scopes)-1)
	for s := range scopes {
		if s != current {
			others = append(others, s)
		}
	}
	if len(others) == 0 {
		return ""
	}
	sort.Strings(others)
	return fmt.Sprintf(" [cross-scope: also in %s]", strings.Join(others, ", "))
}

// writeResultSection renders a single result block. Each block starts
// with an H3 so context-mode's markdown chunker treats it as its own
// indexable section.
//
// Body verbosity is controlled by expand:
//   - false → BodyExcerpt (L2: topic+section+match window, ≤~200 tokens)
//   - true  → full body verbatim (L3: unbounded)
//
// topicScopes is the shared topic→scopes map used to emit the
// cross-scope "tunnel" annotation on entries whose topic appears in
// more than one scope.
func writeResultSection(b *strings.Builder, r Result, query string, expand bool, topicScopes map[string]map[string]bool) {
	slug := filepath.Base(r.Ref.Path)
	slug = strings.TrimSuffix(slug, ".md")

	scopeLabel := string(r.Ref.Scope)
	if r.Ref.Pending {
		scopeLabel += ":pending"
	}

	// Cross-scope "tunnel" suffix — orthogonal to the contradicts
	// Annotation, so a co-retrieved contradicts target that also
	// appears in a second scope surfaces both markers.
	crossScope := crossScopeAnnotation(r, topicScopes)

	if r.Annotation != "" {
		// Co-retrieved entries (e.g. contradicts targets) carry an
		// inline tag on the heading so the reader can immediately
		// see why the entry surfaced — it's a relationship hit,
		// not a keyword match.
		fmt.Fprintf(b, "### [%s] %s · %s · %s · (%s)%s\n", scopeLabel, slug, r.Metadata.Kind, r.Metadata.Date, r.Annotation, crossScope)
	} else {
		fmt.Fprintf(b, "### [%s] %s · %s · %s%s\n", scopeLabel, slug, r.Metadata.Kind, r.Metadata.Date, crossScope)
	}
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
