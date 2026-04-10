// Package config implements mastermind's task-router configuration:
// a single JSON file (at ~/.knowledge/config.json, optionally overlaid
// by <project>/.knowledge/config.json) that binds tasks — "extract",
// "discover", "audit" — to named LLM providers and models.
//
// Before this package, mastermind's model/provider selection was
// scattered across five-plus environment variables and three
// subcommands. Each subcommand read its own env vars, invented its
// own defaults, and had its own precedence rules. This package is the
// single source of truth: each task resolves to a flat Resolved
// struct that carries everything a call needs (flavor, base URL,
// api key, model, mode). Environment variables still work — they
// overlay on top of the config so existing scripts keep working —
// but they are no longer the primary interface.
//
// Design philosophy mirrors soulforge's taskRouter: one config file,
// named providers decoupled from wire protocols (flavors), per-task
// bindings. Different tasks can use different backends; cheap local
// models for high-volume work, frontier models for precision.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Config is the top-level structure loaded from JSON. A zero-value
// Config is safe to Resolve against — it just falls through to env
// vars and built-in defaults.
type Config struct {
	// Providers maps a user-chosen friendly name (e.g. "anthropic",
	// "local-vllm", "my-ollama") to the provider's connection details.
	// The name is a reference label only — the wire protocol is
	// determined by ProviderConfig.Flavor.
	Providers map[string]ProviderConfig `json:"providers,omitempty"`

	// Tasks maps a task name ("extract", "discover", "audit") to its
	// provider binding and model. Any task without an entry falls
	// back to env vars + defaults.
	Tasks map[string]TaskConfig `json:"tasks,omitempty"`
}

// ProviderConfig describes one named LLM endpoint. A provider is
// connection-level metadata: where to call and how to authenticate.
// Models are separately specified per task.
type ProviderConfig struct {
	// Flavor selects the wire protocol used to talk to the endpoint:
	// "anthropic", "openai", or "ollama". This is distinct from the
	// provider name (which is a free-form label). If empty, Resolve
	// infers from the provider name when it's a known flavor
	// ("anthropic"/"openai"/"ollama"), otherwise defaults to "openai"
	// since that's the universal local-LLM protocol.
	Flavor string `json:"flavor,omitempty"`

	// BaseURL is the HTTP endpoint base. Required for "openai" and
	// "ollama" flavors. Ignored for "anthropic" (which always uses
	// api.anthropic.com).
	BaseURL string `json:"base_url,omitempty"`

	// APIKey is the bearer token, inlined directly. Convenient but
	// discouraged — keys in a committed config file are a leak risk.
	// Prefer APIKeyEnv.
	APIKey string `json:"api_key,omitempty"`

	// APIKeyEnv is the name of an environment variable that holds the
	// bearer token. Resolve reads os.Getenv(APIKeyEnv) at resolution
	// time. This keeps secrets out of the config file so the file can
	// be committed or shared.
	APIKeyEnv string `json:"api_key_env,omitempty"`
}

// TaskConfig binds a task name to a provider (by name) and a model.
// All fields are optional; missing fields fall through to env-var
// overrides and then built-in defaults.
type TaskConfig struct {
	// Mode is specific to the "extract" task: "keyword" (default) or
	// "llm". For other tasks this field is ignored.
	Mode string `json:"mode,omitempty"`

	// Provider is the name of an entry in Config.Providers. An empty
	// Provider means "use env vars / defaults" — the resolver will
	// not error on an empty binding.
	Provider string `json:"provider,omitempty"`

	// Model is the model identifier passed to the provider. Shape
	// depends on the flavor: "claude-haiku-4-5-20251001" for
	// anthropic, "llama/gemopus-26b-a4b" for openai-compatible, etc.
	Model string `json:"model,omitempty"`
}

// Resolved is the fully-resolved configuration for a single task
// invocation. It is the flat struct callers actually consume — no
// map lookups, no optional fields, no reference indirection. Build
// one with Config.ResolveTask.
type Resolved struct {
	// TaskName is the name of the task this resolution is for
	// (e.g. "extract", "discover", "audit"). Useful for error
	// messages and logging.
	TaskName string

	// Mode is the extraction mode for the "extract" task:
	// "keyword" or "llm". Empty for other tasks.
	Mode string

	// Flavor is the wire protocol: "anthropic", "openai", or "ollama".
	Flavor string

	// Model is the model identifier to send in API requests.
	Model string

	// BaseURL is the HTTP endpoint base. Empty for anthropic flavor.
	BaseURL string

	// APIKey is the fully-resolved bearer token (inline value OR the
	// env var dereferenced). Empty when none is configured.
	APIKey string

	// ProviderName echoes the name from the Providers map, or "" when
	// the task didn't bind to a named provider. For diagnostics only.
	ProviderName string
}

// Load reads a user-level config file and optionally overlays a
// project-level config file on top. Either path can be empty; missing
// files are silently treated as empty configs (returning a zero-value
// Config is NOT an error — the whole point of this package is that
// mastermind works without a config file at all).
//
// Precedence: projectPath overrides userPath on key collision. A
// project-level task entry completely replaces the user-level entry
// for the same task; they do not merge field-by-field. Providers
// merge by name: a project-defined provider replaces a user-defined
// one with the same name, but providers defined only at one level
// are preserved.
func Load(userPath, projectPath string) (*Config, error) {
	user, err := LoadAt(userPath)
	if err != nil {
		return nil, fmt.Errorf("load user config %s: %w", userPath, err)
	}
	proj, err := LoadAt(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load project config %s: %w", projectPath, err)
	}
	return merge(user, proj), nil
}

// LoadAt reads a single config file. Returns a zero-value Config
// when the path is empty OR the file does not exist (both are
// normal: mastermind may be run without any config at all, and the
// project-level overlay is always optional).
//
// Returns an error for any other failure: unreadable file,
// malformed JSON, etc. The caller is expected to surface the error
// loudly rather than silently fall through, because a broken config
// file indicates user intent that's being ignored.
func LoadAt(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return &c, nil
}

// merge overlays override on top of base. Neither input is mutated.
// Providers with the same name are replaced wholesale (not merged
// field-by-field), because a partial override would create confusing
// provider definitions that mix credentials across files. Same for
// tasks.
func merge(base, override *Config) *Config {
	if base == nil {
		base = &Config{}
	}
	if override == nil {
		override = &Config{}
	}
	out := &Config{
		Providers: make(map[string]ProviderConfig),
		Tasks:     make(map[string]TaskConfig),
	}
	for name, p := range base.Providers {
		out.Providers[name] = p
	}
	for name, p := range override.Providers {
		out.Providers[name] = p
	}
	for name, t := range base.Tasks {
		out.Tasks[name] = t
	}
	for name, t := range override.Tasks {
		out.Tasks[name] = t
	}
	return out
}

// ResolveTask produces a flat Resolved struct for the given task name.
// Resolution order (highest precedence last, so latest overwrites):
//
//  1. Built-in defaults
//  2. Config file task entry
//  3. Config file provider entry (for the task's provider binding)
//  4. Environment variable overrides (backward compat)
//  5. api_key_env resolution (if APIKey still empty and APIKeyEnv set)
//
// An empty taskName, an unknown task, or an empty config all resolve
// successfully — the result falls through to env vars + defaults.
// ResolveTask only returns an error when a task DOES reference a
// provider by name and that provider is not defined anywhere in the
// config (a clear user mistake worth failing loudly on).
func (c *Config) ResolveTask(taskName string) (*Resolved, error) {
	r := &Resolved{TaskName: taskName}

	// Step 1+2: pull task fields if present.
	if c != nil {
		if task, ok := c.Tasks[taskName]; ok {
			r.Mode = task.Mode
			r.Model = task.Model
			r.ProviderName = task.Provider

			// Step 3: dereference provider binding, if any.
			if task.Provider != "" {
				p, ok := c.Providers[task.Provider]
				if !ok {
					return nil, fmt.Errorf("task %q references undefined provider %q (define it under .providers in config)", taskName, task.Provider)
				}
				r.Flavor = p.Flavor
				r.BaseURL = p.BaseURL
				r.APIKey = p.APIKey

				// api_key_env resolution now, before env-var
				// overrides take their shot at APIKey. This lets
				// MASTERMIND_LLM_API_KEY still override a provider's
				// explicit api_key_env binding if both are set.
				if r.APIKey == "" && p.APIKeyEnv != "" {
					r.APIKey = os.Getenv(p.APIKeyEnv)
				}

				// Infer flavor from the provider name if not set.
				if r.Flavor == "" {
					r.Flavor = inferFlavor(task.Provider)
				}
			}
		}
	}

	// Step 4: environment variable overrides. These are BLUNT —
	// they apply to all tasks uniformly because mastermind's existing
	// env-var surface does not differentiate tasks. Any script that
	// currently works must keep working.
	applyEnvOverrides(r)

	// Step 5: built-in defaults for fields that are still empty.
	applyDefaults(r)

	return r, nil
}

// applyEnvOverrides layers legacy env vars on top of the resolution.
// A set env var always wins over a config file value — this is
// intentional for backward compatibility: scripts that set
// MASTERMIND_LLM_MODEL=foo should continue to use foo even after a
// config file is introduced that says something else.
//
// The four generic vars apply to every task:
//
//	MASTERMIND_EXTRACT_MODE   → Mode (extract task only)
//	MASTERMIND_LLM_PROVIDER   → Flavor (anthropic|openai|ollama)
//	MASTERMIND_LLM_MODEL      → Model
//	MASTERMIND_LLM_BASE_URL   → BaseURL
//	MASTERMIND_LLM_API_KEY    → APIKey
//
// Two flavor-specific vars apply only when the resolved flavor
// matches:
//
//	ANTHROPIC_API_KEY         → APIKey (when Flavor=anthropic)
//	MASTERMIND_OLLAMA_URL     → BaseURL (when Flavor=ollama)
func applyEnvOverrides(r *Resolved) {
	if v := os.Getenv("MASTERMIND_EXTRACT_MODE"); v != "" && r.TaskName == "extract" {
		r.Mode = v
	}
	if v := os.Getenv("MASTERMIND_LLM_PROVIDER"); v != "" {
		r.Flavor = v
	}
	if v := os.Getenv("MASTERMIND_LLM_MODEL"); v != "" {
		r.Model = v
	}
	if v := os.Getenv("MASTERMIND_LLM_BASE_URL"); v != "" {
		r.BaseURL = v
	}
	if v := os.Getenv("MASTERMIND_LLM_API_KEY"); v != "" {
		r.APIKey = v
	}
	// Flavor-specific overrides apply after generic ones so they win
	// when both are set (this matches the old scattered behavior
	// where the subcommand code read ANTHROPIC_API_KEY after the
	// generic vars).
	if r.Flavor == "" || r.Flavor == "anthropic" {
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			r.APIKey = v
		}
	}
	if r.Flavor == "ollama" {
		if v := os.Getenv("MASTERMIND_OLLAMA_URL"); v != "" {
			r.BaseURL = v
		}
	}
}

// applyDefaults fills in values that are still empty after config
// and env-var resolution. The defaults mirror what the old scattered
// code had baked into each subcommand.
func applyDefaults(r *Resolved) {
	// Both "extract" and "audit" default Mode to "keyword" because
	// they share the same extractor-mode concept. Other tasks leave
	// Mode empty.
	if (r.TaskName == "extract" || r.TaskName == "audit") && r.Mode == "" {
		r.Mode = "keyword"
	}
	if r.Flavor == "" {
		r.Flavor = "anthropic"
	}
	if r.Flavor == "ollama" && r.BaseURL == "" {
		r.BaseURL = "http://localhost:11434"
	}
}

// inferFlavor guesses the wire protocol from a provider name when
// the config didn't specify one. "anthropic", "openai", and "ollama"
// map to themselves; anything else defaults to "openai" because
// that's the universal local-LLM protocol.
func inferFlavor(providerName string) string {
	name := strings.ToLower(providerName)
	switch name {
	case "anthropic", "openai", "ollama":
		return name
	}
	return "openai"
}
