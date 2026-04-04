package format

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarshalMarkdown serializes an Entry to the on-disk file format.
//
// The output is deterministic — the same Entry always produces the same
// bytes, so git diffs stay clean across round-trips. Field order in the
// frontmatter matches the declaration order in Metadata.
//
// The output always ends with a single trailing newline. Empty bodies
// produce a well-formed frontmatter block followed by one blank line.
func (e *Entry) MarshalMarkdown() ([]byte, error) {
	// Normalize before serializing so defaults land on disk. This makes
	// round-trips (read → write → read) stable.
	normalized := *e
	normalized.Normalize()

	var yamlBuf bytes.Buffer
	enc := yaml.NewEncoder(&yamlBuf)
	enc.SetIndent(2)
	if err := enc.Encode(&normalized.Metadata); err != nil {
		return nil, fmt.Errorf("format: encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("format: close frontmatter encoder: %w", err)
	}

	// yaml.v3 emits a trailing "...\n" end-of-document marker for some
	// inputs; we don't want it in frontmatter. Strip if present.
	yamlBytes := yamlBuf.Bytes()
	yamlBytes = bytes.TrimSuffix(yamlBytes, []byte("...\n"))

	var out bytes.Buffer
	out.Write(frontmatterDelim)
	out.WriteByte('\n')
	out.Write(yamlBytes)
	out.Write(frontmatterDelim)
	out.WriteByte('\n')
	out.WriteByte('\n')

	body := strings.TrimRight(normalized.Body, "\n")
	if body != "" {
		out.WriteString(body)
		out.WriteByte('\n')
	}

	return out.Bytes(), nil
}
