package lean

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type Summarizer struct {
	cache *CacheStore
}

func NewSummarizer(cache *CacheStore) *Summarizer {
	return &Summarizer{cache: cache}
}

func (s *Summarizer) MaybeSummarize(bundle *ContextBundle, snap IndexSnapshot) {
	if bundle.EstimatedTokens <= bundle.TokenBudget {
		return
	}
	key := s.summaryKey(bundle, snap)
	if cached, ok := s.cache.GetSummary(key); ok {
		bundle.Summaries = cached
		trimBundleToBudget(bundle)
		return
	}

	terms := tokenizeQuery(bundle.Query)
	summaries := make([]TraceSummary, 0, len(bundle.Snippets))
	for _, sn := range bundle.Snippets {
		lines := strings.Split(sn.Content, "\n")
		parts := []string{}
		for _, line := range lines {
			ll := strings.ToLower(line)
			if len(terms) > 0 {
				for _, t := range terms {
					if strings.Contains(ll, t) {
						parts = append(parts, strings.TrimSpace(line))
						break
					}
				}
			} else if strings.TrimSpace(line) != "" {
				parts = append(parts, strings.TrimSpace(line))
			}
			if len(parts) >= 3 {
				break
			}
		}
		if len(parts) == 0 && len(lines) > 0 {
			parts = append(parts, strings.TrimSpace(lines[0]))
		}
		summaries = append(summaries, TraceSummary{
			Source: sn.File + ":" + itoa(sn.LineStart) + "-" + itoa(sn.LineEnd),
			Text:   strings.Join(parts, " | "),
		})
	}
	bundle.Summaries = summaries
	s.cache.PutSummary(key, summaries)
	trimBundleToBudget(bundle)
}

func (s *Summarizer) summaryKey(bundle *ContextBundle, snap IndexSnapshot) string {
	fileHashes := []string{}
	for _, sn := range bundle.Snippets {
		if rec, ok := snap.Files[sn.File]; ok {
			fileHashes = append(fileHashes, rec.Hash)
		}
	}
	sort.Strings(fileHashes)
	depParts := []string{}
	for _, d := range bundle.Dependencies {
		depParts = append(depParts, d.From+"->"+d.To+":"+d.Kind)
	}
	sort.Strings(depParts)
	cfgHashes := []string{}
	for _, cfg := range bundle.Configs {
		if rec, ok := snap.Files[cfg.File]; ok {
			cfgHashes = append(cfgHashes, rec.Hash)
		}
	}
	sort.Strings(cfgHashes)
	raw := strings.Join(fileHashes, ",") + "|" + strings.Join(depParts, ",") + "|" + strings.Join(cfgHashes, ",") + "|" + strings.ToLower(bundle.Query)
	h := sha256.Sum256([]byte(raw))
	return "summary_" + hex.EncodeToString(h[:])[:24]
}

func trimBundleToBudget(bundle *ContextBundle) {
	for bundle.EstimatedTokens > bundle.TokenBudget {
		switch {
		case len(bundle.Snippets) > 3:
			bundle.Snippets = bundle.Snippets[:len(bundle.Snippets)-1]
		case len(bundle.Diffs) > 2:
			bundle.Diffs = bundle.Diffs[:len(bundle.Diffs)-1]
		case len(bundle.Dependencies) > 8:
			bundle.Dependencies = bundle.Dependencies[:len(bundle.Dependencies)-2]
		case len(bundle.Symbols) > 20:
			bundle.Symbols = bundle.Symbols[:len(bundle.Symbols)-3]
		case len(bundle.Configs) > 2:
			bundle.Configs = bundle.Configs[:len(bundle.Configs)-1]
		default:
			bundle.Summaries = compactSummaries(bundle.Summaries, bundle.Allocation.SummaryTokens)
			bundle.EstimatedTokens = estimateBundleTokens(*bundle)
			return
		}
		bundle.EstimatedTokens = estimateBundleTokens(*bundle)
	}
}

func compactSummaries(in []TraceSummary, budget int) []TraceSummary {
	if len(in) == 0 {
		return in
	}
	used := 0
	out := []TraceSummary{}
	for _, s := range in {
		cost := EstimateTokens(s.Source + s.Text)
		if used+cost > budget && len(out) > 0 {
			break
		}
		out = append(out, s)
		used += cost
	}
	return out
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
