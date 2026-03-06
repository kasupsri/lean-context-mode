package lean

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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
	aggressivelyCompactBundle(&bundle)
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
	p.cache.CleanupAfterRequest()
	return bundle
}

func estimateBundleTokens(bundle ContextBundle) int {
	b, _ := json.Marshal(bundle)
	return EstimateTokens(string(b))
}

func aggressivelyCompactBundle(bundle *ContextBundle) {
	target := compactTargetTokens(bundle.TokenBudget)

	if len(bundle.Symbols) > 4 {
		bundle.Symbols = bundle.Symbols[:4]
	}
	for i := range bundle.Symbols {
		bundle.Symbols[i].Signature = compactText(bundle.Symbols[i].Signature, 72)
		bundle.Symbols[i].File = compactPath(bundle.Symbols[i].File, 84)
	}

	if len(bundle.Snippets) > 1 {
		bundle.Snippets = bundle.Snippets[:1]
	}
	if len(bundle.Snippets) > 0 {
		sn := &bundle.Snippets[0]
		sn.Content = compactSnippet(sn.Content, 96)
		sn.LineEnd = sn.LineStart
		sn.CachedPointer = ""
		sn.EstimatedToken = EstimateTokens(sn.Content)
	}

	bundle.Dependencies = nil
	bundle.Diffs = nil
	bundle.Configs = nil
	bundle.CachePointers = nil

	if len(bundle.Summaries) == 0 {
		bundle.Summaries = synthesizeSummaries(bundle)
	}
	if len(bundle.Summaries) > 3 {
		bundle.Summaries = bundle.Summaries[:3]
	}
	for i := range bundle.Summaries {
		bundle.Summaries[i].Source = compactPath(bundle.Summaries[i].Source, 96)
		bundle.Summaries[i].Text = compactText(bundle.Summaries[i].Text, 92)
	}

	if len(bundle.Symbols) == 0 && len(bundle.Snippets) > 0 {
		sn := bundle.Snippets[0]
		bundle.Symbols = []SymbolRef{{
			Name:      "snippet_context",
			Kind:      "summary",
			Signature: compactText(sn.Content, 48),
			File:      sn.File,
			LineStart: sn.LineStart,
			LineEnd:   sn.LineEnd,
			Language:  bundle.Language,
		}}
	}
	if len(bundle.Snippets) == 0 && len(bundle.Symbols) > 0 {
		sy := bundle.Symbols[0]
		bundle.Snippets = []Snippet{{
			ID:             "snippet_compact",
			File:           sy.File,
			LineStart:      sy.LineStart,
			LineEnd:        sy.LineStart,
			Content:        compactText(sy.Signature, 48),
			EstimatedToken: EstimateTokens(compactText(sy.Signature, 48)),
			FromDiff:       false,
		}}
	}

	for pass := 0; pass < 6; pass++ {
		bundle.EstimatedTokens = estimateBundleTokens(*bundle)
		if bundle.EstimatedTokens <= target {
			return
		}
		switch pass {
		case 0:
			if len(bundle.Summaries) > 1 {
				bundle.Summaries = bundle.Summaries[:1]
			}
		case 1:
			if len(bundle.Symbols) > 2 {
				bundle.Symbols = bundle.Symbols[:2]
			}
		case 2:
			for i := range bundle.Symbols {
				bundle.Symbols[i].Signature = compactText(bundle.Symbols[i].Signature, 40)
			}
		case 3:
			for i := range bundle.Summaries {
				bundle.Summaries[i].Text = compactText(bundle.Summaries[i].Text, 56)
			}
		case 4:
			if len(bundle.Snippets) > 0 {
				bundle.Snippets[0].Content = compactText(bundle.Snippets[0].Content, 48)
				bundle.Snippets[0].EstimatedToken = EstimateTokens(bundle.Snippets[0].Content)
			}
		case 5:
			if len(bundle.Symbols) > 1 {
				bundle.Symbols = bundle.Symbols[:1]
			}
		}
	}
}

func compactTargetTokens(budget int) int {
	target := int(float64(budget) * 0.18)
	if target < 180 {
		target = 180
	}
	if target > 320 {
		target = 320
	}
	return target
}

func compactText(s string, maxLen int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func compactSnippet(content string, maxLen int) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return compactText(line, maxLen)
		}
	}
	return compactText(content, maxLen)
}

func compactPath(p string, maxLen int) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if len(p) <= maxLen {
		return p
	}
	base := filepath.Base(p)
	if len(base) <= maxLen {
		return base
	}
	return compactText(base, maxLen)
}

func synthesizeSummaries(bundle *ContextBundle) []TraceSummary {
	out := make([]TraceSummary, 0, 3)
	if len(bundle.Snippets) > 0 {
		sn := bundle.Snippets[0]
		out = append(out, TraceSummary{
			Source: compactPath(sn.File+":"+itoa(sn.LineStart), 96),
			Text:   compactText(sn.Content, 92),
		})
	}
	for i := 0; i < len(bundle.Symbols) && len(out) < 3; i++ {
		sy := bundle.Symbols[i]
		txt := sy.Name
		if sy.Signature != "" {
			txt += " :: " + sy.Signature
		}
		out = append(out, TraceSummary{
			Source: compactPath(sy.File+":"+itoa(sy.LineStart), 96),
			Text:   compactText(txt, 92),
		})
	}
	return out
}
