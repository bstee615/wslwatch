package watchdog

import "time"

// FailureTracker is a sliding window failure tracker. Tracks failures for a distro over a time window.
// If failures in the window exceed the threshold, the distro enters backoff.
// After backoff duration expires, the tracker resets.
type FailureTracker struct {
	window     time.Duration // sliding window duration (e.g. 60s)
	threshold  int           // max failures in window before backoff
	backoffDur time.Duration // how long to stay in backoff (0 = no backoff)

	failures     []time.Time // timestamps of recent failures
	backoffUntil time.Time   // zero value if not in backoff

	now func() time.Time // injectable clock for testing
}

// NewFailureTracker creates a new FailureTracker with the given window, threshold, and backoff duration.
func NewFailureTracker(window time.Duration, threshold int, backoffDur time.Duration) *FailureTracker {
	return &FailureTracker{
		window:     window,
		threshold:  threshold,
		backoffDur: backoffDur,
		now:        time.Now,
	}
}

// WithClock sets the clock function for testing purposes.
func (ft *FailureTracker) WithClock(now func() time.Time) *FailureTracker {
	ft.now = now
	return ft
}

// pruneOldFailures removes failures older than the window from the front of the slice.
func (ft *FailureTracker) pruneOldFailures() {
	cutoff := ft.now().Add(-ft.window)
	i := 0
	for i < len(ft.failures) && ft.failures[i].Before(cutoff) {
		i++
	}
	ft.failures = ft.failures[i:]
}

// RecordFailure records a failure at the current time.
// If currently in backoff, the failure is ignored.
func (ft *FailureTracker) RecordFailure() {
	if ft.InBackoff() {
		return
	}

	ft.failures = append(ft.failures, ft.now())
	ft.pruneOldFailures()

	if len(ft.failures) >= ft.threshold && ft.backoffDur > 0 {
		ft.backoffUntil = ft.now().Add(ft.backoffDur)
		ft.failures = nil
	}
}

// InBackoff returns true if the tracker is currently in backoff.
func (ft *FailureTracker) InBackoff() bool {
	if ft.backoffUntil.IsZero() {
		return false
	}
	if ft.now().Before(ft.backoffUntil) {
		return true
	}
	// Backoff has expired; reset state
	ft.backoffUntil = time.Time{}
	ft.failures = nil
	return false
}

// BackoffUntil returns the time when backoff expires (zero if not in backoff).
func (ft *FailureTracker) BackoffUntil() time.Time {
	return ft.backoffUntil
}

// FailureCount returns the number of failures in the current window.
func (ft *FailureTracker) FailureCount() int {
	ft.pruneOldFailures()
	return len(ft.failures)
}

// Reset clears all failure history and exits backoff.
func (ft *FailureTracker) Reset() {
	ft.failures = nil
	ft.backoffUntil = time.Time{}
}
