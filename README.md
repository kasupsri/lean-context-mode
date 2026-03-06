# Lean Context Mode

Lean Context Mode is a Go MCP server that compresses repository context for AI coding agents.
It is designed for high token reduction with predictable, traceable outputs.

## Why This Is Useful

- Reduces context payload size before it reaches the model.
- Keeps long coding sessions stable by minimizing token overhead.
- Returns compact, source-traceable context bundles instead of large raw dumps.
- Works across mixed-language repositories.
- Runs as a local stdio MCP server with no external LLM dependency in the compression pipeline.

## Core Pipeline

Lean Context Mode uses a deterministic 3-stage pipeline:

1. Context Budgeter
2. Retriever
3. Summarizer

The server prioritizes relevant symbols/snippets/diffs, then aggressively compacts output to keep token usage low.

## MCP Tool Surface

- `context.pack`
- `code.symbols`
- `code.snippet`
- `repo.map`
- `changes.focus`
- `cache.clean`
- `workspace.root.get`
- `workspace.root.set`

## Performance and Resource Usage

### Synthetic CPU and Memory Profile
Source: `benchmarks/results.json`  
Generated: `2026-03-06T02:56:14Z`  
Environment: `windows/amd64`, CPU `12th Gen Intel(R) Core(TM) i7-1260P`

- `BenchmarkContextPack-16`
  - iterations: `25`
  - CPU time: `48.167 ms/op`
  - memory: `695,811 B/op`
  - allocations: `2,667 allocs/op`
- `BenchmarkCodeSymbols-16`
  - iterations: `7,567`
  - CPU time: `0.184 ms/op`
  - memory: `149,298 B/op`
  - allocations: `871 allocs/op`

### End-to-End Workspace Savings
Source: `benchmarks/workspace-results.json`  
Generated: `2026-03-06T02:48:48Z`

For `lean-context-mode` repo workload (`20` requests):

- cold start: `17 ms`
- average request latency: `145.55 ms`
- original tokens: `173,096`
- optimized tokens: `5,268`
- tokens saved: `167,828`
- reduction: `96.96%`

This is why the tool is useful in practice: it cuts context volume by ~97% on this benchmark while keeping outputs grounded in repo sources.

## Security Model

- Strict workspace-root path validation.
- Tool input sanitization.
- No shell execution MCP tool exposed.
- File access blocked outside configured workspace roots.

## Cache Behavior

- Cache is memory-only (no cache file reads/writes).
- Default mode is `ephemeral`: cache cleared after each request.
- Optional bounded mode:
  - `LCM_CACHE_MODE=bounded`
  - optional `LCM_CACHE_MAX_AGE_HOURS` (default `168`)

Use `cache.clean` MCP tool when you need explicit cleanup:

- `mode: "expired"` (default)
- `mode: "all"`
- optional `max_age_hours`

## Observability

Metrics are persisted under:

- `.lean-context-mode/metrics.json`

Tracked metrics include:

- requests processed
- latency
- cache hits/misses
- bytes read
- tokens returned/saved

Stats command:

```bash
lean-context-mode stats --root /path/to/repo
```

## Run Locally (Windows)

### Build

PowerShell:

```powershell
cd "C:\path\to\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

cmd.exe:

```bat
cd /d "C:\path\to\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

### Run server

```powershell
.\lean-context-mode.exe serve --root "C:\path\to\your-workspace"
```

### Stats

```powershell
.\lean-context-mode.exe stats --root "C:\path\to\your-workspace"
```

## Run Locally (macOS/Linux)

### Build

```bash
cd /path/to/lean-context-mode
go build -o lean-context-mode ./cmd/lean-context-mode
```

### Run server

```bash
./lean-context-mode serve --root /path/to/your-workspace
```

### Stats

```bash
./lean-context-mode stats --root /path/to/your-workspace
```

## Environment Variables

- `LCM_ROOT`: default root if `--root` is omitted
- `LEAN_CONTEXT_MODE_ROOT`: fallback root env var
- `LCM_ALLOWED_ROOTS`: allowed roots list (comma/semicolon separated)
- `LCM_CACHE_MODE`: `ephemeral` (default) or `bounded`
- `LCM_CACHE_MAX_AGE_HOURS`: max age for bounded mode

## MCP Config Examples

- Cursor example: `examples/cursor.mcp.json`
- Codex example: `examples/codex.config.toml`

For macOS/Linux TOML config:

```toml
[mcp_servers.lean-context-mode]
command = "/path/to/lean-context-mode/lean-context-mode"
args = ["serve", "--root", "/path/to/your-workspace"]
```

## Test and Benchmark

Run tests:

```bash
go test ./...
```

Run synthetic benchmark:

```bash
go test -run=DO_NOT_MATCH -bench Benchmark -benchmem ./internal/lean
```

Run workspace benchmark report:

```bash
LCM_RUN_BENCH_REPORT=1 \
LCM_BENCH_REPOS="/path/repo-a,/path/repo-b" \
go test -run TestBenchmarkWorkspaceRepos ./internal/lean -v
```

## Project Structure

```text
lean-context-mode/
  cmd/lean-context-mode/main.go
  internal/lean/
    budgeter.go
    cache.go
    gitdiff.go
    indexer.go
    mcp.go
    metrics.go
    pipeline.go
    retriever.go
    root_manager.go
    security.go
    service.go
    summarizer.go
    token.go
    types.go
  benchmarks/
    results.json
    workspace-results.json
  examples/
    codex.config.toml
    cursor.mcp.json
  scripts/
    install-vscode-mcp.bat
    set-allowed-roots.bat
    set-root.bat
    serve.bat
    stats.bat
```
