# rtk: Reference Architecture for mastermind

**Critical Finding:** rtk is **NOT an MCP server**. It's a **hook-based CLI interceptor** that integrates with AI coding assistants (Claude Code, Cursor, Cline, Windsurf, etc.) via agent-native hook mechanisms. This is a fundamentally different integration pattern than MCP stdio servers.

---

## 1. Top-Level Layout

**Single Crate (Not a Workspace)**

Root structure:
```
rtk/
├── Cargo.toml                 # Single binary, version 0.34.3
├── src/
│   ├── main.rs               # 2,463 lines — CLI dispatcher and subcommand routing
│   ├── analytics/            # Token savings reporting (gain, cc_economics, session_cmd)
│   ├── cmds/                 # 100+ supported commands organized by ecosystem
│   │   ├── cloud/            # AWS, containers, curl, psql, wget
│   │   ├── dotnet/           # .NET tooling, binlog parsing
│   │   ├── git/              # git, gh (GitHub CLI), git-town
│   │   ├── go/               # go test/build/vet, golangci-lint
│   │   ├── js/               # npm, pnpm, yarn, prettier, eslint, typescript, vitest, prisma
│   │   ├── python/           # pip, mypy, pytest, ruff
│   │   ├── ruby/             # rake, rspec, rubocop
│   │   ├── rust/             # cargo, test runner
│   │   └── system/           # ls, read, grep, find, docker, kubectl, logs
│   ├── core/                 # Infrastructure: filter, tracking (SQLite), config, telemetry
│   ├── hooks/                # Hook lifecycle: init, rewrite, integrity verification, trust
│   ├── learn/                # Command discovery and analysis
│   └── discover/             # Hook rewrite registry
├── hooks/                    # Embedded hook scripts for each agent
│   ├── claude/               # .claude/rtk-rewrite.sh (stdio hook)
│   ├── cursor/               # ~/.cursor/hooks/rtk-rewrite.sh
│   ├── opencode/             # rtk.ts TypeScript plugin
│   └── codex/                # BeforeTool hook for Codex
├── .github/workflows/
│   ├── release.yml           # Multi-target cross-compilation
│   └── ...
├── ARCHITECTURE.md           # 60-page technical deep-dive
└── README.md                 # 9 supported agents + commands reference
```

**No build_support, deploy, docker, or examples directories** — distribution is via GitHub Releases (pre-built binaries) + Homebrew.

---

## 2. Binary Entry Point

**main.rs: 2,463 lines**

### Startup Flow
1. Parse CLI args using **clap derive** (structured, declarative)
2. Extract global flags: `-v` (verbosity, stackable `-vvv`), `-u` (ultra-compact mode), `--skip-env`
3. Route to subcommand handler (e.g., `git`, `cargo`, `read`, `init`)
4. Execute command, apply filtering, emit compressed output to stdout
5. Exit with original command's exit code (critical for CI/CD)

### Clap Usage
```rust
#[derive(Parser)]
#[command(name = "rtk", version, about = "...")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
    
    #[arg(short, long, action = clap::ArgAction::Count, global = true)]
    verbose: u8,
    
    #[arg(short = 'u', long, global = true)]
    ultra_compact: bool,
}

#[derive(Subcommand)]
enum Commands {
    Ls { #[arg(trailing_var_arg = true, allow_hyphen_values = true)] args: Vec<String> },
    Read { file: PathBuf, #[arg(short, long, default_value = "none")] level: FilterLevel, ... },
    Git { #[command(subcommand)] command: GitCommand, ... },
    Cargo { #[command(subcommand)] command: CargoCommand, ... },
    Init { #[arg(short, long)] global: bool, #[arg(long)] agent: Option<AgentTarget>, ... },
    Rewrite { /* Hidden CLI — called by hooks */ },
    Gain { /* Token savings analytics */ },
    // ... ~20 more subcommands
}
```

### Subcommands
- **System I/O:** `ls`, `read`, `grep`, `find`, `tree`, `wc`, `json`
- **Build & Test:** `cargo`, `go`, `npm`, `pnpm`, `pytest`, `rspec`, `vitest`
- **VCS:** `git`, `gh` (GitHub CLI), `gt` (git-town)
- **Linting & Format:** `eslint`, `prettier`, `ruff`, `golangci-lint`, `rubocop`
- **Package Managers:** `pip`, `bundle`, `prisma`
- **Containers:** `docker`, `kubectl`
- **Utilities:** `env`, `curl`, `psql`, `wget`, `aws`
- **Hook System:** `init` (install hooks), `rewrite` (subprocess handler), `hook` (inspect hooks)
- **Analytics:** `gain` (token savings report), `session`, `cc` (economics)

---

## 3. MCP Server Wiring — NOT APPLICABLE

**rtk does NOT expose an MCP server.**

Instead, it uses **agent-native hook mechanisms**:

### How It Actually Works

rtk is installed as a **hook interceptor**. When Claude Code (or other agents) execute a command:

1. **Agent Hook Intercepts:** Claude Code's `BeforeTool` hook triggers before running a command
2. **Hook Calls rtk:** The hook script calls `rtk rewrite <original-command>`
3. **rtk Rewrites Command:** rtk examines the command, decides whether to rewrite (e.g., `git status` → `rtk git status`)
4. **Two Strategies:**
   - **Auto-Rewrite (default):** Hook rewrites silently, agent executes `rtk git status` instead
   - **Suggest (non-intrusive):** Hook emits `systemMessage` hint, Claude decides autonomously (~70-85% adoption)

### Hook Lifecycle (src/hooks/)

**rtk init** bootstraps hooks. Supported agents:

| Agent | Hook File | Hook Protocol | Install Scope |
|-------|-----------|---------------|---------------|
| **Claude Code** (default) | `~/.claude/rtk-rewrite.sh` | stdio BeforeTool | Both global & project |
| **Cursor Agent** | `~/.cursor/hooks/rtk-rewrite.sh` | preToolUse matcher | Global only |
| **Cline / Roo Code** | `.cline/hooks.json` + instructions | system message | Project |
| **Windsurf (Cascade)** | `.windsurfrules` + prefix instructions | Cascade rules | Project |
| **Copilot (VS Code)** | `.github/hooks/rtk-rewrite.json` | PreToolUse hook | Project |
| **Codex** | `~/.codex/hooks/` | BeforeTool (custom) | Global |
| **Gemini CLI** | `~/.gemini/hooks/rtk-hook-gemini.sh` | BeforeTool hook | Global |
| **OpenCode** | `~/.config/opencode/plugins/rtk.ts` | TypeScript plugin | Global |

**Key Modules:**
- `src/hooks/init.rs` — Installation logic, atomic writes, hook script embedding
- `src/hooks/rewrite_cmd.rs` — Thin CLI bridge (hooks call `rtk rewrite` as subprocess)
- `src/discover/registry.rs` — Pattern matching: should command be rewritten? (e.g., `git status` yes, `git clone` no)
- `src/hooks/integrity.rs` — SHA-256 verification of hook scripts (tamper detection)
- `src/hooks/trust.rs` — TOML filter trust gates (project-local `.rtk/filters.toml`)

### Hook Execution Flow (6-Phase)

```
Phase 1: Hook Interception
  ↓ Claude Code's BeforeTool triggers
  ↓ Agent hook script invoked (rtk-rewrite.sh, rtk.ts, etc.)

Phase 2: Command Rewriting
  ↓ Hook calls: rtk rewrite "<original-command>"
  ↓ rtk examines registry: should rewrite?

Phase 3: Rewrite Decision
  ↓ registry.rs applies rules (e.g., git.status → rewrite, git.clone → skip)
  ↓ Two modes: auto-rewrite or suggest

Phase 4: Hook Responds
  ↓ Auto: updatedInput field with rewritten command
  ↓ Suggest: systemMessage with hint + original command

Phase 5: Agent Execution
  ↓ Agent runs (rewritten or original) command
  ↓ Output captured

Phase 6: Filter & Compress
  ↓ Command output piped through rtk filter
  ↓ ANSI stripped, lines deduplicated, truncated
  ↓ Compressed output sent to agent context
```

---

## 4. Dependency Philosophy

**Minimal, intentional, no async runtime.**

Total: **21 direct dependencies** (counted in Cargo.toml)

```toml
[dependencies]
clap = { version = "4", features = ["derive"] }     # CLI parsing
anyhow = "1.0"                                       # Error handling with context
ignore = "0.4"                                       # Fast .gitignore walking
walkdir = "2"                                        # Directory traversal
regex = "1"                                          # Pattern matching
lazy_static = "1.4"                                  # Lazy static init
serde = { version = "1", features = ["derive"] }   # Serialization
serde_json = { version = "1", features = ["preserve_order"] }  # JSON parsing
colored = "2"                                        # Terminal colors
dirs = "5"                                           # XDG config paths
rusqlite = { version = "0.31", features = ["bundled"] }  # SQLite (metrics DB)
toml = "0.8"                                         # Config file parsing
chrono = "0.4"                                       # Timestamps
tempfile = "3"                                       # Atomic writes
sha2 = "0.10"                                        # Integrity verification
ureq = "2"                                           # Lightweight HTTP (telemetry)
hostname = "0.4"                                     # Device identification
getrandom = "0.4"                                    # Random seed
flate2 = "1.0"                                       # Gzip compression
quick-xml = "0.37"                                  # XML parsing (dotnet)
which = "8"                                          # Binary location lookup
automod = "1"                                        # Macro-based module discovery
```

**Distinctive Choice:** No async runtime (no tokio, hyper, async-std). All I/O is **synchronous**. This keeps the binary small (~4.1 MB stripped) and startup fast (~5-10ms overhead).

**Dependency Rationale:**
- **anyhow + clap + serde:** Rust standard tooling ecosystem
- **rusqlite** (bundled): Self-contained metrics DB, zero server setup
- **ureq** (not reqwest/hyper): Lightweight HTTP client (one-way telemetry pings, once/day)
- **ignore + walkdir:** Fast, .gitignore-aware directory scanning

---

## 5. Build and Release

### CI/CD Pipeline (.github/workflows/release.yml)

**Automatic cross-compilation on release tag push.**

Targets:
- **macOS:** `x86_64-apple-darwin`, `aarch64-apple-darwin` (both on macos-latest)
- **Linux:** `x86_64-unknown-linux-musl` (musl), `aarch64-unknown-linux-gnu` (cross-compile with aarch64-linux-gnu-gcc)
- **Windows:** `x86_64-pc-windows-msvc` (windows-latest)

Strategy: Per-target build job, fail-fast disabled. Archives: `.tar.gz` (Unix), `.zip` (Windows).

### Release Artifacts

Published to GitHub Releases:
- `rtk-x86_64-apple-darwin.tar.gz`, `rtk-aarch64-apple-darwin.tar.gz`
- `rtk-x86_64-unknown-linux-musl.tar.gz`, `rtk-aarch64-unknown-linux-gnu.tar.gz`
- `rtk-x86_64-pc-windows-msvc.zip`

### Distribution Channels

1. **GitHub Releases** — Direct download
2. **Homebrew** — `brew install rtk` (official tap: rtk-ai/rtk)
3. **cargo install** — `cargo install rtk` (from crates.io, but note naming conflict with "Rust Type Kit"; prefer `--git` install)
4. **Linux Packages:**
   - **Debian/Ubuntu:** `cargo-deb` configured → `.deb` assets
   - **RPM:** `cargo-generate-rpm` configured → `.rpm` assets

### Build Optimizations (Cargo.toml)

```toml
[profile.release]
opt-level = 3              # Maximum optimization
lto = true                 # Link-time optimization
codegen-units = 1          # Single codegen unit (slower compile, smaller binary)
panic = "abort"            # Smaller binary (no unwind tables)
strip = true               # Remove debug symbols
```

**Result:** ~4.1 MB stripped binary.

---

## 6. Internal Architecture

Beyond main.rs, rtk is organized into 7 top-level modules:

### Core Modules (src/core/)

| File | LOC | Purpose |
|------|-----|---------|
| `filter.rs` | 367 | Filtering engine: ANSI stripping, line deduplication, truncation, JSON compaction |
| `tracking.rs` | 1,268 | SQLite metrics: command count, token input/output, savings % (queries: `rtk gain`) |
| `tee.rs` | 364 | "Tee" mode: capture output while displaying it (for full recovery) |
| `config.rs` | 169 | Config file loading: `~/.config/rtk/config.toml` (telemetry, verbosity, custom filters) |
| `toml_filter.rs` | 1,493 | Custom filter TOML schema parser: `match_command`, `strip_ansi`, `strip_lines_matching`, `max_lines` |
| `telemetry.rs` | 310 | Anonymous metrics: device hash, command count (opt-out via env var) |
| `display_helpers.rs` | 262 | Terminal formatting, color/icon selection |
| `utils.rs` | 609 | Shared utilities: `execute_command()`, `strip_ansi()`, `truncate()`, `ruby_exec()` |

**Load-bearing modules:** `filter.rs`, `tracking.rs`, `toml_filter.rs` — these implement the core token-saving logic.

### Command Modules (src/cmds/)

**Ecosystem-organized (~700 LOC per ecosystem):**

- **git/:** `git.rs` (status, log, diff parsing), `gh.rs` (GitHub CLI), `gt.rs` (git-town)
- **rust/:** `cargo_cmd.rs` (test, build, clippy), `runner.rs` (test filtering)
- **js/:** `npm_cmd.rs`, `pnpm_cmd.rs`, `prettier_cmd.rs`, `eslint_cmd.rs`, `typescript_cmd.rs`, `vitest_cmd.rs`, `prisma_cmd.rs`, `next_cmd.rs`, `playwright_cmd.rs`
- **python/:** `pip_cmd.rs`, `pytest_cmd.rs`, `ruff_cmd.rs`, `mypy_cmd.rs`
- **go/:** `go_cmd.rs` (test/build/vet as sub-enum), `golangci_cmd.rs` (standalone linter)
- **ruby/:** `rake_cmd.rs`, `rspec_cmd.rs`, `rubocop_cmd.rs`
- **dotnet/:** `dotnet_cmd.rs`, `binlog.rs` (binary log parsing), `dotnet_format_report.rs`, `dotnet_trx.rs`
- **cloud/:** `aws_cmd.rs`, `container.rs` (docker/podman), `curl_cmd.rs`, `psql_cmd.rs`, `wget_cmd.rs`
- **system/:** `ls.rs`, `read.rs`, `grep_cmd.rs`, `find_cmd.rs`, `tree.rs`, `wc_cmd.rs`, `json_cmd.rs`, `log_cmd.rs`, `env_cmd.rs`, `format_cmd.rs`, `deps.rs`, `summary.rs`, `local_llm.rs`

**Pattern:** Each command module contains 1-3 output formatters (e.g., parse JSON, parse XML, strip ASCII art).

### Hook System (src/hooks/)

| File | Purpose |
|------|---------|
| `init.rs` | Install/uninstall hooks for 7 agents + 3 special modes (Gemini, Codex, OpenCode) |
| `rewrite_cmd.rs` | Subprocess handler: `rtk rewrite <cmd>` → delegates to discover/registry |
| `hook_cmd.rs` | Hook inspection: `rtk hook check`, `rtk hook audit` |
| `integrity.rs` | SHA-256 verification of embedded hook scripts |
| `trust.rs` | TOML filter trust gates (prompt user before executing project-local filters) |
| `constants.rs` | Hook file paths, agent directories |

**Key Pattern:** Hook scripts are embedded in binary as `const` (via `include_str!("../../hooks/claude/rtk-rewrite.sh")`). When `rtk init` runs, it atomically writes these to the agent's config directory.

### Analytics (src/analytics/)

| File | Purpose |
|------|---------|
| `gain.rs` | Token savings report: totals by day/command/agent |
| `cc_economics.rs` | Context window economics: how many tokens saved, equivalent to N Claude calls |
| `session_cmd.rs` | Current session metrics |

Uses `src/core/tracking.rs` SQLite database as data source.

### Hook Registry (src/discover/)

| File | Purpose |
|------|---------|
| `registry.rs` | Pattern matching: given a command, should rtk rewrite it? (e.g., `git status` yes, `git clone` no) |
| `provider.rs` | Command provider detection (e.g., which package manager is active?) |
| `detector.rs` | Language/tool detection (e.g., is this a Rust project?) |
| `report.rs` | Hook audit reporting |

### Learn & Parser (src/learn/, src/parser/)

- **learn/:** Command discovery (what commands can rtk optimize?)
- **parser/:** Generic output parsing utilities

---

## 7. Style Notes Worth Stealing

### Error Handling

**Pattern: anyhow::Result<()> with .context()**

```rust
fn main() -> Result<()> {
    match cli.command {
        Commands::Git { args, .. } => git::run(&args, verbose)
            .context("Failed to run git command")?,
        _ => { /* ... */ }
    }
    Ok(())
}

fn execute_git_command() -> Result<String> {
    let output = Command::new("git")
        .args(&args)
        .output()
        .context("Failed to execute git process")?;
    Ok(String::from_utf8(output.stdout)
        .context("Git output is not UTF-8")?)
}
```

**Why it works:** Each layer adds context. Final error display shows full chain, making debugging trivial. Clean `?` operator usage.

### Filtering Pipeline

**src/core/filter.rs: Composable, level-based**

```rust
pub enum FilterLevel {
    None,      // No filtering
    Minimal,   // ANSI strip, deduplicate
    Aggressive // Truncate, compact JSON, hide boilerplate
}

// In command handler:
let filtered = filter::apply(&raw_output, level, verbose, &config)?;
eprintln!("{filtered}");
```

**Why it works:** Commands control filter aggressiveness independently. Verbosity overrides filtering.

### Package Manager Detection

**Pattern: Lockfile sniffing**

```rust
let is_pnpm = Path::new("pnpm-lock.yaml").exists();
let is_yarn = Path::new("yarn.lock").exists();

let cmd = if is_pnpm {
    "pnpm exec -- eslint"
} else if is_yarn {
    "yarn exec -- eslint"
} else {
    "npx --no-install -- eslint"
};
```

**Why it works:** No subprocess overhead, respects user's toolchain, handles monorepos.

### Hook Atomicity

**Pattern: tempfile + atomic rename**

```rust
let mut tmp = NamedTempFile::new()?;
tmp.write_all(hook_script.as_bytes())?;
tmp.persist(hook_path)?;  // Atomic rename
```

**Why it works:** Safe crash recovery. Multiple `rtk init` runs are idempotent.

### Global Flags

**Clap pattern: `global = true` on flags**

```rust
#[arg(short, long, action = clap::ArgAction::Count, global = true)]
verbose: u8,

#[arg(short = 'u', long, global = true)]
ultra_compact: bool,
```

Allows `-v`, `-vv`, `-vvv` on any subcommand (`rtk git -vv status`).

### Telemetry Design

**Principles:**
- Anonymous: SHA-256 hash of device (not reversible, salted locally)
- One-shot: Once per day (local flag file tracks last ping)
- Lightweight HTTP: `ureq` (not async, no runtime overhead)
- Opt-out: `RTK_TELEMETRY_DISABLED=1` env var, or `[telemetry] enabled = false` in config

Sent data: version, OS, arch, command count (last 24h), top command names (no args), token savings %.

---

## 8. Things NOT to Copy

### Rust-Specific Patterns (Don't Translate to Go)

1. **anyhow::Context chaining:** Go's error handling is different. Consider wrapping errors with structured fields instead (e.g., `fmt.Errorf("failed to execute git: %w", err)`).

2. **Clap derive macros:** Go's flag libraries (cobra, pflags) don't have declarative derive. Hand-code the subcommand tree or use code generation.

3. **Lazy_static for singletons:** Use `sync.Once` in Go for lazy initialization.

4. **rusqlite SQLite bundling:** Go's `sqlite3` driver may require cgo (complexity). Consider a simpler embedded DB or BoltDB if you want pure Go.

5. **Include_str!() for embedded files:** Go has `//go:embed` (Go 1.16+), use that instead.

### Features rtk Has That mastermind Might Not Need

1. **Telemetry system (src/core/telemetry.rs, src/analytics/):** If mastermind is internal-only, you may not need this. If you do, implement it simpler (no anon hashing, just counter).

2. **100+ command handlers (src/cmds/):** rtk's real bulk. If mastermind targets a subset of languages/tools, you can ship with 20-30 handlers initially.

3. **Package manager detection & workspace handling:** Complex heuristics for monorepos (pnpm, yarn, lerna). If mastermind is for single-repo usage, skip this.

4. **Hook installation for 7 different agents:** If mastermind only supports Claude Code (MCP), you don't need init/trust/integrity logic. Just expose stdio tools directly.

5. **Custom filter TOML system (src/core/toml_filter.rs, 1,493 LOC):** Powerful but complex. If users can't customize filtering, skip it; ship with built-in heuristics only.

6. **"Tee" mode (src/core/tee.rs):** Full output recovery mode. Useful for debugging but not essential.

### Complexity Worth Questioning

1. **Hook integrity verification (SHA-256 in src/hooks/integrity.rs):** rtk does this because hooks are critical security boundary (agent → CLI rewriting). If mastermind is an internal tool, you can skip this.

2. **Exit code preservation (throughout src/cmds/):** rtk is production middleware; it must not swallow exit codes. If mastermind is a helper tool, you can be more lenient.

3. **ANSI stripping in multiple layers (src/core/filter.rs, src/core/display_helpers.rs, ecosystem-specific parsers):** Output format variations are extensive. If mastermind targets fewer tools, this simplifies.

4. **SQLite metrics database (47KB of src/core/tracking.rs):** Full-featured: command aggregation, time-based bucketing, sampling. You might just want a JSON log file.

---

## 9. Concrete Differences: Hook Interceptor vs. MCP Server

| Aspect | rtk (Hook Interceptor) | mastermind (MCP Server) |
|--------|------------------------|------------------------|
| **Protocol** | Agent-native hooks (bash, JSON, TypeScript) | MCP stdio (JSON-RPC) |
| **Integration** | Per-agent hook installation (`rtk init`) | Single stdio server, agents configure MCP in settings.json |
| **Command Rewriting** | Hook rewrites command before agent sees it | Agent calls tool via MCP, server processes request |
| **Extensibility** | Add agent via new hook script + init logic | Add tool via tool registration in server |
| **Attack Surface** | Hook scripts embedded in binary (trust rtk) | Stdio protocol (trust client isolation) |
| **Bootstrap** | User runs `rtk init -g --agent claude` | Agent adds to `mcp_servers` config, restarts |

**Key Implication for mastermind:** If you're building an MCP server, you don't need rtk's hook infrastructure (init, rewrite_cmd, integrity). You can focus entirely on tool registration and stdio handling.

---

## Summary: What to Steal

1. **Minimal dependencies + no async:** Keep mastermind small and fast. Use sync I/O.
2. **anyhow + clap + serde ecosystem:** Standard Rust patterns; translate idiomatically to Go.
3. **Modular command handlers:** Organize by domain (git, cargo, python, etc.).
4. **Filtering pipeline:** Design composable output compression, controlled by verbosity/flags.
5. **Atomic file writes:** Use tempfile + rename for config/state mutations.
6. **Exit code preservation:** Don't swallow process exit codes.
7. **Telemetry opt-out:** If you collect metrics, make it trivial to disable.

**What NOT to steal:**
- Hook installation machinery (if mastermind is MCP-only)
- 100+ command handlers upfront (start with core languages)
- Complex TOML filter system (use hardcoded heuristics initially)
- Integrity verification (unless you have security paranoia)

---

## References

- **ARCHITECTURE.md** in rtk repo: 60-page deep-dive on module organization, filtering strategies, performance characteristics
- **src/hooks/README.md**: Hook lifecycle, trust system, adding new agents
- **src/core/README.md**: Utilities API (execute_command, strip_ansi, etc.)
- **.github/workflows/release.yml**: Cross-compilation matrix and build strategy
- **Cargo.toml**: Full dependency list with rationale comments
