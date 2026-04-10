package main

import (
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
}

// shortTurnThreshold is the message count below which a session
// turn is considered a "short clarification" rather than a
// substantive exchange. Borrowed from shiba-memory's Stop hook
// which uses 4 as the gate for expensive LLM work. Below this
// threshold, mastermind still logs the entry but flags it with
// "short": true so future analysis can filter it out.
const shortTurnThreshold = 4

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
