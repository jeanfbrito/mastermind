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

func TestRunStop_ReadsRealUsageFromTranscript(t *testing.T) {
	home := withFakeHome(t)

	// A realistic-shaped transcript fixture: a user line (no usage),
	// two assistant lines with distinct message ids carrying usage,
	// and a garbage line that must be skipped without aborting the
	// rest of the sum.
	transcript := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","message":{"id":"msg_1","role":"assistant","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
		`{not valid json at all`,
		`{"type":"assistant","message":{"id":"msg_2","role":"assistant","usage":{"input_tokens":200,"output_tokens":75,"cache_creation_input_tokens":0,"cache_read_input_tokens":20}}}`,
	}, "\n") + "\n"

	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := `{"session_id":"sess-real","stop_reason":"end_turn","message_count":8,"transcript_path":"` + strings.ReplaceAll(transcriptPath, `\`, `\\`) + `"}`
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
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}

	if rec.RealInputTokens != 300 {
		t.Errorf("RealInputTokens = %d, want 300", rec.RealInputTokens)
	}
	if rec.RealOutputTokens != 125 {
		t.Errorf("RealOutputTokens = %d, want 125", rec.RealOutputTokens)
	}
	if rec.CacheCreationTokens != 10 {
		t.Errorf("CacheCreationTokens = %d, want 10", rec.CacheCreationTokens)
	}
	if rec.CacheReadTokens != 25 {
		t.Errorf("CacheReadTokens = %d, want 25", rec.CacheReadTokens)
	}
	if rec.APICalls != 2 {
		t.Errorf("APICalls = %d, want 2", rec.APICalls)
	}
}

func TestRunStop_DedupesRepeatedUsageByMessageID(t *testing.T) {
	home := withFakeHome(t)

	// Streaming can emit multiple transcript lines for the same
	// message id, each repeating the same usage block. Only the last
	// one seen per id must be counted, or usage would be over-summed.
	transcript := strings.Join([]string{
		`{"type":"assistant","message":{"id":"msg_1","usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","message":{"id":"msg_1","usage":{"input_tokens":100,"output_tokens":40,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
	}, "\n") + "\n"

	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := `{"session_id":"sess-dedupe","stop_reason":"end_turn","message_count":8,"transcript_path":"` + strings.ReplaceAll(transcriptPath, `\`, `\\`) + `"}`
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
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}

	if rec.APICalls != 1 {
		t.Errorf("APICalls = %d, want 1 (deduped by message id)", rec.APICalls)
	}
	// Only the last usage seen for msg_1 (output_tokens=40) counts.
	if rec.RealInputTokens != 100 {
		t.Errorf("RealInputTokens = %d, want 100", rec.RealInputTokens)
	}
	if rec.RealOutputTokens != 40 {
		t.Errorf("RealOutputTokens = %d, want 40 (last-write-wins per message id)", rec.RealOutputTokens)
	}
}

func TestRunStop_MissingTranscriptPathOmitsRealUsageFields(t *testing.T) {
	home := withFakeHome(t)

	// No transcript_path at all — the older/synthetic payload shape.
	// Real-usage fields must be absent from the JSONL line entirely
	// (zero value + omitempty), not merely zero.
	payload := `{"session_id":"sess-no-transcript","stop_reason":"end_turn","message_count":8,"input_tokens":10,"output_tokens":5}`
	replaceStdinWithPipe(t, payload)

	if err := runStop(); err != nil {
		t.Fatalf("runStop: %v", err)
	}

	logPath := filepath.Join(home, ".knowledge", "logs", "sessions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if strings.Contains(line, "real_input_tokens") || strings.Contains(line, "api_calls") {
		t.Errorf("expected real-usage fields to be omitted, got: %s", line)
	}
}

func TestRunStop_UnreadableTranscriptPathDoesNotError(t *testing.T) {
	home := withFakeHome(t)

	payload := `{"session_id":"sess-bad-path","stop_reason":"end_turn","message_count":8,"transcript_path":"/nonexistent/path/does-not-exist.jsonl"}`
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
	if rec.APICalls != 0 || rec.RealInputTokens != 0 {
		t.Errorf("expected zero real-usage fields for unreadable transcript, got %+v", rec)
	}
}

func TestReadTranscriptUsage_NoUsageDataReturnsNotOK(t *testing.T) {
	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"hi"}}` + "\n"
	if err := os.WriteFile(transcriptPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	usage, apiCalls, ok := readTranscriptUsage(transcriptPath)
	if ok {
		t.Errorf("ok = true, want false (no usage blocks in transcript)")
	}
	if apiCalls != 0 {
		t.Errorf("apiCalls = %d, want 0", apiCalls)
	}
	if usage != (transcriptUsage{}) {
		t.Errorf("usage = %+v, want zero value", usage)
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
