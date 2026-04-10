package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// TestParseJSONFlag covers the three cases: --json present, absent,
// and mixed with other flags. The helper scans os.Args[2:] so every
// test case must set up os.Args as a subcommand invocation.
func TestParseJSONFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", []string{"mastermind", "session-start"}, false},
		{"present", []string{"mastermind", "session-start", "--json"}, true},
		{"mixed with other flags", []string{"mastermind", "discover", "--depth", "5", "--json"}, true},
		{"before positional", []string{"mastermind", "discover", "--json", "git"}, true},
		{"empty tail", []string{"mastermind"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldArgs := os.Args
			os.Args = tc.args
			t.Cleanup(func() { os.Args = oldArgs })
			if got := parseJSONFlag(); got != tc.want {
				t.Errorf("parseJSONFlag() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPrintJSONProducesParseableOutput verifies printJSON writes
// valid indented JSON to stdout that round-trips.
func TestPrintJSONProducesParseableOutput(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	payload := map[string]any{
		"greeting": "hello",
		"count":    3,
		"nested":   []string{"a", "b"},
	}
	if err := printJSON(payload); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	// Indented: must contain newlines and two-space indent.
	if !strings.Contains(string(out), "\n  ") {
		t.Errorf("output missing indent: %q", out)
	}

	// Must round-trip cleanly.
	var back map[string]any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if back["greeting"] != "hello" {
		t.Errorf("round-trip lost greeting: %v", back["greeting"])
	}
}

// TestEntrySummariesFromRefs_ShapeStability verifies that a nil/empty
// input produces an empty slice (not nil) so JSON encoding emits
// `"open_loops": []` rather than `"open_loops": null`. Consumers can
// rely on stable shape.
func TestEntrySummariesFromRefs_ShapeStability(t *testing.T) {
	out := entrySummariesFromRefs(nil)
	if out == nil {
		t.Error("entrySummariesFromRefs(nil) = nil, want non-nil empty slice")
	}
	if len(out) != 0 {
		t.Errorf("entrySummariesFromRefs(nil) len = %d, want 0", len(out))
	}

	// JSON of a struct containing it must encode as `[]` not `null`.
	type wrap struct {
		Items []entrySummary `json:"items"`
	}
	bs, err := json.Marshal(wrap{Items: entrySummariesFromRefs(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bs), `"items":[]`) {
		t.Errorf("shape drift: %s", bs)
	}
}

// TestEntrySummariesFromRefs_CopiesAllFields verifies the conversion
// populates every JSON field from its metadata counterpart.
func TestEntrySummariesFromRefs_CopiesAllFields(t *testing.T) {
	refs := []store.EntryRef{
		{
			Path: "/tmp/knowledge/lessons/foo.md",
			Metadata: format.Metadata{
				Topic:   "A test topic",
				Kind:    format.KindLesson,
				Date:    "2026-04-10",
				Scope:   format.ScopeUserPersonal,
				Project: "mastermind",
			},
		},
	}
	out := entrySummariesFromRefs(refs)
	if len(out) != 1 {
		t.Fatalf("got %d summaries, want 1", len(out))
	}
	got := out[0]
	if got.Topic != "A test topic" {
		t.Errorf("Topic = %q", got.Topic)
	}
	if got.Kind != string(format.KindLesson) {
		t.Errorf("Kind = %q", got.Kind)
	}
	if got.Date != "2026-04-10" {
		t.Errorf("Date = %q", got.Date)
	}
	if got.Scope != string(format.ScopeUserPersonal) {
		t.Errorf("Scope = %q", got.Scope)
	}
	if got.Project != "mastermind" {
		t.Errorf("Project = %q", got.Project)
	}
	if got.Path != "/tmp/knowledge/lessons/foo.md" {
		t.Errorf("Path = %q", got.Path)
	}
}

// TestRunPostCompact_JSONEmitsValidObject exercises the full dispatch
// path in --json mode against an empty store. Output must be valid
// JSON (an empty postCompactJSON object with empty slices, not a
// blank prose output).
func TestRunPostCompact_JSONEmitsValidObject(t *testing.T) {
	withFakeHome(t)

	oldStdin := os.Stdin
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = devNull
	t.Cleanup(func() {
		os.Stdin = oldStdin
		devNull.Close()
	})

	cwd := t.TempDir()
	oldArgs := os.Args
	os.Args = []string{"mastermind", "post-compact", "--cwd", cwd, "--json"}
	t.Cleanup(func() { os.Args = oldArgs })

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	if err := runPostCompact(); err != nil {
		t.Fatalf("runPostCompact: %v", err)
	}

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	// Must be valid JSON even with no knowledge in the store.
	var decoded postCompactJSON
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	// Slices must be empty, not nil (shape stability).
	if decoded.OpenLoops == nil {
		t.Error("OpenLoops is nil, want empty slice")
	}
	if decoded.ProjectEntries == nil {
		t.Error("ProjectEntries is nil, want empty slice")
	}
}

// TestRunSessionStart_JSONEmitsValidObject is the session-start
// counterpart — identical structural check.
func TestRunSessionStart_JSONEmitsValidObject(t *testing.T) {
	withFakeHome(t)

	cwd := t.TempDir()
	oldArgs := os.Args
	os.Args = []string{"mastermind", "session-start", "--cwd", cwd, "--json"}
	t.Cleanup(func() { os.Args = oldArgs })

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	if err := runSessionStart(); err != nil {
		t.Fatalf("runSessionStart: %v", err)
	}

	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	var decoded sessionStartJSON
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if decoded.OpenLoops == nil {
		t.Error("OpenLoops is nil, want empty slice")
	}
	if decoded.ProjectEntries == nil {
		t.Error("ProjectEntries is nil, want empty slice")
	}
}
