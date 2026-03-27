package watchdog

import (
	"sync"
	"time"
)

// FailureTracker tracks failures within a sliding window for a single distro.
type FailureTracker struct {
	mu               sync.Mutex
	window           time.Duration
	threshold        int
	backoffDuration  time.Duration
	failures         []time.Time
	backoffStartedAt *time.Time
	now              func() time.Time // injectable clock for testing
}

// NewFailureTracker creates a new FailureTracker.
func NewFailureTracker(window time.Duration, threshold int, backoffDuration time.Duration) *FailureTracker {
	return &FailureTracker{
		window:          window,
		threshold:       threshold,
		backoffDuration: backoffDuration,
		now:             time.Now,
	}
}

// RecordFailure records a failure at the current time.
func (ft *FailureTracker) RecordFailure() {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	now := ft.now()
	ft.failures = append(ft.failures, now)
	ft.prune(now)

	if len(ft.failures) >= ft.threshold && ft.backoffStartedAt == nil {
		ft.backoffStartedAt = &now
	}
}

// InBackoff returns true if the tracker is currently in a backoff period.
func (ft *FailureTracker) InBackoff() bool {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	if ft.backoffStartedAt == nil {
		return false
	}

	// backoffDuration == 0 means no backoff (retry forever)
	if ft.backoffDuration == 0 {
		return false
	}

	now := ft.now()
	if now.Sub(*ft.backoffStartedAt) >= ft.backoffDuration {
		// Backoff expired, reset
		ft.backoffStartedAt = nil
		ft.failures = nil
		return false
	}
	return true
}

// FailureCount returns the number of failures in the current window.
func (ft *FailureTracker) FailureCount() int {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.prune(ft.now())
	return len(ft.failures)
}

// Reset clears all tracked failures and backoff state.
func (ft *FailureTracker) Reset() {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.failures = nil
	ft.backoffStartedAt = nil
}

// Failures returns a copy of the failure timestamps for display purposes.
func (ft *FailureTracker) Failures() []time.Time {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	ft.prune(ft.now())
	result := make([]time.Time, len(ft.failures))
	copy(result, ft.failures)
	return result
}

// prune removes failures outside the sliding window.
func (ft *FailureTracker) prune(now time.Time) {
	cutoff := now.Add(-ft.window)
	i := 0
	for i < len(ft.failures) && ft.failures[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		ft.failures = ft.failures[i:]
	}
}
