// Package discover implements autonomous knowledge mining from git
// history and source code. It calls an LLM (Anthropic Haiku or any
// OpenAI-compatible endpoint) to analyze commits and packages, then
// returns candidate entries for the pending review queue.
//
// The package is intentionally independent of internal/extract — the
// two pipelines (session extraction vs. autonomous discovery) have
// different prompts, inputs, and lifecycles.
package discover

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// Config controls discovery behavior.
type Config struct {
	Mode        string // "git", "codebase", "all"
	Depth       int    // max commits to scan (default 100)
	Cwd         string // working directory
	ProjectName string

	// LLM config
	LLMProvider string // "anthropic" or "openai"
	LLMModel    string
	BaseURL     string // for openai provider
	APIKey      string // ANTHROPIC_API_KEY or MASTERMIND_LLM_API_KEY
}

// Result summarizes one discovery run.
type Result struct {
	Entries         []format.Entry
	CommitsAnalyzed int
	CommitsSkipped  int
	PackagesScanned int
}

// Discoverer orchestrates knowledge discovery from git history and
// source code.
type Discoverer struct {
	cfg   Config
	store *store.Store
	llm   *llmClient
}

// New creates a Discoverer. Returns an error if LLM config is invalid.
func New(cfg Config, s *store.Store) (*Discoverer, error) {
	if cfg.Depth <= 0 {
		cfg.Depth = 100
	}
	if cfg.Mode == "" {
		cfg.Mode = "all"
	}

	llm, err := newLLMClient(cfg)
	if err != nil {
		return nil, err
	}

	return &Discoverer{cfg: cfg, store: s, llm: llm}, nil
}

// Run executes discovery based on the configured mode and writes
// entries to the store's pending queue.
func (d *Discoverer) Run() (*Result, error) {
	result := &Result{}

	existingTopics := d.collectExistingTopics()

	if d.cfg.Mode == "git" || d.cfg.Mode == "all" {
		entries, analyzed, skipped, err := d.discoverGit(existingTopics)
		if err != nil {
			return nil, fmt.Errorf("git discovery: %w", err)
		}
		result.Entries = append(result.Entries, entries...)
		result.CommitsAnalyzed = analyzed
		result.CommitsSkipped = skipped

		// Add new topics to dedup set for codebase mode.
		for _, e := range entries {
			existingTopics = append(existingTopics, e.Metadata.Topic)
		}
	}

	if d.cfg.Mode == "codebase" || d.cfg.Mode == "all" {
		entries, scanned, err := d.discoverCodebase(existingTopics)
		if err != nil {
			return nil, fmt.Errorf("codebase discovery: %w", err)
		}
		result.Entries = append(result.Entries, entries...)
		result.PackagesScanned = scanned
	}

	// Write all entries to pending.
	for i := range result.Entries {
		scope := format.ScopeProjectShared
		if d.store.Config().ProjectSharedRoot == "" {
			scope = format.ScopeUserPersonal
		}
		result.Entries[i].Metadata.Scope = scope
		if _, err := d.store.Write(&result.Entries[i]); err != nil {
			fmt.Fprintf(os.Stderr, "discover: write failed for %q: %v\n", result.Entries[i].Metadata.Topic, err)
		}
	}

	return result, nil
}

// ─── git discovery ────────────────────────────────────────────────────

func (d *Discoverer) discoverGit(existingTopics []string) ([]format.Entry, int, int, error) {
	knownHashes := d.collectKnownHashes()

	// Get recent commits.
	out, err := d.gitCmd("log", "--oneline", fmt.Sprintf("-%d", d.cfg.Depth))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("git log: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil, 0, 0, nil
	}

	// Filter out already-discovered commits.
	var workList []string
	skipped := 0
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 0 {
			continue
		}
		hash := parts[0]
		if len(hash) >= 7 && knownHashes[hash[:7]] {
			skipped++
			continue
		}
		workList = append(workList, line)
	}

	if len(workList) == 0 {
		return nil, 0, skipped, nil
	}

	// Batch into chunks of 25.
	var allEntries []format.Entry
	for i := 0; i < len(workList); i += 25 {
		end := i + 25
		if end > len(workList) {
			end = len(workList)
		}
		batch := workList[i:end]

		// Get --stat for each commit in the batch.
		var input strings.Builder
		for _, line := range batch {
			hash := strings.SplitN(line, " ", 2)[0]
			stat, _ := d.gitCmd("show", "--stat", "--format=%H %s", hash)
			input.WriteString(stat)
			input.WriteString("\n---\n")
		}

		raw, err := d.llm.Call(gitDiscoveryPrompt, "Analyze these commits:\n\n"+input.String())
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: LLM call failed for batch %d: %v\n", i/25+1, err)
			continue
		}

		entries, err := parseResponse(raw, d.cfg.ProjectName, existingTopics)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: parse failed for batch %d: %v\n", i/25+1, err)
			continue
		}

		allEntries = append(allEntries, entries...)
		// Update dedup set for subsequent batches.
		for _, e := range entries {
			existingTopics = append(existingTopics, e.Metadata.Topic)
		}
	}

	return allEntries, len(workList), skipped, nil
}

// ─── codebase discovery ───────────────────────────────────────────────

func (d *Discoverer) discoverCodebase(existingTopics []string) ([]format.Entry, int, error) {
	packages := d.findPackages()
	if len(packages) > 8 {
		packages = packages[:8]
	}

	var allEntries []format.Entry
	for _, pkg := range packages {
		content := d.readPackageFiles(pkg)
		if content == "" {
			continue
		}

		raw, err := d.llm.Call(codebaseDiscoveryPrompt, "Analyze this package:\n\nPackage: "+pkg+"\n\n"+content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: LLM call failed for %s: %v\n", pkg, err)
			continue
		}

		entries, err := parseResponse(raw, d.cfg.ProjectName, existingTopics)
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover: parse failed for %s: %v\n", pkg, err)
			continue
		}

		allEntries = append(allEntries, entries...)
		for _, e := range entries {
			existingTopics = append(existingTopics, e.Metadata.Topic)
		}
	}

	return allEntries, len(packages), nil
}

// findPackages walks the source tree to find directories containing
// Go files. Returns relative paths sorted by depth (shallowest first).
func (d *Discoverer) findPackages() []string {
	var packages []string
	seen := make(map[string]bool)

	filepath.WalkDir(d.cfg.Cwd, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Skip vendor, .git, node_modules, etc.
		if de.IsDir() {
			base := de.Name()
			if base == "vendor" || base == "node_modules" || base == ".git" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(de.Name(), ".go") || strings.HasSuffix(de.Name(), "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		rel, _ := filepath.Rel(d.cfg.Cwd, dir)
		if rel == "" {
			rel = "."
		}
		if !seen[rel] {
			seen[rel] = true
			packages = append(packages, rel)
		}
		return nil
	})

	// Sort by depth (shallowest first).
	sort.Slice(packages, func(i, j int) bool {
		di := strings.Count(packages[i], string(filepath.Separator))
		dj := strings.Count(packages[j], string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return packages[i] < packages[j]
	})

	return packages
}

// readPackageFiles reads up to 3 of the largest .go files in a package
// directory, capped at 2000 lines total.
func (d *Discoverer) readPackageFiles(pkg string) string {
	dir := filepath.Join(d.cfg.Cwd, pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Collect .go files sorted by size (largest first).
	type fileInfo struct {
		name string
		size int64
	}
	var goFiles []fileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		if info, err := e.Info(); err == nil {
			goFiles = append(goFiles, fileInfo{e.Name(), info.Size()})
		}
	}
	sort.Slice(goFiles, func(i, j int) bool {
		return goFiles[i].size > goFiles[j].size
	})

	var result strings.Builder
	totalLines := 0
	for i, f := range goFiles {
		if i >= 3 || totalLines >= 2000 {
			break
		}
		data, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		remaining := 2000 - totalLines
		if len(lines) > remaining {
			lines = lines[:remaining]
		}
		result.WriteString("// --- " + filepath.Join(pkg, f.name) + " ---\n")
		result.WriteString(strings.Join(lines, "\n"))
		result.WriteString("\n\n")
		totalLines += len(lines)
	}

	return result.String()
}

// ─── dedup helpers ────────────────────────────────────────────────────

// hashPattern matches 7-char hex strings (short git hashes).
var hashPattern = regexp.MustCompile(`\b[0-9a-f]{7,}\b`)

// collectKnownHashes scans all entries (live + pending) across all
// scopes for ## Source sections containing commit hashes. Returns a
// set of 7-char hashes that have already been discovered.
func (d *Discoverer) collectKnownHashes() map[string]bool {
	hashes := make(map[string]bool)
	cfg := d.store.Config()
	roots := []string{cfg.UserPersonalRoot, cfg.ProjectSharedRoot, cfg.ProjectPersonalRoot}

	for _, root := range roots {
		if root == "" {
			continue
		}
		filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			content := string(data)
			idx := strings.Index(content, "## Source")
			if idx < 0 {
				return nil
			}
			sourceSection := content[idx:]
			for _, match := range hashPattern.FindAllString(sourceSection, -1) {
				if len(match) >= 7 {
					hashes[match[:7]] = true
				}
			}
			return nil
		})
	}

	return hashes
}

// collectExistingTopics gathers topic strings from all live and pending
// entries across all scopes for deduplication.
func (d *Discoverer) collectExistingTopics() []string {
	var topics []string
	cfg := d.store.Config()
	roots := []string{cfg.UserPersonalRoot, cfg.ProjectSharedRoot, cfg.ProjectPersonalRoot}

	for _, root := range roots {
		if root == "" {
			continue
		}
		filepath.WalkDir(root, func(path string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			entry, err := format.Parse(data)
			if err != nil {
				return nil
			}
			if entry.Metadata.Topic != "" {
				topics = append(topics, entry.Metadata.Topic)
			}
			return nil
		})
	}

	return topics
}

// ─── git helpers ──────────────────────────────────────────────────────

func (d *Discoverer) gitCmd(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", d.cfg.Cwd}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
