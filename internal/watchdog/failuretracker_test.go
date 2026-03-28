package watchdog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newFakeClock returns a mutable clock function and an advance function for tests.
func newFakeClock(start time.Time) (func() time.Time, func(time.Duration)) {
	current := start
	return func() time.Time {
			return current
		}, func(d time.Duration) {
			current = current.Add(d)
		}
}

func TestNewFailureTracker(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 5, 10*time.Minute)

	assert.Equal(t, 0, ft.FailureCount())
	assert.False(t, ft.InBackoff())
	assert.True(t, ft.BackoffUntil().IsZero())
}

func TestRecordFailure(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 5, 10*time.Minute).WithClock(now)

	assert.False(t, ft.RecordFailure())
	assert.Equal(t, 1, ft.FailureCount())

	assert.False(t, ft.RecordFailure())
	assert.Equal(t, 2, ft.FailureCount())
}

func TestSlidingWindow(t *testing.T) {
	now, advance := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 10, 0).WithClock(now)

	// Record 3 failures
	ft.RecordFailure()
	ft.RecordFailure()
	ft.RecordFailure()
	assert.Equal(t, 3, ft.FailureCount())

	// Advance past the window so earlier failures expire
	advance(61 * time.Second)

	// Record 1 new failure after window
	ft.RecordFailure()

	// Only 1 failure should be in the window now
	assert.Equal(t, 1, ft.FailureCount())
}

func TestBackoffEntry(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Minute).WithClock(now)

	assert.False(t, ft.RecordFailure())
	assert.False(t, ft.RecordFailure())
	assert.False(t, ft.InBackoff())

	// Third failure crosses the threshold
	assert.True(t, ft.RecordFailure())
	assert.True(t, ft.InBackoff())
	assert.False(t, ft.BackoffUntil().IsZero())
}

func TestBackoffExpiry(t *testing.T) {
	now, advance := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Minute).WithClock(now)

	// Trigger backoff
	ft.RecordFailure()
	ft.RecordFailure()
	ft.RecordFailure()
	assert.True(t, ft.InBackoff())

	// Advance past backoff duration
	advance(11 * time.Minute)
	assert.False(t, ft.InBackoff())
	assert.True(t, ft.BackoffUntil().IsZero())

	// After backoff, failures should accumulate again
	ft.RecordFailure()
	assert.Equal(t, 1, ft.FailureCount())
}

func TestNoBackoff(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 0).WithClock(now)

	// Exceed threshold with backoffDur == 0; should never enter backoff
	// but RecordFailure should return true when threshold is reached
	assert.False(t, ft.RecordFailure())
	assert.False(t, ft.RecordFailure())
	assert.True(t, ft.RecordFailure()) // threshold reached

	assert.False(t, ft.InBackoff())
	assert.Equal(t, 0, ft.FailureCount()) // cleared after threshold
}

func TestResetWindow(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Minute).WithClock(now)

	// Record some failures and trigger backoff
	ft.RecordFailure()
	ft.RecordFailure()
	ft.RecordFailure()
	assert.True(t, ft.InBackoff())

	// ResetWindow clears threshold tracking but keeps display history
	ft.ResetWindow()
	assert.False(t, ft.InBackoff())
	assert.Equal(t, 0, ft.FailureCount())
	assert.True(t, len(ft.FailureTimes()) > 0, "display history should be preserved")
}

func TestReset(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Minute).WithClock(now)

	// Trigger backoff
	ft.RecordFailure()
	ft.RecordFailure()
	ft.RecordFailure()
	assert.True(t, ft.InBackoff())

	ft.Reset()

	assert.False(t, ft.InBackoff())
	assert.Equal(t, 0, ft.FailureCount())
	assert.True(t, ft.BackoffUntil().IsZero())
}

func TestFailureCount(t *testing.T) {
	now, advance := newFakeClock(time.Now())
	ft := NewFailureTracker(30*time.Second, 100, 0).WithClock(now)

	ft.RecordFailure()
	advance(10 * time.Second)
	ft.RecordFailure()
	advance(10 * time.Second)
	ft.RecordFailure()
	// All 3 within the 30s window
	assert.Equal(t, 3, ft.FailureCount())

	// Advance so first failure falls outside the window
	advance(11 * time.Second)
	assert.Equal(t, 2, ft.FailureCount())

	// Advance so second failure also falls outside the window
	advance(10 * time.Second)
	assert.Equal(t, 1, ft.FailureCount())
}

func TestBackoffIgnoresNewFailures(t *testing.T) {
	now, _ := newFakeClock(time.Now())
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Minute).WithClock(now)

	// Trigger backoff
	ft.RecordFailure()
	ft.RecordFailure()
	ft.RecordFailure()
	assert.True(t, ft.InBackoff())

	backoffUntil := ft.BackoffUntil()

	// Attempt more failures during backoff; they should be ignored
	ft.RecordFailure()
	ft.RecordFailure()

	// Backoff end time should be unchanged
	assert.True(t, ft.InBackoff())
	assert.Equal(t, backoffUntil, ft.BackoffUntil())
}
