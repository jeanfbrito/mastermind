package discover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// llmClient is a thin HTTP client for calling LLMs. Supports Anthropic
// Messages API and any OpenAI-compatible /v1/chat/completions endpoint
// (Ollama, LM Studio, vLLM, Together.ai, Groq, etc.).
type llmClient struct {
	provider   string // "anthropic" or "openai"
	model      string
	apiKey     string
	baseURL    string // for openai provider
	httpClient *http.Client
}

func newLLMClient(cfg Config) (*llmClient, error) {
	c := &llmClient{
		provider:   cfg.LLMProvider,
		model:      cfg.LLMModel,
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}

	switch c.provider {
	case "anthropic":
		if c.apiKey == "" {
			return nil, fmt.Errorf("discover: ANTHROPIC_API_KEY required for anthropic provider")
		}
		if c.model == "" {
			c.model = "claude-haiku-4-5-20251001"
		}
	case "openai":
		if c.baseURL == "" {
			c.baseURL = "http://localhost:11434/v1"
		}
		if c.model == "" {
			c.model = "llama3.2"
		}
	default:
		return nil, fmt.Errorf("discover: unknown provider %q (use anthropic or openai)", c.provider)
	}

	return c, nil
}

// Call sends a system+user message pair to the configured LLM and returns
// the assistant's text response.
func (c *llmClient) Call(systemPrompt, userMessage string) (string, error) {
	switch c.provider {
	case "anthropic":
		return c.callAnthropic(systemPrompt, userMessage)
	case "openai":
		return c.callOpenAI(systemPrompt, userMessage)
	default:
		return "", fmt.Errorf("discover: unknown provider %q", c.provider)
	}
}

func (c *llmClient) callAnthropic(systemPrompt, userMessage string) (string, error) {
	body := map[string]any{
		"model":      c.model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
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
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(respBody))
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

func (c *llmClient) callOpenAI(systemPrompt, userMessage string) (string, error) {
	body := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMessage},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	url := c.baseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai-compat: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai-compat: empty response (no choices)")
	}
	return result.Choices[0].Message.Content, nil
}
