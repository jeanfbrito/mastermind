package extract

import (
	"regexp"
	"strings"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// KeywordExtractor scans transcripts for patterns that indicate
// extractable knowledge: bug fixes, lessons, decisions, open loops.
// Zero dependencies — uses only stdlib regex.
type KeywordExtractor struct {
	ProjectName string
}

// pattern associates a regex with the kind of entry it indicates
// and a base confidence for that signal's precision.
type pattern struct {
	re         *regexp.Regexp
	kind       format.Kind
	confidence format.Confidence
}

// patterns are compiled once and reused across extractions. Each
// pattern matches a signal phrase that commonly appears when someone
// discovers, decides, or resolves something.
//
// confidence reflects how precisely each phrase predicts extractable
// knowledge: clear first-person statements ("I'll use", "found that")
// are medium; ambiguous connectives ("because", "going to") are low.
var patterns = []pattern{
	// Lessons and fixes
	{re: regexp.MustCompile(`(?i)(?:the (?:fix|solution|answer) was|solved by|the trick is|fixed it by)`), kind: format.KindLesson, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)(?:root cause|the real (?:issue|problem) was|turned out to be|the actual reason)`), kind: format.KindLesson, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)(?:lesson learned|key takeaway|next time|note to self|remember to)`), kind: format.KindLesson, confidence: format.ConfidenceMedium},

	// War stories
	{re: regexp.MustCompile(`(?i)(?:wasted (?:time|hours)|hours debugging|painful|nightmare|kept failing|going in circles)`), kind: format.KindWarStory, confidence: format.ConfidenceMedium},

	// Decisions — soulforge WSM phrases + existing patterns.
	// "we should" is intentionally left in open-loop (below) because
	// it more often signals a future task than a settled decision.
	{re: regexp.MustCompile(`(?i)(?:decided to|decision:|we chose|the tradeoff|chose .+ over .+|went with)`), kind: format.KindDecision, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\bi'll use\b`), kind: format.KindDecision, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\bi'll go with\b`), kind: format.KindDecision, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\blet's use\b`), kind: format.KindDecision, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\bthe plan is\b`), kind: format.KindDecision, confidence: format.ConfidenceMedium},
	// "going to" is common in casual speech; lower precision.
	{re: regexp.MustCompile(`(?i)\bgoing to\b`), kind: format.KindDecision, confidence: format.ConfidenceLow},
	// "because" as a standalone connective catches rationale sentences
	// but has very low precision — keep confidence low.
	{re: regexp.MustCompile(`(?i)\bbecause\b`), kind: format.KindDecision, confidence: format.ConfidenceLow},

	// Patterns
	{re: regexp.MustCompile(`(?i)(?:the pattern is|always do|never do|rule:|best practice|the right way to)`), kind: format.KindPattern, confidence: format.ConfidenceMedium},

	// Open loops
	{re: regexp.MustCompile(`(?i)(?:TODO:|we should|need to .+ later|open loop|come back to|still need to|haven't .+ yet)`), kind: format.KindOpenLoop, confidence: format.ConfidenceMedium},

	// Insights — original phrases plus soulforge WSM discovery additions.
	// "root cause" is kept above as KindLesson (a root-cause finding is a lesson).
	// "realized that", "discovered that", "turns out" stay in the omnibus regex below;
	// new additions ("found that", "the issue was", "discovered", "it seems") are
	// separate entries so they don't duplicate matches.
	{re: regexp.MustCompile(`(?i)(?:realized that|discovered that|turns out|interesting(?:ly)?:|surprisingly|non-obvious)`), kind: format.KindInsight, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\bfound that\b`), kind: format.KindInsight, confidence: format.ConfidenceMedium},
	{re: regexp.MustCompile(`(?i)\bthe issue was\b`), kind: format.KindInsight, confidence: format.ConfidenceMedium},
	// "discovered" without "that" catches standalone usage ("I discovered a bug").
	// Word-boundary anchored to avoid "undiscovered", "rediscovered" false positives.
	{re: regexp.MustCompile(`(?i)\bdiscovered\b`), kind: format.KindInsight, confidence: format.ConfidenceMedium},
	// "it seems" is hedging language — lower precision than clear assertions.
	{re: regexp.MustCompile(`(?i)\bit seems\b`), kind: format.KindInsight, confidence: format.ConfidenceLow},
}

// Extract scans the transcript for pattern matches, groups surrounding
// context into entries, and deduplicates against existing topics.
func (k *KeywordExtractor) Extract(transcript string, existingTopics []string) ([]format.Entry, error) {
	if strings.TrimSpace(transcript) == "" {
		return nil, nil
	}

	lines := strings.Split(transcript, "\n")
	var entries []format.Entry
	seen := make(map[string]bool)

	// Also track existing topics for dedup.
	existingLower := make(map[string]bool)
	for _, t := range existingTopics {
		existingLower[strings.ToLower(t)] = true
	}

	for _, pat := range patterns {
		for i, line := range lines {
			if !pat.re.MatchString(line) {
				continue
			}

			// Extract context: 3 lines before and 5 lines after the match.
			start := i - 3
			if start < 0 {
				start = 0
			}
			end := i + 6
			if end > len(lines) {
				end = len(lines)
			}
			context := strings.Join(lines[start:end], "\n")

			// Derive topic from the matched line. Clean up prefixes.
			topic := deriveTopic(line)
			if topic == "" {
				continue
			}

			// Dedup: skip if we've already extracted this topic or
			// if it closely matches an existing entry.
			topicKey := strings.ToLower(topic)
			if seen[topicKey] {
				continue
			}
			if isDuplicate(topicKey, existingLower) {
				continue
			}
			seen[topicKey] = true

			project := k.ProjectName
			if project == "" {
				project = "general"
			}

			entries = append(entries, format.Entry{
				Metadata: format.Metadata{
					Date:       time.Now().UTC().Format("2006-01-02"),
					Project:    project,
					Topic:      topic,
					Kind:       pat.kind,
					Confidence: pat.confidence,
				},
				Body: strings.TrimSpace(context),
			})
		}
	}

	return entries, nil
}

// deriveTopic extracts a topic string from the matched line.
// Strips common prefixes, trims whitespace, and caps length.
func deriveTopic(line string) string {
	// Remove common assistant/user prefixes from transcript formats.
	line = strings.TrimSpace(line)

	// Remove markdown formatting.
	line = strings.TrimLeft(line, "#*->• ")

	// Remove JSON-ish prefixes that might appear in structured transcripts.
	for _, prefix := range []string{"\"content\":", "\"text\":", "assistant:", "user:"} {
		if idx := strings.Index(strings.ToLower(line), prefix); idx >= 0 {
			line = line[idx+len(prefix):]
		}
	}

	line = strings.TrimSpace(line)
	line = strings.Trim(line, "\"',.")

	if len(line) < 10 {
		return ""
	}
	if len(line) > 120 {
		// Truncate at word boundary.
		if idx := strings.LastIndex(line[:120], " "); idx > 60 {
			line = line[:idx]
		} else {
			line = line[:120]
		}
	}

	return line
}

// isDuplicate checks if a topic closely matches any existing topic.
// Uses simple substring containment — if either contains the other,
// it's considered a duplicate.
func isDuplicate(topicLower string, existing map[string]bool) bool {
	for ex := range existing {
		if strings.Contains(topicLower, ex) || strings.Contains(ex, topicLower) {
			return true
		}
	}
	return false
}
