package lean

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Service struct {
	root     string
	indexer  *Indexer
	git      *GitDiff
	cache    *CacheStore
	metrics  *MetricsStore
	pipeline *Pipeline
}

func NewService(root string) (*Service, error) {
	idx, err := NewIndexer(root)
	if err != nil {
		return nil, err
	}
	cache, err := NewCacheStore(root)
	if err != nil {
		return nil, err
	}
	metrics, err := NewMetricsStore(root)
	if err != nil {
		return nil, err
	}
	git := NewGitDiff(root)
	pipeline := NewPipeline(idx, git, cache, metrics)
	return &Service{root: idx.Root(), indexer: idx, git: git, cache: cache, metrics: metrics, pipeline: pipeline}, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Start(ctx context.Context) error {
	if err := s.indexer.Build(ctx); err != nil {
		return err
	}
	if err := s.indexer.StartWatcher(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) Stop() {
	s.indexer.StopWatcher()
}

func (s *Service) ContextPack(ctx context.Context, in ContextPackInput) ContextBundle {
	return s.pipeline.Pack(ctx, in)
}

func (s *Service) CodeSymbols(_ context.Context, in CodeSymbolsInput) CodeSymbolsOutput {
	if in.MaxSymbols <= 0 {
		in.MaxSymbols = 200
	}
	snap := s.indexer.Snapshot()
	terms := tokenizeQuery(in.Query + " " + in.File)
	lang := strings.ToLower(in.Language)
	fileQ := strings.ToLower(filepath.ToSlash(in.File))
	symbols := []SymbolRef{}
	for _, rec := range snap.Files {
		if fileQ != "" && !strings.Contains(strings.ToLower(rec.Path), fileQ) {
			continue
		}
		for _, sym := range rec.Symbols {
			if lang != "" && !strings.EqualFold(sym.Language, lang) {
				continue
			}
			if len(terms) > 0 {
				hay := strings.ToLower(sym.Name + " " + sym.Signature + " " + sym.File)
				matched := false
				for _, t := range terms {
					if strings.Contains(hay, t) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			symbols = append(symbols, sym)
		}
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].File != symbols[j].File {
			return symbols[i].File < symbols[j].File
		}
		return symbols[i].LineStart < symbols[j].LineStart
	})
	total := len(symbols)
	if len(symbols) > in.MaxSymbols {
		symbols = symbols[:in.MaxSymbols]
	}
	return CodeSymbolsOutput{Symbols: symbols, Total: total}
}

func (s *Service) CodeSnippet(_ context.Context, in CodeSnippetInput) (CodeSnippetOutput, error) {
	abs, rel, err := NormalizeWorkspacePath(s.root, in.File)
	if err != nil {
		return CodeSnippetOutput{}, err
	}
	_ = abs
	if in.LineStart <= 0 || in.LineEnd <= 0 || in.LineEnd < in.LineStart {
		return CodeSnippetOutput{}, fmt.Errorf("invalid line range")
	}
	if in.LineEnd-in.LineStart > 300 {
		in.LineEnd = in.LineStart + 300
	}
	lines, err := s.indexer.ReadLines(rel, in.LineStart, in.LineEnd)
	if err != nil {
		return CodeSnippetOutput{}, err
	}
	content := strings.Join(lines, "\n")
	id := s.cache.PutSnippet(content, fmt.Sprintf("%s:%d", rel, in.LineStart))
	pointer, _ := s.cache.GetSnippetPointer(content)
	snippet := Snippet{
		ID:             id,
		File:           rel,
		LineStart:      in.LineStart,
		LineEnd:        in.LineEnd,
		Content:        content,
		EstimatedToken: EstimateTokens(content),
		CachedPointer:  pointer,
	}
	s.metrics.Record(RequestMetric{
		Date:             time.Now().Format("2006-01-02"),
		Tool:             "code.snippet",
		LatencyMs:        1,
		OriginalTokens:   snippet.EstimatedToken,
		OptimizedTokens:  snippet.EstimatedToken,
		TokensSaved:      0,
		SnippetsReturned: 1,
		CacheHit:         pointer != "",
		BytesRead:        int64(len(content)),
	})
	s.cache.CleanupAfterRequest()
	return CodeSnippetOutput{Snippet: snippet}, nil
}

func (s *Service) RepoMap(_ context.Context, in RepoMapInput) RepoMap {
	return s.indexer.RepoMap(in.MaxDirectories)
}

func (s *Service) ChangesFocus(ctx context.Context, in ChangesFocusInput) ChangesFocus {
	focus := s.git.Focus(ctx, in, s.indexer)
	orig := EstimateTokens(fmt.Sprintf("%+v", focus))
	opt := orig
	s.metrics.Record(RequestMetric{
		Date:             time.Now().Format("2006-01-02"),
		Tool:             "changes.focus",
		LatencyMs:        1,
		OriginalTokens:   orig,
		OptimizedTokens:  opt,
		TokensSaved:      0,
		SnippetsReturned: len(focus.Hunks),
		CacheHit:         false,
		BytesRead:        0,
	})
	return focus
}

func (s *Service) CacheClean(_ context.Context, in CacheCleanInput) (CacheCleanOutput, error) {
	out, err := s.cache.Clean(in.Mode, in.MaxAgeHours)
	if err != nil {
		return CacheCleanOutput{}, err
	}
	out.Root = s.root
	return out, nil
}

func (s *Service) MetricsText() string {
	return s.metrics.StatsText()
}
