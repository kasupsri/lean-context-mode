package lean

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultCacheMaxAgeHours = 168
	defaultAutoPruneEvery   = 15 * time.Minute
	cacheModeEphemeral      = "ephemeral"
	cacheModeBounded        = "bounded"
	cacheBackendMemory      = "memory://cache"
)

type snippetCacheEntry struct {
	ID         string    `json:"id"`
	Hash       string    `json:"hash"`
	Source     string    `json:"source"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
	Hits       int       `json:"hits"`
}

type summaryCacheEntry struct {
	Key        string         `json:"key"`
	Summaries  []TraceSummary `json:"summaries"`
	CreatedAt  time.Time      `json:"created_at"`
	AccessedAt time.Time      `json:"accessed_at"`
	Hits       int            `json:"hits"`
}

type cacheState struct {
	Snippets  map[string]snippetCacheEntry `json:"snippets"`
	Summaries map[string]summaryCacheEntry `json:"summaries"`
}

type CacheStore struct {
	mu             sync.RWMutex
	state          cacheState
	maxSnippets    int
	maxSummaries   int
	maxAge         time.Duration
	autoPruneEvery time.Duration
	lastAutoPrune  time.Time
	mode           string

	hits   int
	misses int
}

func NewCacheStore(_ string) (*CacheStore, error) {
	cs := &CacheStore{
		state:          cacheState{Snippets: map[string]snippetCacheEntry{}, Summaries: map[string]summaryCacheEntry{}},
		maxSnippets:    2000,
		maxSummaries:   3000,
		maxAge:         cacheMaxAgeFromEnv(),
		autoPruneEvery: defaultAutoPruneEvery,
		mode:           cacheModeFromEnv(),
	}
	cs.mu.Lock()
	now := time.Now().UTC()
	cs.lastAutoPrune = now
	if cs.mode == cacheModeEphemeral {
		cs.state.Snippets = map[string]snippetCacheEntry{}
		cs.state.Summaries = map[string]summaryCacheEntry{}
	} else {
		cs.pruneExpiredLocked(now, cs.maxAge)
	}
	cs.mu.Unlock()
	return cs, nil
}

func cacheModeFromEnv() string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LCM_CACHE_MODE")))
	switch raw {
	case "", cacheModeEphemeral:
		return cacheModeEphemeral
	case cacheModeBounded, "persistent":
		return cacheModeBounded
	default:
		return cacheModeEphemeral
	}
}

func cacheMaxAgeFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("LCM_CACHE_MAX_AGE_HOURS"))
	if raw == "" {
		return defaultCacheMaxAgeHours * time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultCacheMaxAgeHours * time.Hour
	}
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Hour
}

func shortHash(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])[:16]
}

func (c *CacheStore) PutSnippet(content, source string) string {
	id, _, _ := c.PutSnippetWithPointer(content, source)
	return id
}

func (c *CacheStore) PutSnippetWithPointer(content, source string) (string, string, bool) {
	h := shortHash(content)
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maybeAutoPruneLocked(now)
	entry, ok := c.state.Snippets[h]
	if ok {
		entry.AccessedAt = now
		entry.Hits++
		c.state.Snippets[h] = entry
		c.hits++
		return entry.ID, "cache://snippet/" + entry.ID, true
	}
	entry = snippetCacheEntry{
		ID:         "snippet_" + h,
		Hash:       h,
		Source:     source,
		Content:    content,
		CreatedAt:  now,
		AccessedAt: now,
		Hits:       0,
	}
	c.state.Snippets[h] = entry
	c.misses++
	trimSnippets(c.state.Snippets, c.maxSnippets)
	return entry.ID, "cache://snippet/" + entry.ID, false
}

func (c *CacheStore) GetSnippetPointer(content string) (string, bool) {
	h := shortHash(content)
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.state.Snippets[h]
	if !ok {
		c.misses++
		return "", false
	}
	entry.AccessedAt = time.Now().UTC()
	entry.Hits++
	c.state.Snippets[h] = entry
	c.hits++
	return "cache://snippet/" + entry.ID, true
}

func (c *CacheStore) PutSummary(key string, summaries []TraceSummary) {
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maybeAutoPruneLocked(now)
	c.state.Summaries[key] = summaryCacheEntry{
		Key:        key,
		Summaries:  summaries,
		CreatedAt:  now,
		AccessedAt: now,
	}
	trimSummaries(c.state.Summaries, c.maxSummaries)
}

func (c *CacheStore) GetSummary(key string) ([]TraceSummary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.state.Summaries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	entry.Hits++
	entry.AccessedAt = time.Now().UTC()
	c.state.Summaries[key] = entry
	c.hits++
	out := make([]TraceSummary, len(entry.Summaries))
	copy(out, entry.Summaries)
	return out, true
}

func (c *CacheStore) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

func (c *CacheStore) HitMiss() (int, int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *CacheStore) Clean(mode string, maxAgeHours int) (CacheCleanOutput, error) {
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" {
		normalizedMode = "expired"
	}

	out := CacheCleanOutput{
		Mode:      normalizedMode,
		CleanedAt: now,
		CacheFile: cacheBackendMemory,
	}

	switch normalizedMode {
	case "all":
		out.SnippetsRemoved = len(c.state.Snippets)
		out.SummariesRemoved = len(c.state.Summaries)
		c.state.Snippets = map[string]snippetCacheEntry{}
		c.state.Summaries = map[string]summaryCacheEntry{}
	case "expired":
		maxAge := c.maxAge
		if maxAgeHours > 0 {
			maxAge = time.Duration(maxAgeHours) * time.Hour
		}
		if maxAge <= 0 {
			maxAge = defaultCacheMaxAgeHours * time.Hour
		}
		out.MaxAgeHours = int(maxAge / time.Hour)
		out.SnippetsRemoved, out.SummariesRemoved = c.pruneExpiredLocked(now, maxAge)
	default:
		return CacheCleanOutput{}, fmt.Errorf("invalid mode %q (expected \"expired\" or \"all\")", normalizedMode)
	}

	out.SnippetsRemaining = len(c.state.Snippets)
	out.SummariesRemaining = len(c.state.Summaries)
	return out, nil
}

func (c *CacheStore) maybeAutoPruneLocked(now time.Time) {
	if c.mode == cacheModeEphemeral {
		return
	}
	if c.maxAge <= 0 {
		return
	}
	if !c.lastAutoPrune.IsZero() && now.Sub(c.lastAutoPrune) < c.autoPruneEvery {
		return
	}
	c.pruneExpiredLocked(now, c.maxAge)
	c.lastAutoPrune = now
}

func (c *CacheStore) CleanupAfterRequest() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != cacheModeEphemeral {
		return
	}
	if len(c.state.Snippets) == 0 && len(c.state.Summaries) == 0 {
		return
	}
	c.state.Snippets = map[string]snippetCacheEntry{}
	c.state.Summaries = map[string]summaryCacheEntry{}
}

func (c *CacheStore) pruneExpiredLocked(now time.Time, maxAge time.Duration) (int, int) {
	if maxAge <= 0 {
		return 0, 0
	}
	cutoff := now.Add(-maxAge)

	removedSnippets := 0
	for k, v := range c.state.Snippets {
		last := v.AccessedAt
		if last.IsZero() {
			last = v.CreatedAt
		}
		if last.Before(cutoff) {
			delete(c.state.Snippets, k)
			removedSnippets++
		}
	}

	removedSummaries := 0
	for k, v := range c.state.Summaries {
		last := v.AccessedAt
		if last.IsZero() {
			last = v.CreatedAt
		}
		if last.Before(cutoff) {
			delete(c.state.Summaries, k)
			removedSummaries++
		}
	}
	return removedSnippets, removedSummaries
}

func trimSnippets(m map[string]snippetCacheEntry, maxN int) {
	if len(m) <= maxN {
		return
	}
	type pair struct {
		k string
		v snippetCacheEntry
	}
	items := make([]pair, 0, len(m))
	for k, v := range m {
		items = append(items, pair{k: k, v: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v.Hits != items[j].v.Hits {
			return items[i].v.Hits < items[j].v.Hits
		}
		return items[i].v.AccessedAt.Before(items[j].v.AccessedAt)
	})
	for i := 0; i < len(items)-maxN; i++ {
		delete(m, items[i].k)
	}
}

func trimSummaries(m map[string]summaryCacheEntry, maxN int) {
	if len(m) <= maxN {
		return
	}
	type pair struct {
		k string
		v summaryCacheEntry
	}
	items := make([]pair, 0, len(m))
	for k, v := range m {
		items = append(items, pair{k: k, v: v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v.Hits != items[j].v.Hits {
			return items[i].v.Hits < items[j].v.Hits
		}
		left := items[i].v.AccessedAt
		if left.IsZero() {
			left = items[i].v.CreatedAt
		}
		right := items[j].v.AccessedAt
		if right.IsZero() {
			right = items[j].v.CreatedAt
		}
		return left.Before(right)
	})
	for i := 0; i < len(items)-maxN; i++ {
		delete(m, items[i].k)
	}
}
