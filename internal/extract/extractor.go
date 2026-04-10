// Package extract implements knowledge extraction from conversation
// transcripts. It provides a pluggable Extractor interface with two
// implementations:
//
//   - KeywordExtractor: regex/pattern-based extraction (default, zero-dep)
//   - LLMExtractor: sends transcript to a model for structured extraction
//     (optional, requires API key or local Ollama)
//
// Extracted entries land in pending/ via store.Write — the user wasn't
// present during extraction, so they need review before promotion.
// This is the capture side of the continuity layer described in
// docs/CONTINUITY.md and docs/EXTRACTION.md.
package extract

import (
	"github.com/jeanfbrito/mastermind/internal/format"
)

// Extractor produces candidate entries from a conversation transcript.
// Implementations should set Confidence to medium (auto-extracted) and
// leave Scope empty (the caller assigns scope based on context).
type Extractor interface {
	// Extract analyzes the transcript text and returns candidate entries.
	// existingTopics is a list of topics already in the store, used for
	// deduplication — extracted entries whose topics closely match an
	// existing one are dropped.
	Extract(transcript string, existingTopics []string) ([]format.Entry, error)
}

// Config controls extraction behavior.
type Config struct {
	// Mode selects the extraction backend: "keyword" (default) or "llm".
	Mode string

	// LLMProvider selects the LLM backend: "anthropic", "ollama", or "openai".
	// The "openai" provider speaks the OpenAI Chat Completions protocol and
	// works against any compatible endpoint (OpenAI, local vLLM, LM Studio,
	// custom inference gateways) via BaseURL + APIKey.
	LLMProvider string

	// LLMModel is the model identifier. Default depends on provider:
	// "claude-haiku-4-5-20251001" for Anthropic, "llama3.2" for Ollama,
	// required (no default) for OpenAI-compatible.
	LLMModel string

	// OllamaURL is the Ollama API endpoint. Default: "http://localhost:11434".
	OllamaURL string

	// AnthropicAPIKey is the Anthropic API key. Falls back to ANTHROPIC_API_KEY env var.
	AnthropicAPIKey string

	// BaseURL is the base URL for the openai provider, e.g.
	// "https://api.openai.com/v1" or "http://192.168.x.x:PORT/v1".
	// The "/chat/completions" path is appended by the client.
	BaseURL string

	// APIKey is the bearer token for the openai provider. Falls back
	// to MASTERMIND_LLM_API_KEY env var.
	APIKey string

	// Strict disables silent fallback to the keyword tier when the LLM
	// call or response parsing fails. Default false preserves the
	// production behavior (runExtract wants graceful degradation:
	// a broken API must not prevent knowledge capture). The audit
	// enables Strict so measurement doesn't silently report keyword
	// numbers under an --mode=llm header.
	Strict bool

	// ProjectName is the detected project name, used to set the project
	// field on extracted entries.
	ProjectName string
}

// DefaultConfig returns a Config with sensible defaults.
// All fields can be overridden via environment variables.
func DefaultConfig() Config {
	return Config{
		Mode:        "keyword",
		LLMProvider: "anthropic",
		LLMModel:    "",
		OllamaURL:   "http://localhost:11434",
	}
}

// NewExtractor creates an Extractor based on the config.
// Falls back to KeywordExtractor if LLM setup fails.
func NewExtractor(cfg Config) Extractor {
	if cfg.Mode == "llm" {
		llm, err := NewLLMExtractor(cfg)
		if err == nil {
			return llm
		}
		// Fall back to keyword extraction on any setup error.
	}
	return &KeywordExtractor{ProjectName: cfg.ProjectName}
}
