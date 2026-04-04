# ByteRover CLI (brv) — Reference Analysis for Mastermind

## 1. Language and Stack

**Language:** TypeScript/Node.js (100% TS codebase)

**Installation:** Via npm global:
```bash
npm install -g byterover-cli
brv  # Entry point is ./bin/run.js
```

**Stack details:**
- CLI framework: oclif
- LLM providers: 20+, via `@ai-sdk/*` (Anthropic, OpenAI, Google, Groq, Mistral, xAI, Cerebras, DeepInfra, Cohere, etc.)
- TUI: React + Ink
- Transport: Custom daemon-based IPC (not HTTP) via `@campfirein/brv-transport-client`
- MCP: Model Context Protocol server implementation
- Data formats: Markdown files, JSON config, SQLite snapshots

**Directory structure:**
```
src/
  ├── tui/                    # React/Ink UI (REPL interface, hub flows, commands)
  ├── server/                 # Core daemon logic
  │   ├── infra/              # Infrastructure layer
  │   │   ├── context-tree/   # File-based node storage
  │   │   ├── executor/       # Query execution, retrieval, ranking
  │   │   ├── mcp/            # MCP server & tools (brv-query, brv-curate)
  │   │   ├── hub/            # Hub registry, auth, install (TIGHTLY COUPLED)
  │   │   ├── transport/      # IPC daemon, event handlers
  │   │   └── config/         # Project & global config management
  │   ├── core/               # Domain models, interfaces
  │   └── constants.ts        # BRV_DIR, CONTEXT_TREE_DIR, etc.
  ├── agent/                  # Agent sandbox tools (LLM worker)
  └── shared/                 # Transport schemas, types
bin/
  ├── run.js                  # Entry point (oclif dispatcher)
  ├── dev.js                  # Dev harness
  └── kill-daemon.js          # Daemon cleanup
```

---

## 2. The "Node" / Entry Format

### File Format & Location

**Format:** Markdown (`.md` files)

**Location:** **Repo-relative, inside `.brv/context-tree/`**
- Path structure: `.brv/context-tree/{domain}/{category}/{node-name}.md`
- Each project has its own `.brv/` directory at the repo root
- NOT machine-level; NOT configurable
- Example: `.brv/context-tree/patterns/auth/jwt-24h-expiry.md`

### Schema

**Node structure (Markdown with YAML frontmatter):**

From `file-context-tree-writer-service.ts` and `file-context-file-reader.ts`:

```markdown
---
name: "JWT Auth Pattern"
tags: ["auth", "security", "middleware"]
keywords: ["jwt", "token", "expiry", "24h"]
---

# JWT Auth Implementation

[Narrative content in markdown]

## Details

Raw concept or structured details here.
```

**Parsed fields (from MarkdownWriter.parseContent()):**
- `name`: From frontmatter "name" or first H1 heading (fallback to file path)
- `tags`: Array of tags from frontmatter
- `keywords`: Array of searchable keywords
- `narrative`: The markdown body content
- `rawConcept`: Optional structured details block
- `path`: Relative path in context-tree (e.g., `patterns/auth/jwt-24h-expiry.md`)
- `title`: Resolved from frontmatter name, H1 heading, or path fallback

**No explicit ID field** — nodes are identified by their file path relative to `.brv/context-tree/`.

### Example Node (Actual brv Usage)

From README quick-start:
```markdown
# Auth Implementation

Uses JWT with 24h expiry. Token stored in HTTP-only cookie.

## Implementation

See @src/middleware/auth.ts for details.
```

Created via:
```
/curate "Auth uses JWT with 24h expiry" @src/middleware/auth.ts
```

The curate tool accepts:
- `context`: Natural language knowledge to store
- `files` (optional, max 5): File paths to include as context
- `folder` (optional): Folder to pack and analyze (takes precedence over files)

---

## 3. The Hub Integration

### What Does Hub Actually Do?

Hub is a **registry of reusable agent skills and bundles** (similar to npm packages). It provides:

1. **Official Registry** (default): `https://hub.byterover.dev` (hardcoded)
2. **Custom Registries**: Users can add private registries via `brv hub-registry add`
3. **Install Mechanism**: `brv hub install <entry-id>` downloads and installs skills/bundles into `.brv/skills/` or `.brv/bundles/`
4. **Authentication**: Token-based (stored encrypted in macOS keychain or XDG data dir)

Hub handlers: `src/server/infra/transport/handlers/hub-handler.ts`

Entry types:
- `agent-skill`: Installable agent plugins
- `bundle`: Reusable configurations or knowledge packages

**Key code paths:**
- Hub list/install: `src/tui/features/hub/api/` (TUI layer)
- Registry management: `src/server/infra/hub/hub-registry-service.ts`
- Auth tokens: `src/server/infra/hub/hub-keychain-store.ts` (AES-256-GCM encrypted)
- Installation: `src/server/infra/hub/hub-install-service.ts`

### Hub Coupling Assessment

**CRITICAL FINDING: Hub is TIGHTLY COUPLED to core, but OPTIONAL for basic operation.**

Evidence:
- `brv-curate` and `brv-query` MCP tools do NOT require hub to function ✓
- `brv query` command works locally without any hub connectivity ✓
- Hub is only invoked if user explicitly runs `brv hub install` or `brv hub-registry add` ✓
- **However:** Hub code is woven throughout the codebase:
  - Hub handler registered in transport server setup
  - Hub registry config stored in `.brv/config.json` (alongside project config)
  - Keychain store and auth headers depend on hub auth scheme types
  - **No flag to disable or skip hub registration entirely**

**Can brv run without hub?** Yes, completely. Hub just adds optional extensibility.

**Is there a disable flag?** No — hub is always initialized, but harmless if unused.

---

## 4. MCP Tool Surface

**Tools registered:** Two primary tools in `src/server/infra/mcp/tools/`

### `brv-query`
```typescript
Input schema:
  - query: string (required)
    "Natural language question about the codebase or project"
  - cwd: string (optional)
    "Working directory of the project (absolute path).
     Required in global mode (e.g., Windsurf).
     Optional in project mode — defaults to project directory."

Output: Markdown response with ranked context
```

**What it does:**
1. Resolves project root via walking up from `cwd` looking for `.brv/config.json`
2. Creates a task via IPC transport: `{type: 'query', content: query, clientCwd, taskId}`
3. Waits for agent to process and return results
4. Returns LLM response as text/markdown

**Retrieval mechanism:**
- Multi-tier strategy (from `query-executor.ts`):
  - **Tier 0:** Exact cache hit (0ms, if fingerprint matches)
  - **Tier 1:** Fuzzy cache match via Jaccard similarity (~50ms)
  - **Tier 2:** Direct search response without LLM (~100-200ms)
    - Uses `SearchKnowledgeService` to find matches
    - If high-confidence (score >0.7), return directly
  - **Tier 3:** LLM call with pre-fetched context (<5s)
    - Retrieves top-5 ranked documents via search
    - Injects context into system prompt before LLM call
  - **Tier 4:** Full agentic loop fallback (8-15s)

**Ranking/Search:** Uses `search-knowledge-service.ts` (agent sandbox tool)
- Likely keyword + semantic search (embeddings-based, via LLM provider)
- No details on vector index; probably delegated to agent's embedding model

### `brv-curate`
```typescript
Input schema:
  - context: string (optional)
    "Knowledge to store: patterns, decisions, errors, or insights.
     Required unless files or folder provided."
  - files: string[] (optional, max 5)
    "File paths with critical context to include"
  - folder: string (optional)
    "Folder path to pack and analyze (takes precedence over files)"
  - cwd: string (optional)
    "Working directory (required in global mode)"

Output: "{taskId}: Context queued for curation"
```

**What it does:**
1. Validates at least one input (context/files/folder) is provided
2. Resolves project root
3. Creates task: `{type: 'curate', content, taskId, files?, folderPath?}`
4. Returns **fire-and-forget** — task queued, response returns immediately
5. Agent processes asynchronously in background

**Task types:**
- `'curate'`: Standard knowledge storage (context + optional files)
- `'curate-folder'`: Pack entire folder and analyze (folder takes precedence)

### Other Tools

No other MCP tools listed. These two are the surface.

---

## 5. Warmup / Init Flow ("brv-init" Concept)

**Init handler:** `src/server/infra/transport/handlers/init-handler.ts`

**What "init" actually does (NOT code exploration):**

Init is a **project setup flow**, not an exploratory walk:
1. User runs `brv` in a project directory (no args)
2. Checks if `.brv/config.json` exists → if yes, skip init
3. If not, prompt user to:
   - Select LLM provider (Anthropic, OpenAI, etc.)
   - Authenticate if needed (login via cloud)
   - Select project space (if cloud-synced)
   - Install connector (e.g., vscode-langserver for code intelligence)
4. Creates minimal `.brv/config.json` and `.brv/context-tree/` directory
5. Initializes empty `.snapshot.json`

**No automatic codebase exploration happens here.**

**"Warmup" context acquisition:**
- **Not built-in** to brv's init flow
- Users manually curate via `/curate "pattern" @file.ts` commands in REPL
- OR via `brv hub install` to pull pre-curated skill bundles from hub
- OR via agent tools in the REPL sandbox (`search-knowledge-service`, codebase traversal)

The **"brv-init" concept** referenced in the user's note likely refers to the skill (separate tool), not core brv functionality.

---

## 6. Retrieval Mechanism

**Full retrieval path (from brv-query → response):**

1. **MCP tool receives query**, resolves project root
2. **Transport creates task** (IPC to daemon)
3. **Agent process wakes up** (AgentProcess subscribes to task:create events)
4. **Search pre-fetch (parallel):**
   - If `searchService` available, call `SearchKnowledgeService.search(query)`
   - Returns ranked results with scores
   - Supplementary entity-based searches if <3 results
5. **Tier-based response selection:**
   - Check cache (exact + fuzzy)
   - Try direct search response (no LLM)
   - Build prefetched context from top-5 search results
6. **LLM call:**
   - System prompt includes prefetched context (RLM pattern)
   - Send query + context to LLM
   - Stream response via transport events
7. **Response cached** (if cache enabled)

**Search algorithm:**
- **Keyword-based:** Likely BM25 or TF-IDF over node text
- **Semantic:** Via LLM embeddings (delegated to agent's model provider)
- **Ranking:** Score threshold (0.7) for high-confidence direct response
- **Similarity metric (for cache):** Jaccard similarity over query keywords

**No vector database:** Retrieval is compute-on-demand, leveraging agent's LLM provider for embeddings.

---

## 7. Project Scoping

**Git detection algorithm:**

From `mcp-mode-detector.ts`:
```typescript
function detectMcpMode(workingDirectory: string): McpModeResult {
  let current = workingDirectory
  let parent = dirname(current)
  
  while (current !== parent) {
    if (existsSync(join(current, '.brv', 'config.json'))) {
      return {mode: 'project', projectRoot: current}
    }
    current = parent
    parent = dirname(current)
  }
  
  // Check root
  if (existsSync(join(current, '.brv', 'config.json'))) {
    return {mode: 'project', projectRoot: current}
  }
  
  return {mode: 'global'}
}
```

**Method:** Walk up filesystem from `cwd` looking for `.brv/config.json`

**Two modes:**
- **Project mode:** MCP server launched from within a project (has `.brv/config.json`)
  - Tools default to project directory
- **Global mode:** MCP server launched from non-project directory (e.g., Windsurf global context)
  - Each tool call must provide explicit `cwd` parameter

**Registry of projects:** None. brv discovers projects dynamically by walking up from client cwd.

**Multi-project support:** Each project is independent (its own `.brv/` dir). Hub registry config is stored per-project in `.brv/config.json`.

---

## 8. Machine-Level vs Repo-Level Storage

### Per-Project (Repo-Level)
```
.brv/
  ├── config.json                 # Project config (JSON)
  │   └── Fields: createdAt, cwd, ide, spaceId, spaceName, teamId, teamName, cipherAgentContext, etc.
  ├── context-tree/               # Knowledge nodes (Markdown)
  │   ├── patterns/
  │   ├── decisions/
  │   ├── errors/
  │   └── {domain}/{category}/{node}.md
  ├── context-tree-backup/        # Sync backups
  ├── context-tree-conflicts/     # Merge conflict resolution
  ├── .snapshot.json              # CoGit snapshot for change tracking
  └── skills/                      # Installed hub skills (optional)
       └── {skill-id}/
```

### Machine-Level (XDG/OS-Specific)
```
~/.config/brv/                     # Global config directory (XDG_CONFIG_HOME/brv)
  └── config.json                  # User's global config (LLM provider, auth state, etc.)

~/.local/share/brv/                # Global data directory (XDG_DATA_HOME/brv on Linux)
  ├── .hub-registry-keys           # Encryption key for hub credentials
  └── hub-registry-credentials     # Encrypted hub auth tokens (AES-256-GCM)

# macOS equivalent
~/Library/Preferences/brv/         # macOS config
~/Library/Application Support/brv/ # macOS data
```

**Config sync:** When project is initialized, `syncConfigToXdg()` copies project config to global XDG store (for multi-IDE access).

**No centralized registry** of projects. brv discovers on-demand via `.brv/config.json` walking.

---

## 9. Things to Steal

These design patterns/behaviors are clearly right:

1. **Fire-and-forget curate pattern**
   - User calls `brv-curate`, gets immediate confirmation (taskId)
   - Processing happens asynchronously
   - No blocking on LLM/indexing
   - **Reason:** Feels responsive, good UX for memory storage

2. **Multi-tier query response strategy**
   - Cache hits (exact + fuzzy)
   - Direct search response (confidence threshold)
   - LLM with context
   - Fallback agentic loop
   - **Reason:** Orders responses by speed, reduces latency for common queries

3. **Project-relative storage (not centralized)**
   - `.brv/` at repo root, travels with code
   - Enables version control, multi-IDE access, team sharing
   - **Reason:** Knowledge lives with code; no separate sync backend needed (unless you want cloud)

4. **Walk-up project detection**
   - From any subdirectory, find project root
   - No config registry needed
   - **Reason:** Simplicity, works in nested repos/monorepos, IDE-agnostic

5. **Markdown + YAML frontmatter node format**
   - Human-readable, git-friendly, diffable
   - Frontmatter for machine metadata
   - **Reason:** Text-first, low friction, works with existing tools

6. **Transport daemon architecture**
   - Long-lived daemon (IPC, not HTTP)
   - Clients connect via daemon for query/curate
   - Enables efficient resource sharing, warmup optimization
   - **Reason:** Faster RPC, no network overhead, better sandbox isolation

7. **Hub registry as optional plugin system**
   - Skills and bundles are installable, not bundled
   - Works without hub (local-only mode)
   - **Reason:** Extensibility without bloat; users opt-in to features

8. **Global XDG config + project-level override**
   - User-level LLM provider choice
   - Project-level space/team binding
   - **Reason:** Sensible defaults + project-specific context

---

## 10. Things to Avoid

**Complexity/coupling to eliminate in mastermind:**

1. **Hub as core coupling (BIGGEST)**
   - Hub code is mixed into transport handlers, config store, auth flow
   - No clean abstraction boundary
   - Even though hub is optional, its infrastructure is everywhere
   - **Avoid:** Don't mix hub concerns into core query/curate/project logic
   - **Better:** Hub should be a separate plugin system, loaded at startup if available

2. **Cloud sync in the middle of everything**
   - CoGit pull/push logic in handlers, snapshot service, context-tree writer
   - Makes local-only operation hard to reason about
   - **Avoid:** Separate cloud sync from local storage layer
   - **Better:** Local storage is complete on its own; sync is an optional middleware layer

3. **TUI tightly coupled to business logic**
   - React/Ink components directly call infra services
   - Makes testing and reusing logic hard
   - **Avoid:** Don't mix TUI event handlers with domain logic
   - **Better:** Use clear UseCase layer, TUI calls use cases, uses cases call infra

4. **Daemon IPC in the critical path**
   - MCP tools always go through transport layer
   - Adds latency, indirection
   - **Avoid:** Direct query/curate from MCP without daemon hop if possible
   - **Better:** MCP tools can call local services directly (no IPC) if in same process

5. **Snapshot state explosion**
   - CoGit snapshots, context-tree snapshots, sync conflict dirs
   - Hard to reason about actual state
   - **Avoid:** Multiple sources of truth
   - **Better:** Single snapshot per context-tree, clear merge semantics

6. **Agent sandbox as default retrieval**
   - SearchKnowledgeService is an agent tool (requires sandbox execution)
   - Query executor depends on agent availability
   - **Avoid:** Making LLM availability mandatory for search
   - **Better:** Pluggable search (local BM25, embeddings, agent-backed — user choice)

7. **Package.json as central dependency tree**
   - 20+ AI SDK providers listed as direct dependencies
   - Bloats install, even if unused
   - **Avoid:** Monolithic dependency tree
   - **Better:** Lazy-load or plugin providers; core should not depend on all of them

---

## Summary for Mastermind Rewrite (Go)

**What to keep:**
- Markdown nodes in `.mm/knowledge-tree/` (local, repo-relative)
- YAML frontmatter schema (name, tags, keywords, etc.)
- Walk-up project detection (look for `.mm/config.json`)
- Fire-and-forget curate pattern (async queueing)
- Multi-tier query strategy (cache → direct → LLM)
- Daemon/service architecture for query execution

**What to replace:**
- Skip hub entirely (no plugin registry initially)
- Separate cloud sync from core (it's optional)
- Direct storage layer without daemon hop (Go binary is faster anyway)
- Local-first search (BM25 or SQLite FTS, not LLM-dependent)
- Single snapshot mechanism (CoGit is overkill for local-only)

**Architecture template:**
```
mastermind/
  ├── cmd/mm/                  # CLI entry point
  ├── internal/
  │   ├── storage/             # Knowledge tree + node I/O
  │   ├── search/              # BM25 or FTS retrieval
  │   ├── query/               # Query execution (LLM optional)
  │   ├── project/             # Project detection + config
  │   └── mcp/                 # MCP server (optional initially)
  ├── .mm/                     # Per-project storage
  │   ├── config.json
  │   ├── knowledge-tree/
  │   └── .snapshot.json
  └── ~/.config/mm/            # Global config
```

