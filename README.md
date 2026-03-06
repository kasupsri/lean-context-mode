# Lean Context Mode

Lean Context Mode is a universal **Go MCP server** optimized for token-efficient repository context retrieval for AI coding agents.

It implements a deterministic 3-stage pipeline:

1. **Context Budgeter**
2. **Retriever**
3. **Summarizer**

Primary goals:
- minimal tokens
- minimal latency
- minimal memory usage
- high cache reuse
- language-agnostic repository support

## MCP Tool Surface

- `context.pack`
- `code.symbols`
- `code.snippet`
- `repo.map`
- `changes.focus`
- `cache.clean`
- `workspace.root.get`
- `workspace.root.set`

## Architecture

### 1) Context Budgeter
Input:
- `query`
- `file_hints`
- `language`
- `token_budget`

Deterministic allocation buckets:
- symbol signatures
- code snippets
- dependency edges
- git diffs
- configuration slices

### 2) Retriever
Optimizations:
- symbol-first ranking
- changed-file priority from git status/diff
- windowed snippet extraction
- dependency edge selection
- config slice selection
- snippet dedup and cache pointers

### 3) Summarizer
- runs only when bundle exceeds budget
- extractive and traceable (`file:line-range`)
- cached with keys built from file hash + dependency hash + configuration hash + query

## Repository Indexing

- startup full index
- incremental updates via fsnotify watcher
- language-agnostic regex heuristics for symbols/imports/calls
- relationship maps for imports/dependents/tests
- lightweight ecosystem signals
  - .NET: solution/DI/EF patterns
  - React/TS: hooks/routing/API client patterns

## Security

- strict workspace-root path validation
- tool input sanitization
- no exposed shell execution tool
- file access is blocked outside workspace root

## Observability and Metrics

Persisted under:
- `.lean-context-mode/cache/cache.json`
- `.lean-context-mode/metrics.json`

Tracked:
- requests processed
- latency
- cache hits/misses
- bytes read
- tokens returned/saved

Cache lifecycle:
- default mode is `ephemeral`: cache is cleared after each request to stay lightweight
- optional bounded mode:
  - set `LCM_CACHE_MODE=bounded`
  - entries are then age-pruned on startup and periodically during writes
  - default max age in bounded mode: `168` hours (7 days)
  - override with `LCM_CACHE_MAX_AGE_HOURS` (for example `72`), or set `0` to disable age pruning

MCP cache cleanup tool:
- call `cache.clean` with:
  - `mode: "expired"` (default) to delete stale entries
  - `mode: "all"` to clear the cache fully
  - optional `max_age_hours` for `expired` mode

Stats command:

```bash
lean-context-mode stats --root /path/to/repo
```

Outputs:
- daily token savings
- total tokens saved
- average reduction percentage

## Run Locally (Windows Native)

### Prerequisites
- Go installed and available in `PATH` (`go version`)
- Git available in `PATH` (for `changes.focus`)

### Build (PowerShell)
```powershell
cd "C:\path\to\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

### Build (cmd.exe)
```bat
cd /d "C:\path\to\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

### Local sanity check
```powershell
.\lean-context-mode.exe stats --root "C:\path\to\lean-context-mode"
```

### Run MCP server (stdio)
```powershell
.\lean-context-mode.exe serve --root "C:\path\to\your-workspace"
```

`serve` is a stdio MCP server and is intended to be started by Cursor/Codex/Claude MCP.  
If stdin closes, the process exits.

### Run without building
```powershell
go run .\cmd\lean-context-mode serve --root "C:\path\to\your-workspace"
```

### Use environment variable for root
If `--root` is omitted, the CLI uses:
1. `LCM_ROOT`
2. `LEAN_CONTEXT_MODE_ROOT`
3. current directory (`.`)

`LCM_ALLOWED_ROOTS` (optional) constrains dynamic root switching for MCP tools.  
Format: semicolon/comma separated absolute paths.

PowerShell:
```powershell
$env:LCM_ROOT = "C:\path\to\your-workspace"
.\lean-context-mode.exe serve
.\lean-context-mode.exe stats
```

cmd.exe:
```bat
set LCM_ROOT=C:\path\to\your-workspace
lean-context-mode.exe serve
lean-context-mode.exe stats
```

### BAT helpers (Windows)
Included scripts:
- `scripts\set-root.bat` (persist `LCM_ROOT` with `setx`)
- `scripts\set-allowed-roots.bat` (persist `LCM_ALLOWED_ROOTS` with `setx`)
- `scripts\serve.bat` (run server; uses `LCM_ROOT` or current directory)
- `scripts\stats.bat` (show stats; uses `LCM_ROOT` or current directory)

Examples:
```bat
cd /d "C:\path\to\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode

scripts\set-root.bat "C:\path\to\workspace-a"
scripts\set-allowed-roots.bat "C:\path\to\workspace-a;C:\path\to\workspace-b"
scripts\stats.bat
scripts\serve.bat
```

### Dynamic Workspace Path (for AI/tool calls)
All major tools accept optional `workspace_root`:
- `context.pack.workspace_root`
- `code.symbols.workspace_root`
- `code.snippet.workspace_root`
- `repo.map.workspace_root`
- `changes.focus.workspace_root`

You can also switch the server’s active workspace root:
- `workspace.root.get` returns active + allowed roots
- `workspace.root.set` updates active root (must be within allowed roots)

## MCP Client Config Examples

- Cursor example: `examples/cursor.mcp.json`
- Codex example: `examples/codex.config.toml`

Both examples call `scripts\serve.bat` via `cmd /c`.

### Install into Cursor + Codex (Windows)

Use the helper script to install MCP config and set workspace root in one step:

```bat
cd /d "C:\path\to\lean-context-mode"
scripts\install-vscode-mcp.bat "C:\path\to\your-workspace"
```

What this script does:
- writes `%USERPROFILE%\.cursor\mcp.json` with `lean-context-mode`
- appends MCP server entry to `%USERPROFILE%\.codex\config.toml` if missing
- sets `LCM_ROOT` (when you pass a workspace path)

After running it, restart Cursor / VS Code.

## Tests

Automated tests cover:
- token budgeting determinism
- retrieval/context pack behavior
- cache behavior
- MCP tool interface surface

Run tests (native Go):

```bash
go test ./...
```

Run tests in Docker (fallback if native test execution has local permission/toolchain issues):

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.25 go test ./...
```

Run synthetic benchmarks (always available):

```bash
go test -run=DO_NOT_MATCH -bench Benchmark -benchmem ./internal/lean
```

## GitHub Pipelines

- `CI` (`.github/workflows/ci.yml`)
  - gofmt check, `go vet`, `go test`, `go build`
  - runs on Ubuntu + Windows
- `Benchmark` (`.github/workflows/benchmark.yml`)
  - synthetic benchmark on every manual trigger/nightly
  - uploads benchmark artifacts
- `Release` (`.github/workflows/release.yml`)
  - on `v*` tags
  - builds Windows/Linux/macOS binaries
  - publishes GitHub release assets + checksums

## Benchmark Results

Lean Context Mode synthetic benchmark snapshot (`benchmarks/results.json`):

- `BenchmarkContextPack-16`
  - iterations: `3`
  - latency: `345.994 ms/op`
  - memory: `1,556,522 B/op`
  - allocations: `7,041 allocs/op`
- `BenchmarkCodeSymbols-16`
  - iterations: `9,165`
  - latency: `0.115 ms/op`
  - memory: `149,298 B/op`
  - allocations: `871 allocs/op`

Reproduce benchmark (native):

```bash
go test -run=DO_NOT_MATCH -bench Benchmark -benchmem ./internal/lean
```

Reproduce benchmark (Docker):

```bash
docker run --rm \
  -v "$PWD":/src \
  -w /src \
  golang:1.25 \
  go test -run=DO_NOT_MATCH -bench Benchmark -benchmem ./internal/lean
```

## Project Structure

```text
lean-context-mode/
  .github/workflows/
    ci.yml
    benchmark.yml
    release.yml
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
    security.go
    service.go
    summarizer.go
    token.go
    types.go
    root_manager.go
    perf_bench_test.go
  benchmarks/results.json
  examples/
    cursor.mcp.json
    codex.config.toml
  scripts/
    install-vscode-mcp.bat
    set-allowed-roots.bat
    set-root.bat
    serve.bat
    stats.bat
```
