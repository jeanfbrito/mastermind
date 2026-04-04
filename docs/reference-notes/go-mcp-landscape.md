# Go MCP Landscape — Reference Notes

Research date: 2026-04-04. All data pulled live from GitHub REST API and raw source files on `main` branches. Every claim is cited with a URL.

---

## Part 1 — Go MCP SDKs

Three serious candidates exist. A fourth tier (tooling wrappers like `mcp-sdk-go` forks) is not worth evaluating.

### 1. `github.com/modelcontextprotocol/go-sdk` — **OFFICIAL**

- **Repo**: https://github.com/modelcontextprotocol/go-sdk
- **Maintainer**: Official Anthropic / Model Context Protocol org, maintained **in collaboration with Google** (per repo description). This is the canonical SDK.
- **Stars**: 4,297 — **Forks**: 393 — **Open issues**: 42
- **Activity**: Last push 2026-04-03, **88 commits since 2026-01-01**. Extremely active.
- **Releases**: Stable tagged releases. Latest stable `v1.4.1` (2026-03-13). `v1.5.0-pre.1` out 2026-03-31. 21 releases total. Supports MCP spec versions 2024-11-05 through 2025-11-25. Past v1.0 — API stability commitment.
- **Transports**: `StdioTransport` (primary — what Claude Code uses), plus streamable HTTP, SSE, and a `CommandTransport` for clients spawning subprocesses. Full transport surface.
- **Go version**: `go 1.25.0` (aggressive — requires current Go).
- **License**: NOASSERTION (repo license file is Apache-2.0-style but GitHub doesn't detect; Google-maintained projects are Apache-2.0 in practice).
- **Dependencies** (from `go.mod`): minimal and high-quality —
  - `github.com/google/jsonschema-go` (Google's schema lib)
  - `github.com/golang-jwt/jwt/v5`, `golang.org/x/oauth2` (for auth)
  - `github.com/segmentio/encoding` (fast JSON)
  - `github.com/yosida95/uritemplate/v3`
  - **No web framework**, no gin, no echo. 2 indirect deps.
- **Tool registration API** (from README):
  ```go
  type Input struct {
      Name string `json:"name" jsonschema:"the name of the person to greet"`
  }
  type Output struct {
      Greeting string `json:"greeting" jsonschema:"the greeting"`
  }

  func SayHi(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Output, error) {
      return nil, Output{Greeting: "Hi " + input.Name}, nil
  }

  func main() {
      server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)
      mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)
      if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
          log.Fatal(err)
      }
  }
  ```
- **Argument validation**: Generic `mcp.AddTool[In, Out]` uses Go generics. Input/output are plain structs with `json:` and `jsonschema:` struct tags. The SDK generates JSON Schema via `google/jsonschema-go` and validates incoming arguments before calling your handler. Type-safe at compile time — no `map[string]any` fishing.
- **Known users**:
  - `github/github-mcp-server` (28,552 stars) — pinned to a v1.3.1 pseudo-version (https://github.com/github/github-mcp-server/blob/main/go.mod)
  - `containers/kubernetes-mcp-server` (1,382 stars) — on `v1.4.1` (https://github.com/containers/kubernetes-mcp-server)
- **Verdict**: Yes — this is the SDK you stake a long-lived personal tool on. Official, Google-backed, past v1.0, generic type-safe API, minimal deps, used by GitHub's own MCP server.

### 2. `github.com/mark3labs/mcp-go` — **COMMUNITY LEADER**

- **Repo**: https://github.com/mark3labs/mcp-go
- **Maintainer**: Community (Ed Zynda / mark3labs). Pre-dated the official SDK; has the largest community footprint.
- **Stars**: 8,536 (highest in the Go MCP ecosystem) — **Forks**: 806 — **Open issues**: 24
- **Activity**: Last push 2026-04-04 (today), **56 commits since 2026-01-01**. Very active.
- **Releases**: Frequent. Latest `v0.47.0` (2026-04-04). **Pre-1.0** — no API stability promise. That is the one structural concern.
- **Transports**: Stdio (`server.ServeStdio`), HTTP, SSE, in-process. Full set.
- **Go version**: `go 1.23.0` (more forgiving than the official SDK's 1.25).
- **License**: MIT.
- **Dependencies** (from `go.mod`): lean —
  - `github.com/google/jsonschema-go v0.4.2` (same as official SDK)
  - `github.com/google/uuid`, `github.com/spf13/cast`, `github.com/yosida95/uritemplate/v3`
  - Test-only: `stretchr/testify`. **No web framework.** Three indirect deps.
- **Tool registration API** (from README):
  ```go
  s := server.NewMCPServer("Demo", "1.0.0", server.WithToolCapabilities(false))

  tool := mcp.NewTool("hello_world",
      mcp.WithDescription("Say hello to someone"),
      mcp.WithString("name", mcp.Required(), mcp.Description("Name of the person")),
  )
  s.AddTool(tool, helloHandler)
  server.ServeStdio(s)

  func helloHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
      name, err := req.RequireString("name")
      if err != nil { return mcp.NewToolResultError(err.Error()), nil }
      return mcp.NewToolResultText("Hello, " + name), nil
  }
  ```
- **Argument validation**: Two paths. Builder API (shown above): `mcp.WithString`, `mcp.WithNumber`, etc. — schema built imperatively, args fetched via `req.RequireString("name")` at runtime. There is also a generic typed path (`mcp.NewTypedToolHandler`) for struct-based input similar to the official SDK. Recent releases added middleware support via `Use()` (PR #767, 2026-04-04).
- **Known users** (real, shipping): **grafana/mcp-grafana** (2,715 stars, on `v0.45.0`, https://github.com/grafana/mcp-grafana), **hashicorp/terraform-mcp-server** (1,302 stars, on `v0.46.0`, https://github.com/hashicorp/terraform-mcp-server), **isaacphi/mcp-language-server** (1,505 stars, on `v0.25.0`, https://github.com/isaacphi/mcp-language-server), **Gentleman-Programming/engram** (on `v0.44.0`, https://github.com/Gentleman-Programming/engram). mark3labs has more high-profile users in the wild than the official SDK.
- **Verdict**: Yes, but with reservation. Pre-1.0 means breaking changes can land in a `v0.xx` bump. For a personal tool where you can pin and upgrade on your schedule, the risk is acceptable. More real-world battle-testing than the official SDK right now.

### 3. `github.com/metoro-io/mcp-golang` — **DEPRIORITIZED / STALLING**

- **Repo**: https://github.com/metoro-io/mcp-golang
- **Maintainer**: Metoro (a company that appears to have deprioritized it).
- **Stars**: 1,210 — **Forks**: 120 — **Open issues**: 44
- **Activity**: Last commit **2025-09-02**, last push 2026-02-25 (likely a tag re-publish). **0 commits in 2026.** 44 open issues. This is effectively unmaintained.
- **Releases**: Latest `v0.16.1` (2026-02-25, hotfix). Prior stable `v0.16.0` was 2025-08-21. Pre-1.0 and stalled.
- **Transports**: Stdio, HTTP.
- **Go version**: `go 1.21` (ancient by 2026 standards).
- **License**: MIT.
- **Dependencies** (from `go.mod`): **bloated** — drags in the entire `gin-gonic/gin` web framework and its validator chain (`go-playground/validator`, `ugorji/go/codec`, `goccy/go-json`, `wk8/go-ordered-map`, `tidwall/sjson`, `tidwall/gjson`, `invopop/jsonschema`, and ~30 indirect deps). This alone is disqualifying for a minimal-dependency tool.
- **Tool registration API** (from README): struct-tag-based, similar to the official SDK, `server.RegisterTool("hello", "desc", handlerFn)`. The ergonomics are actually fine — but the maintenance status and transitive dep footprint kill it.
- **Verdict**: No. Unmaintained since fall 2025, pre-1.0, drags in gin + 30 indirect deps. Do not use for a long-lived tool.

---

## Part 2 — Real Go MCP Servers in the Wild

Five solid references, ranked by relevance to mastermind (single-binary CLI, small tool surface, stdio, knowledge domain).

### 1. **Gentleman-Programming/engram** — ⭐ NEAR-PERFECT MATCH

- **URL**: https://github.com/Gentleman-Programming/engram
- **What it does**: "Persistent memory system for AI coding agents. Agent-agnostic Go binary with SQLite + FTS5, MCP server, HTTP API, CLI, and TUI." This is basically mastermind's cousin.
- **Last commit**: 2026-03-30. Active.
- **SDK**: `github.com/mark3labs/mcp-go v0.44.0`
- **Extra deps worth noting**: `modernc.org/sqlite` (pure-Go SQLite, CGO_ENABLED=0), `charmbracelet/bubbletea` + `bubbles` + `lipgloss` (TUI stack), `a-h/templ` (web UI).
- **Layout**:
  ```
  cmd/engram/main.go
  internal/mcp/mcp.go        <-- tool registration lives here, single file
  internal/store/             <-- SQLite + FTS5 storage
  internal/server/            <-- HTTP API
  internal/tui/               <-- bubbletea TUI
  internal/project/ setup/ sync/ version/
  ```
  32 Go files total. Clean `cmd/` + `internal/` split, no `pkg/`. Size 18 MB (includes `assets/`, `docs/`, `plugin/`, `skills/` — Go source is a small fraction).
- **Distribution**: `.goreleaser.yaml` at repo root. `CGO_ENABLED=0`, `goos: [linux, darwin, windows]`, `goarch: [amd64, arm64]`, tar.gz archives, zip for Windows, checksums, ldflags inject version. **Homebrew tap** publish to `Gentleman-Programming/homebrew-tap`. Latest release `v1.11.0` (2026-03-30) with assets for darwin/linux/windows × amd64/arm64.
- **Commit worth reading**: `perf(mcp): defer 4 rare tools to reduce session startup tokens` (2026-03-26) — they've already thought about the mastermind-adjacent problem of tool-surface budget for MCP clients.
- **LoC estimate**: ~32 Go files → very roughly 2-4k LoC excluding vendored generated code. Comparable in scale to mastermind's target 2-3k.
- **Why this is the reference**: Same domain (memory/knowledge tool for AI agents), same storage (SQLite + FTS5), same transport (MCP stdio), same distribution story (single-binary goreleaser + Homebrew), same SDK choice you're evaluating. You can copy the `cmd/` → `internal/mcp/` → `internal/store/` skeleton almost verbatim.

### 2. **github/github-mcp-server** — credibility anchor

- **URL**: https://github.com/github/github-mcp-server
- **What it does**: GitHub's official MCP server. 28,552 stars — largest real Go MCP deployment.
- **Last commit**: 2026-04-02.
- **SDK**: `github.com/modelcontextprotocol/go-sdk v1.3.1-0.20260220105450-b17143f71798` (pseudo-version — tracks main ahead of stable releases).
- **Layout**: `cmd/` + `internal/` + `pkg/` + `e2e/` + `docs/` + `third-party/`. Larger, 52 MB repo — not a small-tool reference, but proves the SDK works at scale.
- **Distribution**: `.goreleaser.yaml`. Latest release `v0.32.0` (2026-03-06) with full asset matrix: Darwin arm64/x86_64, Linux arm64/i386/x86_64, Windows arm64/i386/x86_64 (tar.gz + zip).
- **Relevance**: Use as **proof** that the official SDK is production-ready. Don't copy its layout — it's too big for mastermind.

### 3. **grafana/mcp-grafana** — mid-size mark3labs reference

- **URL**: https://github.com/grafana/mcp-grafana
- **Last commit**: 2026-04-03. Very active.
- **SDK**: `github.com/mark3labs/mcp-go v0.45.0`
- **Layout**: Mixed — some files at the repo root (`mcpgrafana.go`, `tools.go`, `session.go`, `client_cache.go`), `cmd/`, `internal/`, `tools/`. Not clean for a small tool. 2.4 MB.
- **Distribution**: `.goreleaser.yaml`. Latest release has **both** goreleaser-standard assets (`mcp-grafana_Darwin_arm64.tar.gz` …) **and** custom-named assets (`darwin.arm64.grafana.tar.gz` …) for Grafana plugin distribution. Cross-platform full matrix.
- **Relevance**: Second-best mark3labs shipping reference. Use if you want to see how a larger team uses the SDK with middleware, session management, and proxied clients.

### 4. **hashicorp/terraform-mcp-server** — tiny, tidy mark3labs reference

- **URL**: https://github.com/hashicorp/terraform-mcp-server
- **Last commit**: 2026-04-03. 1,302 stars.
- **SDK**: `github.com/mark3labs/mcp-go v0.46.0` (current)
- **Layout**: `cmd/` + `pkg/` + `e2e/` + `instructions/`. 951 KB — smallest real reference. No `internal/` directory, uses `pkg/` for public-ish packages.
- **Distribution**: No `.goreleaser.yaml` at repo root (uses a `.release/` directory — HashiCorp's internal release tooling). Latest `v0.5.0` (2026-04-01) but release assets empty in API (HashiCorp publishes through their own channels). **Not a good distribution reference.**
- **Relevance**: Compact mark3labs layout example, but poor for copying the release story.

### 5. **containers/kubernetes-mcp-server** — official-SDK reference

- **URL**: https://github.com/containers/kubernetes-mcp-server
- **Last commit**: 2026-04-02. 1,382 stars.
- **SDK**: `github.com/modelcontextprotocol/go-sdk v1.4.1` (current stable — **not** a pseudo-version, which is cleaner than github-mcp-server's setup).
- **Layout**: `cmd/` + `internal/` + `pkg/` + `build/` + `charts/` + `docs/` + `evals/` + `hack/` + `manifest.json` + `npm/` + `python/` + `smithery.yaml`. Larger than mastermind needs, but clean.
- **Distribution**: No goreleaser. Releases are hand-built via Makefile. Latest `v0.0.60` (2026-04-01) publishes individual binaries (`kubernetes-mcp-server-darwin-amd64`, etc.) **and** `.mcpb` (MCP bundle) files for each platform. Full matrix: darwin/linux/windows × amd64/arm64.
- **Relevance**: Second-best reference for **official SDK** usage (after github-mcp-server, which is too big). Shows v1.4.1 stable pinning.

---

## Part 3 — Go CLI Shape (skipped)

Not needed. Engram is a perfect apples-to-apples reference: same domain, same storage, same distribution model. Filing a one-line note: if mastermind ever wants a TUI on top of the CLI, engram is already using `charmbracelet/bubbletea` and can be copied for that too.

---

## Part 4 — Recommendations

### 4.1 Which Go MCP SDK to depend on?

**Pick `github.com/modelcontextprotocol/go-sdk` (v1.4.x).**

Rationale: It's the official SDK, past v1.0 with stable semver, maintained in collaboration with Google, minimal dep footprint (no web framework), uses Go generics for compile-time-typed `mcp.AddTool[In, Out]`, and is already pinned at stable `v1.4.1` by a real shipping server (containers/kubernetes-mcp-server). For a tool you want to maintain for years, API stability > community size. The pseudo-version pin in github-mcp-server is also a mild signal that the SDK is trusted enough that GitHub tracks its main branch.

**Runner-up: `github.com/mark3labs/mcp-go`.** It has more stars (8.5k vs 4.3k), more shipping users in the wild (grafana, terraform, engram, mcp-language-server), and a slightly more forgiving Go version requirement (1.23 vs 1.25). It loses because it's pre-1.0 — `v0.46` → `v0.47` can and does introduce breaking changes, and for a long-lived personal tool, recurring breakage is a tax you don't need. If the official SDK didn't exist or was stalled, mark3labs would be the pick.

**Do not use metoro-io/mcp-golang.** Unmaintained since Sept 2025, zero 2026 commits, drags in gin + ~30 indirect deps.

### 4.2 Which Go repo to use as a direct implementation reference?

**Pick `Gentleman-Programming/engram`** — https://github.com/Gentleman-Programming/engram.

Rationale: It is mastermind's structural twin. Pure-Go SQLite (`modernc.org/sqlite`, CGO_ENABLED=0) + FTS5 + MCP stdio + single-binary CLI + goreleaser + Homebrew tap + `cmd/<name>` + `internal/mcp/`, `internal/store/`, `internal/server/` layout. Its `internal/mcp/mcp.go` is a one-file tool-registration module, which matches mastermind's Phase 0 "3-5 tools" scope exactly. It even solves problems you'll hit soon (e.g. deferring rare tools to keep the MCP client startup token budget down — commit 2026-03-26). Read engram first, translate rtk's algorithms second.

**Caveat**: engram uses mark3labs/mcp-go, not the official SDK you'll be depending on. So treat engram as the reference for **project layout, storage module, distribution, and CLI shape** — not for the literal MCP tool-registration call sites. For the registration API, use the official SDK's README example and the `examples/` directory in `modelcontextprotocol/go-sdk`. This is a minor translation (both SDKs use struct-tag JSON Schema input types — the move is mechanical).

### 4.3 Red flags about the Go MCP ecosystem?

**None that should make you reconsider Rust.** The ecosystem is healthier than expected:

- Two actively-maintained SDKs with weekly commits.
- An official SDK past v1.0 with a Google co-maintainer.
- Real, production servers shipping from GitHub, Grafana, HashiCorp, Kubernetes/OpenShift, and indie devs.
- Both active SDKs have minimal dep footprints (no web framework creep).
- Engram proves the exact (Go + SQLite+FTS5 + MCP + single binary + goreleaser + Homebrew) combination you want is already battle-tested.

**Minor concerns worth naming:**
1. **API churn risk on mark3labs**: 47 minor releases in ~18 months. If you had chosen mark3labs, budget ~1-2 hours per year for breaking-change upgrades.
2. **Official SDK is on Go 1.25**: If your deployment target ships older Go, you're pinned to a recent toolchain. Not a problem for a personal tool you compile yourself.
3. **Ecosystem fragmentation**: The existence of both a thriving community SDK (mark3labs, 8.5k stars) and a thriving official SDK (4.3k stars) means you'll find tutorials/blog posts that disagree on the API. Stick to the official SDK's own `examples/` directory and ignore older blog posts.

Bottom line: Go is a credible target for mastermind. The ergonomics of `mcp.AddTool[Input, Output]` with struct-tag JSON Schema are genuinely nice — arguably nicer than hand-rolled JSON-RPC in Rust. You lose rtk's Rust patterns as a direct copy, but you gain engram as a domain-twin reference and a mature SDK. Proceed with Go.

---

## Sources

All URLs verified 2026-04-04 via GitHub REST API. Version pins, commit dates, stars, and release assets retrieved programmatically — not eyeballed from README badges.

**SDKs**
- https://github.com/modelcontextprotocol/go-sdk (v1.4.1, go.mod, README)
- https://github.com/mark3labs/mcp-go (v0.47.0, go.mod, README)
- https://github.com/metoro-io/mcp-golang (v0.16.1, go.mod, README)

**Shipping servers**
- https://github.com/Gentleman-Programming/engram (goreleaser, cmd/, internal/mcp/mcp.go)
- https://github.com/github/github-mcp-server (go.mod pin, release v0.32.0)
- https://github.com/grafana/mcp-grafana (go.mod pin v0.45.0)
- https://github.com/hashicorp/terraform-mcp-server (go.mod pin v0.46.0)
- https://github.com/isaacphi/mcp-language-server (go.mod pin v0.25.0)
- https://github.com/containers/kubernetes-mcp-server (go.mod pin v1.4.1, release v0.0.60)
