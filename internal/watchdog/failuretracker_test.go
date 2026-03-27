package watchdog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewFailureTracker(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 5, 30*time.Second)
	assert.Equal(t, 0, ft.FailureCount())
	assert.False(t, ft.InBackoff())
}

func TestRecordFailure_BelowThreshold(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 5, 30*time.Second)

	for i := 0; i < 4; i++ {
		ft.RecordFailure()
	}

	assert.Equal(t, 4, ft.FailureCount())
	assert.False(t, ft.InBackoff())
}

func TestRecordFailure_AtThreshold_TriggersBackoff(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 5, 30*time.Second)

	for i := 0; i < 5; i++ {
		ft.RecordFailure()
	}

	assert.Equal(t, 5, ft.FailureCount())
	assert.True(t, ft.InBackoff())
}

func TestBackoff_Expires(t *testing.T) {
	now := time.Now()
	ft := NewFailureTracker(60*time.Second, 3, 10*time.Second)
	ft.now = func() time.Time { return now }

	// Record enough failures to trigger backoff
	for i := 0; i < 3; i++ {
		ft.RecordFailure()
	}
	assert.True(t, ft.InBackoff())

	// Advance past backoff duration
	ft.now = func() time.Time { return now.Add(11 * time.Second) }
	assert.False(t, ft.InBackoff())

	// Failures should be reset
	assert.Equal(t, 0, ft.FailureCount())
}

func TestBackoff_ZeroDuration_NeverBacksOff(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 3, 0)

	for i := 0; i < 10; i++ {
		ft.RecordFailure()
	}

	// backoff_duration: 0 means no limit (retry forever)
	assert.False(t, ft.InBackoff())
}

func TestSlidingWindow_OldFailuresPruned(t *testing.T) {
	now := time.Now()
	ft := NewFailureTracker(10*time.Second, 5, 30*time.Second)
	ft.now = func() time.Time { return now }

	// Record failures at current time
	for i := 0; i < 3; i++ {
		ft.RecordFailure()
	}
	assert.Equal(t, 3, ft.FailureCount())

	// Advance time past window
	ft.now = func() time.Time { return now.Add(11 * time.Second) }
	assert.Equal(t, 0, ft.FailureCount())
}

func TestSlidingWindow_MixedOldAndNew(t *testing.T) {
	now := time.Now()
	ft := NewFailureTracker(10*time.Second, 5, 30*time.Second)

	// Record 2 old failures
	ft.now = func() time.Time { return now }
	ft.RecordFailure()
	ft.RecordFailure()

	// Record 2 new failures 8 seconds later
	ft.now = func() time.Time { return now.Add(8 * time.Second) }
	ft.RecordFailure()
	ft.RecordFailure()

	// At t+8s, all 4 should be in window
	assert.Equal(t, 4, ft.FailureCount())

	// At t+11s, only the 2 new ones should remain
	ft.now = func() time.Time { return now.Add(11 * time.Second) }
	assert.Equal(t, 2, ft.FailureCount())
}

func TestReset(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 3, 30*time.Second)

	for i := 0; i < 5; i++ {
		ft.RecordFailure()
	}
	assert.True(t, ft.InBackoff())

	ft.Reset()
	assert.Equal(t, 0, ft.FailureCount())
	assert.False(t, ft.InBackoff())
}

func TestFailures_ReturnsCopy(t *testing.T) {
	ft := NewFailureTracker(60*time.Second, 5, 30*time.Second)
	ft.RecordFailure()
	ft.RecordFailure()

	failures := ft.Failures()
	assert.Len(t, failures, 2)

	// Modifying the copy shouldn't affect the tracker
	failures[0] = time.Time{}
	assert.Len(t, ft.Failures(), 2)
}
