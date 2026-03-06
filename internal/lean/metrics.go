package lean

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type MetricsStore struct {
	mu             sync.Mutex
	path           string
	snapshot       MetricsSnapshot
	latencySamples int64
	latencyTotal   int64
}

func NewMetricsStore(root string) (*MetricsStore, error) {
	stateDir := filepath.Join(root, ".lean-context-mode")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	ms := &MetricsStore{
		path:     filepath.Join(stateDir, "metrics.json"),
		snapshot: MetricsSnapshot{Daily: map[string]DailyMetrics{}, UpdatedAt: time.Now().UTC()},
	}
	_ = ms.load()
	return ms, nil
}

func (m *MetricsStore) load() error {
	b, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	var snap MetricsSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	if snap.Daily == nil {
		snap.Daily = map[string]DailyMetrics{}
	}
	m.snapshot = snap
	return nil
}

func (m *MetricsStore) saveLocked() {
	m.snapshot.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(m.snapshot, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(m.path, b, 0o644)
}

func (m *MetricsStore) Record(r RequestMetric) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.snapshot.RequestsProcessed++
	m.snapshot.TotalOriginal += r.OriginalTokens
	m.snapshot.TotalOptimized += r.OptimizedTokens
	m.snapshot.TotalSaved += r.TokensSaved
	if r.CacheHit {
		m.snapshot.CacheHits++
	} else {
		m.snapshot.CacheMisses++
	}
	m.snapshot.BytesRead += r.BytesRead
	m.snapshot.SnippetsReturned += r.SnippetsReturned
	m.latencySamples++
	m.latencyTotal += r.LatencyMs
	if m.latencySamples > 0 {
		m.snapshot.AvgLatencyMs = float64(m.latencyTotal) / float64(m.latencySamples)
	}

	date := r.Date
	d := m.snapshot.Daily[date]
	d.Date = date
	d.Requests++
	d.OriginalTokens += r.OriginalTokens
	d.OptimizedTokens += r.OptimizedTokens
	d.TokensSaved += r.TokensSaved
	if d.OriginalTokens > 0 {
		d.ReductionPercent = (float64(d.TokensSaved) / float64(d.OriginalTokens)) * 100
	}
	m.snapshot.Daily[date] = d
	m.saveLocked()
}

func (m *MetricsStore) Snapshot() MetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.snapshot
	out.Daily = make(map[string]DailyMetrics, len(m.snapshot.Daily))
	for k, v := range m.snapshot.Daily {
		out.Daily[k] = v
	}
	return out
}

func (m *MetricsStore) StatsText() string {
	s := m.Snapshot()
	today := time.Now().Format("2006-01-02")
	daily := s.Daily[today]
	avgReduction := 0.0
	if s.TotalOriginal > 0 {
		avgReduction = (float64(s.TotalSaved) / float64(s.TotalOriginal)) * 100
	}
	return fmt.Sprintf(
		"lean-context-mode stats\n"+
			"date: %s\n"+
			"daily token savings: %d\n"+
			"total tokens saved: %d\n"+
			"average reduction: %.2f%%\n"+
			"requests processed: %d\n"+
			"cache hits: %d\n"+
			"cache misses: %d\n"+
			"avg latency: %.2f ms\n",
		today,
		daily.TokensSaved,
		s.TotalSaved,
		avgReduction,
		s.RequestsProcessed,
		s.CacheHits,
		s.CacheMisses,
		s.AvgLatencyMs,
	)
}
