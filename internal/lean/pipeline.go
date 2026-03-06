package lean

import (
	"context"
	"encoding/json"
	"time"
)

type Pipeline struct {
	budgeter   *Budgeter
	retriever  *Retriever
	summarizer *Summarizer
	indexer    *Indexer
	metrics    *MetricsStore
	cache      *CacheStore
}

func NewPipeline(indexer *Indexer, git *GitDiff, cache *CacheStore, metrics *MetricsStore) *Pipeline {
	return &Pipeline{
		budgeter:   NewBudgeter(),
		retriever:  NewRetriever(indexer, git, cache),
		summarizer: NewSummarizer(cache),
		indexer:    indexer,
		metrics:    metrics,
		cache:      cache,
	}
}

func (p *Pipeline) Pack(ctx context.Context, in ContextPackInput) ContextBundle {
	start := time.Now()
	in.Query = SanitizeUserText(in.Query, 2000)
	if in.Query == "" {
		in.Query = "repository context"
	}
	for i := range in.FileHints {
		in.FileHints[i] = SanitizeUserText(in.FileHints[i], 200)
	}
	in.Language = SanitizeUserText(in.Language, 64)
	in.TokenBudget = ValidateTokenBudget(in.TokenBudget)

	alloc := p.budgeter.Allocate(BudgetInput{Query: in.Query, FileHints: in.FileHints, Language: in.Language, TokenBudget: in.TokenBudget})
	rr := p.retriever.Retrieve(ctx, in, alloc)

	bundle := ContextBundle{
		Query:         in.Query,
		Language:      in.Language,
		TokenBudget:   in.TokenBudget,
		Allocation:    alloc,
		Symbols:       rr.symbols,
		Snippets:      rr.snippets,
		Dependencies:  rr.dependencies,
		Diffs:         rr.diffs,
		Configs:       rr.configs,
		CachePointers: rr.pointers,
	}
	bundle.EstimatedTokens = estimateBundleTokens(bundle)
	originalTokens := bundle.EstimatedTokens
	if rr.rawTokens > originalTokens {
		originalTokens = rr.rawTokens
	}

	snap := p.indexer.Snapshot()
	p.summarizer.MaybeSummarize(&bundle, snap)
	bundle.EstimatedTokens = estimateBundleTokens(bundle)
	if bundle.EstimatedTokens > originalTokens {
		// Guardrail: summarization must never increase token usage.
		bundle.Summaries = nil
		bundle.EstimatedTokens = originalTokens
	}

	hits, misses := p.cache.HitMiss()
	reduction := 0.0
	saved := originalTokens - bundle.EstimatedTokens
	if originalTokens > 0 {
		reduction = float64(saved) / float64(originalTokens) * 100
	}
	bundle.Metrics = BundleMetrics{
		DurationMs:             time.Since(start).Milliseconds(),
		OriginalEstimatedToken: originalTokens,
		OptimizedTokens:        bundle.EstimatedTokens,
		TokensSaved:            saved,
		ReductionPercent:       reduction,
		SnippetsReturned:       len(bundle.Snippets),
		CacheHitRate:           p.cache.HitRate(),
	}

	date := time.Now().Format("2006-01-02")
	cacheHit := hits > misses
	p.metrics.Record(RequestMetric{
		Date:             date,
		Tool:             "context.pack",
		LatencyMs:        bundle.Metrics.DurationMs,
		OriginalTokens:   originalTokens,
		OptimizedTokens:  bundle.EstimatedTokens,
		TokensSaved:      saved,
		SnippetsReturned: len(bundle.Snippets),
		CacheHit:         cacheHit,
		BytesRead:        rr.bytesRead,
	})
	return bundle
}

func estimateBundleTokens(bundle ContextBundle) int {
	b, _ := json.Marshal(bundle)
	return EstimateTokens(string(b))
}
