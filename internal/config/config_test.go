package config

import (
	"os"
	"path/filepath"
	"testing"
)

// savedEnv captures the current value of all env vars this package
// reads, so tests can mutate them freely and restore at the end.
// Using t.Setenv is simpler but forces each subtest to be a T.Run,
// so we snapshot once per test.
var trackedEnv = []string{
	"MASTERMIND_EXTRACT_MODE",
	"MASTERMIND_LLM_PROVIDER",
	"MASTERMIND_LLM_MODEL",
	"MASTERMIND_LLM_BASE_URL",
	"MASTERMIND_LLM_API_KEY",
	"ANTHROPIC_API_KEY",
	"MASTERMIND_OLLAMA_URL",
	"PROBE_TEST_KEY", // used by api_key_env test
}

// clearEnv unsets all tracked env vars for the duration of a test.
// Restores prior values via t.Cleanup so parallel tests don't leak.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range trackedEnv {
		prior, had := os.LookupEnv(k)
		os.Unsetenv(k)
		k := k // capture
		if had {
			prior := prior
			t.Cleanup(func() { os.Setenv(k, prior) })
		} else {
			t.Cleanup(func() { os.Unsetenv(k) })
		}
	}
}

func TestLoadAt_EmptyPathReturnsEmptyConfig(t *testing.T) {
	c, err := LoadAt("")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected non-nil Config, got nil")
	}
	if len(c.Providers) != 0 || len(c.Tasks) != 0 {
		t.Errorf("expected empty config, got %+v", c)
	}
}

func TestLoadAt_MissingFileReturnsEmptyConfig(t *testing.T) {
	c, err := LoadAt("/nonexistent/path/does/not/exist.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if c == nil || len(c.Providers) != 0 {
		t.Errorf("expected empty config, got %+v", c)
	}
}

func TestLoadAt_InvalidJSONErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAt(path); err == nil {
		t.Error("expected error on malformed JSON, got nil")
	}
}

func TestLoadAt_ValidConfigLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{
      "providers": {
        "anthropic": {"api_key_env": "ANTHROPIC_API_KEY"},
        "local": {"base_url": "http://x/v1", "api_key": "sk-test", "flavor": "openai"}
      },
      "tasks": {
        "extract": {"mode": "llm", "provider": "local", "model": "foo"},
        "discover": {"provider": "anthropic", "model": "claude-haiku-4-5"}
      }
    }`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Providers) != 2 {
		t.Errorf("want 2 providers, got %d", len(c.Providers))
	}
	if c.Providers["local"].BaseURL != "http://x/v1" {
		t.Errorf("local.BaseURL = %q, want http://x/v1", c.Providers["local"].BaseURL)
	}
	if c.Tasks["extract"].Mode != "llm" {
		t.Errorf("extract.Mode = %q, want llm", c.Tasks["extract"].Mode)
	}
}

func TestLoad_ProjectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.json")
	projPath := filepath.Join(dir, "project.json")

	userRaw := `{
      "providers": {
        "anthropic": {"api_key_env": "USER_KEY"},
        "other": {"base_url": "http://user/v1"}
      },
      "tasks": {
        "extract": {"provider": "anthropic", "model": "user-model"},
        "discover": {"provider": "other", "model": "d1"}
      }
    }`
	projRaw := `{
      "providers": {
        "anthropic": {"api_key_env": "PROJ_KEY"}
      },
      "tasks": {
        "extract": {"provider": "anthropic", "model": "proj-model"}
      }
    }`
	if err := os.WriteFile(userPath, []byte(userRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projPath, []byte(projRaw), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(userPath, projPath)
	if err != nil {
		t.Fatal(err)
	}

	// anthropic provider: project wins.
	if c.Providers["anthropic"].APIKeyEnv != "PROJ_KEY" {
		t.Errorf("anthropic.APIKeyEnv = %q, want PROJ_KEY (project override failed)", c.Providers["anthropic"].APIKeyEnv)
	}
	// other provider: only in user, must survive.
	if c.Providers["other"].BaseURL != "http://user/v1" {
		t.Errorf("other.BaseURL = %q, want http://user/v1 (user-only provider lost)", c.Providers["other"].BaseURL)
	}
	// extract task: project wins.
	if c.Tasks["extract"].Model != "proj-model" {
		t.Errorf("extract.Model = %q, want proj-model (project override failed)", c.Tasks["extract"].Model)
	}
	// discover task: only in user, must survive.
	if c.Tasks["discover"].Model != "d1" {
		t.Errorf("discover.Model = %q, want d1 (user-only task lost)", c.Tasks["discover"].Model)
	}
}

func TestResolveTask_EmptyConfigReturnsDefaults(t *testing.T) {
	clearEnv(t)
	c := &Config{}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != "keyword" {
		t.Errorf("Mode = %q, want keyword (default)", r.Mode)
	}
	if r.Flavor != "anthropic" {
		t.Errorf("Flavor = %q, want anthropic (default)", r.Flavor)
	}
}

func TestResolveTask_BindsProviderByName(t *testing.T) {
	clearEnv(t)
	c := &Config{
		Providers: map[string]ProviderConfig{
			"local": {
				Flavor:  "openai",
				BaseURL: "http://local/v1",
				APIKey:  "sk-inline",
			},
		},
		Tasks: map[string]TaskConfig{
			"extract": {Mode: "llm", Provider: "local", Model: "llama3"},
		},
	}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != "llm" || r.Flavor != "openai" || r.Model != "llama3" {
		t.Errorf("unexpected resolution: %+v", r)
	}
	if r.BaseURL != "http://local/v1" || r.APIKey != "sk-inline" {
		t.Errorf("provider fields not bound: %+v", r)
	}
	if r.ProviderName != "local" {
		t.Errorf("ProviderName = %q, want local", r.ProviderName)
	}
}

func TestResolveTask_UndefinedProviderErrors(t *testing.T) {
	clearEnv(t)
	c := &Config{
		Tasks: map[string]TaskConfig{
			"extract": {Provider: "ghost", Model: "x"},
		},
	}
	if _, err := c.ResolveTask("extract"); err == nil {
		t.Error("expected error for undefined provider reference, got nil")
	}
}

func TestResolveTask_APIKeyEnvResolved(t *testing.T) {
	clearEnv(t)
	os.Setenv("PROBE_TEST_KEY", "sk-from-env")
	c := &Config{
		Providers: map[string]ProviderConfig{
			"local": {
				Flavor:    "openai",
				BaseURL:   "http://local/v1",
				APIKeyEnv: "PROBE_TEST_KEY",
			},
		},
		Tasks: map[string]TaskConfig{
			"extract": {Mode: "llm", Provider: "local", Model: "m"},
		},
	}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.APIKey != "sk-from-env" {
		t.Errorf("APIKey = %q, want sk-from-env (api_key_env resolution failed)", r.APIKey)
	}
}

func TestResolveTask_EnvVarsOverrideConfig(t *testing.T) {
	clearEnv(t)
	os.Setenv("MASTERMIND_LLM_MODEL", "override-model")
	os.Setenv("MASTERMIND_LLM_BASE_URL", "http://override/v1")
	os.Setenv("MASTERMIND_LLM_API_KEY", "sk-override")
	os.Setenv("MASTERMIND_LLM_PROVIDER", "openai")

	c := &Config{
		Providers: map[string]ProviderConfig{
			"local": {Flavor: "openai", BaseURL: "http://config/v1", APIKey: "sk-config"},
		},
		Tasks: map[string]TaskConfig{
			"extract": {Mode: "llm", Provider: "local", Model: "config-model"},
		},
	}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Model != "override-model" {
		t.Errorf("Model not overridden by env: %q", r.Model)
	}
	if r.BaseURL != "http://override/v1" {
		t.Errorf("BaseURL not overridden by env: %q", r.BaseURL)
	}
	if r.APIKey != "sk-override" {
		t.Errorf("APIKey not overridden by env: %q", r.APIKey)
	}
	if r.Flavor != "openai" {
		t.Errorf("Flavor not overridden by env: %q", r.Flavor)
	}
}

func TestResolveTask_ExtractModeEnvOnlyAppliesToExtract(t *testing.T) {
	clearEnv(t)
	os.Setenv("MASTERMIND_EXTRACT_MODE", "llm")
	c := &Config{}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != "llm" {
		t.Errorf("extract Mode = %q, want llm", r.Mode)
	}
	// Mode env var should NOT apply to a discover task.
	r2, err := c.ResolveTask("discover")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Mode != "" {
		t.Errorf("discover Mode = %q, want empty (env var should be extract-only)", r2.Mode)
	}
}

func TestResolveTask_AnthropicAPIKeyFromEnv(t *testing.T) {
	clearEnv(t)
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-env")
	c := &Config{}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Flavor != "anthropic" {
		t.Errorf("Flavor = %q, want anthropic (default)", r.Flavor)
	}
	if r.APIKey != "sk-ant-env" {
		t.Errorf("APIKey = %q, want sk-ant-env", r.APIKey)
	}
}

func TestResolveTask_OllamaURLFromEnv(t *testing.T) {
	clearEnv(t)
	os.Setenv("MASTERMIND_LLM_PROVIDER", "ollama")
	os.Setenv("MASTERMIND_OLLAMA_URL", "http://ollama-custom:9999")
	c := &Config{}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Flavor != "ollama" {
		t.Errorf("Flavor = %q, want ollama", r.Flavor)
	}
	if r.BaseURL != "http://ollama-custom:9999" {
		t.Errorf("BaseURL = %q, want http://ollama-custom:9999", r.BaseURL)
	}
}

func TestResolveTask_OllamaDefaultBaseURL(t *testing.T) {
	clearEnv(t)
	os.Setenv("MASTERMIND_LLM_PROVIDER", "ollama")
	c := &Config{}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want http://localhost:11434 (ollama default)", r.BaseURL)
	}
}

func TestInferFlavor(t *testing.T) {
	cases := map[string]string{
		"anthropic":   "anthropic",
		"Anthropic":   "anthropic",
		"openai":      "openai",
		"ollama":      "ollama",
		"local-vllm":  "openai", // unknown → openai
		"my-gateway":  "openai",
		"":            "openai",
	}
	for input, want := range cases {
		if got := inferFlavor(input); got != want {
			t.Errorf("inferFlavor(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveTask_InferFlavorFromProviderName(t *testing.T) {
	clearEnv(t)
	c := &Config{
		Providers: map[string]ProviderConfig{
			// No Flavor set — should infer "anthropic" from the name.
			"anthropic": {APIKey: "sk-x"},
		},
		Tasks: map[string]TaskConfig{
			"extract": {Mode: "llm", Provider: "anthropic", Model: "m"},
		},
	}
	r, err := c.ResolveTask("extract")
	if err != nil {
		t.Fatal(err)
	}
	if r.Flavor != "anthropic" {
		t.Errorf("Flavor = %q, want anthropic (inferred from name)", r.Flavor)
	}
}
