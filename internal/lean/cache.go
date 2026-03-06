package lean

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
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
	Key       string         `json:"key"`
	Summaries []TraceSummary `json:"summaries"`
	CreatedAt time.Time      `json:"created_at"`
	Hits      int            `json:"hits"`
}

type cacheState struct {
	Snippets  map[string]snippetCacheEntry `json:"snippets"`
	Summaries map[string]summaryCacheEntry `json:"summaries"`
}

type CacheStore struct {
	mu           sync.RWMutex
	path         string
	state        cacheState
	maxSnippets  int
	maxSummaries int

	hits   int
	misses int
}

func NewCacheStore(root string) (*CacheStore, error) {
	cacheDir := filepath.Join(root, ".lean-context-mode", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	cs := &CacheStore{
		path:         filepath.Join(cacheDir, "cache.json"),
		state:        cacheState{Snippets: map[string]snippetCacheEntry{}, Summaries: map[string]summaryCacheEntry{}},
		maxSnippets:  2000,
		maxSummaries: 3000,
	}
	_ = cs.load()
	return cs, nil
}

func (c *CacheStore) load() error {
	b, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	var st cacheState
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	if st.Snippets == nil {
		st.Snippets = map[string]snippetCacheEntry{}
	}
	if st.Summaries == nil {
		st.Summaries = map[string]summaryCacheEntry{}
	}
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
	return nil
}

func (c *CacheStore) saveLocked() {
	b, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.path, b, 0o644)
}

func shortHash(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])[:16]
}

func (c *CacheStore) PutSnippet(content, source string) string {
	h := shortHash(content)
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.state.Snippets[h]
	if ok {
		entry.AccessedAt = now
		entry.Hits++
		c.state.Snippets[h] = entry
		c.hits++
		c.saveLocked()
		return entry.ID
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
	c.saveLocked()
	return entry.ID
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
	c.saveLocked()
	return "cache://snippet/" + entry.ID, true
}

func (c *CacheStore) PutSummary(key string, summaries []TraceSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Summaries[key] = summaryCacheEntry{Key: key, Summaries: summaries, CreatedAt: time.Now().UTC()}
	trimSummaries(c.state.Summaries, c.maxSummaries)
	c.saveLocked()
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
	c.state.Summaries[key] = entry
	c.hits++
	c.saveLocked()
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
		return items[i].v.CreatedAt.Before(items[j].v.CreatedAt)
	})
	for i := 0; i < len(items)-maxN; i++ {
		delete(m, items[i].k)
	}
}
