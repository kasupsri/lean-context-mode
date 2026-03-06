package lean

import (
	"path/filepath"
	"testing"
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
