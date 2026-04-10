# Config — task router for model / provider selection

mastermind's model and provider selection lives in **one config file**: `~/.knowledge/config.json`. A per-project file at `<project>/.knowledge/config.json` can override the user-level file on a per-task basis. Both files are optional — with neither present, mastermind falls back to env vars and built-in defaults, which is exactly how it worked before this file existed.

**Design inspiration**: soulforge's `taskRouter` — each task (`extract`, `discover`, `audit`) binds to a named provider and a model, so cheap local models can handle high-volume work while frontier models handle precision work.

## Shape

```json
{
  "providers": {
    "anthropic": {
      "api_key_env": "ANTHROPIC_API_KEY"
    },
    "openai": {
      "base_url": "https://api.openai.com/v1",
      "api_key_env": "OPENAI_API_KEY"
    },
    "local-vllm": {
      "base_url": "http://192.168.13.62:20128/v1",
      "api_key_env": "LOCAL_LLM_KEY"
    },
    "ollama": {
      "base_url": "http://localhost:11434"
    }
  },
  "tasks": {
    "extract":  { "mode": "llm", "provider": "local-vllm", "model": "llama/gemopus-26b-a4b" },
    "discover": { "provider": "anthropic", "model": "claude-haiku-4-5-20251001" },
    "audit":    { "provider": "local-vllm", "model": "llama/gemopus-26b-a4b" }
  }
}
```

### `providers` — named endpoints

Each entry under `providers` is a friendly label pointing at one LLM endpoint. The label is free-form — `anthropic`, `openai`, `ollama`, `local-vllm`, `work-proxy`, whatever. Fields:

| field         | required | meaning |
|---------------|----------|---------|
| `flavor`      | optional | wire protocol: `anthropic`, `openai`, or `ollama`. Inferred from the provider name when the name is a known flavor, otherwise defaults to `openai` (the universal local-LLM protocol). |
| `base_url`    | yes for `openai`/`ollama` | HTTP endpoint base. The `/chat/completions` path is appended by the client. Ignored for `anthropic` (which always uses `api.anthropic.com`). |
| `api_key`     | optional | bearer token, **inlined**. Convenient but discouraged — commits of this file would leak the key. Prefer `api_key_env`. |
| `api_key_env` | optional | name of an environment variable holding the bearer token. mastermind reads `os.Getenv(api_key_env)` at resolution time so secrets stay out of the config file. |

Named providers are decoupled from wire protocols: you can define two `openai`-flavor providers (one for openai.com, one for a local vLLM) and have different tasks use each. The name is a label; `flavor` is the protocol.

### `tasks` — per-task bindings

Each entry under `tasks` binds a task name to a provider reference and a model. Tasks are:

- `extract` — knowledge extraction from session transcripts (`runExtract`, PreCompact hook)
- `discover` — git-history + codebase mining (`mastermind discover`)
- `audit` — measurement harness (`mastermind extract-audit`)

Fields:

| field      | meaning |
|------------|---------|
| `mode`     | `keyword` (default) or `llm`. Applies only to `extract` and `audit`; ignored for other tasks. |
| `provider` | name of an entry in `providers`. Empty means "fall through to env vars / defaults". Referencing an undefined provider is an error. |
| `model`    | model identifier sent to the provider. Shape depends on flavor: `claude-haiku-4-5-20251001`, `llama/gemopus-26b-a4b`, etc. |

Any task without an entry in `tasks` falls back to env vars and built-in defaults — just like mastermind behaved before this file existed.

## Precedence

Highest precedence wins:

1. **CLI flags** (per-invocation, e.g. `extract-audit --model foo`)
2. **Env vars** — backward compat with every script that pre-dates this file:
   - `MASTERMIND_EXTRACT_MODE` → `tasks.extract.mode`
   - `MASTERMIND_LLM_PROVIDER` → flavor (overrides provider resolution)
   - `MASTERMIND_LLM_MODEL` → model
   - `MASTERMIND_LLM_BASE_URL` → provider base URL
   - `MASTERMIND_LLM_API_KEY` → api key (any flavor)
   - `ANTHROPIC_API_KEY` → api key (anthropic flavor only)
   - `MASTERMIND_OLLAMA_URL` → base URL (ollama flavor only)
3. **Project config** — `<project-root>/.knowledge/config.json`, overlays the user-level file
4. **User config** — `~/.knowledge/config.json`
5. **Built-in defaults** — `mode: keyword`, `flavor: anthropic`, `ollama URL: http://localhost:11434`

Env vars intentionally beat the config file so existing scripts don't silently change behavior when a new config file is introduced. If you want a script to respect the config file, unset the relevant env var.

## Migration from env vars

If you currently export any of these, you're already set — they keep working unchanged:

```sh
export ANTHROPIC_API_KEY=sk-ant-...
export MASTERMIND_EXTRACT_MODE=llm
export MASTERMIND_LLM_PROVIDER=openai
export MASTERMIND_LLM_BASE_URL=https://my-gateway/v1
export MASTERMIND_LLM_API_KEY=sk-...
export MASTERMIND_LLM_MODEL=llama/gemopus-26b-a4b
```

Migrating to `~/.knowledge/config.json` lets you express per-task differentiation (e.g. `extract` via cheap local, `discover` via Haiku) that env vars can't. A minimal equivalent of the shell environment above:

```json
{
  "providers": {
    "anthropic":   { "api_key_env": "ANTHROPIC_API_KEY" },
    "my-gateway":  { "base_url": "https://my-gateway/v1", "api_key_env": "MASTERMIND_LLM_API_KEY" }
  },
  "tasks": {
    "extract":  { "mode": "llm", "provider": "my-gateway", "model": "llama/gemopus-26b-a4b" },
    "discover": { "provider": "anthropic", "model": "claude-haiku-4-5-20251001" },
    "audit":    { "provider": "my-gateway", "model": "llama/gemopus-26b-a4b" }
  }
}
```

## Common patterns

### Cheap local for extract, frontier for discover

```json
{
  "providers": {
    "anthropic": { "api_key_env": "ANTHROPIC_API_KEY" },
    "local":     { "base_url": "http://localhost:8000/v1", "api_key_env": "LOCAL_KEY" }
  },
  "tasks": {
    "extract":  { "mode": "llm", "provider": "local", "model": "qwen2.5-32b-instruct" },
    "discover": { "provider": "anthropic", "model": "claude-haiku-4-5-20251001" }
  }
}
```

### Project override — use a work-provisioned endpoint inside one repo

User `~/.knowledge/config.json`:

```json
{
  "providers": { "anthropic": { "api_key_env": "ANTHROPIC_API_KEY" } },
  "tasks":     { "extract": { "mode": "llm", "provider": "anthropic", "model": "claude-haiku-4-5-20251001" } }
}
```

Project `.knowledge/config.json` (inside one specific repo, checked-in or gitignored):

```json
{
  "providers": { "work-gateway": { "base_url": "https://internal-llm.corp/v1", "api_key_env": "WORK_LLM_KEY" } },
  "tasks":     { "extract": { "mode": "llm", "provider": "work-gateway", "model": "internal/claude-haiku" } }
}
```

Inside that project, `extract` uses the work gateway. Everywhere else, it uses Anthropic. Providers merge across the two files; tasks are replaced per-task.

### Keyword-only (no LLM, fastest default)

```json
{
  "tasks": {
    "extract": { "mode": "keyword" }
  }
}
```

This is the same as not having a config file at all — keyword is the default mode, anthropic is the default flavor, and no API calls happen during extraction until you opt in.

## Security

- **Never commit `api_key` inline values.** Use `api_key_env` and keep the actual secret in your shell environment or a secret manager.
- **Project-level config files** (`<project>/.knowledge/config.json`) should be reviewed before commit. If the project is public, double-check that `api_key_env` references only exist and no `api_key` inlines slipped in.
- **mastermind does not encrypt config files.** They are plain JSON with standard file permissions. Protect them with filesystem permissions (`chmod 600 ~/.knowledge/config.json` if you do inline a key).
