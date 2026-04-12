"""
mastermind — Hermes memory provider plugin.

Connects Hermes Agent to mastermind's persistent knowledge store via the
mastermind binary's MCP stdio interface. No API keys required — just the
mastermind binary on PATH (or MASTERMIND_BIN env var).

Tools exposed to the agent:
  mm_search     — search knowledge across all scopes
  mm_write      — write an entry to the live store
  mm_promote    — promote a pending entry to live
  mm_close_loop — mark an open-loop as resolved

Optional hooks:
  system_prompt_block()  — injects session-start context (open loops + project knowledge)
  prefetch(query)        — runs mm_search before each turn
  on_pre_compress()      — extracts knowledge before context compression
  on_session_end()       — extracts knowledge at session close

auto_capture / mirror_memory_writes are disabled by default (mastermind
philosophy: writes are user-initiated, not automatic).
"""

import json
import logging
import os
import subprocess
import tempfile
import threading
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


class MastermindMCPClient:
    """Minimal MCP stdio client for the mastermind binary.

    Keeps the subprocess alive across calls so initialization overhead is
    paid once per session, not once per tool call. Thread-safe: a lock
    serializes request/response pairs since MCP over stdio is inherently
    sequential (one outstanding request at a time).
    """

    def __init__(self, binary: str, cwd: str) -> None:
        self._binary = binary
        self._cwd = cwd
        self._proc: subprocess.Popen | None = None
        self._lock = threading.Lock()
        self._next_id = 1

    def _ensure_started(self) -> None:
        if self._proc is not None and self._proc.poll() is None:
            return
        env = {**os.environ, "PWD": self._cwd}
        self._proc = subprocess.Popen(
            [self._binary],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            cwd=self._cwd,
            env=env,
        )
        # MCP handshake: initialize → wait for response → send initialized.
        self._send(
            {
                "jsonrpc": "2.0",
                "id": self._next_id,
                "method": "initialize",
                "params": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {},
                    "clientInfo": {"name": "hermes-mastermind", "version": "1.0"},
                },
            }
        )
        self._next_id += 1
        self._read()  # discard initialize response
        self._send(
            {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}
        )

    def _send(self, msg: dict) -> None:
        data = (json.dumps(msg) + "\n").encode()
        self._proc.stdin.write(data)
        self._proc.stdin.flush()

    def _read(self) -> dict:
        line = self._proc.stdout.readline()
        if not line:
            raise RuntimeError("mastermind MCP server closed stdout unexpectedly")
        return json.loads(line.decode())

    def call_tool(self, name: str, arguments: dict) -> dict:
        """Call a mastermind MCP tool. Returns the parsed JSON result."""
        with self._lock:
            self._ensure_started()
            req_id = self._next_id
            self._next_id += 1
            self._send(
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "method": "tools/call",
                    "params": {"name": name, "arguments": arguments},
                }
            )
            resp = self._read()
            if "error" in resp:
                raise RuntimeError(f"{name} error: {resp['error']}")
            # MCP result shape: {"result": {"content": [{"type": "text", "text": "..."}]}}
            content = resp.get("result", {}).get("content", [])
            if content and content[0].get("type") == "text":
                try:
                    return json.loads(content[0]["text"])
                except json.JSONDecodeError:
                    return {"text": content[0]["text"]}
            return resp.get("result", {})

    def shutdown(self) -> None:
        if self._proc and self._proc.poll() is None:
            try:
                self._proc.stdin.close()
                self._proc.wait(timeout=3)
            except Exception:
                self._proc.kill()
        self._proc = None


try:
    from agent.memory_provider import MemoryProvider
except ImportError:
    # Allow importing outside of Hermes (e.g. for unit tests).
    class MemoryProvider:  # type: ignore[no-redef]
        pass


class MastermindProvider(MemoryProvider):
    """Hermes memory provider backed by mastermind."""

    @property
    def name(self) -> str:
        return "mastermind"

    def is_available(self) -> bool:
        """Check if the mastermind binary is on PATH. No network calls."""
        binary = os.environ.get("MASTERMIND_BIN", "mastermind")
        try:
            r = subprocess.run([binary, "version"], capture_output=True, timeout=3)
            return r.returncode == 0
        except (FileNotFoundError, subprocess.TimeoutExpired):
            return False

    def initialize(self, session_id: str, **kwargs) -> None:
        self._session_id = session_id
        self._hermes_home = str(kwargs.get("hermes_home", Path.home() / ".hermes"))
        self._binary = os.environ.get("MASTERMIND_BIN", "mastermind")
        self._cwd = os.getcwd()
        self._client = MastermindMCPClient(self._binary, self._cwd)

        # Load non-secret config written by save_config().
        cfg_path = Path(self._hermes_home) / "mastermind.json"
        self._config: dict = {}
        if cfg_path.exists():
            try:
                self._config = json.loads(cfg_path.read_text())
            except Exception:
                pass

    # ── Tool surface ────────────────────────────────────────────────────

    def get_tool_schemas(self) -> list[dict]:
        return [
            {
                "name": "mm_search",
                "description": (
                    "Search mastermind's persistent knowledge base across all scopes. "
                    "Call at the start of any non-trivial task and whenever the user "
                    "references prior work. Provide either 'query' (single string) or "
                    "'queries' (array) — not both."
                ),
                "input_schema": {
                    "type": "object",
                    "properties": {
                        "query": {
                            "type": "string",
                            "description": "Single query string",
                        },
                        "queries": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Multiple queries; each gets its own result block",
                        },
                        "scopes": {
                            "type": "array",
                            "items": {
                                "type": "string",
                                "enum": [
                                    "user-personal",
                                    "project-shared",
                                    "project-personal",
                                ],
                            },
                            "description": "Optional scope filter",
                        },
                        "kinds": {
                            "type": "array",
                            "items": {
                                "type": "string",
                                "enum": [
                                    "lesson",
                                    "insight",
                                    "war-story",
                                    "decision",
                                    "pattern",
                                    "open-loop",
                                ],
                            },
                            "description": "Optional kind filter",
                        },
                        "project": {
                            "type": "string",
                            "description": "Optional project name filter (case-insensitive)",
                        },
                        "tags": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "All listed tags must be present (AND semantics)",
                        },
                        "limit": {
                            "type": "integer",
                            "description": "Max results per query (default 10)",
                        },
                        "expand": {
                            "type": "boolean",
                            "description": "Return full body (L3) instead of trimmed excerpt (L2)",
                        },
                    },
                },
            },
            {
                "name": "mm_write",
                "description": (
                    "Write an entry directly to mastermind's live knowledge store. "
                    "Use for explicit in-session captures — the user is present and "
                    "this is their decision. Do NOT use for session summaries."
                ),
                "input_schema": {
                    "type": "object",
                    "required": ["topic", "body", "scope", "kind", "project", "category"],
                    "properties": {
                        "topic": {
                            "type": "string",
                            "description": "One-line human summary of the entry",
                        },
                        "body": {
                            "type": "string",
                            "description": "Markdown body (what/why/how/lesson sections recommended)",
                        },
                        "scope": {
                            "type": "string",
                            "enum": [
                                "user-personal",
                                "project-shared",
                                "project-personal",
                            ],
                        },
                        "kind": {
                            "type": "string",
                            "enum": [
                                "lesson",
                                "insight",
                                "war-story",
                                "decision",
                                "pattern",
                                "open-loop",
                            ],
                        },
                        "project": {
                            "type": "string",
                            "description": "Project name; use 'general' for cross-project entries",
                        },
                        "category": {
                            "type": "string",
                            "description": "Topic directory path (1-2 segments), e.g. 'go/modules', 'testing'",
                        },
                        "tags": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Free-form lowercase tags",
                        },
                        "date": {
                            "type": "string",
                            "description": "ISO 8601 date (YYYY-MM-DD); defaults to today UTC if omitted",
                        },
                        "confidence": {
                            "type": "string",
                            "enum": ["high", "medium", "low"],
                        },
                    },
                },
            },
            {
                "name": "mm_promote",
                "description": (
                    "Move a mastermind pending entry to the live store. "
                    "Only call when the user has explicitly reviewed and approved a candidate. "
                    "Do NOT auto-promote."
                ),
                "input_schema": {
                    "type": "object",
                    "required": ["pending_path"],
                    "properties": {
                        "pending_path": {
                            "type": "string",
                            "description": "Absolute path to the pending entry (from mm_search with include_pending: true)",
                        },
                    },
                },
            },
            {
                "name": "mm_close_loop",
                "description": (
                    "Mark a mastermind open-loop as resolved. "
                    "Call when the user signals they have finished something previously captured as in-progress."
                ),
                "input_schema": {
                    "type": "object",
                    "required": ["entry_path"],
                    "properties": {
                        "entry_path": {
                            "type": "string",
                            "description": "Absolute path to the open-loop entry (from mm_search results)",
                        },
                        "resolution": {
                            "type": "string",
                            "description": "Optional one-line note about how the loop was resolved",
                        },
                    },
                },
            },
        ]

    def handle_tool_call(self, name: str, args: dict) -> Any:
        _known = {"mm_search", "mm_write", "mm_promote", "mm_close_loop"}
        if name not in _known:
            raise ValueError(f"Unknown mastermind tool: {name!r}")
        try:
            result = self._client.call_tool(name, args)
            # mm_search: return the markdown string directly so Hermes injects it
            # as readable context rather than a raw JSON blob.
            if name == "mm_search":
                return result.get("markdown", "")
            return result
        except Exception as e:
            logger.warning("mastermind %s failed: %s", name, e)
            return {"error": str(e)}

    # ── Optional hooks ──────────────────────────────────────────────────

    def system_prompt_block(self) -> str:
        """Return mastermind's session-start block for injection into the system prompt."""
        try:
            r = subprocess.run(
                [self._binary, "session-start", "--cwd", self._cwd],
                capture_output=True,
                timeout=10,
                text=True,
            )
            return r.stdout.strip()
        except Exception as e:
            logger.warning("mastermind session-start failed: %s", e)
            return ""

    def prefetch(self, query: str) -> str:
        """Search mastermind for context relevant to the upcoming turn."""
        if not query or not query.strip():
            return ""
        try:
            result = self._client.call_tool("mm_search", {"query": query, "limit": 5})
            return result.get("markdown", "")
        except Exception as e:
            logger.warning("mastermind prefetch failed: %s", e)
            return ""

    def sync_turn(self, user_content: str, assistant_content: str) -> None:
        """Capture knowledge-worthy exchanges directly to the live store.

        In the Hermes context the agent IS the user, so the mastermind
        rule "user-initiated writes go live" applies here: captured entries
        bypass pending/ and land in the live store immediately — no review
        step needed because the agent's judgment replaces it.

        Disabled by default (set auto_capture: true in mastermind.json).
        When enabled, trivial turns are filtered out and only exchanges
        that contain recognisable knowledge signals are written.
        Must be non-blocking per the Hermes threading contract.
        """
        if not self._config.get("auto_capture", False):
            return

        def _sync() -> None:
            combined = (user_content or "") + (assistant_content or "")
            if len(combined) < 300:
                return
            if not self._has_knowledge_signal(assistant_content or ""):
                return
            topic = self._extract_topic(user_content, assistant_content)
            if not topic:
                return
            body = (
                "**User:**\n"
                + (user_content or "")[:600]
                + "\n\n**Assistant:**\n"
                + (assistant_content or "")[:1200]
            )
            try:
                self._client.call_tool(
                    "mm_write",
                    {
                        "topic": topic,
                        "body": body,
                        "scope": "project-personal",
                        "kind": "insight",
                        "project": self._detect_project(),
                        "category": "hermes/session",
                        "confidence": "medium",
                    },
                )
            except Exception as e:
                logger.warning("sync_turn write failed: %s", e)

        threading.Thread(target=_sync, daemon=True).start()

    _KNOWLEDGE_SIGNALS = (
        "the issue was", "the problem was", "i found that", "turns out",
        "the fix was", "the solution is", "we decided", "the reason is",
        "important to note", "worth remembering", "key insight",
        "lesson learned", "the mistake was", "don't forget",
        "fixed by", "the root cause", "the workaround", "the tradeoff",
        "the decision was", "we chose", "we rejected",
    )

    def _has_knowledge_signal(self, text: str) -> bool:
        """Return True if text contains a recognisable knowledge signal."""
        lower = text.lower()
        return any(s in lower for s in self._KNOWLEDGE_SIGNALS)

    def _extract_topic(self, user: str, assistant: str) -> str:
        """Return a one-line topic for the turn, or empty string if none found."""
        for line in (user or "").splitlines():
            line = line.strip()
            if len(line) > 20:
                return line[:80]
        for line in (assistant or "").splitlines():
            line = line.strip()
            if len(line) > 20:
                return line[:80]
        return ""

    def _detect_project(self) -> str:
        """Detect project name from cwd via git remote, fallback to dirname."""
        try:
            r = subprocess.run(
                ["git", "remote", "get-url", "origin"],
                capture_output=True,
                text=True,
                cwd=self._cwd,
                timeout=3,
            )
            if r.returncode == 0:
                remote = r.stdout.strip().rstrip("/")
                name = remote.split("/")[-1].replace(".git", "")
                if name:
                    return name
        except Exception:
            pass
        return os.path.basename(self._cwd) or "general"

    def on_memory_write(self, action: str, target: str, content: str) -> None:
        """Mirror built-in MEMORY.md/USER.md writes to mastermind.

        Disabled by default (set mirror_memory_writes: true in
        ~/.hermes/mastermind.json to enable). When enabled, writes go
        directly to the live store — the agent acting as the user is
        sufficient review for this path.
        """
        if not self._config.get("mirror_memory_writes", False):
            return
        try:
            self._client.call_tool(
                "mm_write",
                {
                    "topic": f"[hermes] {target}: {content[:60]}",
                    "body": content,
                    "scope": "project-personal",
                    "kind": "insight",
                    "project": "general",
                    "category": "hermes/memory",
                },
            )
        except Exception as e:
            logger.warning("mastermind mirror write failed: %s", e)

    def on_pre_compress(self, messages: list) -> None:
        """Extract knowledge from the conversation before context compression."""
        self._run_extract(messages, source="pre-compress")

    def on_session_end(self, messages: list) -> None:
        """Extract knowledge at session close."""
        self._run_extract(messages, source="session-end")

    def _run_extract(self, messages: list, source: str = "") -> None:
        """Serialize messages to a temp transcript and run mastermind extract async."""
        if not messages:
            return
        try:
            lines: list[str] = []
            for msg in messages:
                role = msg.get("role", "unknown") if isinstance(msg, dict) else "unknown"
                content = (msg.get("content", "") if isinstance(msg, dict) else str(msg))
                if isinstance(content, list):
                    text_parts = [
                        p.get("text", "")
                        for p in content
                        if isinstance(p, dict) and p.get("type") == "text"
                    ]
                    content = "\n".join(text_parts)
                lines.append(f"[{role}] {content}")

            transcript = "\n\n".join(lines)
            with tempfile.NamedTemporaryFile(
                mode="w",
                suffix=".txt",
                prefix=f"mastermind-{source}-",
                delete=False,
            ) as f:
                f.write(transcript)
                tmp_path = f.name

            def _worker() -> None:
                try:
                    subprocess.run(
                        [
                            self._binary,
                            "extract",
                            "--transcript",
                            tmp_path,
                            "--cwd",
                            self._cwd,
                        ],
                        capture_output=True,
                        timeout=30,
                    )
                finally:
                    try:
                        os.unlink(tmp_path)
                    except OSError:
                        pass

            threading.Thread(target=_worker, daemon=True).start()
        except Exception as e:
            logger.warning("mastermind extract (%s) failed: %s", source, e)

    # ── Config ──────────────────────────────────────────────────────────

    def get_config_schema(self) -> list[dict]:
        return [
            {
                "key": "binary_path",
                "description": "Path to mastermind binary (leave blank to use PATH lookup)",
                "required": False,
                "env_var": "MASTERMIND_BIN",
            },
            {
                "key": "auto_capture",
                "description": (
                    "Capture knowledge-worthy turns directly to the live store after each response. "
                    "The agent's judgment replaces the review step (agent = user in Hermes). "
                    "Default: false."
                ),
                "required": False,
                "default": False,
            },
        ]

    def save_config(self, values: dict, hermes_home: str) -> None:
        cfg_path = Path(hermes_home) / "mastermind.json"
        existing: dict = {}
        if cfg_path.exists():
            try:
                existing = json.loads(cfg_path.read_text())
            except Exception:
                pass
        existing.update({k: v for k, v in values.items() if v})
        cfg_path.write_text(json.dumps(existing, indent=2))

    def shutdown(self) -> None:
        self._client.shutdown()


def register(ctx) -> None:
    ctx.register_memory_provider(MastermindProvider())
