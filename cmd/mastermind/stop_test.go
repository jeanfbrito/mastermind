package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// replaceStdinWithPipe writes payload to a pipe and redirects
// os.Stdin to read from it. Used by tests that simulate Claude
// Code passing Stop hook JSON on stdin.
func replaceStdinWithPipe(t *testing.T, payload string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(payload); err != nil {
		t.Fatal(err)
	}
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		r.Close()
	})
}

func TestRunStop_AppendsJSONLRecord(t *testing.T) {
	home := withFakeHome(t)

	payload := `{"session_id":"sess-abc","stop_reason":"end_turn","message_count":12,"input_tokens":8423,"output_tokens":1205,"cwd":"/tmp/work"}`
	replaceStdinWithPipe(t, payload)

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	logPath := filepath.Join(home, ".knowledge", "logs", "sessions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1:\n%s", len(lines), data)
	}

	var rec stopLogRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, lines[0])
	}
	if rec.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want sess-abc", rec.SessionID)
	}
	if rec.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", rec.StopReason)
	}
	if rec.MessageCount != 12 {
		t.Errorf("MessageCount = %d, want 12", rec.MessageCount)
	}
	if rec.InputTokens != 8423 {
		t.Errorf("InputTokens = %d, want 8423", rec.InputTokens)
	}
	if rec.OutputTokens != 1205 {
		t.Errorf("OutputTokens = %d, want 1205", rec.OutputTokens)
	}
	if rec.Short {
		t.Errorf("Short = true, want false (message_count=12 >= 4)")
	}
	if rec.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
	if rec.Cwd != "/tmp/work" {
		t.Errorf("Cwd = %q, want /tmp/work", rec.Cwd)
	}
}

func TestRunStop_ShortTurnFlagged(t *testing.T) {
	home := withFakeHome(t)

	// message_count below threshold (4) should get Short=true.
	payload := `{"session_id":"sess-xyz","stop_reason":"end_turn","message_count":2,"input_tokens":100,"output_tokens":50}`
	replaceStdinWithPipe(t, payload)

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	logPath := filepath.Join(home, ".knowledge", "logs", "sessions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var rec stopLogRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !rec.Short {
		t.Errorf("Short = false, want true (message_count=2 < 4)")
	}
}

func TestRunStop_AppendsAcrossInvocations(t *testing.T) {
	home := withFakeHome(t)

	for i := 0; i < 3; i++ {
		payload := `{"session_id":"sess-multi","stop_reason":"end_turn","message_count":5,"input_tokens":10,"output_tokens":5}`
		replaceStdinWithPipe(t, payload)
		if err := runStop(); err != nil {
			t.Fatalf("runStop iter %d: %v", i, err)
		}
	}

	logPath := filepath.Join(home, ".knowledge", "logs", "sessions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3 (append should accumulate)", len(lines))
	}
}

func TestRunStop_RespectsNoAutoInit(t *testing.T) {
	home := withFakeHome(t)
	t.Setenv("MASTERMIND_NO_AUTO_INIT", "1")

	payload := `{"session_id":"sess-no-init","stop_reason":"end_turn","message_count":5,"input_tokens":10,"output_tokens":5}`
	replaceStdinWithPipe(t, payload)

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	// Log directory must NOT have been created.
	logsDir := filepath.Join(home, ".knowledge", "logs")
	if _, err := os.Stat(logsDir); !os.IsNotExist(err) {
		t.Errorf("logs dir was created despite MASTERMIND_NO_AUTO_INIT: err=%v", err)
	}
}

func TestRunStop_EmptyStdinDoesNotError(t *testing.T) {
	// A Stop hook with empty stdin (manual test invocation, or a
	// Claude Code version that doesn't send JSON) should succeed
	// silently and still write a record with zero-value fields.
	home := withFakeHome(t)
	replaceStdinWithPipe(t, "")

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	logPath := filepath.Join(home, ".knowledge", "logs", "sessions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var rec stopLogRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// With empty stdin, message_count is 0 — must still be flagged
	// as short so analytics can filter it out.
	if !rec.Short {
		t.Error("empty stdin: Short = false, want true")
	}
	if rec.Timestamp == "" {
		t.Error("empty stdin: Timestamp should still be populated")
	}
}

func TestRunStop_MalformedJSONDoesNotError(t *testing.T) {
	// Malformed JSON should fall through to a zero-value record
	// rather than blowing up. Stop hook errors would spam stderr
	// on every turn and that is NOT acceptable for a silent hook.
	withFakeHome(t)
	replaceStdinWithPipe(t, "{not valid json")

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}
	// Log file may or may not exist; the contract is "don't error".
}
