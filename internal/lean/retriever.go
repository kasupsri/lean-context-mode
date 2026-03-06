package lean

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Retriever struct {
	index *Indexer
	git   *GitDiff
	cache *CacheStore
}

func NewRetriever(index *Indexer, git *GitDiff, cache *CacheStore) *Retriever {
	return &Retriever{index: index, git: git, cache: cache}
}

func tokenizeQuery(query string) []string {
	query = strings.ToLower(query)
	repl := strings.NewReplacer(
		",", " ",
		".", " ",
		";", " ",
		":", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"/", " ",
		"\\", " ",
	)
	query = repl.Replace(query)
	parts := strings.Fields(query)
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		if len(p) < 2 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

type retrievalResult struct {
	symbols      []SymbolRef
	snippets     []Snippet
	dependencies []DependencyEdge
	diffs        []DiffHunk
	configs      []ConfigSlice
	pointers     []string
	bytesRead    int64
	rawTokens    int
}

func (r *Retriever) Retrieve(ctx context.Context, in ContextPackInput, alloc BudgetAllocation) retrievalResult {
	snap := r.index.Snapshot()
	changed := r.git.ChangedFileSet(ctx)
	terms := tokenizeQuery(in.Query + " " + strings.Join(in.FileHints, " "))
	language := strings.ToLower(strings.TrimSpace(in.Language))

	symbols := r.rankSymbols(snap, terms, in.FileHints, language, changed, alloc.SignaturesTokens)
	snippets, pointers, bytesRead := r.collectSnippets(symbols, terms, changed, alloc.SnippetsTokens)
	deps := r.collectDependencies(snap, symbols, alloc.DependencyTokens)
	diffs := r.collectDiffs(ctx, changed, alloc.DiffTokens)
	configs := r.collectConfigs(snap, in.FileHints, alloc.ConfigTokens)
	rawTokens := r.estimateRawTokens(snap, symbols, changed, configs)

	return retrievalResult{
		symbols:      symbols,
		snippets:     snippets,
		dependencies: deps,
		diffs:        diffs,
		configs:      configs,
		pointers:     pointers,
		bytesRead:    bytesRead,
		rawTokens:    rawTokens,
	}
}

func (r *Retriever) rankSymbols(snap IndexSnapshot, terms []string, hints []string, language string, changed map[string]ChangedFile, budget int) []SymbolRef {
	type scored struct {
		s     SymbolRef
		score int
	}
	all := []scored{}
	hintSet := make([]string, 0, len(hints))
	for _, h := range hints {
		hintSet = append(hintSet, strings.ToLower(filepath.ToSlash(h)))
	}
	for _, rec := range snap.Files {
		for _, sym := range rec.Symbols {
			score := 0
			name := strings.ToLower(sym.Name)
			sig := strings.ToLower(sym.Signature)
			for _, t := range terms {
				if t == name {
					score += 50
				} else if strings.Contains(name, t) {
					score += 28
				}
				if strings.Contains(sig, t) {
					score += 10
				}
				if strings.Contains(strings.ToLower(sym.File), t) {
					score += 8
				}
			}
			for _, h := range hintSet {
				if strings.Contains(strings.ToLower(sym.File), h) {
					score += 22
				}
			}
			if language != "" && strings.EqualFold(sym.Language, language) {
				score += 12
			}
			if _, ok := changed[sym.File]; ok {
				score += 26
			}
			if strings.Contains(strings.ToLower(sym.File), "test") && hasAny(strings.Join(terms, " "), "test", "failing", "assert") {
				score += 8
			}
			if score <= 0 {
				continue
			}
			sym.Score = score
			all = append(all, scored{s: sym, score: score})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if all[i].s.File != all[j].s.File {
			return all[i].s.File < all[j].s.File
		}
		return all[i].s.LineStart < all[j].s.LineStart
	})
	used := 0
	maxSymbols := 250
	result := make([]SymbolRef, 0, min(len(all), maxSymbols))
	for _, sc := range all {
		cost := EstimateTokens(sc.s.Signature) + 8
		if used+cost > budget && len(result) > 0 {
			break
		}
		result = append(result, sc.s)
		used += cost
		if len(result) >= maxSymbols {
			break
		}
	}
	return result
}

func (r *Retriever) collectSnippets(symbols []SymbolRef, terms []string, changed map[string]ChangedFile, budget int) ([]Snippet, []string, int64) {
	if len(symbols) == 0 {
		return nil, nil, 0
	}
	used := 0
	bytesRead := int64(0)
	out := []Snippet{}
	pointers := []string{}
	seen := map[string]struct{}{}
	for _, sym := range symbols {
		pre := 2
		post := 14
		if _, ok := changed[sym.File]; ok {
			post = 24
		}
		start := sym.LineStart - pre
		if start < 1 {
			start = 1
		}
		end := sym.LineStart + post
		lines, err := r.index.ReadLines(sym.File, start, end)
		if err != nil || len(lines) == 0 {
			continue
		}
		content := strings.Join(lines, "\n")
		if len(terms) > 0 {
			content = maybeFocusSnippet(content, terms)
		}
		tokens := EstimateTokens(content)
		if used+tokens > budget && len(out) > 0 {
			break
		}
		bytesRead += int64(len(content))
		if _, ok := seen[content]; ok {
			continue
		}
		seen[content] = struct{}{}
		id, pointer, hit := r.cache.PutSnippetWithPointer(content, fmt.Sprintf("%s:%d", sym.File, sym.LineStart))
		s := Snippet{
			ID:             id,
			File:           sym.File,
			LineStart:      start,
			LineEnd:        end,
			Content:        content,
			EstimatedToken: tokens,
			FromDiff:       false,
		}
		if hit {
			s.CachedPointer = pointer
			pointers = appendUnique(pointers, pointer)
		}
		out = append(out, s)
		used += tokens
	}
	return out, pointers, bytesRead
}

func maybeFocusSnippet(content string, terms []string) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 12 {
		return content
	}
	match := make([]string, 0, 16)
	for _, l := range lines {
		ll := strings.ToLower(l)
		for _, t := range terms {
			if strings.Contains(ll, t) {
				match = append(match, l)
				break
			}
		}
		if len(match) >= 16 {
			break
		}
	}
	if len(match) < 3 {
		return content
	}
	return strings.Join(match, "\n")
}

func (r *Retriever) collectDependencies(snap IndexSnapshot, symbols []SymbolRef, budget int) []DependencyEdge {
	used := 0
	edges := []DependencyEdge{}
	seen := map[string]struct{}{}
	for _, sym := range symbols {
		rec, ok := snap.Files[sym.File]
		if !ok {
			continue
		}
		for _, imp := range rec.Imports {
			edge := DependencyEdge{From: sym.File, To: imp, Kind: "import"}
			key := edge.From + "->" + edge.To + ":" + edge.Kind
			if _, ok := seen[key]; ok {
				continue
			}
			cost := EstimateTokens(edge.From+edge.To) + 4
			if used+cost > budget && len(edges) > 0 {
				return edges
			}
			seen[key] = struct{}{}
			edges = append(edges, edge)
			used += cost
		}
		for _, dep := range snap.Dependents[sym.File] {
			edge := DependencyEdge{From: dep, To: sym.File, Kind: "dependent"}
			key := edge.From + "->" + edge.To + ":" + edge.Kind
			if _, ok := seen[key]; ok {
				continue
			}
			cost := EstimateTokens(edge.From+edge.To) + 4
			if used+cost > budget && len(edges) > 0 {
				return edges
			}
			seen[key] = struct{}{}
			edges = append(edges, edge)
			used += cost
		}
		for _, testFile := range snap.TestsBySrc[sym.File] {
			edge := DependencyEdge{From: testFile, To: sym.File, Kind: "test"}
			key := edge.From + "->" + edge.To + ":" + edge.Kind
			if _, ok := seen[key]; ok {
				continue
			}
			cost := EstimateTokens(edge.From+edge.To) + 4
			if used+cost > budget && len(edges) > 0 {
				return edges
			}
			seen[key] = struct{}{}
			edges = append(edges, edge)
			used += cost
		}
	}
	return edges
}

func (r *Retriever) collectDiffs(ctx context.Context, changed map[string]ChangedFile, budget int) []DiffHunk {
	if len(changed) == 0 {
		return nil
	}
	used := 0
	out := []DiffHunk{}
	paths := make([]string, 0, len(changed))
	for p := range changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		hs, err := r.git.hunks(ctx, p, 3, true)
		if err != nil {
			continue
		}
		for _, h := range hs {
			cost := EstimateTokens(h.Header+strings.Join(h.Lines, "\n")+strings.Join(h.Symbols, ",")) + 8
			if used+cost > budget && len(out) > 0 {
				return out
			}
			out = append(out, h)
			used += cost
		}
	}
	return out
}

func (r *Retriever) collectConfigs(snap IndexSnapshot, hints []string, budget int) []ConfigSlice {
	used := 0
	out := []ConfigSlice{}
	files := make([]FileRecord, 0)
	for _, f := range snap.Files {
		if f.IsConfig {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		score := func(f FileRecord) int {
			s := 0
			for _, h := range hints {
				if strings.Contains(strings.ToLower(f.Path), strings.ToLower(filepath.ToSlash(h))) {
					s += 20
				}
			}
			if strings.Contains(strings.ToLower(f.Path), "appsettings") || strings.Contains(strings.ToLower(f.Path), "config") {
				s += 8
			}
			return s
		}
		si, sj := score(files[i]), score(files[j])
		if si != sj {
			return si > sj
		}
		return files[i].Path < files[j].Path
	})
	for _, f := range files {
		lines, err := r.index.ReadLines(f.Path, 1, 24)
		if err != nil || len(lines) == 0 {
			continue
		}
		content := strings.Join(lines, "\n")
		cost := EstimateTokens(content) + 8
		if used+cost > budget && len(out) > 0 {
			break
		}
		out = append(out, ConfigSlice{File: f.Path, LineStart: 1, LineEnd: len(lines), Content: content})
		used += cost
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (r *Retriever) estimateRawTokens(snap IndexSnapshot, symbols []SymbolRef, changed map[string]ChangedFile, configs []ConfigSlice) int {
	files := map[string]struct{}{}
	for _, s := range symbols {
		files[s.File] = struct{}{}
	}
	for path := range changed {
		files[path] = struct{}{}
	}
	for _, cfg := range configs {
		files[cfg.File] = struct{}{}
	}
	if len(files) == 0 {
		return 0
	}
	total := 0
	for file := range files {
		rec, ok := snap.Files[file]
		if !ok {
			continue
		}
		// Approximate raw-context baseline as full-file content tokens.
		total += EstimateTokens(strings.Repeat("x", int(rec.Size)))
	}
	return total
}
