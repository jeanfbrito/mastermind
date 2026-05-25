#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Install mastermind for Codex.

Usage:
  install/codex.sh [options]

Options:
  --codex-home DIR     Codex home directory (default: $CODEX_HOME or ~/.codex)
  --binary PATH        Use an existing mastermind binary instead of go install
  --link              Symlink skills from this checkout instead of copying
  --copy              Copy skills into Codex home (default)
  --force             Replace existing installed skill directories
  --skip-binary       Do not build/install the mastermind binary
  --skip-skills       Do not install Codex skills
  --skip-config       Do not update Codex config.toml
  --skip-knowledge    Do not create ~/.knowledge
  -h, --help          Show this help

Environment:
  CODEX_HOME                 Overrides the Codex home directory.
  MASTERMIND_BIN             Existing binary path to register.
  MASTERMIND_SKILL_MODE      copy or link.
  MASTERMIND_KNOWLEDGE_HOME  Knowledge store path (default: ~/.knowledge).
EOF
}

log() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
require_cmd() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "$script_dir/.." && pwd)"

codex_home="${CODEX_HOME:-$HOME/.codex}"
binary_path="${MASTERMIND_BIN:-}"
skill_mode="${MASTERMIND_SKILL_MODE:-copy}"
force=0
skip_binary=0
skip_skills=0
skip_config=0
skip_knowledge=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --codex-home) [ "$#" -ge 2 ] || die "--codex-home needs a value"; codex_home="$2"; shift 2 ;;
    --binary) [ "$#" -ge 2 ] || die "--binary needs a value"; binary_path="$2"; shift 2 ;;
    --link) skill_mode="link"; shift ;;
    --copy) skill_mode="copy"; shift ;;
    --force) force=1; shift ;;
    --skip-binary) skip_binary=1; shift ;;
    --skip-skills) skip_skills=1; shift ;;
    --skip-config) skip_config=1; shift ;;
    --skip-knowledge) skip_knowledge=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[ "$skill_mode" = "copy" ] || [ "$skill_mode" = "link" ] || die "MASTERMIND_SKILL_MODE must be copy or link"

if [ "$skip_binary" -eq 0 ]; then
  if [ -n "$binary_path" ]; then
    [ -x "$binary_path" ] || die "binary is not executable: $binary_path"
    log "using existing binary: $binary_path"
  else
    require_cmd go
    log "installing mastermind binary with go install ./cmd/mastermind"
    (cd "$repo_root" && go install ./cmd/mastermind)

    gobin="$(go env GOBIN)"
    if [ -z "$gobin" ]; then
      gopath="$(go env GOPATH)"
      gopath="${gopath%%:*}"
      [ -n "$gopath" ] || gopath="$HOME/go"
      gobin="$gopath/bin"
    fi
    binary_path="$gobin/mastermind"
    [ -x "$binary_path" ] || die "installed binary not found: $binary_path"
  fi
fi

if [ "$skip_skills" -eq 0 ]; then
  skills_home="$codex_home/skills"
  mkdir -p "$skills_home"

  for src in "$repo_root"/skills/mm-*; do
    [ -d "$src" ] || continue
    [ -f "$src/SKILL.md" ] || continue
    name="$(basename "$src")"
    dest="$skills_home/$name"

    if [ -e "$dest" ] || [ -L "$dest" ]; then
      if [ "$force" -eq 1 ]; then
        rm -rf "$dest"
      else
        log "skill exists, keeping: $dest"
        continue
      fi
    fi

    if [ "$skill_mode" = "link" ]; then
      ln -s "$src" "$dest"
      log "linked skill: $dest -> $src"
    else
      cp -R "$src" "$dest"
      log "copied skill: $dest"
    fi
  done
fi

if [ "$skip_config" -eq 0 ]; then
  [ -n "$binary_path" ] || die "cannot update config without a binary path"
  mkdir -p "$codex_home"
  config_file="$codex_home/config.toml"
  touch "$config_file"

  if grep -q '^\[mcp_servers\.mastermind\]' "$config_file"; then
    log "Codex MCP config already has [mcp_servers.mastermind]"
  else
    escaped_binary="${binary_path//\\/\\\\}"
    escaped_binary="${escaped_binary//\"/\\\"}"
    {
      printf '\n[mcp_servers.mastermind]\n'
      printf 'command = "%s"\n' "$escaped_binary"
    } >> "$config_file"
    log "added Codex MCP server to $config_file"
  fi
fi

if [ "$skip_knowledge" -eq 0 ]; then
  knowledge_home="${MASTERMIND_KNOWLEDGE_HOME:-$HOME/.knowledge}"
  mkdir -p "$knowledge_home"
  if command -v git >/dev/null 2>&1 && [ ! -d "$knowledge_home/.git" ]; then
    git -c init.defaultBranch=main -C "$knowledge_home" init >/dev/null
    log "initialized knowledge git repo: $knowledge_home"
  else
    log "knowledge store ready: $knowledge_home"
  fi
fi

log "done. Restart Codex to load the mastermind MCP server and skills."
