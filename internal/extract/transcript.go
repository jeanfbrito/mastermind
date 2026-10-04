package extract

import (
	"encoding/json"
	"strings"
)

// NormalizeTranscript detects agent JSONL transcripts and returns a
// plaintext concatenation of user/assistant prose only. Tool calls,
// tool results, attachments, reasoning, and system lines are stripped.
// If the input does not look like a known JSONL transcript, the input
// is returned unchanged — plain-text inputs pass through untouched so
// this is safe to call unconditionally on every transcript path.
//
// Three line shapes are recognised:
//
//   - Claude Code: {"type":"user"|"assistant","message":{...}}. Lines
//     with "isMeta":true (injected skill bodies, caveats) are dropped.
//   - Cursor agent: {"role":"user"|"assistant","message":{...}}.
//   - ACP (Grok Build and other Agent Client Protocol hosts):
//     {"method":"session/update","params":{"update":{...}}}. Only
//     user_message_chunk and agent_message_chunk carry prose;
//     agent_thought_chunk is the agent's private reasoning and is
//     dropped with tool calls and hook events. Consecutive chunks of
//     the same role are joined into one turn.
//
// Rationale: the keyword extractor scans text for signal phrases. When
// fed raw JSONL, it matches phrases inside tool_use_id strings,
// tool_result content, and other structural JSON. The Phase 3 polish
// audit showed 10/10 false-positive extractions came from this, and
// the 2026-10-04 cleanup found 92 open loops whose bodies were raw
// Cursor and ACP JSON that this function used to pass through.
//
// The output format is "<Role>: <text>\n\n" per turn, with Role in
// {"User", "Assistant"}. The role prefix prevents adjacent turns from
// smashing together into one paragraph, which matters for the context
// windows the keyword extractor uses around matches.
func NormalizeTranscript(raw string) string {
	if !looksLikeJSONL(raw) {
		return raw
	}

	var turns []turn
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var env jsonlEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			continue // skip malformed lines silently
		}
		role, text, chunk := env.prose()
		if role == "" || strings.TrimSpace(text) == "" {
			continue
		}
		if chunk && len(turns) > 0 {
			last := &turns[len(turns)-1]
			if last.chunk && last.role == role {
				last.text += text
				continue
			}
		}
		turns = append(turns, turn{role: role, text: text, chunk: chunk})
	}

	var out strings.Builder
	for _, t := range turns {
		out.WriteString(t.role)
		out.WriteString(": ")
		out.WriteString(t.text)
		out.WriteString("\n\n")
	}
	return out.String()
}

// turn is one speaker's prose in the normalized output. chunk marks a
// turn built from streamed ACP chunks, which later chunks may extend.
type turn struct {
	role  string
	text  string
	chunk bool
}

// looksLikeJSONL reports whether the first non-empty line decodes as a
// JSON object in one of the recognised transcript shapes. Cheap —
// decodes one line, no more.
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
		return probe.Type != "" || probe.Role != "" || probe.Method != ""
	}
	return false
}

// jsonlEnvelope is the union of the transcript line shapes we read:
// Claude Code (type + message), Cursor (role + message) and ACP
// (method + params).
type jsonlEnvelope struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	IsMeta  bool            `json:"isMeta"`
	Message json.RawMessage `json:"message"`
	Method  string          `json:"method"`
	Params  struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Content       struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"update"`
	} `json:"params"`
}

// prose returns the speaker ("User" or "Assistant") and text of one
// transcript line, or an empty role when the line carries no prose.
// chunk reports a streamed ACP chunk.
func (e jsonlEnvelope) prose() (role, text string, chunk bool) {
	switch {
	case e.Method != "":
		if !strings.HasSuffix(e.Method, "session/update") {
			return "", "", false
		}
		u := e.Params.Update
		if u.Content.Type != "text" {
			return "", "", false
		}
		switch u.SessionUpdate {
		case "user_message_chunk":
			return "User", u.Content.Text, true
		case "agent_message_chunk":
			return "Assistant", u.Content.Text, true
		}
		return "", "", false
	case e.Type != "":
		if e.IsMeta {
			return "", "", false
		}
		return speaker(e.Type), extractMessageText(e.Message), false
	default:
		return speaker(e.Role), extractMessageText(e.Message), false
	}
}

// speaker maps a transcript role to its output label.
func speaker(role string) string {
	switch role {
	case "user":
		return "User"
	case "assistant":
		return "Assistant"
	}
	return ""
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
