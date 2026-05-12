package access

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestTrackerEmptyLoad(t *testing.T) {
	tmp := t.TempDir()
	tr := New(tmp)

	count, last := tr.Get(filepath.Join(tmp, "entry.md"))
	if count != 0 || last != "" {
		t.Errorf("empty tracker: got count=%d last=%q, want 0/\"\"", count, last)
	}
}

func TestTrackerIncrementAndGet(t *testing.T) {
	tmp := t.TempDir()
	tr := New(tmp)

	path := filepath.Join(tmp, "a.md")
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)

	tr.Increment(path, now)
	count, last := tr.Get(path)
	if count != 1 {
		t.Errorf("after 1 increment: count = %d, want 1", count)
	}
	if last != "2026-05-12" {
		t.Errorf("after 1 increment: last = %q, want 2026-05-12", last)
	}

	tr.Increment(path, now)
	tr.Increment(path, now)
	count, _ = tr.Get(path)
	if count != 3 {
		t.Errorf("after 3 increments: count = %d, want 3", count)
	}
}

func TestTrackerSaveAndReload(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "b.md")
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)

	// Write via first tracker.
	tr1 := New(tmp)
	for i := 0; i < 5; i++ {
		tr1.Increment(path, now)
	}

	// Reload via second tracker — must see persisted counts.
	tr2 := New(tmp)
	count, last := tr2.Get(path)
	if count != 5 {
		t.Errorf("reloaded count = %d, want 5", count)
	}
	if last != "2026-05-12" {
		t.Errorf("reloaded last = %q, want 2026-05-12", last)
	}
}

func TestTrackerSidecarPath(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "c.md")
	now := time.Now()

	tr := New(tmp)
	tr.Increment(path, now)

	// The sidecar must exist at <root>/.cache/access.json.
	expected := filepath.Join(tmp, ".cache", "access.json")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("sidecar not found at %s: %v", expected, err)
	}
}

func TestTrackerSeedDoesNotOverwrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "d.md")
	now := time.Now()

	tr := New(tmp)
	// Real telemetry first.
	tr.Increment(path, now)
	tr.Increment(path, now)

	// Seed should be ignored because real data already exists.
	tr.Seed(path, 100, "2020-01-01")
	tr.Save()

	tr2 := New(tmp)
	count, _ := tr2.Get(path)
	if count != 2 {
		t.Errorf("seed overwrote real data: count = %d, want 2", count)
	}
}

func TestTrackerSeedFillsEmpty(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "e.md")

	tr := New(tmp)
	tr.Seed(path, 7, "2025-03-15")
	tr.Save()

	tr2 := New(tmp)
	count, last := tr2.Get(path)
	if count != 7 {
		t.Errorf("seeded count = %d, want 7", count)
	}
	if last != "2025-03-15" {
		t.Errorf("seeded last = %q, want 2025-03-15", last)
	}
}

func TestTrackerConcurrentIncrements(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.md")
	now := time.Now()

	tr := New(tmp)
	const goroutines = 20
	const increments = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < increments; j++ {
				tr.Increment(path, now)
			}
		}()
	}
	wg.Wait()

	count, _ := tr.Get(path)
	if count != goroutines*increments {
		t.Errorf("concurrent count = %d, want %d", count, goroutines*increments)
	}
}

func TestTrackerMissingRootIsNoop(t *testing.T) {
	// A tracker whose root doesn't exist yet should not panic. It starts
	// cold and creates the cache dir on first save.
	tmp := t.TempDir()
	root := filepath.Join(tmp, "nonexistent")
	path := filepath.Join(root, "g.md")
	now := time.Now()

	tr := New(root)
	// Should not panic even though root doesn't exist.
	tr.Increment(path, now)
	count, _ := tr.Get(path)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestTrackerCorruptSidecarStartsCold(t *testing.T) {
	tmp := t.TempDir()
	cacheD := filepath.Join(tmp, ".cache")
	if err := os.MkdirAll(cacheD, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write garbage JSON.
	if err := os.WriteFile(filepath.Join(cacheD, "access.json"), []byte("not json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(tmp, "h.md")
	tr := New(tmp)
	count, _ := tr.Get(path)
	if count != 0 {
		t.Errorf("corrupt sidecar: count = %d, want 0 (cold start)", count)
	}
	// Should be able to increment cleanly after corrupt load.
	tr.Increment(path, time.Now())
	count, _ = tr.Get(path)
	if count != 1 {
		t.Errorf("after increment on cold: count = %d, want 1", count)
	}
}
