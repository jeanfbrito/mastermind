# mastermind — Hermes memory provider

Connects [Hermes Agent](https://hermes-agent.nousresearch.com) to
[mastermind](https://github.com/jeanfbrito/mastermind)'s persistent knowledge
store. No cloud account or API key required — just the local binary.

## How it works

| Hook | What it does |
|---|---|
| `system_prompt_block` | Injects mastermind's session-start block (open loops + project knowledge) into the system prompt at startup |
| `prefetch` | Runs `mm_search` before each turn; top-5 results pre-warm context |
| `sync_turn` | After each response, captures knowledge-worthy exchanges **directly to the live store** (opt-in, off by default) |
| `on_memory_write` | Optionally mirrors MEMORY.md/USER.md writes to mastermind (opt-in, off by default) |
| `on_pre_compress` | Extracts knowledge before Hermes compresses context (mirrors mastermind's PreCompact hook) |
| `on_session_end` | Extracts knowledge at session close |

Tools injected into the agent: `mm_search`, `mm_write`, `mm_promote`, `mm_close_loop`.

## Requirements

- mastermind binary on `PATH` (or set `MASTERMIND_BIN`)
- `~/.knowledge/` initialized (mastermind auto-creates this on first use)

Install mastermind:
```bash
go install github.com/jeanfbrito/mastermind/cmd/mastermind@latest
```

## Installation

Copy (or symlink) this directory into your Hermes plugins directory:

```bash
# From the mastermind repo root:
cp -r plugins/hermes-memory-provider \
  "$(hermes config get plugins_dir)/memory/mastermind"

# Or symlink for live development:
ln -s "$(pwd)/plugins/hermes-memory-provider" \
  "$(hermes config get plugins_dir)/memory/mastermind"
```

Then activate:
```bash
hermes memory setup   # select "mastermind"
# or:
hermes config set memory.provider mastermind
```

If the mastermind binary is not on PATH, set the env var before running Hermes:
```bash
export MASTERMIND_BIN=/path/to/mastermind
```

## Configuration

Config file: `$HERMES_HOME/mastermind.json`

| Key | Default | Description |
|---|---|---|
| `mirror_memory_writes` | `false` | Mirror MEMORY.md/USER.md writes to mastermind as `project-personal` entries. Only enable if you want Hermes built-in memory and mastermind to stay in sync. |

Example:
```json
{
  "mirror_memory_writes": false
}
```

## Transport

The plugin speaks MCP JSON-RPC 2.0 over stdio directly to the mastermind
binary. A single subprocess is kept alive per Hermes session — initialization
overhead is paid once, not per tool call. The MCP connection is closed on
`shutdown()`.

## Design notes

**Agent = user.** In the Hermes context the agent IS the user, so mastermind's
rule "user-initiated writes go live" applies to `sync_turn` captures. When
`auto_capture: true`, knowledge-worthy turns are written **directly to the live
store** — the agent's judgment replaces the human review step. Trivial turns
(under 300 chars combined, or no detectable knowledge signal) are silently
skipped. `sync_turn` is non-blocking (daemon thread) per Hermes's threading
contract.

`on_pre_compress` and `on_session_end` run `mastermind extract` which writes
to `pending/` for review — these handle bulk session extraction where human
review of the candidates is still valuable.

**`mirror_memory_writes` is opt-in.** The `on_memory_write` hook fires when
Hermes writes to its built-in MEMORY.md/USER.md. Enabling this mirrors those
writes to mastermind as `project-personal` insights — useful if you want a
single knowledge store. Off by default because most users will want to review
what lands in mastermind rather than inheriting every Hermes memory write.
