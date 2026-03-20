package order

import (
	"sync"
	"testing"
)

func TestTracker_NewID(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	if tracker.IsDuplicate(1) {
		t.Error("first occurrence should not be duplicate")
	}
}

func TestTracker_Duplicate(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	tracker.IsDuplicate(42) // first — not duplicate
	if !tracker.IsDuplicate(42) {
		t.Error("second occurrence should be duplicate")
	}
}

func TestTracker_Reset(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	tracker.IsDuplicate(1)
	tracker.Reset()
	if tracker.IsDuplicate(1) {
		t.Error("after reset, ID 1 should not be duplicate")
	}
}

func TestTracker_Concurrent(t *testing.T) {
	t.Parallel()

	tracker := NewTracker()
	var wg sync.WaitGroup
	const goroutines = 50

	for i := range goroutines {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			tracker.IsDuplicate(id)
		}(int64(i))
	}
	wg.Wait()

	// After all goroutines, IDs 0-49 should all be duplicates.
	for i := range goroutines {
		if !tracker.IsDuplicate(int64(i)) {
			t.Errorf("ID %d should be duplicate after concurrent insert", i)
		}
	}
}
