package driver

import (
	"sync"
	"testing"
)

func assertEqual[T comparable](t *testing.T, actual, expected T, description string) {
	if expected != actual {
		t.Errorf("%s: expected %v != actual %v", description, expected, actual)
	}
}

func TestNewInFlightMountTracker(t *testing.T) {
	checker := NewInFlightMountTracker(5)
	assertEqual(t, checker.maxCount, 5, "Max inflight count")
	assertEqual(t, checker.count, 0, "Inflight count")

	checker = NewInFlightMountTracker(UnsetMaxInflightMountCounts)
	assertEqual(t, checker, nil, "Nil checker for negative max inflight mount counts")

	checker = NewInFlightMountTracker(0)
	assertEqual(t, checker, nil, "Nil checker for zero max inflight mount counts")
}

func TestIncrement(t *testing.T) {
	maxFlightCount := int64(2)
	checker := NewInFlightMountTracker(maxFlightCount)

	if !checker.increment() {
		t.Errorf("First increment should succeed with max inflight count=%d", maxFlightCount)
	}
	assertEqual(t, checker.count, 1, "Inflight count after first increment")

	if !checker.increment() {
		t.Errorf("Second increment should succeed with max inflight count=%d", maxFlightCount)
	}
	assertEqual(t, checker.count, 2, "Inflight count after second increment")

	if checker.increment() {
		t.Errorf("Third increment should fail with max inflight count=%d", maxFlightCount)
	}
	assertEqual(t, checker.count, 2, "Inflight count after third increment")
}

func TestDecrement(t *testing.T) {
	maxFlightCount := int64(2)
	checker := NewInFlightMountTracker(maxFlightCount)
	checker.increment()
	checker.increment()

	checker.decrement()
	assertEqual(t, checker.count, 1, "Inflight count after first decrement")

	checker.decrement()
	assertEqual(t, checker.count, 0, "Inflight count after second decrement")

	// Should not decrement further when the count is already zero
	checker.decrement()
	assertEqual(t, checker.count, 0, "Inflight count after decrement when count is already zero")
}

func TestConcurrency(t *testing.T) {
	// Run multiple iterations to increase chance of catching race conditions
	for i := 0; i < 100; i++ {
		maxFlightCount := int64(500)
		checker := NewInFlightMountTracker(maxFlightCount)
		var wg sync.WaitGroup
		var mu sync.Mutex

		numGoRoutinesForIncrement := 400
		for range numGoRoutinesForIncrement {
			wg.Add(1)
			go func() {
				defer wg.Done()
				checker.increment()
			}()
		}

		numGoRoutinesForDecrement := 350
		actualDecrements := 0
		for range numGoRoutinesForDecrement {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if checker.decrement() {
					mu.Lock()
					actualDecrements++
					mu.Unlock()
				}
			}()
		}

		wg.Wait()
		assertEqual(t, checker.count, int64(numGoRoutinesForIncrement-actualDecrements), "inflight count")
	}
}
