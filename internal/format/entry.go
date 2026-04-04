// Package format defines the mastermind entry format: YAML frontmatter +
// markdown body.
//
// The schema is the long-term contract between present-you and future-you.
// It must stay backward-compatible. See docs/FORMAT.md for the full spec
// and the migration policy.
//
// Responsibilities:
//   - Parse an entry from raw file bytes (see Parse in parse.go).
//   - Validate required fields and enum values ((*Entry).Validate).
//   - Serialize an Entry back to its on-disk form ((*Entry).MarshalMarkdown).
//
// Non-responsibilities: filesystem I/O lives in internal/store; search
// indexing lives in internal/search.
package format

import (
	"fmt"
	"strings"
	"time"
)

// Entry is a single mastermind knowledge entry.
//
// The on-disk format is:
//
//	---
//	<YAML frontmatter>
//	---
//
//	<markdown body>
//
// Entry is the in-memory form of that file. See docs/FORMAT.md for the
// schema contract. The schema is append-only — new fields may be added
// as optional, but existing fields must never change meaning or type.
type Entry struct {
	Metadata Metadata
	Body     string
}

// Metadata is the YAML frontmatter block.
//
// Field order in this struct matches the canonical serialization order.
// Changing the order changes the on-disk format for newly written files,
// which creates noisy git diffs. Don't reorder without reason.
type Metadata struct {
	Date       string     `yaml:"date"`                 // ISO 8601 capture date (YYYY-MM-DD)
	Project    string     `yaml:"project"`              // free-form project identifier, "general" for cross-project
	Tags       []string   `yaml:"tags,omitempty"`       // free-form lowercase strings
	Topic      string     `yaml:"topic"`                // one-line human summary
	Kind       Kind       `yaml:"kind"`                 // enum: lesson, insight, war-story, decision, pattern, open-loop
	Scope      Scope      `yaml:"scope,omitempty"`      // enum: user-personal, project-shared, project-personal
	Confidence Confidence `yaml:"confidence,omitempty"` // enum: high, medium, low (default: high)
}

// Kind classifies an entry. See docs/FORMAT.md for the meaning of each.
//
// The enum is deliberately small. Adding a new kind requires a DECISIONS.md
// entry and a year of dogfooding the existing five (now six) kinds first.
type Kind string

const (
	KindLesson    Kind = "lesson"
	KindInsight   Kind = "insight"
	KindWarStory  Kind = "war-story"
	KindDecision  Kind = "decision"
	KindPattern   Kind = "pattern"
	KindOpenLoop  Kind = "open-loop"
)

// AllKinds returns every valid Kind, in canonical order. Useful for
// validation loops and tests.
func AllKinds() []Kind {
	return []Kind{KindLesson, KindInsight, KindWarStory, KindDecision, KindPattern, KindOpenLoop}
}

// Valid reports whether k is one of the recognized Kind values.
func (k Kind) Valid() bool {
	for _, known := range AllKinds() {
		if k == known {
			return true
		}
	}
	return false
}

// Scope identifies which store an entry belongs to.
//
// Scope is only required when an entry is being routed (e.g., extraction
// candidates landing in pending/). Entries already in a known scope
// directory can omit the field — the store infers scope from the path.
type Scope string

const (
	ScopeUserPersonal    Scope = "user-personal"
	ScopeProjectShared   Scope = "project-shared"
	ScopeProjectPersonal Scope = "project-personal"
)

// AllScopes returns every valid Scope, in canonical order.
func AllScopes() []Scope {
	return []Scope{ScopeUserPersonal, ScopeProjectShared, ScopeProjectPersonal}
}

// Valid reports whether s is one of the recognized Scope values.
// The empty scope is also considered valid — it means "unspecified,
// caller will route via path".
func (s Scope) Valid() bool {
	if s == "" {
		return true
	}
	for _, known := range AllScopes() {
		if s == known {
			return true
		}
	}
	return false
}

// Confidence is how sure the author (human or extractor) is about an entry.
//
// "high" is the default; when Confidence is empty after parsing, callers
// should treat it as ConfidenceHigh. See (*Entry).Normalize.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Valid reports whether c is a recognized Confidence value. The empty
// string is considered valid (it means "not specified, use default").
func (c Confidence) Valid() bool {
	if c == "" {
		return true
	}
	return c == ConfidenceHigh || c == ConfidenceMedium || c == ConfidenceLow
}

// Normalize fills in default values for optional fields. After Normalize,
// Confidence is never empty and Tags is never nil.
//
// Normalize does NOT validate required fields — use Validate for that.
// The two operations are separate so callers can normalize before
// presenting an entry to a human reviewer without falsely failing on
// missing required fields.
func (e *Entry) Normalize() {
	if e.Metadata.Confidence == "" {
		e.Metadata.Confidence = ConfidenceHigh
	}
	if e.Metadata.Tags == nil {
		e.Metadata.Tags = []string{}
	}
}

// Validate checks that required fields are present and enum fields have
// recognized values. It returns all errors found, not just the first —
// the caller should show a human every missing field at once rather
// than one at a time.
//
// Required fields: Date, Project, Topic, Kind.
// Optional fields: Tags, Scope, Confidence (validated only if non-empty).
func (e *Entry) Validate() []error {
	var errs []error

	if strings.TrimSpace(e.Metadata.Date) == "" {
		errs = append(errs, fmt.Errorf("metadata.date is required"))
	} else if _, err := time.Parse("2006-01-02", e.Metadata.Date); err != nil {
		errs = append(errs, fmt.Errorf("metadata.date must be ISO 8601 (YYYY-MM-DD): %w", err))
	}

	if strings.TrimSpace(e.Metadata.Project) == "" {
		errs = append(errs, fmt.Errorf("metadata.project is required"))
	}

	if strings.TrimSpace(e.Metadata.Topic) == "" {
		errs = append(errs, fmt.Errorf("metadata.topic is required"))
	}

	if e.Metadata.Kind == "" {
		errs = append(errs, fmt.Errorf("metadata.kind is required"))
	} else if !e.Metadata.Kind.Valid() {
		errs = append(errs, fmt.Errorf("metadata.kind %q is not a recognized kind; want one of %v", e.Metadata.Kind, AllKinds()))
	}

	if !e.Metadata.Scope.Valid() {
		errs = append(errs, fmt.Errorf("metadata.scope %q is not a recognized scope; want one of %v or empty", e.Metadata.Scope, AllScopes()))
	}

	if !e.Metadata.Confidence.Valid() {
		errs = append(errs, fmt.Errorf("metadata.confidence %q is not a recognized confidence; want high, medium, low, or empty", e.Metadata.Confidence))
	}

	return errs
}
