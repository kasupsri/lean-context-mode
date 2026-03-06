package lean

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCacheSnippetAndSummary(t *testing.T) {
	root := t.TempDir()
	cache, err := NewCacheStore(root)
	if err != nil {
		t.Fatal(err)
	}
	content := "func Test(){ return }"
	id := cache.PutSnippet(content, "a.go:1")
	if id == "" {
		t.Fatalf("expected snippet id")
	}
	ptr, ok := cache.GetSnippetPointer(content)
	if !ok || ptr == "" {
		t.Fatalf("expected cache pointer")
	}
	if got := cache.HitRate(); got <= 0 {
		t.Fatalf("expected positive hit rate, got %f", got)
	}
	key := "summary_key"
	summaries := []TraceSummary{{Source: "a.go:1-3", Text: "summary"}}
	cache.PutSummary(key, summaries)
	cached, ok := cache.GetSummary(key)
	if !ok || len(cached) != 1 {
		t.Fatalf("expected cached summary")
	}
	if _, err := filepath.Abs(root); err != nil {
		t.Fatal(err)
	}
}

func TestCacheCleanExpiredAndAll(t *testing.T) {
	root := t.TempDir()
	cache, err := NewCacheStore(root)
	if err != nil {
		t.Fatal(err)
	}

	_ = cache.PutSnippet("old content", "old.go:1")
	cache.PutSummary("old_summary", []TraceSummary{{Source: "old.go:1-2", Text: "old"}})

	cache.mu.Lock()
	oldTime := time.Now().UTC().Add(-72 * time.Hour)
	for k, v := range cache.state.Snippets {
		v.AccessedAt = oldTime
		v.CreatedAt = oldTime
		cache.state.Snippets[k] = v
	}
	for k, v := range cache.state.Summaries {
		v.AccessedAt = oldTime
		v.CreatedAt = oldTime
		cache.state.Summaries[k] = v
	}
	cache.mu.Unlock()

	expiredOut, err := cache.Clean("expired", 24)
	if err != nil {
		t.Fatal(err)
	}
	if expiredOut.SnippetsRemoved == 0 {
		t.Fatalf("expected expired snippet entries to be removed")
	}
	if expiredOut.SummariesRemoved == 0 {
		t.Fatalf("expected expired summary entries to be removed")
	}
	if expiredOut.SnippetsRemaining != 0 || expiredOut.SummariesRemaining != 0 {
		t.Fatalf("expected cache to be empty after expired cleanup, got snippets=%d summaries=%d", expiredOut.SnippetsRemaining, expiredOut.SummariesRemaining)
	}

	_ = cache.PutSnippet("new content", "new.go:1")
	cache.PutSummary("new_summary", []TraceSummary{{Source: "new.go:1-2", Text: "new"}})

	allOut, err := cache.Clean("all", 0)
	if err != nil {
		t.Fatal(err)
	}
	if allOut.SnippetsRemaining != 0 || allOut.SummariesRemaining != 0 {
		t.Fatalf("expected cache to be empty after full cleanup, got snippets=%d summaries=%d", allOut.SnippetsRemaining, allOut.SummariesRemaining)
	}
}

func TestCacheCleanupAfterRequestEphemeralDefault(t *testing.T) {
	root := t.TempDir()
	cache, err := NewCacheStore(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = cache.PutSnippet("content", "a.go:1")
	cache.PutSummary("k", []TraceSummary{{Source: "a.go:1-2", Text: "summary"}})

	cache.CleanupAfterRequest()

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.state.Snippets) != 0 || len(cache.state.Summaries) != 0 {
		t.Fatalf("expected ephemeral cache to be cleared after request")
	}
}

func TestCacheCleanupAfterRequestBoundedMode(t *testing.T) {
	t.Setenv("LCM_CACHE_MODE", "bounded")
	root := t.TempDir()
	cache, err := NewCacheStore(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = cache.PutSnippet("content", "a.go:1")
	cache.PutSummary("k", []TraceSummary{{Source: "a.go:1-2", Text: "summary"}})

	cache.CleanupAfterRequest()

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.state.Snippets) == 0 || len(cache.state.Summaries) == 0 {
		t.Fatalf("expected bounded cache to retain entries after request cleanup")
	}
}
