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
cd "C:\Work\Kasup\Context Mode\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

### Build (cmd.exe)
```bat
cd /d "C:\Work\Kasup\Context Mode\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode
```

### Local sanity check
```powershell
.\lean-context-mode.exe stats --root "C:\Work\Kasup\Context Mode\lean-context-mode"
```

### Run MCP server (stdio)
```powershell
.\lean-context-mode.exe serve --root "C:\Work\Kasup\Context Mode\<your-workspace>"
```

`serve` is a stdio MCP server and is intended to be started by Cursor/Codex/Claude MCP.  
If stdin closes, the process exits.

### Run without building
```powershell
go run .\cmd\lean-context-mode serve --root "C:\Work\Kasup\Context Mode\<your-workspace>"
```

### Use environment variable for root
If `--root` is omitted, the CLI uses:
1. `LCM_ROOT`
2. `LEAN_CONTEXT_MODE_ROOT`
3. current directory (`.`)

PowerShell:
```powershell
$env:LCM_ROOT = "C:\Work\Kasup\Context Mode\context-mode"
.\lean-context-mode.exe serve
.\lean-context-mode.exe stats
```

cmd.exe:
```bat
set LCM_ROOT=C:\Work\Kasup\Context Mode\context-mode
lean-context-mode.exe serve
lean-context-mode.exe stats
```

### BAT helpers (Windows)
Included scripts:
- `scripts\set-root.bat` (persist `LCM_ROOT` with `setx`)
- `scripts\serve.bat` (run server; uses `LCM_ROOT` or current directory)
- `scripts\stats.bat` (show stats; uses `LCM_ROOT` or current directory)

Examples:
```bat
cd /d "C:\Work\Kasup\Context Mode\lean-context-mode"
go build -o lean-context-mode.exe .\cmd\lean-context-mode

scripts\set-root.bat "C:\Work\Kasup\Context Mode\context-mode"
scripts\stats.bat
scripts\serve.bat
```

## MCP Client Config Examples

- Cursor example: `examples/cursor.mcp.json`
- Codex example: `examples/codex.config.toml`

Both examples call `scripts\serve.bat` via `cmd /c`.  
Set `LCM_ROOT` first (for example with `scripts\set-root.bat`) or pass an explicit root directly when running the binary.

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

## Benchmark Results

Benchmark basis:
- `context-mode`
- `universal-context-mode`

Generated from `benchmarks/results.json`:

- `context-mode`
  - cold start: `194 ms`
  - avg request latency: `43.95 ms`
  - cache hit rate: `0.85`
  - tokens saved: `978,164` (`96.50%`)
- `universal-context-mode`
  - cold start: `37 ms`
  - avg request latency: `6.65 ms`
  - cache hit rate: `0.85`
  - tokens saved: `185,208` (`82.13%`)

Reproduce benchmark:

```bash
docker run --rm \
  -v '/mnt/c/Work/Kasup/Context Mode':/workspace \
  -w /workspace/lean-context-mode \
  -e LCM_BENCH_ROOT=/workspace \
  golang:1.25 \
  go test ./internal/lean -run TestBenchmarkReferenceRepos -v
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
    security.go
    service.go
    summarizer.go
    token.go
    types.go
  benchmarks/results.json
  examples/
    cursor.mcp.json
    codex.config.toml
  scripts/
    set-root.bat
    serve.bat
    stats.bat
```
