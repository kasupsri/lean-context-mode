package lean

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type repoBenchmark struct {
	Repo                 string  `json:"repo"`
	ColdStartMs          int64   `json:"cold_start_ms"`
	AverageLatencyMs     float64 `json:"average_request_latency_ms"`
	CacheHitRate         float64 `json:"cache_hit_rate"`
	TotalOriginalTokens  int     `json:"total_original_tokens"`
	TotalOptimizedTokens int     `json:"total_optimized_tokens"`
	TokensSaved          int     `json:"tokens_saved"`
	ReductionPercent     float64 `json:"reduction_percent"`
	Requests             int     `json:"requests"`
}

type benchmarkReport struct {
	GeneratedAt string          `json:"generated_at"`
	Results     []repoBenchmark `json:"results"`
}

func TestBenchmarkWorkspaceRepos(t *testing.T) {
	if os.Getenv("LCM_RUN_BENCH_REPORT") != "1" {
		t.Skip("set LCM_RUN_BENCH_REPORT=1 to run workspace benchmark report generation")
	}
	repos := parseBenchRepoList(os.Getenv("LCM_BENCH_REPOS"))
	if len(repos) == 0 {
		t.Skip("set LCM_BENCH_REPOS to comma/semicolon separated workspace paths")
	}
	queries := []ContextPackInput{
		{Query: "where are mcp tools registered", TokenBudget: 1200},
		{Query: "show changed files and symbols", TokenBudget: 1200},
		{Query: "find token budgeting logic", TokenBudget: 1200},
		{Query: "locate caching and stats tracking", TokenBudget: 1200},
		{Query: "find security path validation", TokenBudget: 1200},
	}

	results := []repoBenchmark{}
	for _, repo := range repos {
		if st, err := os.Stat(repo); err != nil || !st.IsDir() {
			t.Logf("skip benchmark, repo missing: %s", repo)
			continue
		}
		workCopy := filepath.Join(t.TempDir(), filepath.Base(repo))
		if err := copyWorkspace(repo, workCopy); err != nil {
			t.Fatalf("copy workspace for %s: %v", repo, err)
		}
		start := time.Now()
		svc, err := NewService(workCopy)
		if err != nil {
			t.Fatalf("new service for %s: %v", repo, err)
		}
		ctx := context.Background()
		if err := svc.Start(ctx); err != nil {
			t.Fatalf("start service for %s: %v", repo, err)
		}
		cold := time.Since(start).Milliseconds()

		latTotal := int64(0)
		orig := 0
		opt := 0
		cacheRate := 0.0
		reqs := 0
		for i := 0; i < 20; i++ {
			q := queries[i%len(queries)]
			t0 := time.Now()
			bundle := svc.ContextPack(ctx, q)
			latTotal += time.Since(t0).Milliseconds()
			orig += bundle.Metrics.OriginalEstimatedToken
			opt += bundle.Metrics.OptimizedTokens
			cacheRate = bundle.Metrics.CacheHitRate
			reqs++
		}
		svc.Stop()
		saved := orig - opt
		reduction := 0.0
		if orig > 0 {
			reduction = float64(saved) / float64(orig) * 100
		}
		avgLatency := 0.0
		if reqs > 0 {
			avgLatency = float64(latTotal) / float64(reqs)
		}
		results = append(results, repoBenchmark{
			Repo:                 filepath.Base(repo),
			ColdStartMs:          cold,
			AverageLatencyMs:     avgLatency,
			CacheHitRate:         cacheRate,
			TotalOriginalTokens:  orig,
			TotalOptimizedTokens: opt,
			TokensSaved:          saved,
			ReductionPercent:     reduction,
			Requests:             reqs,
		})
	}

	if len(results) == 0 {
		t.Skip("no reference repos available")
	}

	report := benchmarkReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Results: results}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	moduleRoot := findModuleRoot(t)
	outDir := filepath.Join(moduleRoot, "benchmarks")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(outDir, "workspace-results.json")
	if err := os.WriteFile(outPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("benchmark report written: %s", outPath)
}

func parseBenchRepoList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(filepath.Clean(p)); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cur := wd
	for i := 0; i < 6; i++ {
		if st, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil && !st.IsDir() {
			return cur
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	t.Fatalf("module root not found from %s", wd)
	return ""
}

func copyWorkspace(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if base == ".git" || base == "node_modules" || base == ".lean-context-mode" || base == ".tmp" || base == ".gocache" || base == ".cache" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
