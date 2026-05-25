# Codex setup

mastermind works in Codex as a stdio MCP server plus optional skills.

## Quick install

```bash
git clone https://github.com/jeanfbrito/mastermind.git
cd mastermind
./install/codex.sh
```

Restart Codex after the installer finishes.

## What the installer does

- Builds the local checkout with `go install ./cmd/mastermind`.
- Finds the installed binary from `GOBIN` or `GOPATH`.
- Copies `skills/mm-*` into `$CODEX_HOME/skills` (`~/.codex/skills` by default).
- Adds this MCP entry to `$CODEX_HOME/config.toml`:

```toml
[mcp_servers.mastermind]
command = "/absolute/path/to/mastermind"
```

- Creates `~/.knowledge` and initializes it as a git repo when possible.

The config uses an absolute binary path so Codex does not depend on shell `PATH`.

## Portable options

Use environment variables when installing on other machines or agent homes:

```bash
CODEX_HOME=/path/to/agent/home ./install/codex.sh
MASTERMIND_BIN=/opt/bin/mastermind ./install/codex.sh
MASTERMIND_SKILL_MODE=link ./install/codex.sh
MASTERMIND_KNOWLEDGE_HOME=/path/to/knowledge ./install/codex.sh
```

Useful flags:

```bash
./install/codex.sh --link
./install/codex.sh --force
./install/codex.sh --skip-binary
./install/codex.sh --skip-skills
./install/codex.sh --skip-config
./install/codex.sh --skip-knowledge
```

For disposable checkouts, keep the default copy mode. For a long-lived local clone,
`--link` keeps Codex skills pointed at the checkout.

## Manual install

```bash
go install github.com/jeanfbrito/mastermind/cmd/mastermind@latest
```

Then add the installed binary path to `~/.codex/config.toml`:

```toml
[mcp_servers.mastermind]
command = "/absolute/path/to/mastermind"
```

Install optional skills:

```bash
mkdir -p ~/.codex/skills
cp -R skills/mm-* ~/.codex/skills/
```

Initialize the knowledge store:

```bash
mkdir -p ~/.knowledge
git -C ~/.knowledge init
```
