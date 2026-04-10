package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jeanfbrito/mastermind/internal/format"
)

// LLMExtractor sends the transcript to a language model for structured
// knowledge extraction. Supports Anthropic API (Haiku) and Ollama
// (local models). Falls back to KeywordExtractor on any error.
type LLMExtractor struct {
	cfg         Config
	keyword     *KeywordExtractor // fallback
	httpClient  *http.Client
}

// NewLLMExtractor creates an LLMExtractor. Returns an error if the
// required configuration is missing (e.g., no API key for Anthropic).
func NewLLMExtractor(cfg Config) (*LLMExtractor, error) {
	if cfg.LLMProvider == "anthropic" {
		key := cfg.AnthropicAPIKey
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
		if key == "" {
			return nil, fmt.Errorf("extract: ANTHROPIC_API_KEY not set")
		}
		cfg.AnthropicAPIKey = key
		if cfg.LLMModel == "" {
			cfg.LLMModel = "claude-haiku-4-5-20251001"
		}
	} else if cfg.LLMProvider == "ollama" {
		if cfg.OllamaURL == "" {
			cfg.OllamaURL = "http://localhost:11434"
		}
		if cfg.LLMModel == "" {
			cfg.LLMModel = "llama3.2"
		}
	} else if cfg.LLMProvider == "openai" {
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("extract: openai provider requires BaseURL (e.g. https://api.openai.com/v1)")
		}
		key := cfg.APIKey
		if key == "" {
			key = os.Getenv("MASTERMIND_LLM_API_KEY")
		}
		if key == "" {
			return nil, fmt.Errorf("extract: openai provider requires APIKey or MASTERMIND_LLM_API_KEY env var")
		}
		cfg.APIKey = key
		if cfg.LLMModel == "" {
			return nil, fmt.Errorf("extract: openai provider requires LLMModel (no default)")
		}
	} else {
		return nil, fmt.Errorf("extract: unknown LLM provider %q", cfg.LLMProvider)
	}

	// Timeout is generous because audits against local inference
	// gateways (vLLM, LM Studio, etc.) can be slow on large models,
	// and session transcripts are already capped at 100KB below.
	return &LLMExtractor{
		cfg:     cfg,
		keyword: &KeywordExtractor{ProjectName: cfg.ProjectName},
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
	}, nil
}

// extractionPrompt is the system prompt for LLM-based extraction.
//
// Design notes: the prompt deliberately does NOT ask the LLM for a "scope"
// field. Scope is assigned by the caller in cmd/mastermind/main.go based on
// the store configuration (project-shared when a project store exists, else
// user-personal) — see internal/extract/extractor.go for the contract.
//
// The prompt is deliberately high-recall. The pending/ queue plus
// /mm-review exist precisely so that false positives are cheap to prune
// but missed lessons are impossible to recover. When in doubt, extract it.
const extractionPrompt = `You are a knowledge extraction agent. Analyze the conversation transcript and extract every lesson, decision, pattern, insight, war-story, or open-loop worth preserving.

For each extracted entry, return a JSON object with:
- "topic": one-line summary (under 120 chars)
- "kind": one of "lesson", "insight", "war-story", "decision", "pattern", "open-loop"
- "body": at least 3 lines explaining what happened, why it matters, and the actionable takeaway. Use as many more lines as needed to capture the full context — do NOT compress or summarize, preserve the reasoning and any verbatim quotes that carry signal
- "source_quote": a REQUIRED verbatim substring (10-25 words) copied EXACTLY from the transcript that anchors this entry. Must be a literal substring — no paraphrasing, no ellipsis. Pick the phrase that most concentrates the signal. HARD CONSTRAINT: the phrase MUST NOT contain any double-quote character ("). If the natural phrase has quotes in it, pick an adjacent quote-free region — every transcript has plenty. Backticks, asterisks, en-dashes, parentheses are all fine. Only double quotes are forbidden, because they break JSON encoding downstream. If you cannot find a 10+ word quote-free span, do not include the entry.
- "tags": 2-5 lowercase tags
- "category": topic directory path, 1-2 segments (e.g., "go", "electron/ipc", "mcp")

Return a JSON array of objects. If nothing worth extracting, return [].

Open-loop signals to scan for: "I'll come back to this", "let me finish this later", "I should ask X about Y", "after the deploy I'll...", "remind me to...", "we still need to...", or any unresolved thread the user clearly intended to continue but didn't finish in this session. Open-loops are first-class — do not skip them because they feel incomplete, that IS the point of the kind.

All six kinds matter equally. Do not de-prioritize insight, pattern, war-story, or open-loop in favor of decision and lesson — a missed pattern or war-story is just as lost as a missed decision.

Be thorough — extract EVERYTHING worth remembering. When uncertain whether something is worth capturing, extract it. The pending review queue exists to prune false positives cheaply; a missed lesson cannot be recovered.

CRITICAL: source_quote is mandatory and must be VERBATIM. Downstream systems match entries back to the transcript using this field — a paraphrased or missing quote makes the entry unreachable for audit and deduplication. Copy the phrase character-for-character including punctuation. If the phrase has markdown formatting (**bold**, backticks), keep them. If it has unusual punctuation (— en-dash, quotes), keep them.`

// Extract sends the transcript to the configured LLM and parses the
// structured response into entries. Falls back to KeywordExtractor
// on any error (unless Strict is set on the config).
//
// Gap-fill skip (2026-04-10): the keyword tier always runs first,
// regardless of Mode. If it returns at least GapFillThreshold
// entries, the LLM call is skipped entirely — the rule-based tier
// was already rich enough, and paying API cost to re-extract the
// same signals is wasteful. Threshold defaults to 5 (see
// DefaultConfig); zero disables the skip. Borrowed from soulforge's
// compaction-v2 buildV2Summary pattern — see DECISIONS.md.
func (l *LLMExtractor) Extract(transcript string, existingTopics []string) ([]format.Entry, error) {
	// Truncate transcript if too long for the model. 400000 chars is
	// roughly 100k tokens, which fits comfortably inside a 256k
	// context window alongside the extraction prompt and a large
	// JSON output. Smaller context windows will still error loudly
	// rather than silently lose data — adjust this cap if you wire
	// a model with a tighter limit.
	if len(transcript) > 400000 {
		transcript = transcript[:400000]
	}

	// Pass 1: keyword tier always runs first. If it's already rich
	// enough, return without touching the LLM.
	kwEntries, kwErr := l.keyword.Extract(transcript, existingTopics)
	if kwErr == nil && l.cfg.GapFillThreshold > 0 && len(kwEntries) >= l.cfg.GapFillThreshold {
		return kwEntries, nil
	}

	// Pass 2: LLM. Prepend a session-timestamp header so the model
	// can correctly ground relative temporal references ("tomorrow",
	// "next sprint", "by end of month") against today's date.
	// Borrowed from OpenViking's extraction prompt structure — see
	// docs/reference-notes/openviking.md and DECISIONS.md.
	transcript = sessionTimestampHeader() + transcript

	var rawJSON string
	var err error

	switch l.cfg.LLMProvider {
	case "anthropic":
		rawJSON, err = l.callAnthropic(transcript)
	case "ollama":
		rawJSON, err = l.callOllama(transcript)
	case "openai":
		rawJSON, err = l.callOpenAI(transcript)
	}

	if err != nil {
		if l.cfg.Strict {
			return nil, fmt.Errorf("llm call failed: %w", err)
		}
		// Fall back to the already-computed keyword results.
		// kwErr may be non-nil; pass it through so the caller sees
		// the root cause if BOTH paths failed.
		if kwErr != nil {
			return nil, kwErr
		}
		return kwEntries, nil
	}

	entries, err := parseExtractionResponse(rawJSON, l.cfg.ProjectName, existingTopics)
	if err != nil {
		if l.cfg.Strict {
			// Dump the full raw response to a temp file so the
			// operator can inspect what the model actually produced.
			// A 200-char preview in the error is rarely enough to
			// diagnose JSON breakage in long outputs.
			dump := fmt.Sprintf("/tmp/mastermind-llm-raw-%d.json", time.Now().UnixNano())
			_ = os.WriteFile(dump, []byte(rawJSON), 0o644)
			return nil, fmt.Errorf("llm response parse failed (full raw saved to %s, %d bytes): %w", dump, len(rawJSON), err)
		}
		if kwErr != nil {
			return nil, kwErr
		}
		return kwEntries, nil
	}

	return entries, nil
}

// sessionTimestampHeader returns a one-line header that grounds the
// LLM in today's date. Prepended to the transcript in Extract so the
// model can resolve relative temporal references correctly. The
// format matches OpenViking's extraction prompt anchor:
//
//	Session time: 2026-04-10 (Monday)
//
// Unit tests that want deterministic output can override this via
// the sessionNowForTest hook (kept in the test file).
func sessionTimestampHeader() string {
	return fmt.Sprintf("Session time: %s\n\n", sessionNow().Format("2006-01-02 (Monday)"))
}

// sessionNow is the wall-clock source for sessionTimestampHeader.
// Indirected through a package-level var so tests can freeze time
// without patching time.Now globally. Production value is time.Now.
var sessionNow = time.Now

// callAnthropic sends the transcript to the Anthropic Messages API.
func (l *LLMExtractor) callAnthropic(transcript string) (string, error) {
	body := map[string]interface{}{
		"model":      l.cfg.LLMModel,
		"max_tokens": 4096,
		"system":     extractionPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": "Extract knowledge from this conversation transcript:\n\n" + transcript},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", l.cfg.AnthropicAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("anthropic: status %d", resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return result.Content[0].Text, nil
}

// callOllama sends the transcript to a local Ollama instance.
func (l *LLMExtractor) callOllama(transcript string) (string, error) {
	body := map[string]interface{}{
		"model":  l.cfg.LLMModel,
		"stream": false,
		"messages": []map[string]string{
			{"role": "system", "content": extractionPrompt},
			{"role": "user", "content": "Extract knowledge from this conversation transcript:\n\n" + transcript},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	resp, err := l.httpClient.Post(l.cfg.OllamaURL+"/api/chat", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Message.Content, nil
}

// callOpenAI sends the transcript to any OpenAI-compatible chat
// completions endpoint (openai.com, vLLM, LM Studio, local inference
// gateways, etc.) identified by l.cfg.BaseURL. The base URL should
// already include the /v1 prefix; this function appends /chat/completions.
func (l *LLMExtractor) callOpenAI(transcript string) (string, error) {
	body := map[string]interface{}{
		"model": l.cfg.LLMModel,
		"messages": []map[string]string{
			{"role": "system", "content": extractionPrompt},
			{"role": "user", "content": "Extract knowledge from this conversation transcript:\n\n" + transcript},
		},
		// temperature 0 keeps the JSON shape stable across runs, which
		// matters for audit reproducibility.
		"temperature": 0,
		// Generous max_tokens because (a) reasoning models (Gemopus,
		// DeepSeek-R1, etc.) spend most of their budget in a private
		// reasoning phase before emitting final content, (b) the
		// extraction prompt asks for a JSON array of multi-line
		// entries, and (c) real sessions can produce 10+ entries
		// each with a few hundred tokens of body. 64k fits inside
		// a 256k-context model alongside a ~100k-token transcript.
		// Smaller endpoints will clamp this and truncation recovery
		// in parseExtractionResponse handles the aftermath.
		"max_tokens": 65536,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := strings.TrimRight(l.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Decode both "content" and "reasoning_content". Some reasoning
	// models (Gemopus, DeepSeek-R1, GLM-4.5) stream their chain of
	// thought into reasoning_content and leave content empty when
	// they run out of budget before producing final output. In that
	// case reasoning_content is the only thing we have, and the
	// existing parser's bracket-trimming handles the prose-wrapped
	// JSON that typically ends up there.
	var result struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("openai decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
	}
	msg := result.Choices[0].Message
	// Prefer final content; fall back to reasoning_content if the
	// model ran out of budget mid-reasoning. The parser does its
	// own bracket/fence trimming so prose-wrapped JSON still works.
	if strings.TrimSpace(msg.Content) != "" {
		return msg.Content, nil
	}
	if strings.TrimSpace(msg.ReasoningContent) != "" {
		return msg.ReasoningContent, nil
	}
	return "", fmt.Errorf("openai: empty content and empty reasoning_content (finish_reason=%s)", result.Choices[0].FinishReason)
}

// recoverTruncatedArray attempts to salvage a JSON array that was cut
// off mid-object by walking the input with a tiny state machine and
// finding the last position where a complete object closed at array
// level. It returns the recovered string (with a closing `]` appended)
// and true on success, or ("", false) if no complete object exists.
//
// The state machine tracks brace depth while respecting string
// literals and escape sequences, so `{` inside a string value doesn't
// mess with depth. Array depth starts at 1 after the opening `[`;
// an object at array level is depth 2, and we mark the position every
// time a `}` brings us back to depth 1.
func recoverTruncatedArray(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "[") {
		return "", false
	}

	depth := 0
	inString := false
	escape := false
	lastGoodEnd := -1

	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if c == '}' && depth == 1 {
				lastGoodEnd = i
			}
		}
	}

	if lastGoodEnd < 0 {
		return "", false
	}
	return raw[:lastGoodEnd+1] + "]", true
}

// extractedEntry is the JSON schema the LLM is asked to produce.
type extractedEntry struct {
	Topic       string   `json:"topic"`
	Kind        string   `json:"kind"`
	Body        string   `json:"body"`
	SourceQuote string   `json:"source_quote"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
}

// parseExtractionResponse parses the LLM's JSON output into entries.
// Handles three common failure modes:
//
//   - markdown code fences (```json … ```) — stripped via bracket scan
//   - prose before/after the array — stripped via bracket scan
//   - truncation (model hit max_tokens mid-object) — recovery closes
//     the array at the last well-formed object boundary and retries
//
// The last one is the dangerous one: without recovery, a single
// truncated entry kills the entire extraction pass, which is
// exactly what happened in the Phase 3 audit against local
// reasoning models that produce lots of output.
func parseExtractionResponse(raw string, projectName string, existingTopics []string) ([]format.Entry, error) {
	// The LLM might wrap JSON in markdown code fences.
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[idx:]
	}
	if idx := strings.LastIndex(raw, "]"); idx >= 0 {
		raw = raw[:idx+1]
	}

	var extracted []extractedEntry
	err := json.Unmarshal([]byte(raw), &extracted)
	if err != nil {
		// Truncation recovery: walk backwards to find the last
		// position where a complete object ended. A "complete
		// object" is identified by brace balancing outside of
		// string literals. Once we find the last `}` that closes
		// an object at depth 1 inside the top-level array, we
		// truncate to that position and append a closing `]`.
		if recovered, ok := recoverTruncatedArray(raw); ok {
			if err2 := json.Unmarshal([]byte(recovered), &extracted); err2 == nil {
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("parse extraction response: %w", err)
		}
	}

	existingLower := make(map[string]bool)
	for _, t := range existingTopics {
		existingLower[strings.ToLower(t)] = true
	}

	project := projectName
	if project == "" {
		project = "general"
	}

	var entries []format.Entry
	for _, e := range extracted {
		if e.Topic == "" || e.Body == "" {
			continue
		}

		kind := format.Kind(e.Kind)
		if !kind.Valid() {
			kind = format.KindLesson
		}

		topicLower := strings.ToLower(e.Topic)
		if isDuplicate(topicLower, existingLower) {
			continue
		}

		// Append the verbatim source_quote to the body as a
		// "## Source" section. This anchors the paraphrased
		// body back to the transcript and lets the audit's
		// substring matcher find labels whose key_phrase lands
		// inside the quoted region. Without this section, LLM
		// extractions never match audit labels because the
		// body is paraphrased, not verbatim.
		body := e.Body
		if quote := strings.TrimSpace(e.SourceQuote); quote != "" {
			body = body + "\n\n## Source\n" + quote
		}

		entries = append(entries, format.Entry{
			Metadata: format.Metadata{
				Date:       time.Now().UTC().Format("2006-01-02"),
				Project:    project,
				Tags:       e.Tags,
				Topic:      e.Topic,
				Kind:       kind,
				Category:   e.Category,
				Confidence: format.ConfidenceMedium,
			},
			Body: body,
		})
	}

	return entries, nil
}
