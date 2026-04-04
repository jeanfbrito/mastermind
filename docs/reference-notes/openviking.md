# OpenViking Memory Extraction Architecture

**Date:** 2026-04-04  
**Focus:** End-of-session auto-extraction and hierarchical memory scoping

## Critical Finding: Yes, auto-extraction exists and is production-grade

OpenViking **does implement** end-of-session auto-extraction as a core feature. When a session is committed (via `/api/v1/sessions/{id}/commit`), extraction is triggered automatically in the background. This is not manual curation—it's a robust, asynchronous pipeline with distributed locking, telemetry, and transaction safety.

---

## 1. Project Shape

OpenViking is a **hybrid Rust+Python project** (~1500+ lines of extraction logic). The Rust layer (`crates/ov_cli/`) is a thin HTTP client; the real work lives in Python (`openviking/` directory). The project implements a full LLM-powered memory extraction system with hierarchical scope support (user/agent/project), a virtual filesystem abstraction (`viking://` URIs), and a schema-driven prompt templating system. Total codebase is substantial (~30K+ LOC), but extraction is concentrated in `openviking/session/` and `openviking/server/routers/`.

---

## 2. Where End-of-Session Extraction Lives

**Router entry point:**
- `/Users/jean/Github/OpenViking/openviking/server/routers/sessions.py` — lines 223–230
  - Endpoint: `POST /api/v1/sessions/{session_id}/commit` → calls `service.sessions.extract()`

**Service orchestration:**
- `/Users/jean/Github/OpenViking/openviking/service/session_service.py` — lines 195–215
  - Method: `async def extract()` — routes to compressor

**Session state machine & Phase 2 (background extraction):**
- `/Users/jean/Github/OpenViking/openviking/session/session.py` — lines 352–468 (commit_async) + 470–600 (_run_memory_extraction)
  - **Two-phase design:**
    - Phase 1 (fast, lock-protected): Archive messages to disk, clear live session, return immediately.
    - Phase 2 (background): Run memory extraction via `asyncio.create_task()` (no waiting).

**Extraction orchestrator (ReAct loop):**
- `/Users/jean/Github/OpenViking/openviking/session/memory/extract_loop.py` — 498 lines
  - Class: `ExtractLoop` — implements ReAct-style iterative refinement (read/search tools, then output operations).

**Memory extractor (candidate generation):**
- `/Users/jean/Github/OpenViking/openviking/session/memory_extractor.py` — 1505 lines
  - Classes: `CandidateMemory`, `ToolSkillCandidateMemory`, `MergedMemoryPayload`
  - Defines 8 memory categories: `PROFILE`, `PREFERENCES`, `ENTITIES`, `EVENTS` (user-scope) + `CASES`, `PATTERNS` (agent-scope) + `TOOLS`, `SKILLS`.

**Compressor (pipeline entry):**
- `/Users/jean/Github/OpenViking/openviking/session/compressor_v2.py` — lines 80–268
  - Method: `async def extract_long_term_memories()` — initializes orchestrator, manages distributed locking, applies operations to storage.

---

## 3. How Extraction Is Triggered

**Trigger type:** **Automatic + Explicit**

- **Automatic:** When user calls `/api/v1/sessions/{id}/commit` (or `session.commit_async()`), Phase 2 extraction is scheduled asynchronously via `asyncio.create_task()`. Returns immediately with a `task_id` for polling.
- **Explicit:** User can poll `/api/v1/sessions/{id}/extraction_stats?task_id={task_id}` to track progress.
- **No manual review queue** — extracted memories are written directly to storage (subject to deduplication & merge strategy).

**Flow:**
```
POST /commit
  ↓
Phase 1 (sync, ~100ms): Archive messages, clear session
  ↓
Return task_id
  ↓
Phase 2 (async): ExtractLoop → LLM calls → Write to viking_fs
```

---

## 4. The Extraction Prompt (Verbatim)

**File:** `/Users/jean/Github/OpenViking/openviking/session/memory/session_extract_context_provider.py`  
**Method:** `instruction()` (lines 55–88)

```python
goal = f"""You are a memory extraction agent. Your task is to analyze conversations and update memories.

## Workflow
1. Analyze the conversation and pre-fetched context
2. If you need more information, use the available tools (read/search)
3. When you have enough information, output ONLY a JSON object (no extra text before or after)

## Critical
- ONLY read and search tools are available - DO NOT use write tool
- Before editing ANY existing memory file, you MUST first read its complete content
- ONLY read URIs that are explicitly listed in ls tool results or returned by previous tool calls

## Target Output Language
All memory content MUST be written in {output_language}.

## URI Handling
The system automatically generates URIs based on memory_type and fields. Just provide correct memory_type and fields.

## Edit Overview Files
After writing new memories, you MUST also update the corresponding .overview.md file.
- Provide memory_type to identify which directory's overview to update

## Overview Format
Two options:
1. **PREFERRED: Direct string** - Just provide the complete new overview content:
   {{"memory_type": "events", "overview": "# Events Overview\n- [event1](event1.md) - Description"}}
2. **SEARCH/REPLACE** - Only use if you must modify a small portion:
   {{"memory_type": "events", "overview": {{"blocks": [{{"search": "exact line to change", "replace": "new line"}}]}}}}

See GenericOverviewEdit in the JSON Schema below."""
```

**Execution model:** The prompt is combined with:
1. A `system` message containing the above instructions + JSON schema for memory types
2. A `user` message containing the conversation history (assembled from session messages)
3. Optional tool calls if the LLM needs to read/search before generating operations

Language is **auto-detected** from conversation; falls back to config (`language_fallback` in config).

---

## 5. Input to the Extractor

**Input shape:**

```python
# From session.py line 80-88
async def extract_long_term_memories(
    messages: List[Message],           # Full session transcript
    user: Optional["UserIdentifier"],  # User context (for scope)
    session_id: Optional[str],         # Session ID for archive reference
    ctx: Optional[RequestContext],     # Request context (auth, user, account)
    strict_extract_errors: bool = False,
    latest_archive_overview: str = "",  # Previous archive's overview (for context)
) -> List[Context]:
```

**Message structure:**
- `messages` is a list of `Message` objects (from `openviking.message`).
- Each message has `.role` (user/assistant), `.content` (text), `.parts` (list of `TextPart`/`ToolPart`/`ContextPart`).
- Full transcript is **assembled as a single conversation string** (not chunked).

**Assembly:** `_assemble_conversation()` formats messages with timestamps, tool invocations, and context citations.

---

## 6. Output Shape

**Output type:** `MemoryOperations` dataclass (from `openviking.session.memory.dataclass`)

```python
class MemoryOperations:
    write_uris: Dict[str, str]  # {"viking://path/to/memory.md": "content"}
    edit_uris: Dict[str, Any]   # {"viking://path": {"blocks": [{"search": "...", "replace": "..."}]}}
    delete_uris: List[str]      # ["viking://path/to/old_memory.md"]
```

**Conversion:**
- Written to a staging area on disk (in session archive directory).
- Applied directly to storage (viking_fs) in Phase 2.
- No user review step — memories are auto-committed (but merge operations can protect existing data).

**Telemetry tracked:**
```python
telemetry.set("memory.extract.created", n)      # New memories created
telemetry.set("memory.extract.merged", n)       # Existing memories merged
telemetry.set("memory.extract.deleted", n)      # Memories deleted
telemetry.set("memory.extract.candidates.total", n)  # Candidates considered
```

---

## 7. Scope Assignment (Hierarchical Memory)

**Yes, the extractor decides scope via memory_type.**

**Mechanism:**

Memory schemas define both the `directory` (scope) and `memory_type`:

**File:** `/Users/jean/Github/OpenViking/openviking/prompts/templates/memory/profile.yaml`
```yaml
memory_type: profile
directory: "viking://user/{{ user_space }}/memories"  # Jinja2 template
filename_template: "profile.md"
```

**File:** `/Users/jean/Github/OpenViking/openviking/prompts/templates/memory/cases.yaml`
```yaml
memory_type: cases
directory: "viking://agent/{{ agent_space }}/memories/cases"  # Agent scope
filename_template: "{{ case_name }}.md"
```

**Scope hierarchy:**
- `viking://user/{user_space_name}/memories/` — user-personal (profile, preferences, entities, events)
- `viking://agent/{agent_space_name}/memories/` — agent-specific (cases, patterns, tools, skills)
- `viking://resources/{project}/` — project/resource scope (not extracted, but part of storage model)

**Scope assignment flow:**
1. LLM receives list of all enabled memory schemas (with directory templates).
2. LLM outputs `memory_type` in each operation (e.g., `"memory_type": "profile"`).
3. Jinja2 renderer in compressor expands `{{ user_space }}` and `{{ agent_space }}` to actual values.
4. Orchestrator writes to the fully-resolved URI.

**No secondary LLM call** — scope is implicit in the schema choice.

---

## 8. Review/Approval Flow

**No review queue.** Extracted memories are:

1. **Proposed by LLM** in a single JSON output.
2. **Validated** against schema (JSON parsing, field type checks).
3. **Applied immediately** to storage (write_uris, edit_uris, delete_uris).
4. **Merged with existing memories** using merge operators (`patch`, `immutable`, `sum`).

**Merge strategy** (defined per field in memory schemas):
- `patch`: Overwrite/update the field.
- `immutable`: Only set if field doesn't exist (never overwrite).
- `sum`: Accumulate numeric values.

**Deduplication:** Before writing, the system checks for duplicates using hash-based comparison + embedding similarity (if vectordb enabled).

**No intermediate staging.** The "accepted" status is returned to the client *before* memory writing completes (Phase 2 is async).

---

## 9. Directory Layout for Memory (Viking:// URIs)

**Real filesystem mapping:**

The `viking://` URI scheme maps to actual filesystem paths via `VikingFS` abstraction.

**Example mappings:**

```
viking://user/user_123/memories/profile.md
  → ~/.openviking/data/user/user_123/memories/profile.md

viking://agent/agent_456/memories/cases/problem_x.md
  → ~/.openviking/data/agent/agent_456/memories/cases/problem_x.md

viking://resources/my_project/
  → ~/.openviking/data/resources/my_project/

viking://session/user_123/session_abc/history/
  → ~/.openviking/data/session/user_123/session_abc/history/
```

**Storage backend:**
- **Primary:** Local filesystem (AGFS = async-safe filesystem wrapper).
- **Optional:** Remote filesystem (e.g., S3 via AGFS plugin).
- **Index:** LevelDB or equivalent for metadata.
- **VectorDB:** Optional (for semantic search in retrieval).

**Memory files are genuine Markdown files** with:
- `.overview.md` at each scope root (index of all memories in that directory).
- Individual memory files (e.g., `profile.md`, `event_1.md`).
- Structured fields as Markdown headings + YAML frontmatter (not enforced, but convention).

---

## 10. Things Worth Translating to Go

### A. The ReAct Loop Pattern

Extract the core pattern from `extract_loop.py`:
- **Iteration with tools:** LLM call → read/search tools → continue or output final operations.
- **Max iterations limit** (default 3) prevents infinite loops.
- **Schema-driven:** Memory types and fields are defined in schemas, not hardcoded.

This is elegant and generalizable. The orchestrator doesn't assume specific memory types—it reads schemas at runtime.

### B. Language Detection & Auto-Fallback

`_detect_language()` in `session_extract_context_provider.py` detects conversation language, falls back to config. This prevents outputting English memories for Chinese conversations (or vice versa).

### C. Jinja2 Template Expansion for Scopes

Using Jinja2 to expand directory templates (`viking://user/{{ user_space }}/memories`) is clean. Go equivalent: simple string templates or Go's `text/template`.

### D. Telemetry & Phase Timing

Two-phase commit with distributed locking is robust:
- Phase 1 (fast path): archive and clear session.
- Phase 2 (background): run extraction without blocking client.

Tracks per-stage timing (prepare inputs, LLM call, normalize, dedup, etc.). Useful for debugging slow extractions.

### E. Merge Operators for Existing Memories

Polymorphic merge strategies (`patch`, `immutable`, `sum`) prevent accidental overwrites and allow field-level conflict resolution. Worth replicating in mastermind.

---

## 11. Things to NOT Translate (Architecture Antipatterns)

### A. The ReAct Loop Overhead

OpenViking's `ExtractLoop` uses LLM tool calls (read/search) before outputting operations. This is powerful but **adds latency**:
- Example: "Extract profile" → LLM decides to read existing profile → tool call → re-analyze → output merge.
- For a simple "add event" memory, this is overkill.

**Recommendation for mastermind:** Start with a single-shot extraction (no tools). Add tools only if accuracy demands it (e.g., entity deduplication across thousands of events).

### B. The Distributed Lock Complexity

OpenViking uses filesystem locks (PathLock) and a transaction manager for concurrent writes across workers. This is necessary for a multi-tenant server but **adds complexity**:

```python
# From compressor_v2.py lines 120-163
if viking_fs and hasattr(viking_fs, "agfs") and viking_fs.agfs:
    init_lock_manager(viking_fs.agfs)
    lock_manager = get_lock_manager()
    transaction_handle = lock_manager.create_handle()
    # ... acquire_subtree_batch with timeout=None (infinite wait)
```

**Recommendation:** If mastermind is single-user/single-agent, skip the lock manager entirely. Use file-based serialization (e.g., flock) only if concurrent sessions are possible.

### C. The Skill/Tool Memory Subsystem

OpenViking extracts 8 categories, including `TOOLS` and `SKILLS` (tool usage statistics, invocation patterns, etc.). This is server-specific and entangled with the VikingBot agent framework. **Not relevant for mastermind** unless you want to track "which CLI commands were useful today."

**Recommendation:** Start with 4 categories: `profile`, `events`, `entities`, `preferences`. Add `cases` and `patterns` later if needed.

### D. Schema Type Registry & Dynamic Schema Generation

OpenViking uses a `MemoryTypeRegistry` + `SchemaModelGenerator` to dynamically create Pydantic models from YAML schema definitions. This is powerful but **requires significant infrastructure**:
- YAML file parsing.
- Pydantic dynamic model generation.
- JSON schema derivation from models.

**Recommendation for mastermind:** Define memory types statically in Go structs. Use JSON schema generation libraries (or write schema manually). Avoid dynamic model generation unless you need plugin-based custom memory types.

### E. The Entire Skill/Agent Framework Integration

OpenViking is deeply tied to VikingBot (an agent framework), Helm charts, Docker, multi-account RBAC, etc. **None of this applies to mastermind.**

---

## 12. Specific Behaviors Worth Emulating

### A. Conversation Assembly with Timestamps

Before extraction, the system assembles a formatted conversation string:

```python
# From session_extract_context_provider.py lines 115–125
def _build_conversation_message(self) -> Dict[str, Any]:
    """Assemble conversation with session time and day-of-week."""
    session_time = messages[0].created_at or datetime.now()
    time_display = f"{session_time.strftime('%Y-%m-%d %H:%M')} ({day_of_week})"
    conversation = self._assemble_conversation(self.messages)
    return {
        "role": "user",
        "content": f"""## Conversation History
**Session Time:** {time_display}
Relative times (e.g., 'last week', 'next month') are based on Session Time, not today.

{conversation}

After exploring, analyze the conversation and output ALL memory write/edit/delete operations in a single response."""
    }
```

**Why this matters:** Including the session timestamp and day-of-week helps the LLM ground relative times ("tomorrow" = tomorrow from session start, not today). This is subtle but prevents off-by-one errors in extracted event dates.

### B. Language Fallback Configuration

```python
output_language = detect_language_from_conversation(conversation, fallback_language="en")
```

The LLM's output language is auto-selected but can be overridden via config. Store the output language in the prompt so the LLM knows what to write.

### C. Overview File Management

After writing new memories, the system updates `.overview.md` (a directory index). This is not done by the LLM (LLM can't reliably update markdown lists); instead, the system manages it:

```yaml
1. **PREFERRED: Direct string** - Just provide the complete new overview content:
   {"memory_type": "events", "overview": "# Events Overview\n- [event1](event1.md) - Description"}
2. **SEARCH/REPLACE** - Only use if you must modify a small portion:
   {"memory_type": "events", "overview": {"blocks": [{"search": "exact line to change", "replace": "new line"}]}}
```

LLM can provide the full overview as a string (preferred), or a search/replace patch. Post-processing applies the changes to disk.

---

## Summary for Mastermind Implementation

**The core idea OpenViking implements well:**

1. **Automatic extraction on session commit** → implemented via async Phase 2.
2. **Hierarchical scopes** → via `memory_type` + directory templates.
3. **Iterative refinement with tools** → ReAct loop (optional complexity).
4. **Merge operators** → field-level conflict resolution.
5. **Schema-driven prompts** → YAML templates define memory categories + fields.

**Mastermind adaptation:**

- Start simple: single-shot extraction, no ReAct tools.
- Use static Go structs for memory types (profile, events, entities, preferences).
- Implement two-phase commit (immediate return, async extraction) only if needed.
- Define merge operators (`patch`, `immutable`) per field.
- Use Markdown + YAML frontmatter for storage (same as OpenViking).
- Auto-detect conversation language; include session timestamp in prompt.

**High-value artifacts to steal:**

- The extraction prompt structure (system + conversation + schema).
- The language detection + fallback logic.
- The two-phase commit pattern (fast archive, async extraction).
- The merge operator concept.
- The `.overview.md` index pattern.

---

## Files to Consult

| Path | Purpose |
|------|---------|
| `openviking/session/memory/extract_loop.py` | ReAct orchestrator (optional; start without it) |
| `openviking/session/memory/session_extract_context_provider.py` | Prompt assembly + language detection |
| `openviking/session/session.py` lines 352–600 | Two-phase commit implementation |
| `openviking/session/compressor_v2.py` lines 80–268 | Extraction pipeline entry point |
| `openviking/server/routers/sessions.py` | HTTP endpoint routing |
| `openviking/prompts/templates/memory/*.yaml` | Memory type schemas (copy the pattern) |
| `openviking/session/memory_extractor.py` | Memory category definitions |

