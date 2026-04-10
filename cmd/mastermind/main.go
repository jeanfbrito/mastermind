// Command mastermind is the ADHD cure for agents that you always
// dreamed for yourself.
//
// It runs as an MCP server over stdio plus CLI subcommands wired to
// Claude Code hooks. Together they form a continuity layer: context is
// surfaced automatically at session start and after context compaction,
// lessons are extracted automatically before compaction, and the user's
// working memory is never taxed by the tool itself.
//
// See the project docs for the design:
//   - docs/CONTINUITY.md   — the load-bearing behaviors
//   - docs/ARCHITECTURE.md — module layout and MCP tool surface
//   - docs/FORMAT.md       — the entry schema (the long-term contract)
//   - docs/EXTRACTION.md   — the capture pipeline
//   - docs/ARCHIVE.md      — working set vs lifelong archive
//   - docs/DECISIONS.md    — the why behind every architectural choice
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jeanfbrito/mastermind/internal/discover"
	"github.com/jeanfbrito/mastermind/internal/extract"
	"github.com/jeanfbrito/mastermind/internal/format"
	"github.com/jeanfbrito/mastermind/internal/mcp"
	"github.com/jeanfbrito/mastermind/internal/project"
	"github.com/jeanfbrito/mastermind/internal/search"
	"github.com/jeanfbrito/mastermind/internal/store"
)

// version is set at build time via -ldflags "-X main.version=..."
// Falls back to debug.ReadBuildInfo() for `go install` builds, then to
// "dev" as a last resort. Pattern borrowed from engram's main.go.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("mastermind %s\n", version)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		case "session-start":
			if err := runSessionStart(); err != nil {
				fmt.Fprintf(os.Stderr, "mastermind session-start: %s\n", err)
				os.Exit(1)
			}
			return
		case "post-compact":
			if err := runPostCompact(); err != nil {
				fmt.Fprintf(os.Stderr, "mastermind post-compact: %s\n", err)
				os.Exit(1)
			}
			return
		case "session-close":
			// TODO(phase3b): implement in CONTINUITY.md phase 3b.
			fmt.Fprintln(os.Stderr, "mastermind session-close: not implemented yet — see docs/EXTRACTION.md and docs/ROADMAP.md Phase 3b")
			os.Exit(1)
		case "extract":
			if err := runExtract(); err != nil {
				fmt.Fprintf(os.Stderr, "mastermind extract: %s\n", err)
				os.Exit(1)
			}
			return
		case "suggest":
			if err := runSuggest(); err != nil {
				fmt.Fprintf(os.Stderr, "mastermind suggest: %s\n", err)
				os.Exit(1)
			}
			return
		case "discover":
			if err := runDiscover(); err != nil {
				fmt.Fprintf(os.Stderr, "mastermind discover: %s\n", err)
				os.Exit(1)
			}
			return
		case "mcp":
			// Explicit MCP mode (matches engram's convention: `engram mcp`
			// to start the stdio server). Fall through to default.
		default:
			fmt.Fprintf(os.Stderr, "mastermind: unknown command %q\n\n", os.Args[1])
			printHelp()
			os.Exit(2)
		}
	}

	// Default: start the MCP server over stdio. This is the mode
	// Claude Code spawns mastermind in.
	if err := runMCPServer(); err != nil {
		fmt.Fprintf(os.Stderr, "mastermind: %s\n", err)
		os.Exit(1)
	}
}

// buildSessionConfig constructs a store.Config with all three scope
// roots populated for the current session:
//
//   - UserPersonalRoot: ~/.knowledge (from store.DefaultConfig, which resolves
//     $HOME).
//   - ProjectSharedRoot: <root>/.knowledge when walking upward from cwd finds
//     a .knowledge/ directory. Left empty otherwise — the scope disables
//     silently rather than creating a new .knowledge/ the user never asked for.
//   - ProjectPersonalRoot: ~/.claude/projects/<slug>/memory when cwd is
//     inside a git repository. The slug comes from project.DetectFromGit,
//     which reads the origin remote first and falls back to the git
//     working-tree basename. If cwd is NOT inside a git repo (or
//     git is unavailable), the scope is left empty — this is a
//     deliberate guard against spawning garbage directories under
//     ~/.claude/projects/<random-tmpdir-name>/ every time the binary
//     is run from a non-project cwd.
//
// The chosen naming convention for project-personal — slug, not
// dash-encoded cwd — means two clones of the same project on two
// machines (e.g., ~/Github/mastermind and ~/code/mastermind) map to
// the same directory and the entries merge cleanly on sync. This is
// load-bearing for the cross-machine memory story. See the promoted
// pattern entry .knowledge/nodes/store-defaultconfig-returns-a-skeleton-...md
// and the closed open-loop that originally flagged this design call.
//
// Escape hatch for the edge case where a slug collision is unwanted
// (two unrelated projects that normalize to the same name): a future
// MASTERMIND_PROJECT_DIR env var can override this path. Not
// implemented yet — add it when a real collision surfaces, not before.
func buildSessionConfig(cwd string) (store.Config, error) {
	cfg, err := store.DefaultConfig()
	if err != nil {
		return store.Config{}, err
	}

	if root := store.FindProjectRoot(cwd); root != "" {
		cfg.ProjectSharedRoot = filepath.Join(root, ".knowledge")
	} else if os.Getenv("MASTERMIND_NO_AUTO_INIT") == "" {
		// Auto-create .knowledge/ at the git root so project-shared
		// scope works out of the box. Without this, agents silently
		// fall back to user-personal and project-specific lessons get
		// lost. Opt out with MASTERMIND_NO_AUTO_INIT=1.
		if gitRoot := project.GitRoot(cwd); gitRoot != "" {
			knowledgeDir := filepath.Join(gitRoot, ".knowledge")
			if err := os.MkdirAll(knowledgeDir, 0o755); err == nil {
				cfg.ProjectSharedRoot = knowledgeDir
				// Seed .gitignore so pending/ (auto-extracted, pre-review)
				// stays out of version control.
				gi := filepath.Join(knowledgeDir, ".gitignore")
				if _, err := os.Stat(gi); os.IsNotExist(err) {
					_ = os.WriteFile(gi, []byte("pending/\n"), 0o644)
				}
			}
		}
	}

	if slug := project.DetectFromGit(cwd); slug != "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.ProjectPersonalRoot = filepath.Join(home, ".claude", "projects", slug, "memory")
		}
	}

	return cfg, nil
}

// runMCPServer boots the three-scope store, wires up the searcher and
// the MCP server, and runs until the client disconnects or a signal
// arrives. Returns any error that escapes the SDK run loop.
func runMCPServer() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get cwd: %w", err)
	}
	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return fmt.Errorf("build session config: %w", err)
	}

	s := store.New(cfg)

	// Optional auto-promote: when PendingBehavior is "auto-promote",
	// old pending entries are silently promoted to the live store at
	// startup. Default (keep-forever) is a no-op. See DECISIONS.md
	// "Reverse auto-expire" for why entries are never deleted.
	_, _ = s.AutoPromoteStale()

	searcher := search.NewKeywordSearcher(s)

	server, err := mcp.NewServer(mcp.Options{
		Store:    s,
		Searcher: searcher,
		Version:  version,
	})
	if err != nil {
		return fmt.Errorf("build mcp server: %w", err)
	}

	// Run the server in a context that's cancelled by SIGINT/SIGTERM
	// so a clean shutdown happens on Ctrl-C or kill. The SDK's Run
	// returns when the transport closes, which normally happens when
	// the parent (Claude Code) exits.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx)
}

// runSessionStart implements the session-start subcommand. It prints
// compact markdown to stdout summarizing open loops, project-relevant
// entries, and pending counts. The output is injected as system context
// by the Claude Code SessionStart hook.
//
// If there is nothing to surface, it prints nothing and exits 0.
// This honors the silent-unless-needed rule from CONTINUITY.md.
func runSessionStart() error {
	cwd := parseCwdFlag()
	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return fmt.Errorf("build session config: %w", err)
	}

	s := store.New(cfg)
	projectName := project.DetectFromGit(cwd)

	// Collect open loops from all scopes (live + pending).
	openLoops, err := collectOpenLoops(s)
	if err != nil {
		return fmt.Errorf("collect open loops: %w", err)
	}

	// Collect project-relevant entries (non-open-loop).
	projectEntries, err := collectProjectEntries(s, projectName)
	if err != nil {
		return fmt.Errorf("collect project entries: %w", err)
	}

	// Count pending entries across all scopes.
	pendingCount, err := countPending(s)
	if err != nil {
		return fmt.Errorf("count pending: %w", err)
	}

	output := formatSessionStart(openLoops, projectEntries, pendingCount)
	if output != "" {
		fmt.Print(output)
	}
	return nil
}

// parseCwdFlag scans os.Args for --cwd <path> after the subcommand.
// Falls back to os.Getwd().
func parseCwdFlag() string {
	for i := 2; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--cwd" {
			return os.Args[i+1]
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// collectOpenLoops gathers all open-loop entries from live and pending
// across all three scopes. Open loops are the most critical thing to
// surface — they represent in-progress work that would otherwise be
// forgotten.
func collectOpenLoops(s *store.Store) ([]store.EntryRef, error) {
	var loops []store.EntryRef

	for _, scope := range format.AllScopes() {
		live, err := s.ListLive(scope)
		if err != nil {
			return nil, err
		}
		for _, ref := range live {
			if ref.Metadata.Kind == format.KindOpenLoop {
				loops = append(loops, ref)
			}
		}

		pending, err := s.ListPending(scope)
		if err != nil {
			return nil, err
		}
		for _, ref := range pending {
			if ref.Metadata.Kind == format.KindOpenLoop {
				loops = append(loops, ref)
			}
		}
	}

	// Sort by date descending (newest first).
	sortByDateDesc(loops)
	return loops, nil
}

// collectProjectEntries gathers non-open-loop entries relevant to the
// current project. From project-shared and project-personal scopes it
// takes everything; from user-personal it filters by project name.
// Returns at most 5 entries, sorted by date descending.
func collectProjectEntries(s *store.Store, projectName string) ([]store.EntryRef, error) {
	var entries []store.EntryRef

	// Project-shared and project-personal: all entries are project-relevant by definition.
	for _, scope := range []format.Scope{format.ScopeProjectShared, format.ScopeProjectPersonal} {
		live, err := s.ListLive(scope)
		if err != nil {
			return nil, err
		}
		for _, ref := range live {
			if ref.Metadata.Kind != format.KindOpenLoop {
				entries = append(entries, ref)
			}
		}
	}

	// User-personal: only entries matching the current project.
	if projectName != "" {
		live, err := s.ListLive(format.ScopeUserPersonal)
		if err != nil {
			return nil, err
		}
		for _, ref := range live {
			if ref.Metadata.Kind != format.KindOpenLoop &&
				strings.EqualFold(ref.Metadata.Project, projectName) {
				entries = append(entries, ref)
			}
		}
	}

	sortByDateDesc(entries)

	// Cap at 5.
	if len(entries) > 5 {
		entries = entries[:5]
	}
	return entries, nil
}

// countPending returns the total number of pending entries across all scopes.
func countPending(s *store.Store) (int, error) {
	var count int
	for _, scope := range format.AllScopes() {
		refs, err := s.ListPending(scope)
		if err != nil {
			return 0, err
		}
		count += len(refs)
	}
	return count, nil
}

// formatSessionStart renders the session-start output as compact markdown.
// Returns empty string when there's nothing to surface.
func formatSessionStart(openLoops, projectEntries []store.EntryRef, pendingCount int) string {
	if len(openLoops) == 0 && len(projectEntries) == 0 && pendingCount == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## mastermind\n\n")

	if len(openLoops) > 0 {
		fmt.Fprintf(&b, "**Open loops** (%d):\n", len(openLoops))
		for _, ref := range openLoops {
			fmt.Fprintf(&b, "- %s (%s)\n", ref.Metadata.Topic, ref.Metadata.Date)
		}
		b.WriteByte('\n')
	}

	if len(projectEntries) > 0 {
		fmt.Fprintf(&b, "**Project knowledge** (%d entries):\n", len(projectEntries))
		for _, ref := range projectEntries {
			fmt.Fprintf(&b, "- %s · %s\n", ref.Metadata.Topic, ref.Metadata.Kind)
		}
		b.WriteByte('\n')
	}

	if pendingCount > 0 {
		fmt.Fprintf(&b, "**Pending review**: %d entries awaiting review.\n\n", pendingCount)
	}

	b.WriteString("_Use mm_search before complex tasks. Call mm_write to capture lessons as you work._\n")
	return b.String()
}

// ─── post-compact subcommand ──────────────────────────────────────────

// runPostCompact implements the post-compact subcommand. It fires after
// Claude Code compresses the conversation context, at which point the
// agent has lost most of its working memory. Re-injecting the curated
// project slice re-hydrates project context so the next turn starts
// oriented rather than blank.
//
// Scope is narrower than session-start: project-shared and
// project-personal only. User-personal and cross-project open loops
// are omitted — PostCompact is specifically about re-hydrating the
// project the agent was just working in, not the full session picture.
//
// PostCompact hook input JSON may include session_id and cwd fields.
// We read them if present but fall back to cwd flag / os.Getwd().
// Silent on stdin errors (hook may not provide input in all versions).
//
// If there is nothing to surface, prints nothing and exits 0.
// This honors the silent-unless-needed rule from CONTINUITY.md.
func runPostCompact() error {
	// PostCompact hook sends JSON on stdin (same shape as hookInput).
	// Try to decode it, but don't fail if stdin is empty or malformed.
	cwd := parseCwdFlag()
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err == nil {
		if input.Cwd != "" {
			cwd = input.Cwd
		}
	}

	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return fmt.Errorf("build session config: %w", err)
	}

	s := store.New(cfg)
	projectName := project.DetectFromGit(cwd)

	// Collect project-scoped open loops only (not user-personal).
	openLoops, err := collectProjectOpenLoops(s)
	if err != nil {
		return fmt.Errorf("collect project open loops: %w", err)
	}

	// Collect project-relevant knowledge entries.
	projectEntries, err := collectProjectEntries(s, projectName)
	if err != nil {
		return fmt.Errorf("collect project entries: %w", err)
	}

	output := formatPostCompact(openLoops, projectEntries)
	if output != "" {
		fmt.Print(output)
	}
	return nil
}

// collectProjectOpenLoops gathers open-loop entries from project-scoped
// stores only (project-shared and project-personal). User-personal open
// loops are excluded — PostCompact is about re-hydrating the current
// project, not the full user knowledge picture.
func collectProjectOpenLoops(s *store.Store) ([]store.EntryRef, error) {
	var loops []store.EntryRef

	for _, scope := range []format.Scope{format.ScopeProjectShared, format.ScopeProjectPersonal} {
		live, err := s.ListLive(scope)
		if err != nil {
			return nil, err
		}
		for _, ref := range live {
			if ref.Metadata.Kind == format.KindOpenLoop {
				loops = append(loops, ref)
			}
		}

		pending, err := s.ListPending(scope)
		if err != nil {
			return nil, err
		}
		for _, ref := range pending {
			if ref.Metadata.Kind == format.KindOpenLoop {
				loops = append(loops, ref)
			}
		}
	}

	sortByDateDesc(loops)
	return loops, nil
}

// formatPostCompact renders the post-compact injection as compact markdown.
// Returns empty string when there's nothing to surface.
// Omits pending count (noise after compaction) and keeps the header terse.
func formatPostCompact(openLoops, projectEntries []store.EntryRef) string {
	if len(openLoops) == 0 && len(projectEntries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## mastermind (post-compact)\n\n")

	if len(openLoops) > 0 {
		fmt.Fprintf(&b, "**Open loops** (%d):\n", len(openLoops))
		for _, ref := range openLoops {
			fmt.Fprintf(&b, "- %s (%s)\n", ref.Metadata.Topic, ref.Metadata.Date)
		}
		b.WriteByte('\n')
	}

	if len(projectEntries) > 0 {
		fmt.Fprintf(&b, "**Project knowledge** (%d entries):\n", len(projectEntries))
		for _, ref := range projectEntries {
			fmt.Fprintf(&b, "- %s · %s\n", ref.Metadata.Topic, ref.Metadata.Kind)
		}
		b.WriteByte('\n')
	}

	b.WriteString("_Context was just compacted. Use mm_search if you need deeper context._\n")
	return b.String()
}

// ─── extract subcommand ────────────────────────────────────────────────

// hookInput is the JSON structure Claude Code sends to hooks on stdin.
type hookInput struct {
	TranscriptPath string `json:"transcript_path"`
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
}

// runExtract implements the extract subcommand. It reads the conversation
// transcript (either from a hook's stdin JSON or from a --transcript flag)
// and extracts knowledge entries into pending/.
//
// Extraction mode is controlled by env vars:
//   - MASTERMIND_EXTRACT_MODE: "keyword" (default) or "llm"
//   - MASTERMIND_LLM_PROVIDER: "anthropic" (default) or "ollama"
//   - MASTERMIND_LLM_MODEL: model identifier
func runExtract() error {
	var transcriptPath, cwd string

	// Check for --from-hook flag: read JSON from stdin.
	fromHook := false
	for _, arg := range os.Args[2:] {
		if arg == "--from-hook" {
			fromHook = true
		}
	}

	if fromHook {
		var input hookInput
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			return fmt.Errorf("decode hook input: %w", err)
		}
		transcriptPath = input.TranscriptPath
		cwd = input.Cwd
	} else {
		// Manual mode: --transcript <path>
		for i := 2; i < len(os.Args)-1; i++ {
			switch os.Args[i] {
			case "--transcript":
				transcriptPath = os.Args[i+1]
			case "--cwd":
				cwd = os.Args[i+1]
			}
		}
	}

	if transcriptPath == "" {
		return fmt.Errorf("no transcript path provided (use --from-hook or --transcript <path>)")
	}

	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}

	// Read the transcript.
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		return fmt.Errorf("read transcript: %w", err)
	}
	transcript := string(data)
	if strings.TrimSpace(transcript) == "" {
		return nil // nothing to extract
	}

	// Build store config for the cwd context.
	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return fmt.Errorf("build session config: %w", err)
	}
	s := store.New(cfg)

	// Collect existing topics for deduplication.
	existingTopics := collectExistingTopics(s)

	// Configure the extractor.
	projectName := project.DetectFromGit(cwd)
	if projectName == "" {
		projectName = project.Detect(cwd)
	}

	extractCfg := extract.Config{
		Mode:        envOrDefault("MASTERMIND_EXTRACT_MODE", "keyword"),
		LLMProvider: envOrDefault("MASTERMIND_LLM_PROVIDER", "anthropic"),
		LLMModel:    os.Getenv("MASTERMIND_LLM_MODEL"),
		OllamaURL:   envOrDefault("MASTERMIND_OLLAMA_URL", "http://localhost:11434"),
		ProjectName: projectName,
	}
	extractor := extract.NewExtractor(extractCfg)

	// Extract entries.
	entries, err := extractor.Extract(transcript, existingTopics)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "mastermind extract: no entries extracted\n")
		return nil
	}

	// Write each entry to pending/.
	var written int
	for i := range entries {
		// Assign scope: project-shared if project store is configured,
		// otherwise user-personal.
		if cfg.ProjectSharedRoot != "" {
			entries[i].Metadata.Scope = format.ScopeProjectShared
		} else {
			entries[i].Metadata.Scope = format.ScopeUserPersonal
		}

		if _, err := s.Write(&entries[i]); err != nil {
			fmt.Fprintf(os.Stderr, "mastermind extract: write failed for %q: %v\n", entries[i].Metadata.Topic, err)
			continue
		}
		written++
	}

	fmt.Fprintf(os.Stderr, "mastermind extract: %d entries written to pending/\n", written)
	return nil
}

// collectExistingTopics gathers all topic strings from the live store
// across all scopes. Used for deduplication during extraction.
func collectExistingTopics(s *store.Store) []string {
	var topics []string
	for _, scope := range format.AllScopes() {
		refs, err := s.ListLive(scope)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			topics = append(topics, ref.Metadata.Topic)
		}
	}
	return topics
}

// envOrDefault returns the environment variable value or a default.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// ─── discover subcommand ──────────────────────────────────────────────

// runDiscover implements the discover subcommand. It mines git history
// and/or source code for knowledge using an LLM (Haiku or any
// OpenAI-compatible endpoint) and writes entries to pending/.
func runDiscover() error {
	mode := "all"
	depth := 100
	cwd, _ := os.Getwd()

	// Simple flag parsing (no flag package — matches other subcommands).
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "--depth":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &depth)
				i++
			}
		case "--cwd":
			if i+1 < len(args) {
				cwd = args[i+1]
				i++
			}
		default:
			// Positional: treat as mode if it's a known value.
			if args[i] == "git" || args[i] == "codebase" || args[i] == "all" {
				mode = args[i]
			}
		}
	}

	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return err
	}
	s := store.New(cfg)

	projectName := project.DetectFromGit(cwd)
	if projectName == "" {
		projectName = "general"
	}

	provider := envOrDefault("MASTERMIND_LLM_PROVIDER", "anthropic")
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if provider == "openai" {
		apiKey = os.Getenv("MASTERMIND_LLM_API_KEY")
	}

	disc, err := discover.New(discover.Config{
		Mode:        mode,
		Depth:       depth,
		Cwd:         cwd,
		ProjectName: projectName,
		LLMProvider: provider,
		LLMModel:    os.Getenv("MASTERMIND_LLM_MODEL"),
		BaseURL:     os.Getenv("MASTERMIND_LLM_BASE_URL"),
		APIKey:      apiKey,
	}, s)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "mastermind discover: mode=%s depth=%d provider=%s\n", mode, depth, provider)

	result, err := disc.Run()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nDiscovery complete: %d entries written to pending/\n", len(result.Entries))
	if result.CommitsAnalyzed > 0 || result.CommitsSkipped > 0 {
		fmt.Fprintf(os.Stderr, "  Git: %d commits analyzed, %d skipped (already discovered)\n", result.CommitsAnalyzed, result.CommitsSkipped)
	}
	if result.PackagesScanned > 0 {
		fmt.Fprintf(os.Stderr, "  Codebase: %d packages scanned\n", result.PackagesScanned)
	}
	if len(result.Entries) > 0 {
		fmt.Fprintf(os.Stderr, "\nRun /mm-review to promote the good ones.\n")
	}

	return nil
}

// ─── suggest subcommand ────────────────────────────────────────────────

// suggestHookInput is the JSON structure Claude Code sends to PostToolUse hooks.
type suggestHookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"session_id"`
}

// suggestToolInput extracts file_path from the tool_input JSON.
type suggestToolInput struct {
	FilePath string `json:"file_path"`
}

// skipSegments are directory names too generic to match against topic dirs.
var skipSegments = map[string]bool{
	"internal": true, "src": true, "lib": true, "pkg": true,
	"cmd": true, "app": true, "test": true, "tests": true,
	"spec": true, "specs": true, "build": true, "dist": true,
	"node_modules": true, "vendor": true, "components": true,
	"utils": true, "helpers": true, "common": true, "shared": true,
	"main": true, "index": true, "types": true, "models": true,
}

// runSuggest implements the suggest subcommand. It reads PostToolUse
// hook input from stdin, checks if mastermind has knowledge about the
// file being touched, and outputs a one-line nudge if so.
//
// Design: directory stat, not search. Checks if topic directories
// matching the file path exist in any .knowledge/ scope. If yes,
// counts entries and outputs a nudge. If no, outputs nothing.
// Executes in <10ms.
func runSuggest() error {
	var input suggestHookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil // silent on bad input — don't block the agent
	}

	var toolInput suggestToolInput
	if err := json.Unmarshal(input.ToolInput, &toolInput); err != nil || toolInput.FilePath == "" {
		return nil // no file path — nothing to suggest
	}

	// Extract keywords from the file path.
	keywords := extractPathKeywords(toolInput.FilePath)
	if len(keywords) == 0 {
		return nil
	}

	// Build store config to find scope roots.
	cwd := input.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	cfg, err := buildSessionConfig(cwd)
	if err != nil {
		return nil
	}

	// Check each keyword against topic dirs in all scope roots.
	roots := []string{cfg.UserPersonalRoot, cfg.ProjectSharedRoot, cfg.ProjectPersonalRoot}

	type topicMatch struct {
		dir      string // directory keyword that matched
		count    int
		topTopic string // topic from the best entry (most recent)
	}
	var matches []topicMatch
	seen := make(map[string]bool)

	for _, kw := range keywords {
		if seen[kw] {
			continue
		}
		seen[kw] = true

		total := 0
		var bestTopic string
		for _, root := range roots {
			if root == "" {
				continue
			}
			dir := filepath.Join(root, kw)
			total += countEntriesInDir(dir)
			if bestTopic == "" {
				bestTopic = bestEntryTopic(dir)
			}
		}
		if total > 0 {
			matches = append(matches, topicMatch{dir: kw, count: total, topTopic: bestTopic})
		}
	}

	if len(matches) == 0 {
		return nil
	}

	// Debounce: skip if we suggested the same file recently.
	debounceFile := filepath.Join(os.TempDir(), "mastermind-suggest-debounce")
	debounceKey := toolInput.FilePath
	if data, err := os.ReadFile(debounceFile); err == nil {
		parts := strings.SplitN(string(data), "|", 2)
		if len(parts) == 2 && parts[1] == debounceKey {
			if ts, err := time.Parse(time.RFC3339, parts[0]); err == nil {
				if time.Since(ts) < 60*time.Second {
					return nil // debounced
				}
			}
		}
	}
	_ = os.WriteFile(debounceFile, []byte(time.Now().Format(time.RFC3339)+"|"+debounceKey), 0o644)

	// Format nudge with the top entry's topic for immediate context.
	best := matches[0]
	if best.topTopic != "" {
		extra := best.count - 1
		if extra > 0 {
			fmt.Printf("_mastermind: \"%s\" + %d more in %q — consider mm_search._\n", best.topTopic, extra, best.dir)
		} else {
			fmt.Printf("_mastermind: \"%s\" — consider mm_search._\n", best.topTopic)
		}
	} else {
		fmt.Printf("_mastermind has knowledge about %q (%d entries) — consider mm_search._\n", best.dir, best.count)
	}
	return nil
}

// extractPathKeywords pulls meaningful keywords from a file path.
// Skips generic directory names and returns lowercase segments.
func extractPathKeywords(filePath string) []string {
	// Take the last 4 segments of the path (dirs + filename).
	parts := strings.Split(filepath.Clean(filePath), string(filepath.Separator))
	if len(parts) > 4 {
		parts = parts[len(parts)-4:]
	}

	var keywords []string
	for _, p := range parts {
		// Strip file extension for the filename.
		if ext := filepath.Ext(p); ext != "" {
			p = strings.TrimSuffix(p, ext)
		}
		p = strings.ToLower(p)
		if p == "" || skipSegments[p] || len(p) < 2 {
			continue
		}
		keywords = append(keywords, p)
	}
	return keywords
}

// countEntriesInDir counts .md files in a directory and its subdirs.
// Returns 0 if the directory doesn't exist.
func countEntriesInDir(dir string) int {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return 0
	}
	count := 0
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			count++
		}
		return nil
	})
	return count
}

// bestEntryTopic reads the most recently modified .md file in dir and
// returns its topic from frontmatter. Returns "" if the dir doesn't
// exist, has no entries, or parsing fails. Designed to add <1ms to the
// suggest path — reads one small file.
func bestEntryTopic(dir string) string {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}

	var bestPath string
	var bestTime time.Time
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			if bestPath == "" || fi.ModTime().After(bestTime) {
				bestPath = path
				bestTime = fi.ModTime()
			}
		}
		return nil
	})

	if bestPath == "" {
		return ""
	}
	data, err := os.ReadFile(bestPath)
	if err != nil {
		return ""
	}
	entry, err := format.Parse(data)
	if err != nil {
		return ""
	}
	return entry.Metadata.Topic
}

// sortByDateDesc sorts entry refs by date descending (newest first).
// Entries with identical dates are sorted by path for determinism.
func sortByDateDesc(refs []store.EntryRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Metadata.Date != refs[j].Metadata.Date {
			return refs[i].Metadata.Date > refs[j].Metadata.Date
		}
		return refs[i].Path < refs[j].Path
	})
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `mastermind %s — the ADHD cure for agents that you always dreamed for yourself

Usage:
  mastermind                    Start MCP server over stdio (default; used by Claude Code)
  mastermind mcp                Explicit: start MCP server
  mastermind session-start      Claude Code session-start hook (surfaces open loops + project context)
  mastermind post-compact       Claude Code PostCompact hook (re-injects project context after compaction)
  mastermind session-close      Claude Code session-close hook (phase 3b, not implemented)
  mastermind extract            Extract knowledge from a conversation transcript
  mastermind suggest            PostToolUse hook — nudge when knowledge exists for a file
  mastermind discover           Mine git history + codebase for knowledge (Haiku / OpenAI-compat)
  mastermind version            Print version and exit
  mastermind help               Show this help

Discover options:
  mastermind discover [git|codebase|all] [--depth N] [--cwd DIR]

  Env vars: MASTERMIND_LLM_PROVIDER (anthropic|openai)
            MASTERMIND_LLM_MODEL, MASTERMIND_LLM_BASE_URL, MASTERMIND_LLM_API_KEY
            ANTHROPIC_API_KEY (for anthropic provider)

MCP tools (for agent use):
  mm_search       Search persistent knowledge across scopes
  mm_write        Write a candidate entry to the pending-review queue
  mm_promote      Move a pending entry to the live store (user-gated)
  mm_close_loop   Mark an open-loop as resolved (phase 3, not implemented)

Setup:
  mastermind expects a ~/.knowledge/ directory. Initialize it as a git repo
  with a remote for cross-machine sync. Then add mastermind to your
  Claude Code MCP config:

    {
      "mcpServers": {
        "mastermind": {
          "type": "stdio",
          "command": "mastermind"
        }
      }
    }

Docs: see the project docs/ directory for the full design.
`, version)
}
