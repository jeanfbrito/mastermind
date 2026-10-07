package search

import (
	"strings"
	"unicode/utf8"
)

// shortBodyThreshold is the character count below which we return the
// body verbatim rather than trimming it — trimming a short body saves
// nothing and loses context.
const shortBodyThreshold = 800

// maxExcerptChars caps every excerpt. Windows are picked by line, and a
// single line can be huge: an extractor once stored raw transcript JSON
// as lines of 100K+ characters, and three such results made one
// mm_search response 250K characters long.
const maxExcerptChars = 2 * shortBodyThreshold

// BodyExcerpt returns a trimmed view of body for L2 mm_search responses.
//
// Rules (in priority order):
//  1. If body is shorter than shortBodyThreshold, return it verbatim.
//  2. If query is non-empty and matches a line in the body, return that
//     line plus ±3 lines of context (match-anchored excerpt).
//  3. If no match (pure topic/tag hit with no body match) or query is
//     empty, return the first ## section (header + its content up to
//     the next ## or end of body).
//  4. If the body has no ## sections, return the first shortBodyThreshold
//     chars broken at a word boundary.
//
// Rules 2 and 3 are clipped to maxExcerptChars; an oversized match
// window shrinks to that many characters around the match.
//
// This is NOT a summarizer — it never paraphrases. It is a window
// selector: pick the most informative window into the existing text.
//
// Callers that need the full body set expand=true; this function is
// only called when expand=false.
func BodyExcerpt(body, query string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	// Rule 1: short body → return verbatim.
	if len(body) <= shortBodyThreshold {
		return body
	}

	lines := strings.Split(body, "\n")

	// Rule 2: query match → ±3-line context window.
	if query != "" {
		tokens := tokenize(query)
		if idx := findMatchLine(lines, tokens); idx >= 0 {
			if window := contextWindow(lines, idx, 3); len(window) <= maxExcerptChars {
				return window
			}
			return clipAround(lines[idx], tokens, maxExcerptChars)
		}
	}

	// Rule 3: first ## section.
	if sec := firstSection(lines); sec != "" {
		return wordTrim(sec, maxExcerptChars)
	}

	// Rule 4: fallback — word-boundary trim.
	return wordTrim(body, shortBodyThreshold)
}

// findMatchLine returns the index of the first line in lines that
// contains any of the query tokens (case-insensitive). Returns -1 if
// no line matches.
func findMatchLine(lines []string, tokens []string) int {
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, tok := range tokens {
			if tok != "" && strings.Contains(lower, tok) {
				return i
			}
		}
	}
	return -1
}

// contextWindow returns lines[center-radius..center+radius] joined by
// newlines, clamped to valid indices. If the window would include a ##
// header line that isn't center itself, we clip at the header to avoid
// crossing section boundaries — the reader can use expand:true for
// multi-section views.
func contextWindow(lines []string, center, radius int) string {
	start := center - radius
	if start < 0 {
		start = 0
	}
	end := center + radius
	if end >= len(lines) {
		end = len(lines) - 1
	}

	window := lines[start : end+1]
	return strings.Join(window, "\n")
}

// firstSection returns the content of the first ## section in lines:
// the header line itself plus all lines until the next ## header or
// end of body. Returns "" if no ## section exists.
func firstSection(lines []string) string {
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(lines[start])
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			break
		}
		sb.WriteByte('\n')
		sb.WriteString(lines[i])
	}
	return strings.TrimSpace(sb.String())
}

// wordTrim truncates body at n characters, breaking at a word boundary.
// Appends an ellipsis only when truncation occurred.
func wordTrim(body string, n int) string {
	if len(body) <= n {
		return body
	}
	cut := body[:n]
	if idx := strings.LastIndexAny(cut, " \t\n"); idx > n/2 {
		cut = cut[:idx]
	}
	return strings.TrimRight(cut, " \t\n") + "…"
}

// clipAround returns about n characters of line centred on the first
// query token it contains, with an ellipsis on each clipped side.
func clipAround(line string, tokens []string, n int) string {
	if len(line) <= n {
		return line
	}
	lower := strings.ToLower(line)
	at := 0
	for _, tok := range tokens {
		if i := strings.Index(lower, tok); tok != "" && i >= 0 {
			at = i
			break
		}
	}
	start := at - n/2
	if start < 0 {
		start = 0
	}
	end := start + n
	if end > len(line) {
		end = len(line)
		start = end - n
	}
	for start > 0 && !utf8.RuneStart(line[start]) {
		start--
	}
	for end < len(line) && !utf8.RuneStart(line[end]) {
		end--
	}
	clip := line[start:end]
	if start > 0 {
		clip = "…" + clip
	}
	if end < len(line) {
		clip += "…"
	}
	return clip
}
