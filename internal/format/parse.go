package format

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// frontmatterDelim is the fence used to bracket the YAML frontmatter block.
// It must appear at the very start of the file, on its own line, and again
// on its own line to close the block. Anything after the closing fence is
// the markdown body.
var frontmatterDelim = []byte("---")

// Parse reads a mastermind entry from raw file bytes.
//
// Expected shape:
//
//	---
//	<YAML frontmatter>
//	---
//
//	<markdown body>
//
// The leading `---` must be on the first line (after any optional BOM).
// A trailing newline after the closing `---` is stripped from the body.
//
// Parse does not call Normalize or Validate. Callers typically want:
//
//	entry, err := format.Parse(data)
//	if err != nil { ... }
//	entry.Normalize()
//	if errs := entry.Validate(); len(errs) > 0 { ... }
//
// This lets the parser be reused for both strict writes (full validation)
// and lenient reads (just surface whatever's on disk).
func Parse(data []byte) (*Entry, error) {
	// Tolerate a UTF-8 BOM at the very start. Not all editors add one,
	// but it's cheap to survive.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	// The file must start with the delimiter on its own line.
	if !bytes.HasPrefix(data, frontmatterDelim) {
		return nil, fmt.Errorf("format: missing frontmatter opening delimiter %q at start of file", frontmatterDelim)
	}

	// Advance past the opening delimiter and its newline.
	rest := data[len(frontmatterDelim):]
	rest = consumeNewline(rest)

	// Find the closing delimiter. It must be on its own line, which
	// means preceded by a newline and followed by either a newline
	// or end-of-file.
	closeIdx := findClosingDelim(rest)
	if closeIdx < 0 {
		return nil, fmt.Errorf("format: missing frontmatter closing delimiter %q on its own line", frontmatterDelim)
	}

	yamlBlock := rest[:closeIdx]
	afterClose := rest[closeIdx+len(frontmatterDelim):]
	afterClose = consumeNewline(afterClose)
	// The body conventionally has a blank line after the closing fence.
	// Consume a second newline if present, for a cleaner round-trip.
	afterClose = consumeNewline(afterClose)

	var meta Metadata
	if err := yaml.Unmarshal(yamlBlock, &meta); err != nil {
		return nil, fmt.Errorf("format: parse frontmatter: %w", err)
	}

	return &Entry{
		Metadata: meta,
		Body:     string(bytes.TrimRight(afterClose, "\n")),
	}, nil
}

// findClosingDelim locates the second `---` fence.
//
// It must be at the start of a line — either position 0, or immediately
// after a newline. It must be followed by either a newline or end-of-file
// (not, e.g., "----" or "--- some text").
//
// Returns the byte offset of the fence within rest, or -1 if not found.
func findClosingDelim(rest []byte) int {
	start := 0
	for start < len(rest) {
		idx := bytes.Index(rest[start:], frontmatterDelim)
		if idx < 0 {
			return -1
		}
		absIdx := start + idx

		// Must be at line start: position 0, or preceded by '\n'.
		if absIdx != 0 && rest[absIdx-1] != '\n' {
			start = absIdx + len(frontmatterDelim)
			continue
		}

		// Must be followed by EOF or newline (so we don't match "----").
		after := absIdx + len(frontmatterDelim)
		if after == len(rest) || rest[after] == '\n' || rest[after] == '\r' {
			return absIdx
		}
		start = after
	}
	return -1
}

// consumeNewline strips a leading \n or \r\n from b, if present.
// Safe on empty input.
func consumeNewline(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	if b[0] == '\r' && len(b) > 1 && b[1] == '\n' {
		return b[2:]
	}
	if b[0] == '\n' {
		return b[1:]
	}
	return b
}
