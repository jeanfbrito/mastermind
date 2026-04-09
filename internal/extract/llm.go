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
	} else {
		return nil, fmt.Errorf("extract: unknown LLM provider %q", cfg.LLMProvider)
	}

	return &LLMExtractor{
		cfg:     cfg,
		keyword: &KeywordExtractor{ProjectName: cfg.ProjectName},
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// extractionPrompt is the system prompt for LLM-based extraction.
const extractionPrompt = `You are a knowledge extraction agent. Analyze the conversation transcript and extract every lesson, decision, pattern, insight, war-story, or open-loop worth preserving.

For each extracted entry, return a JSON object with:
- "topic": one-line summary (under 120 chars)
- "kind": one of "lesson", "insight", "war-story", "decision", "pattern", "open-loop"
- "body": 3-10 lines explaining what happened, why it matters, and the actionable takeaway
- "tags": 2-5 lowercase tags
- "category": topic directory path, 1-2 segments (e.g., "go", "electron/ipc", "mcp")

Return a JSON array of objects. If nothing worth extracting, return [].
Be thorough — extract EVERYTHING worth remembering. Err on the side of capturing more.
Do NOT extract trivial observations or restate what was done — focus on LESSONS and DECISIONS.`

// Extract sends the transcript to the configured LLM and parses the
// structured response into entries. Falls back to KeywordExtractor
// on any error.
func (l *LLMExtractor) Extract(transcript string, existingTopics []string) ([]format.Entry, error) {
	// Truncate transcript if too long for the model.
	if len(transcript) > 100000 {
		transcript = transcript[:100000]
	}

	var rawJSON string
	var err error

	switch l.cfg.LLMProvider {
	case "anthropic":
		rawJSON, err = l.callAnthropic(transcript)
	case "ollama":
		rawJSON, err = l.callOllama(transcript)
	}

	if err != nil {
		// Fall back to keyword extraction.
		return l.keyword.Extract(transcript, existingTopics)
	}

	entries, err := parseExtractionResponse(rawJSON, l.cfg.ProjectName, existingTopics)
	if err != nil {
		return l.keyword.Extract(transcript, existingTopics)
	}

	return entries, nil
}

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

// extractedEntry is the JSON schema the LLM is asked to produce.
type extractedEntry struct {
	Topic    string   `json:"topic"`
	Kind     string   `json:"kind"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"`
}

// parseExtractionResponse parses the LLM's JSON output into entries.
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
	if err := json.Unmarshal([]byte(raw), &extracted); err != nil {
		return nil, fmt.Errorf("parse extraction response: %w", err)
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
			Body: e.Body,
		})
	}

	return entries, nil
}
