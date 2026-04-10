package extract

import (
	"strings"
	"testing"
)

func TestNormalizeTranscript_PlainTextPassesThrough(t *testing.T) {
	in := "This is just a regular note.\nNo JSON in sight.\nThe fix was to unplug and plug back in."
	out := NormalizeTranscript(in)
	if out != in {
		t.Errorf("plain text should pass through unchanged\ngot:  %q\nwant: %q", out, in)
	}
}

func TestNormalizeTranscript_EmptyInput(t *testing.T) {
	if got := NormalizeTranscript(""); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
}

func TestNormalizeTranscript_JSONLUserStringContent(t *testing.T) {
	in := `{"type":"user","message":{"role":"user","content":"I'll use the keyword extractor first."}}`
	out := NormalizeTranscript(in)
	want := "User: I'll use the keyword extractor first.\n\n"
	if out != want {
		t.Errorf("user string content not extracted\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_JSONLAssistantArrayContent(t *testing.T) {
	in := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The root cause was a race condition."}]}}`
	out := NormalizeTranscript(in)
	want := "Assistant: The root cause was a race condition.\n\n"
	if out != want {
		t.Errorf("assistant array content not extracted\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_StripsToolUseAndToolResult(t *testing.T) {
	in := `{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"text","text":"Let me read the file."},` +
		`{"type":"tool_use","id":"toolu_01","name":"Read","input":{"file_path":"/x"}}` +
		`]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"toolu_01","content":"file contents"}` +
		`]}}`
	out := NormalizeTranscript(in)
	want := "Assistant: Let me read the file.\n\n"
	if out != want {
		t.Errorf("tool_use/tool_result should be stripped\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_SkipsNonUserAssistantLines(t *testing.T) {
	in := `{"type":"attachment","data":"..."}` + "\n" +
		`{"type":"system","content":"..."}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"hello"}}`
	out := NormalizeTranscript(in)
	want := "User: hello\n\n"
	if out != want {
		t.Errorf("attachment/system lines should be skipped\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_SkipsMalformedLines(t *testing.T) {
	// First line is valid JSONL so we enter JSONL mode; middle line is
	// garbage and should be skipped silently without aborting.
	in := `{"type":"user","message":{"role":"user","content":"first"}}` + "\n" +
		`not valid json at all` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second"}]}}`
	out := NormalizeTranscript(in)
	want := "User: first\n\nAssistant: second\n\n"
	if out != want {
		t.Errorf("malformed middle line should not abort\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_PreservesTurnOrder(t *testing.T) {
	in := `{"type":"user","message":{"role":"user","content":"Q1"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"A1"}]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"Q2"}}`
	out := NormalizeTranscript(in)
	want := "User: Q1\n\nAssistant: A1\n\nUser: Q2\n\n"
	if out != want {
		t.Errorf("turn order not preserved\ngot:  %q\nwant: %q", out, want)
	}
}

func TestNormalizeTranscript_JSONBlobNotJSONLStaysPlain(t *testing.T) {
	// A standalone JSON object with no "type" field should be treated
	// as plain text — the JSONL sniff requires a non-empty type.
	in := `{"foo":"bar","baz":42}`
	out := NormalizeTranscript(in)
	if out != in {
		t.Errorf("non-JSONL JSON should pass through unchanged\ngot:  %q\nwant: %q", out, in)
	}
}

func TestNormalizeTranscript_StripsNoiseInRealShape(t *testing.T) {
	// Mini-fixture mirroring the Phase 3 audit finding: tool_result
	// blocks containing JSON-like strings must NOT survive into the
	// output where keyword.go would scan them.
	in := `{"type":"user","message":{"role":"user","content":"what's the decision?"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[` +
		`{"type":"text","text":"I'll use the audit harness approach."},` +
		`{"type":"tool_use","id":"toolu_xx","name":"Write","input":{"file_path":"/x.md"}}` +
		`]}}` + "\n" +
		`{"type":"user","message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"toolu_xx","content":"decided to refactor"}` +
		`]}}`
	out := NormalizeTranscript(in)
	if strings.Contains(out, "toolu_") {
		t.Errorf("output should not contain tool_use_id noise: %q", out)
	}
	if strings.Contains(out, "tool_use") || strings.Contains(out, "tool_result") {
		t.Errorf("output should not contain structural JSON keys: %q", out)
	}
	// The tool_result content "decided to refactor" must NOT survive —
	// it's inside a tool_result block and should be dropped.
	if strings.Contains(out, "decided to refactor") {
		t.Errorf("tool_result content should be dropped: %q", out)
	}
	// But the real assistant text MUST survive.
	if !strings.Contains(out, "audit harness approach") {
		t.Errorf("real assistant text missing from output: %q", out)
	}
}
