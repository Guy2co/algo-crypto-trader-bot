// Package order provides in-memory order tracking utilities.
package order

import (
	"sync"
)

// Tracker deduplicates fill events by TradeID to prevent double-processing
// on WebSocket reconnects.
type Tracker struct {
	mu   sync.Mutex
	seen map[int64]struct{}
}

// NewTracker returns a new Tracker with an empty seen set.
func NewTracker() *Tracker {
	return &Tracker{
		seen: make(map[int64]struct{}),
	}
}

// IsDuplicate returns true if tradeID has been seen before, and records it
// as seen. Safe for concurrent use.
func (t *Tracker) IsDuplicate(tradeID int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.seen[tradeID]; ok {
		return true
	}
	t.seen[tradeID] = struct{}{}
	return false
}

// Reset clears all seen trade IDs.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seen = make(map[int64]struct{})
}
