package lean

import "time"

const (
	DefaultTokenBudget = 1800
	MinTokenBudget     = 200
	MaxTokenBudget     = 8192
)

type BudgetInput struct {
	Query       string
	FileHints   []string
	Language    string
	TokenBudget int
}

type BudgetAllocation struct {
	TotalTokens       int    `json:"total_tokens"`
	MetadataTokens    int    `json:"metadata_tokens"`
	SignaturesTokens  int    `json:"signatures_tokens"`
	SnippetsTokens    int    `json:"snippets_tokens"`
	DependencyTokens  int    `json:"dependency_tokens"`
	DiffTokens        int    `json:"diff_tokens"`
	ConfigTokens      int    `json:"config_tokens"`
	SummaryTokens     int    `json:"summary_tokens"`
	EffectiveTokens   int    `json:"effective_tokens"`
	DeterministicSeed string `json:"deterministic_seed"`
}

type SymbolRef struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	File      string `json:"file"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Language  string `json:"language"`
	Score     int    `json:"score,omitempty"`
}

type Snippet struct {
	ID             string `json:"id"`
	File           string `json:"file"`
	LineStart      int    `json:"line_start"`
	LineEnd        int    `json:"line_end"`
	Content        string `json:"content"`
	EstimatedToken int    `json:"estimated_tokens"`
	CachedPointer  string `json:"cached_pointer,omitempty"`
	FromDiff       bool   `json:"from_diff"`
}

type DependencyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type DiffHunk struct {
	File       string   `json:"file"`
	Header     string   `json:"header"`
	Added      int      `json:"added"`
	Deleted    int      `json:"deleted"`
	Lines      []string `json:"lines,omitempty"`
	Symbols    []string `json:"symbols,omitempty"`
	IsStaged   bool     `json:"is_staged"`
	IsUnstaged bool     `json:"is_unstaged"`
}

type ConfigSlice struct {
	File      string `json:"file"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Content   string `json:"content"`
}

type TraceSummary struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

type ContextBundle struct {
	Query           string           `json:"query"`
	Language        string           `json:"language,omitempty"`
	TokenBudget     int              `json:"token_budget"`
	EstimatedTokens int              `json:"estimated_tokens"`
	Allocation      BudgetAllocation `json:"allocation"`
	Symbols         []SymbolRef      `json:"symbols"`
	Snippets        []Snippet        `json:"snippets"`
	Dependencies    []DependencyEdge `json:"dependencies"`
	Diffs           []DiffHunk       `json:"diffs"`
	Configs         []ConfigSlice    `json:"configs"`
	Summaries       []TraceSummary   `json:"summaries,omitempty"`
	CachePointers   []string         `json:"cache_pointers,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	Metrics         BundleMetrics    `json:"metrics"`
}

type BundleMetrics struct {
	DurationMs             int64   `json:"duration_ms"`
	OriginalEstimatedToken int     `json:"original_estimated_tokens"`
	OptimizedTokens        int     `json:"optimized_tokens"`
	TokensSaved            int     `json:"tokens_saved"`
	ReductionPercent       float64 `json:"reduction_percent"`
	SnippetsReturned       int     `json:"snippets_returned"`
	CacheHitRate           float64 `json:"cache_hit_rate"`
}

type FileRecord struct {
	Path       string
	Language   string
	Hash       string
	Size       int64
	Imports    []string
	Calls      []string
	Symbols    []SymbolRef
	IsConfig   bool
	IsTest     bool
	Tags       []string
	UpdatedAt  time.Time
	LineCount  int
	Dependency string
}

type RepoMap struct {
	Root                 string         `json:"root"`
	FilesIndexed         int            `json:"files_indexed"`
	SymbolsIndexed       int            `json:"symbols_indexed"`
	Languages            map[string]int `json:"languages"`
	ConfigFiles          int            `json:"config_files"`
	TestFiles            int            `json:"test_files"`
	WorkspaceHash        string         `json:"workspace_hash"`
	IndexesUpdatedAt     time.Time      `json:"indexes_updated_at"`
	TopDirectories       []string       `json:"top_directories"`
	HeuristicSignals     map[string]int `json:"heuristic_signals"`
	ImportRelationshipSz int            `json:"import_relationships"`
}

type ChangedFile struct {
	Path       string
	Added      int
	Deleted    int
	Status     string
	IsStaged   bool
	IsUnstaged bool
}

type ChangesFocus struct {
	Root        string        `json:"root"`
	Files       []ChangedFile `json:"files"`
	Hunks       []DiffHunk    `json:"hunks"`
	Affected    []SymbolRef   `json:"affected_symbols"`
	Warnings    []string      `json:"warnings,omitempty"`
	CollectedAt time.Time     `json:"collected_at"`
}

type RequestMetric struct {
	Date             string
	Tool             string
	LatencyMs        int64
	OriginalTokens   int
	OptimizedTokens  int
	TokensSaved      int
	SnippetsReturned int
	CacheHit         bool
	BytesRead        int64
}

type DailyMetrics struct {
	Date             string  `json:"date"`
	Requests         int     `json:"requests"`
	OriginalTokens   int     `json:"original_tokens"`
	OptimizedTokens  int     `json:"optimized_tokens"`
	TokensSaved      int     `json:"tokens_saved"`
	ReductionPercent float64 `json:"reduction_percent"`
}

type MetricsSnapshot struct {
	RequestsProcessed int                     `json:"requests_processed"`
	TotalOriginal     int                     `json:"total_original_tokens"`
	TotalOptimized    int                     `json:"total_optimized_tokens"`
	TotalSaved        int                     `json:"total_tokens_saved"`
	CacheHits         int                     `json:"cache_hits"`
	CacheMisses       int                     `json:"cache_misses"`
	BytesRead         int64                   `json:"bytes_read"`
	SnippetsReturned  int                     `json:"snippets_returned"`
	AvgLatencyMs      float64                 `json:"avg_latency_ms"`
	UpdatedAt         time.Time               `json:"updated_at"`
	Daily             map[string]DailyMetrics `json:"daily"`
}

// MCP tool inputs/outputs

type ContextPackInput struct {
	Query         string   `json:"query" jsonschema:"Natural-language coding query"`
	FileHints     []string `json:"file_hints,omitempty" jsonschema:"Optional file path hints to bias retrieval"`
	Language      string   `json:"language,omitempty" jsonschema:"Optional language hint (go, ts, python, etc.)"`
	TokenBudget   int      `json:"token_budget,omitempty" jsonschema:"Max output token budget"`
	WorkspaceRoot string   `json:"workspace_root,omitempty" jsonschema:"Optional workspace root override (must be inside allowed roots)"`
}

type CodeSymbolsInput struct {
	Query         string `json:"query,omitempty" jsonschema:"Optional symbol search query"`
	File          string `json:"file,omitempty" jsonschema:"Optional file path to filter symbols"`
	Language      string `json:"language,omitempty" jsonschema:"Optional language filter"`
	MaxSymbols    int    `json:"max_symbols,omitempty" jsonschema:"Max symbols to return"`
	WorkspaceRoot string `json:"workspace_root,omitempty" jsonschema:"Optional workspace root override (must be inside allowed roots)"`
}

type CodeSnippetInput struct {
	File          string `json:"file" jsonschema:"File path relative to workspace root"`
	LineStart     int    `json:"line_start" jsonschema:"1-based line start"`
	LineEnd       int    `json:"line_end" jsonschema:"1-based line end"`
	WorkspaceRoot string `json:"workspace_root,omitempty" jsonschema:"Optional workspace root override (must be inside allowed roots)"`
}

type RepoMapInput struct {
	MaxDirectories int    `json:"max_directories,omitempty" jsonschema:"Max top directories to include"`
	WorkspaceRoot  string `json:"workspace_root,omitempty" jsonschema:"Optional workspace root override (must be inside allowed roots)"`
}

type ChangesFocusInput struct {
	MaxFiles        int    `json:"max_files,omitempty" jsonschema:"Max changed files to return"`
	MaxHunksPerFile int    `json:"max_hunks_per_file,omitempty" jsonschema:"Max hunks per file"`
	IncludeHunks    bool   `json:"include_hunks,omitempty" jsonschema:"Whether to include compact hunk lines"`
	WorkspaceRoot   string `json:"workspace_root,omitempty" jsonschema:"Optional workspace root override (must be inside allowed roots)"`
}

type CodeSymbolsOutput struct {
	Symbols []SymbolRef `json:"symbols"`
	Total   int         `json:"total"`
}

type CodeSnippetOutput struct {
	Snippet Snippet `json:"snippet"`
}

type WorkspaceRootGetInput struct{}

type WorkspaceRootSetInput struct {
	WorkspaceRoot string `json:"workspace_root" jsonschema:"New active workspace root (must be inside allowed roots)"`
}

type WorkspaceRootOutput struct {
	ActiveRoot   string   `json:"active_root"`
	AllowedRoots []string `json:"allowed_roots"`
}
