package search

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// TestSupersedesBoostRanksHigherWithinClass verifies that two
// same-class entries with otherwise identical scores get ranked
// correctly when one supersedes another. Boost is within-class —
// class still dominates — but a supersedes link is a confidence
// signal within the same tier.
func TestSupersedesBoostRanksHigherWithinClass(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Two entries, both class 3 (all tokens in topic), same date
	// (so date tiebreak is neutral). One supersedes two other
	// entries (1.4× boost), one supersedes nothing (1.0×).
	boosted := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "goroutines explained clearly",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
			Supersedes: []string{"old-goroutines-post", "older-goroutines-note"},
		},
		Body: "body",
	}
	plain := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "goroutines explained briefly",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
		},
		Body: "body",
	}
	for _, e := range []*format.Entry{plain, boosted} {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "goroutines"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// Both are class 3 (topic token match). Boosted entry must rank first.
	if !strings.Contains(results[0].Metadata.Topic, "clearly") {
		t.Errorf("results[0] = %q, want the supersedes-boosted entry (clearly)", results[0].Metadata.Topic)
	}
}

// TestSupersedesBoostCapsAtThreeLinks verifies the anti-gaming cap.
// An entry listing 10 supersedes slugs gets the same boost as one
// listing 3. Test by constructing two entries: one with 3 links,
// one with 10. After the boost pass their scores must be equal.
// Indirect check: they keep their natural (date) ordering rather
// than the 10-linked one jumping ahead.
func TestSupersedesBoostCapsAtThreeLinks(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	threeLinks := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-02", Project: "mm", // later date — would naturally sort first
			Topic: "gamma three links here",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
			Supersedes: []string{"a", "b", "c"},
		},
		Body: "body",
	}
	tenLinks := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm", // earlier date
			Topic: "gamma ten links inflated",
			Kind:  format.KindLesson, Scope: format.ScopeUserPersonal,
			Supersedes: []string{"a1", "b1", "c1", "d1", "e1", "f1", "g1", "h1", "i1", "j1"},
		},
		Body: "body",
	}
	for _, e := range []*format.Entry{tenLinks, threeLinks} {
		p, err := s.Write(e)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Promote(p); err != nil {
			t.Fatal(err)
		}
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "gamma"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	// Both are class 3 with equal boosted scores (capped at 3 links).
	// Date tiebreak: threeLinks has 2026-04-02, wins.
	if !strings.Contains(results[0].Metadata.Topic, "three links") {
		t.Errorf("results[0] = %q, want three-links entry (cap should prevent 10-link runaway)", results[0].Metadata.Topic)
	}
}

// TestContradictsCoRetrievalSurfacesTarget verifies the co-retrieval
// pass: an entry A with `contradicts: [b]` appears in results, and
// B is pulled in with a "(contradicts ...)" annotation even if B
// doesn't match the query keyword at all.
func TestContradictsCoRetrievalSurfacesTarget(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// Entry B: unrelated to the query, but will be co-retrieved.
	entB := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-03-01", Project: "mm",
			Topic: "older benchmark claim",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
		},
		Body: "the old claim was that X was faster",
	}
	bp, err := s.Write(entB)
	if err != nil {
		t.Fatal(err)
	}
	bp, err = s.Promote(bp)
	if err != nil {
		t.Fatal(err)
	}
	_ = bp
	bSlug := slugFromPath(bp)

	// Entry A: matches the query, contradicts B.
	entA := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "measurements show Y is actually faster",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
			Contradicts: []string{bSlug},
		},
		Body: "body",
	}
	ap, err := s.Write(entA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(ap); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "measurements"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want 2 (A + co-retrieved B)", len(results))
	}

	// Find entry B in the results via its annotation.
	var foundCoRetrieved bool
	for _, r := range results {
		if strings.Contains(r.Metadata.Topic, "older benchmark") {
			foundCoRetrieved = true
			if r.Annotation == "" {
				t.Error("co-retrieved entry has empty Annotation, want contradicts tag")
			}
			if !strings.Contains(r.Annotation, "contradicts") {
				t.Errorf("Annotation = %q, want contains 'contradicts'", r.Annotation)
			}
		}
	}
	if !foundCoRetrieved {
		t.Error("contradicts target B was not co-retrieved into results")
	}
}

// TestContradictsCoRetrievalDoesNotDoubleCount verifies that if an
// entry already matches the query AND is contradicted by another
// top result, it is NOT duplicated in the output.
func TestContradictsCoRetrievalDoesNotDoubleCount(t *testing.T) {
	tmp := t.TempDir()
	cfg := store.Config{UserPersonalRoot: filepath.Join(tmp, "user"), Now: time.Now}
	s := store.New(cfg)

	// B matches the query on its own AND is contradicted by A.
	entB := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-03-01", Project: "mm",
			Topic: "older widgets benchmark",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
		},
		Body: "body",
	}
	bp, err := s.Write(entB)
	if err != nil {
		t.Fatal(err)
	}
	bp, err = s.Promote(bp)
	if err != nil {
		t.Fatal(err)
	}
	bSlug := slugFromPath(bp)

	entA := &format.Entry{
		Metadata: format.Metadata{
			Date: "2026-04-01", Project: "mm",
			Topic: "new widgets data refutes older",
			Kind:  format.KindInsight, Scope: format.ScopeUserPersonal,
			Contradicts: []string{bSlug},
		},
		Body: "body",
	}
	ap, err := s.Write(entA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(ap); err != nil {
		t.Fatal(err)
	}

	searcher := NewKeywordSearcher(s)
	results, err := searcher.Search(Query{QueryText: "widgets"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Count how many times B appears — must be exactly 1.
	var bCount int
	for _, r := range results {
		if strings.Contains(r.Metadata.Topic, "older widgets benchmark") {
			bCount++
		}
	}
	if bCount != 1 {
		t.Errorf("B appears %d times in results, want exactly 1 (dedup)", bCount)
	}
}

// TestSlugFromPath covers the slug helper directly.
func TestSlugFromPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/home/u/.knowledge/lessons/electron-ipc.md", "electron-ipc"},
		{"/tmp/x.md", "x"},
		{"plain-name.md", "plain-name"},
		{"no-extension", "no-extension"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := slugFromPath(tc.in); got != tc.want {
			t.Errorf("slugFromPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
