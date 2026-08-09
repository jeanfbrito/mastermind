package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stopHookInput is the JSON structure Claude Code sends to Stop
// hooks on stdin. This is the ONLY data available — Claude Code
// does NOT pass the assistant's response text, so any ambition to
// detect "unresolved open-loops from what the assistant just said"
// is impossible under the current hook contract. See the mining
// update at .knowledge/hooks/phase-5-experiment-stop-hook-*.md
// for the blocker details.
type stopHookInput struct {
	SessionID    string `json:"session_id"`
	StopReason   string `json:"stop_reason"`
	MessageCount int    `json:"message_count"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Cwd          string `json:"cwd,omitempty"` // optional; some Claude Code versions include it

	// TranscriptPath is what Claude Code's real Stop hook payload
	// actually carries — input_tokens/output_tokens above are NOT
	// present on real payloads (they were an early assumption that
	// turned out wrong; kept for backward compat with any caller
	// that does send them directly). When TranscriptPath is set,
	// runStop derives real usage numbers by summing the transcript's
	// per-API-call usage blocks instead. See readTranscriptUsage.
	TranscriptPath string `json:"transcript_path,omitempty"`
}

// stopLogRecord is the JSONL line written to sessions.jsonl.
// Keys match the stdin shape plus a timestamp and a "short"
// flag that distinguishes trivial clarification turns from
// substantive ones. Field names are stable — this file is meant
// to be parsed by future tooling.
type stopLogRecord struct {
	Timestamp    string `json:"timestamp"`
	SessionID    string `json:"session_id,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	MessageCount int    `json:"message_count"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Short        bool   `json:"short,omitempty"`
	Cwd          string `json:"cwd,omitempty"`

	// Real-usage fields, populated only when transcript_path was
	// present and readable. Stop fires once per assistant turn, and
	// the transcript accumulates every turn of the session, so these
	// are cumulative session-to-date totals, NOT per-turn deltas —
	// a session with 10 turns writes 10 records each carrying the
	// running total up to that point. Analysis that wants per-turn
	// cost must diff consecutive records for the same session_id.
	RealInputTokens     int `json:"real_input_tokens,omitempty"`
	RealOutputTokens    int `json:"real_output_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
	APICalls            int `json:"api_calls,omitempty"`
}

// shortTurnThreshold is the message count below which a session
// turn is considered a "short clarification" rather than a
// substantive exchange. Borrowed from shiba-memory's Stop hook
// which uses 4 as the gate for expensive LLM work. Below this
// threshold, mastermind still logs the entry but flags it with
// "short": true so future analysis can filter it out.
const shortTurnThreshold = 4

// transcriptLine is the minimal shape mastermind cares about from a
// Claude Code transcript JSONL line. Transcript lines are large,
// nested objects (thinking blocks, tool_use payloads, full message
// history) — this struct decodes only the two fields that matter
// and lets json.Unmarshal silently ignore everything else.
//
// Usage is left as a json.RawMessage rather than a concrete struct
// so presence can be distinguished from absence: user-role lines and
// some assistant lines (e.g. pure text deltas) have no "usage" key
// at all, and that must not be confused with a zero-value usage.
type transcriptLine struct {
	Message struct {
		ID    string          `json:"id"`
		Usage json.RawMessage `json:"usage"`
	} `json:"message"`
}

// transcriptUsage mirrors the subset of Anthropic's usage block that
// mastermind sums. Verified against a real local transcript
// (~/.claude/projects/*/*.jsonl): assistant lines carry
// message.usage.{input_tokens,output_tokens,cache_creation_input_tokens,
// cache_read_input_tokens} alongside fields mastermind doesn't need
// (server_tool_use, service_tier, cache_creation, iterations, ...).
type transcriptUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// readTranscriptUsage sums real token usage across every assistant
// API call recorded in the transcript at path. It returns ok=false
// (with all-zero totals) whenever the transcript can't be read or
// contains no usage data at all — callers must treat that as "no
// real-usage data available," never as an error worth surfacing.
//
// Streaming responses can emit several transcript lines that share
// the same message.id (one per delta), each repeating the *same*
// usage block for that API call. Summing every line would massively
// over-count, so — following the pattern used by other Claude Code
// tooling that parses these transcripts (openwolf's
// readTranscriptUsage) — this keeps only the *last* usage seen per
// message id and sums those instead. Lines with no id are treated as
// distinct one-off calls via an incrementing anonymous key.
//
// Reading uses bufio.Reader.ReadBytes('\n') rather than
// bufio.Scanner: transcript lines can carry multi-megabyte tool
// results, and Scanner's default 64KB token buffer would silently
// truncate (or error on) such lines. ReadBytes has no such cap — it
// grows its internal buffer to fit whatever line it's given — and,
// unlike a single json.Decoder streaming over the whole file, a
// malformed line here is just skipped: decoding line-by-line means
// one bad line can't desync parsing of the lines that follow it.
func readTranscriptUsage(path string) (usage transcriptUsage, apiCalls int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return transcriptUsage{}, 0, false
	}
	defer f.Close()

	byID := make(map[string]transcriptUsage)
	anon := 0
	reader := bufio.NewReader(f)
	for {
		raw, readErr := reader.ReadBytes('\n')
		line := bytes.TrimSpace(raw)
		if len(line) > 0 {
			var entry transcriptLine
			// Best-effort: a malformed line (partial write, non-JSON
			// noise) is skipped, not fatal — the rest of the
			// transcript is still worth summing.
			if jsonErr := json.Unmarshal(line, &entry); jsonErr == nil {
				if len(entry.Message.Usage) > 0 && !bytes.Equal(bytes.TrimSpace(entry.Message.Usage), []byte("null")) {
					var u transcriptUsage
					if json.Unmarshal(entry.Message.Usage, &u) == nil {
						key := entry.Message.ID
						if key == "" {
							key = fmt.Sprintf("anon-%d", anon)
							anon++
						}
						byID[key] = u
					}
				}
			}
		}
		if readErr != nil {
			break // EOF, or an I/O error — either way, stop with what we have.
		}
	}

	if len(byID) == 0 {
		return transcriptUsage{}, 0, false
	}
	var total transcriptUsage
	for _, u := range byID {
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.CacheCreationInputTokens += u.CacheCreationInputTokens
		total.CacheReadInputTokens += u.CacheReadInputTokens
	}
	return total, len(byID), true
}

// runStop implements the `mastermind stop` subcommand. It reads
// Claude Code Stop hook JSON from stdin and appends a single JSONL
// line to ~/.knowledge/logs/sessions.jsonl. Scope is intentionally
// tiny: no LLM calls, no extraction, no knowledge writes. This is
// mastermind's first usage-telemetry surface.
//
// The hook contract (timeout 5s) forbids anything expensive. In
// practice this subcommand completes in single-digit milliseconds
// because it only does one stdin decode, one file open, one write.
//
// Respects MASTERMIND_NO_AUTO_INIT: when set, the subcommand skips
// directory creation and the log write entirely but still succeeds
// silently — a CI environment or a user who has deliberately
// excluded mastermind from auto-persistence must not see errors
// from a background hook.
//
// Silent failure is also the default for decode errors and file
// I/O issues: a Stop hook that errors could spam stderr on every
// turn, and the user didn't invoke mastermind — Claude Code did.
// Diagnostic output would be noise.
func runStop() error {
	var input stopHookInput
	// Decode is best-effort. A malformed or empty stdin is common
	// during manual testing and shouldn't error.
	_ = json.NewDecoder(os.Stdin).Decode(&input)

	if os.Getenv("MASTERMIND_NO_AUTO_INIT") != "" {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// No home dir means no telemetry target. Silent success.
		return nil
	}
	logsDir := filepath.Join(home, ".knowledge", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil
	}

	record := stopLogRecord{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		SessionID:    input.SessionID,
		StopReason:   input.StopReason,
		MessageCount: input.MessageCount,
		InputTokens:  input.InputTokens,
		OutputTokens: input.OutputTokens,
		Short:        input.MessageCount < shortTurnThreshold,
		Cwd:          input.Cwd,
	}

	if input.TranscriptPath != "" {
		if usage, apiCalls, ok := readTranscriptUsage(input.TranscriptPath); ok {
			record.RealInputTokens = usage.InputTokens
			record.RealOutputTokens = usage.OutputTokens
			record.CacheCreationTokens = usage.CacheCreationInputTokens
			record.CacheReadTokens = usage.CacheReadInputTokens
			record.APICalls = apiCalls
		}
		// ok == false (missing/unreadable/no-usage transcript) leaves
		// the real-usage fields at their zero values, which omitempty
		// drops from the JSONL line entirely — silent, per the file's
		// no-stderr-spam-from-a-background-hook philosophy.
	}

	logPath := filepath.Join(logsDir, "sessions.jsonl")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	defer f.Close()

	line, err := json.Marshal(record)
	if err != nil {
		return nil
	}
	// Each record is one line of JSONL — single newline terminator.
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return nil
	}
	return nil
}
