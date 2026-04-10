package extract

import (
	"encoding/json"
	"strings"
)

// NormalizeTranscript detects Claude Code JSONL transcripts and returns
// a plaintext concatenation of user/assistant prose only. Tool calls,
// tool results, attachments, and system lines are stripped. If the
// input does not look like JSONL (first non-empty line is not a JSON
// object with a "type" field), the input is returned unchanged —
// plain-text inputs pass through untouched so this is safe to call
// unconditionally on every transcript path.
//
// Rationale: the keyword extractor scans text for signal phrases. When
// fed raw Claude Code JSONL, it matches phrases inside tool_use_id
// strings, tool_result content, and other structural JSON. The Phase 3
// polish audit showed 10/10 false-positive extractions came from this.
// Normalizing to prose before extraction eliminates that class of noise
// and gives the regex tier a clean signal to scan.
//
// The output format is "<Role>: <text>\n\n" per turn, with Role in
// {"User", "Assistant"}. The role prefix prevents adjacent turns from
// smashing together into one paragraph, which matters for the context
// windows the keyword extractor uses around matches.
func NormalizeTranscript(raw string) string {
	if !looksLikeJSONL(raw) {
		return raw
	}

	var out strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var env jsonlEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			continue // skip malformed lines silently
		}
		if env.Type != "user" && env.Type != "assistant" {
			continue
		}
		text := extractMessageText(env.Message)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if env.Type == "user" {
			out.WriteString("User: ")
		} else {
			out.WriteString("Assistant: ")
		}
		out.WriteString(text)
		out.WriteString("\n\n")
	}
	return out.String()
}

// looksLikeJSONL reports whether the first non-empty line decodes as a
// JSON object with a "type" field. Cheap — decodes one line, no more.
func looksLikeJSONL(raw string) bool {
	for _, line := range strings.SplitN(raw, "\n", 32) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var probe jsonlEnvelope
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return false
		}
		return probe.Type != ""
	}
	return false
}

// jsonlEnvelope is the minimal shape of a Claude Code .jsonl line we
// care about: the record type and the inline message payload.
type jsonlEnvelope struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

// extractMessageText pulls prose out of the .message field. Claude
// encodes content two ways: as a bare string (common for simple user
// turns) or as an array of typed blocks (assistant turns, and user
// turns that carry tool_result blocks). Only blocks of type "text"
// contribute to the output; tool_use, tool_result, and image blocks
// are dropped.
func extractMessageText(msg json.RawMessage) string {
	if len(msg) == 0 {
		return ""
	}
	var wrapper struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(msg, &wrapper); err != nil || len(wrapper.Content) == 0 {
		return ""
	}
	// Try string first — most user turns.
	var asString string
	if err := json.Unmarshal(wrapper.Content, &asString); err == nil {
		return asString
	}
	// Fall back to array of content blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}
	if err := json.Unmarshal(wrapper.Content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
