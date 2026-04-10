package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jeanfbrito/mastermind/internal/extract"
	"github.com/jeanfbrito/mastermind/internal/format"
)

// extract-audit measures the extractor's recall/precision against a
// hand-labeled corpus of real transcripts. It is the measurement harness
// for Phase 3 extractor polish — every tweak to keyword.go, llm.go, or
// the prompt should be validated with a recall/precision delta before
// landing. See the Phase 3 polish open-loop.
//
// Matching rule: an extraction matches a label when
//
//   entry.Metadata.Kind == label.Kind
//   AND strings.Contains(lower(topic+body), lower(key_phrase))
//
// One-to-one greedy matching: each label is matched at most once, each
// extraction is consumed at most once. Iteration order is by label index,
// then first unmatched extraction that satisfies the predicate.

// auditCorpus is the on-disk corpus schema. See testdata/audit/corpus.json.
type auditCorpus struct {
	Transcripts []auditTranscript `json:"transcripts"`
}

type auditTranscript struct {
	ID          string       `json:"id"`
	Path        string       `json:"path"`
	Description string       `json:"description,omitempty"`
	Labels      []auditLabel `json:"labels"`
}

type auditLabel struct {
	Kind      string `json:"kind"`
	KeyPhrase string `json:"key_phrase"`
	Notes     string `json:"notes,omitempty"`

	// Tier is the extractor tier expected to catch this label:
	// "keyword" — keyword tier (signal-phrase regexes) should catch it
	// "llm"     — LLM tier (semantic reasoning) should catch it
	// "both"    — either tier should catch it
	//
	// Defaults to "both" when empty. Used by the audit to filter which
	// labels count toward a given --mode run: a keyword-tier audit only
	// measures recall against labels with tier in {keyword, both}, and
	// likewise for llm. Labels whose tier excludes the current mode are
	// skipped entirely — they don't inflate or deflate recall.
	//
	// Set this to "llm" for semantic extractions (no signal phrase in
	// the surrounding prose) so the keyword-tier baseline isn't punished
	// for out-of-scope labels.
	Tier string `json:"tier,omitempty"`
}

// inScope reports whether this label should be measured under the given
// extractor mode. Empty Tier is treated as "both" for backward compat
// with unlabeled corpora.
func (l auditLabel) inScope(mode string) bool {
	tier := l.Tier
	if tier == "" {
		tier = "both"
	}
	if tier == "both" {
		return true
	}
	return tier == mode
}

// auditStats accumulates per-kind and overall counts.
type auditStats struct {
	Labels    int
	Extracted int
	Matched   int
}

// runExtractAudit implements `mastermind extract-audit`. It loads the
// corpus, runs the configured extractor against each transcript, matches
// extractions to labels, and prints a per-kind recall/precision table.
//
// Flags:
//
//	--corpus <path>   path to corpus.json (default: testdata/audit/corpus.json)
//	--mode <mode>     extractor mode: keyword (default) or llm
//	--provider <p>    LLM provider when --mode=llm (anthropic|ollama)
//	--model <id>      LLM model identifier
//	--verbose         print unmatched labels and extractions for each transcript
//	--json            emit machine-readable JSON instead of a table
func runExtractAudit() error {
	corpusPath := "testdata/audit/corpus.json"
	// These start empty so the config resolver provides the actual
	// defaults. A non-empty value from the CLI overrides whatever
	// the config + env-var layer resolved. This is the per-invocation
	// override layer that sits on top of the resolver stack.
	var mode, provider, model, baseURL string
	verbose := false
	jsonOut := false
	dumpOnly := false

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--corpus":
			if i+1 < len(args) {
				corpusPath = args[i+1]
				i++
			}
		case "--mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "--provider":
			if i+1 < len(args) {
				provider = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				model = args[i+1]
				i++
			}
		case "--base-url":
			if i+1 < len(args) {
				baseURL = args[i+1]
				i++
			}
		case "--verbose", "-v":
			verbose = true
		case "--json":
			jsonOut = true
		case "--dump-extractions":
			dumpOnly = true
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	corpus, err := loadAuditCorpus(corpusPath)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	if len(corpus.Transcripts) == 0 {
		return fmt.Errorf("corpus %s has no transcripts", corpusPath)
	}

	// Resolve the audit task from config + env overlay. CLI flags
	// override on top — the caller can explicitly pick a provider or
	// model for a single audit run without editing the config file.
	cwd, _ := os.Getwd()
	resolved, err := resolveTaskConfig(cwd, "audit")
	if err != nil {
		return fmt.Errorf("resolve audit config: %w", err)
	}
	if mode == "" {
		mode = resolved.Mode
	}
	if provider == "" {
		provider = resolved.Flavor
	}
	if model == "" {
		model = resolved.Model
	}
	if baseURL == "" {
		baseURL = resolved.BaseURL
	}
	apiKey := resolved.APIKey

	// Build the extractor with strict mode selection. Unlike runExtract,
	// the audit MUST NOT silently fall back from llm to keyword on setup
	// failure — a silent fallback would report keyword-tier numbers
	// under an --mode=llm header and corrupt the measurement.
	var extractor extract.Extractor
	switch mode {
	case "keyword":
		extractor = &extract.KeywordExtractor{ProjectName: "audit"}
	case "llm":
		// Strict=true disables silent fallback to the keyword tier.
		// Production extract wants graceful degradation, but the audit
		// MUST NOT report keyword numbers under an --mode=llm header.
		// The flavor-specific key/url fields are populated based on
		// the resolved flavor so NewLLMExtractor's switch finds them.
		llmCfg := extract.Config{
			Mode:        mode,
			LLMProvider: provider,
			LLMModel:    model,
			Strict:      true,
			ProjectName: "audit",
		}
		switch provider {
		case "anthropic":
			llmCfg.AnthropicAPIKey = apiKey
		case "ollama":
			llmCfg.OllamaURL = baseURL
		case "openai":
			llmCfg.BaseURL = baseURL
			llmCfg.APIKey = apiKey
		}
		llm, err := extract.NewLLMExtractor(llmCfg)
		if err != nil {
			return fmt.Errorf("llm extractor unavailable (%w) — check ~/.knowledge/config.json or ANTHROPIC_API_KEY / MASTERMIND_LLM_API_KEY / MASTERMIND_LLM_BASE_URL env vars", err)
		}
		extractor = llm
	default:
		return fmt.Errorf("unknown mode %q (want keyword|llm)", mode)
	}

	// --dump-extractions: qualitative inspection mode. Runs the
	// extractor on each corpus transcript and prints the topic and
	// source_quote (or body excerpt for keyword-tier entries) with
	// no matching, no scoring. The point is to eyeball whether the
	// LLM tier's output looks useful on a given session, without
	// forcing a substring-match metric that can't fairly score
	// paraphrasing extractors. Labels are ignored.
	if dumpOnly {
		for _, t := range corpus.Transcripts {
			raw, err := os.ReadFile(t.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "transcript %s: read failed: %v\n", t.ID, err)
				continue
			}
			prose := extract.NormalizeTranscript(string(raw))
			entries, err := extractor.Extract(prose, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "transcript %s: extract failed: %v\n", t.ID, err)
				continue
			}
			fmt.Printf("── %s (%d entries) ──\n", t.ID, len(entries))
			if t.Description != "" {
				fmt.Printf("   %s\n", t.Description)
			}
			fmt.Println()
			for _, e := range entries {
				fmt.Printf("  [%s] %s\n", e.Metadata.Kind, e.Metadata.Topic)
				if src := extractSourceQuoteFromBody(e.Body); src != "" {
					fmt.Printf("    source: %s\n", truncateForDump(src, 180))
				} else {
					// Keyword extraction: show the first non-empty
					// line of the body as the inline excerpt.
					for _, ln := range strings.Split(e.Body, "\n") {
						if s := strings.TrimSpace(ln); s != "" {
							fmt.Printf("    excerpt: %s\n", truncateForDump(s, 180))
							break
						}
					}
				}
			}
			fmt.Println()
		}
		return nil
	}

	perTranscript := make([]auditResult, 0, len(corpus.Transcripts))
	totals := make(map[string]*auditStats)

	for _, t := range corpus.Transcripts {
		result, err := auditOneTranscript(extractor, t, mode)
		if err != nil {
			return fmt.Errorf("audit transcript %s: %w", t.ID, err)
		}
		perTranscript = append(perTranscript, result)
		for kind, st := range result.PerKind {
			agg, ok := totals[kind]
			if !ok {
				agg = &auditStats{}
				totals[kind] = agg
			}
			agg.Labels += st.Labels
			agg.Extracted += st.Extracted
			agg.Matched += st.Matched
		}
	}

	if jsonOut {
		return writeAuditJSON(corpusPath, mode, perTranscript, totals)
	}
	return writeAuditTable(os.Stdout, corpusPath, mode, perTranscript, totals, verbose)
}

// auditResult bundles one transcript's outcome.
type auditResult struct {
	Transcript         auditTranscript       `json:"transcript"`
	NumExtracted       int                   `json:"extracted"`
	NumLabelsTotal     int                   `json:"labels_total"`
	NumLabelsInScope   int                   `json:"labels_in_scope"`
	PerKind            map[string]auditStats `json:"per_kind"`
	UnmatchedLabels    []unmatchedLabel      `json:"unmatched_labels,omitempty"`
	UnmatchedExtracted []extractedSummary    `json:"unmatched_extracted,omitempty"`
}

// unmatchedLabel augments an unmatched audit label with a diagnosis of
// why it was missed. This is the feedback loop for regex polish: each
// miss type points at a different class of fix.
//
//   - "kind-mismatch" — the key_phrase WAS found in an extraction body,
//     but that extraction has the wrong kind. The extractor saw the
//     signal but labeled it something else. Fix: signal-to-kind
//     mapping, or relabel the ground truth.
//
//   - "phrase-miss" — no extraction contains the key_phrase at all.
//     The extractor never had a chance at this region. Fix: new regex
//     pattern, or widened context window, or tier escalation to LLM.
type unmatchedLabel struct {
	auditLabel
	MissType    string   `json:"miss_type"`
	ActualKinds []string `json:"actual_kinds,omitempty"` // set for kind-mismatch: kinds the phrase appeared as
}

// extractedSummary is a compact view of a format.Entry for audit output.
type extractedSummary struct {
	Kind  string `json:"kind"`
	Topic string `json:"topic"`
	Body  string `json:"body,omitempty"`
}

// auditOneTranscript runs the extractor on a single transcript and
// matches results against labels. The matching is greedy one-to-one.
// Labels whose Tier excludes the current mode are skipped entirely —
// they don't contribute to either recall or precision.
func auditOneTranscript(extractor extract.Extractor, t auditTranscript, mode string) (auditResult, error) {
	raw, err := os.ReadFile(t.Path)
	if err != nil {
		return auditResult{}, fmt.Errorf("read transcript: %w", err)
	}

	// Normalize JSONL to prose before extraction. This mirrors the
	// production path in runExtract — audits must feed the extractor
	// the same thing production does, otherwise the numbers lie.
	prose := extract.NormalizeTranscript(string(raw))

	// Pre-flight label validation: every label's key_phrase MUST
	// appear in the normalized prose, because that's the string the
	// extractor actually sees. A phrase that only exists in raw JSONL
	// (e.g. inside a tool_use Write block that NormalizeTranscript
	// strips) is unreachable by any tier and the audit will silently
	// report a recall miss that no code change can ever fix. Fail
	// loudly instead so the corpus author can relabel or refine.
	proseLower := strings.ToLower(prose)
	var broken []string
	for _, l := range t.Labels {
		if !strings.Contains(proseLower, strings.ToLower(l.KeyPhrase)) {
			broken = append(broken, fmt.Sprintf("[%s/%s] %q", l.Kind, l.Tier, l.KeyPhrase))
		}
	}
	if len(broken) > 0 {
		return auditResult{}, fmt.Errorf("transcript %s has %d label(s) whose key_phrase is missing from the normalized prose (likely inside a stripped tool_use/tool_result block). Fix the corpus and retry:\n  %s",
			t.ID, len(broken), strings.Join(broken, "\n  "))
	}

	entries, err := extractor.Extract(prose, nil)
	if err != nil {
		return auditResult{}, fmt.Errorf("extract: %w", err)
	}

	// Filter labels to those in scope for the current extractor mode.
	// Out-of-scope labels (e.g. semantic-only labels against a keyword
	// tier run) are dropped so recall/precision reflect what this tier
	// can reasonably be expected to catch.
	scoped := make([]auditLabel, 0, len(t.Labels))
	for _, l := range t.Labels {
		if l.inScope(mode) {
			scoped = append(scoped, l)
		}
	}

	// Initialize per-kind stats. Seed with every kind that appears in
	// in-scope labels OR extractions so recall/precision zero rows are
	// visible. Extractions always count — the kinds the extractor
	// emits are part of what we measure, whether or not labels exist.
	perKind := make(map[string]auditStats)
	for _, l := range scoped {
		st := perKind[l.Kind]
		st.Labels++
		perKind[l.Kind] = st
	}
	for _, e := range entries {
		k := string(e.Metadata.Kind)
		st := perKind[k]
		st.Extracted++
		perKind[k] = st
	}

	// Greedy one-to-one matching.
	usedEntry := make([]bool, len(entries))
	matchedLabel := make([]bool, len(scoped))

	for li, label := range scoped {
		needle := strings.ToLower(label.KeyPhrase)
		for ei, entry := range entries {
			if usedEntry[ei] {
				continue
			}
			if string(entry.Metadata.Kind) != label.Kind {
				continue
			}
			hay := strings.ToLower(entry.Metadata.Topic + " " + entry.Body)
			if strings.Contains(hay, needle) {
				usedEntry[ei] = true
				matchedLabel[li] = true
				st := perKind[label.Kind]
				st.Matched++
				perKind[label.Kind] = st
				break
			}
		}
	}

	result := auditResult{
		Transcript:       t,
		NumExtracted:     len(entries),
		NumLabelsTotal:   len(t.Labels),
		NumLabelsInScope: len(scoped),
		PerKind:          perKind,
	}

	// Diagnostic second pass: for each unmatched label, classify WHY
	// it was missed. A kind-mismatch means the extractor saw the phrase
	// but emitted it under the wrong kind — that points at signal-to-kind
	// mapping work. A phrase-miss means the extractor never found the
	// region — that points at regex coverage or context-window work.
	// These two miss types demand different fixes, so reporting them
	// separately is the whole point of the audit feedback loop.
	for li, ok := range matchedLabel {
		if ok {
			continue
		}
		label := scoped[li]
		needle := strings.ToLower(label.KeyPhrase)
		var kindsSeen []string
		seen := make(map[string]bool)
		for _, e := range entries {
			hay := strings.ToLower(e.Metadata.Topic + " " + e.Body)
			if strings.Contains(hay, needle) {
				k := string(e.Metadata.Kind)
				if !seen[k] {
					seen[k] = true
					kindsSeen = append(kindsSeen, k)
				}
			}
		}
		u := unmatchedLabel{auditLabel: label}
		if len(kindsSeen) > 0 {
			u.MissType = "kind-mismatch"
			u.ActualKinds = kindsSeen
		} else {
			u.MissType = "phrase-miss"
		}
		result.UnmatchedLabels = append(result.UnmatchedLabels, u)
	}
	for ei, ok := range usedEntry {
		if !ok {
			body := entries[ei].Body
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			result.UnmatchedExtracted = append(result.UnmatchedExtracted, extractedSummary{
				Kind:  string(entries[ei].Metadata.Kind),
				Topic: entries[ei].Metadata.Topic,
				Body:  body,
			})
		}
	}
	return result, nil
}

func loadAuditCorpus(path string) (auditCorpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return auditCorpus{}, err
	}
	var c auditCorpus
	if err := json.Unmarshal(data, &c); err != nil {
		return auditCorpus{}, fmt.Errorf("parse corpus json: %w", err)
	}
	// Resolve relative transcript paths against the corpus file's
	// directory so the corpus is portable — it can be checked in with
	// relative paths and run from anywhere.
	corpusDir := filepath.Dir(path)
	for i := range c.Transcripts {
		p := c.Transcripts[i].Path
		if p != "" && !filepath.IsAbs(p) {
			c.Transcripts[i].Path = filepath.Join(corpusDir, p)
		}
	}
	// Validate kinds and tiers up front so audit runs don't silently miscount.
	for ti, t := range c.Transcripts {
		for li, l := range t.Labels {
			if !format.Kind(l.Kind).Valid() {
				return auditCorpus{}, fmt.Errorf("transcript[%d] %s label[%d]: invalid kind %q", ti, t.ID, li, l.Kind)
			}
			if strings.TrimSpace(l.KeyPhrase) == "" {
				return auditCorpus{}, fmt.Errorf("transcript[%d] %s label[%d]: empty key_phrase", ti, t.ID, li)
			}
			switch l.Tier {
			case "", "keyword", "llm", "both":
				// ok
			default:
				return auditCorpus{}, fmt.Errorf("transcript[%d] %s label[%d]: invalid tier %q (want keyword|llm|both)", ti, t.ID, li, l.Tier)
			}
		}
	}
	return c, nil
}

// writeAuditTable prints a human-readable recall/precision report.
func writeAuditTable(out *os.File, corpusPath, mode string, results []auditResult, totals map[string]*auditStats, verbose bool) error {
	fmt.Fprintf(out, "mastermind extract-audit — corpus=%s mode=%s\n\n", corpusPath, mode)

	for _, r := range results {
		fmt.Fprintf(out, "── %s ", r.Transcript.ID)
		if r.Transcript.Description != "" {
			fmt.Fprintf(out, "(%s) ", r.Transcript.Description)
		}
		fmt.Fprintf(out, "──\n")
		fmt.Fprintf(out, "  transcript: %s\n", r.Transcript.Path)
		if r.NumLabelsInScope != r.NumLabelsTotal {
			fmt.Fprintf(out, "  labels: %d in-scope of %d total (tier filter active)  extracted: %d\n\n",
				r.NumLabelsInScope, r.NumLabelsTotal, r.NumExtracted)
		} else {
			fmt.Fprintf(out, "  labels: %d  extracted: %d\n\n", r.NumLabelsInScope, r.NumExtracted)
		}

		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  Kind\tLabels\tExtracted\tMatched\tRecall\tPrecision")
		for _, kind := range sortedKinds(r.PerKind) {
			st := r.PerKind[kind]
			fmt.Fprintf(tw, "  %s\t%d\t%d\t%d\t%s\t%s\n",
				kind, st.Labels, st.Extracted, st.Matched,
				fmtRatio(st.Matched, st.Labels),
				fmtRatio(st.Matched, st.Extracted),
			)
		}
		tw.Flush()
		fmt.Fprintln(out)

		// Miss-type summary always shown (not gated on verbose) when
		// there ARE unmatched labels — it's a one-line signal about
		// where the next polish pass should aim.
		if len(r.UnmatchedLabels) > 0 {
			phraseMiss, kindMismatch := 0, 0
			for _, u := range r.UnmatchedLabels {
				switch u.MissType {
				case "kind-mismatch":
					kindMismatch++
				case "phrase-miss":
					phraseMiss++
				}
			}
			fmt.Fprintf(out, "  misses: %d phrase-miss, %d kind-mismatch\n\n",
				phraseMiss, kindMismatch)
		}
		if verbose && len(r.UnmatchedLabels) > 0 {
			fmt.Fprintf(out, "  unmatched labels (missed recall):\n")
			for _, l := range r.UnmatchedLabels {
				tag := l.MissType
				if tag == "kind-mismatch" && len(l.ActualKinds) > 0 {
					tag = fmt.Sprintf("kind-mismatch: extractor said %s", strings.Join(l.ActualKinds, ","))
				}
				fmt.Fprintf(out, "    - [%s] %q\n", l.Kind, l.KeyPhrase)
				fmt.Fprintf(out, "      why: %s\n", tag)
				if l.Notes != "" {
					fmt.Fprintf(out, "      note: %s\n", l.Notes)
				}
			}
			fmt.Fprintln(out)
		}
		if verbose && len(r.UnmatchedExtracted) > 0 {
			fmt.Fprintf(out, "  unmatched extractions (potential false positives):\n")
			for _, e := range r.UnmatchedExtracted {
				fmt.Fprintf(out, "    - [%s] %s\n", e.Kind, e.Topic)
			}
			fmt.Fprintln(out)
		}
	}

	// Totals block — always printed so a single-transcript run still
	// gets a summary row, and a multi-transcript run gets the aggregate.
	{
		var totalLabels, totalExtracted, totalMatched int
		fmt.Fprintln(out, "── totals ──")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  Kind\tLabels\tExtracted\tMatched\tRecall\tPrecision")
		kinds := make([]string, 0, len(totals))
		for k := range totals {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			st := totals[kind]
			totalLabels += st.Labels
			totalExtracted += st.Extracted
			totalMatched += st.Matched
			fmt.Fprintf(tw, "  %s\t%d\t%d\t%d\t%s\t%s\n",
				kind, st.Labels, st.Extracted, st.Matched,
				fmtRatio(st.Matched, st.Labels),
				fmtRatio(st.Matched, st.Extracted),
			)
		}
		fmt.Fprintf(tw, "  TOTAL\t%d\t%d\t%d\t%s\t%s\n",
			totalLabels, totalExtracted, totalMatched,
			fmtRatio(totalMatched, totalLabels),
			fmtRatio(totalMatched, totalExtracted),
		)
		tw.Flush()
	}
	return nil
}

// writeAuditJSON emits the audit as a single JSON object on stdout.
func writeAuditJSON(corpusPath, mode string, results []auditResult, totals map[string]*auditStats) error {
	payload := map[string]interface{}{
		"corpus":      corpusPath,
		"mode":        mode,
		"transcripts": results,
		"totals":      totals,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// sortedKinds returns the kinds in canonical order (as defined by
// format.AllKinds), with any extras appended alphabetically.
func sortedKinds(perKind map[string]auditStats) []string {
	canonical := format.AllKinds()
	seen := make(map[string]bool)
	out := make([]string, 0, len(perKind))
	for _, k := range canonical {
		if _, ok := perKind[string(k)]; ok {
			out = append(out, string(k))
			seen[string(k)] = true
		}
	}
	extras := make([]string, 0)
	for k := range perKind {
		if !seen[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	out = append(out, extras...)
	return out
}

// extractSourceQuoteFromBody pulls the verbatim source quote out of an
// LLM-extracted entry's body. Extractions from the LLM tier have the
// quote appended as a "## Source" section by parseExtractionResponse;
// keyword-tier entries don't have this section and return "".
func extractSourceQuoteFromBody(body string) string {
	marker := "\n\n## Source\n"
	idx := strings.LastIndex(body, marker)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(body[idx+len(marker):])
}

// truncateForDump clamps a string to n characters and appends "…" if
// it was cut. Used by --dump-extractions to keep output readable.
func truncateForDump(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// fmtRatio formats a ratio as "0.667". Returns "  n/a" when denominator is 0.
func fmtRatio(num, denom int) string {
	if denom == 0 {
		return "  n/a"
	}
	return fmt.Sprintf("%.3f", float64(num)/float64(denom))
}
