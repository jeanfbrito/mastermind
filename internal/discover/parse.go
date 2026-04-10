package discover

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// discoveredEntry is the JSON schema the LLM is asked to produce.
type discoveredEntry struct {
	Topic    string   `json:"topic"`
	Kind     string   `json:"kind"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"`
	Source   string   `json:"source"`
}

// parseResponse parses the LLM's JSON output into entries, deduplicating
// against existingTopics. The source field is appended as a ## Source
// section in the body for provenance tracking and incremental cursor.
func parseResponse(raw, projectName string, existingTopics []string) ([]format.Entry, error) {
	// The LLM might wrap JSON in markdown code fences.
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[idx:]
	}
	if idx := strings.LastIndex(raw, "]"); idx >= 0 {
		raw = raw[:idx+1]
	}

	var extracted []discoveredEntry
	if err := json.Unmarshal([]byte(raw), &extracted); err != nil {
		return nil, fmt.Errorf("parse discovery response: %w", err)
	}

	existingLower := make(map[string]bool)
	for _, t := range existingTopics {
		existingLower[strings.ToLower(t)] = true
	}

	project := projectName
	if project == "" {
		project = "general"
	}

	var entries []format.Entry
	for _, e := range extracted {
		if e.Topic == "" || e.Body == "" {
			continue
		}

		kind := format.Kind(e.Kind)
		if !kind.Valid() {
			kind = format.KindLesson
		}

		topicLower := strings.ToLower(e.Topic)
		if isDuplicate(topicLower, existingLower) {
			continue
		}
		// Add to dedup set so later entries in the same batch don't duplicate.
		existingLower[topicLower] = true

		body := strings.TrimSpace(e.Body)
		if e.Source != "" {
			body += "\n\n## Source\n" + e.Source
		}

		entries = append(entries, format.Entry{
			Metadata: format.Metadata{
				Date:       time.Now().UTC().Format("2006-01-02"),
				Project:    project,
				Tags:       e.Tags,
				Topic:      e.Topic,
				Kind:       kind,
				Category:   e.Category,
				Confidence: format.ConfidenceMedium,
			},
			Body: body,
		})
	}

	return entries, nil
}

// isDuplicate checks if a topic closely matches any existing topic
// using simple substring containment.
func isDuplicate(topicLower string, existing map[string]bool) bool {
	for ex := range existing {
		if strings.Contains(topicLower, ex) || strings.Contains(ex, topicLower) {
			return true
		}
	}
	return false
}
