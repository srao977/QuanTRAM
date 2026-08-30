package ingestion

import (
	"sync"

	"quantram/internal/config"
	"quantram/internal/domain"
)

type WindowStore struct {
	mu     sync.RWMutex
	limit  int
	bars   map[string][]domain.Bar
	seen   map[string]map[string]struct{}
	lastAt map[string]domain.Bar
}

func NewWindowStore(limit int) *WindowStore {
	if limit <= 0 {
		limit = config.WindowLimit
	}
	return &WindowStore{
		limit:  limit,
		bars:   make(map[string][]domain.Bar),
		seen:   make(map[string]map[string]struct{}),
		lastAt: make(map[string]domain.Bar),
	}
}

func (w *WindowStore) Add(bar domain.Bar) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen[bar.Symbol] == nil {
		w.seen[bar.Symbol] = make(map[string]struct{})
	}
	key := bar.DedupKey()
	if _, exists := w.seen[bar.Symbol][key]; exists {
		return false
	}
	w.seen[bar.Symbol][key] = struct{}{}
	w.bars[bar.Symbol] = append(w.bars[bar.Symbol], bar)
	if len(w.bars[bar.Symbol]) > w.limit {
		evicted := w.bars[bar.Symbol][0]
		delete(w.seen[bar.Symbol], evicted.DedupKey())
		w.bars[bar.Symbol] = w.bars[bar.Symbol][1:]
	}
	current, ok := w.lastAt[bar.Symbol]
	if !ok || bar.IntervalStart.After(current.IntervalStart) {
		w.lastAt[bar.Symbol] = bar
	}
	return true
}

func (w *WindowStore) Last(symbol string) (domain.Bar, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bar, ok := w.lastAt[symbol]
	return bar, ok
}

func (w *WindowStore) Window(symbol string, limit int) []domain.Bar {
	w.mu.RLock()
	defer w.mu.RUnlock()
	items := w.bars[symbol]
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]domain.Bar, limit)
	copy(out, items[len(items)-limit:])
	return out
}
