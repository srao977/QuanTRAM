package ingestion

import (
	"slices"
	"sync"

	"quantram/internal/config"
	"quantram/internal/domain"
)

// WindowStore retains a bounded, chronological bar history per symbol.
// Its lock owns bars, deduplication keys, and newest-bar indexes as one invariant.
type WindowStore struct {
	mu     sync.RWMutex
	limit  int
	bars   map[string][]domain.Bar
	seen   map[string]map[string]struct{}
	lastAt map[string]domain.Bar
}

// NewWindowStore creates a store with a per-symbol retention limit.
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

// Add inserts or replaces a bar according to generation and finality precedence.
// It returns false when the existing interval representation remains authoritative.
func (w *WindowStore) Add(bar domain.Bar) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen[bar.Symbol] == nil {
		w.seen[bar.Symbol] = make(map[string]struct{})
	}
	key := bar.DedupKey()
	bars := w.bars[bar.Symbol]
	if idx := indexOfKey(bars, key); idx >= 0 {
		if !shouldReplace(bars[idx], bar) {
			return false
		}
		bars[idx] = bar
		w.bars[bar.Symbol] = bars
		w.refreshLast(bar.Symbol)
		return true
	}
	// Out-of-order recovery remains sorted so the last element is always newest.
	insertAt, _ := slices.BinarySearchFunc(bars, bar, func(existing, incoming domain.Bar) int {
		return existing.IntervalStart.Compare(incoming.IntervalStart)
	})
	bars = slices.Insert(bars, insertAt, bar)
	w.seen[bar.Symbol][key] = struct{}{}
	if len(bars) > w.limit {
		evicted := bars[0]
		bars = bars[1:]
		delete(w.seen[bar.Symbol], evicted.DedupKey())
	}
	w.bars[bar.Symbol] = bars
	w.refreshLast(bar.Symbol)
	return true
}

// Last returns the chronologically newest retained bar for a symbol.
func (w *WindowStore) Last(symbol string) (domain.Bar, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	bar, ok := w.lastAt[symbol]
	return bar, ok
}

// Window returns a copy of the newest retained bars in chronological order.
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

func (w *WindowStore) refreshLast(symbol string) {
	items := w.bars[symbol]
	if len(items) == 0 {
		delete(w.lastAt, symbol)
		return
	}
	w.lastAt[symbol] = items[len(items)-1]
}

func indexOfKey(bars []domain.Bar, key string) int {
	for i, bar := range bars {
		if bar.DedupKey() == key {
			return i
		}
	}
	return -1
}

func shouldReplace(existing, incoming domain.Bar) bool {
	if sameGeneration(existing, incoming) {
		return false
	}
	if existing.IsFinal && !incoming.IsFinal {
		return false
	}
	if liveComplete(existing) && incoming.IsBackfilled {
		return false
	}
	return true
}

func liveComplete(bar domain.Bar) bool {
	return bar.IsFinal && bar.QualityStatus == domain.QualityComplete && !bar.IsBackfilled
}

func sameGeneration(left, right domain.Bar) bool {
	if left.MarketSnapshotID != "" && left.MarketSnapshotID == right.MarketSnapshotID {
		return true
	}
	return left.Open == right.Open &&
		left.High == right.High &&
		left.Low == right.Low &&
		left.Close == right.Close &&
		left.Volume == right.Volume &&
		left.IsFinal == right.IsFinal &&
		left.IsBackfilled == right.IsBackfilled &&
		left.QualityStatus == right.QualityStatus
}
