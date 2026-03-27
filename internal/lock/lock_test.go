package lock

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLock_AcquireAndRelease(t *testing.T) {
	// Clean up any existing lock
	os.Remove("/tmp/wslwatch.lock")

	l := NewLock()

	acquired, err := l.Acquire()
	require.NoError(t, err)
	assert.True(t, acquired)

	// Release
	err = l.Release()
	assert.NoError(t, err)
}

func TestLock_DoubleAcquire(t *testing.T) {
	os.Remove("/tmp/wslwatch.lock")

	l1 := NewLock()
	l2 := NewLock()

	acquired, err := l1.Acquire()
	require.NoError(t, err)
	assert.True(t, acquired)

	// Second acquire should fail
	acquired2, err := l2.Acquire()
	require.NoError(t, err)
	assert.False(t, acquired2)

	// Clean up
	err = l1.Release()
	assert.NoError(t, err)
}

func TestLock_ReleaseAllowsReacquire(t *testing.T) {
	os.Remove("/tmp/wslwatch.lock")

	l := NewLock()

	acquired, err := l.Acquire()
	require.NoError(t, err)
	assert.True(t, acquired)

	err = l.Release()
	require.NoError(t, err)

	// Should be able to acquire again
	acquired, err = l.Acquire()
	require.NoError(t, err)
	assert.True(t, acquired)

	err = l.Release()
	assert.NoError(t, err)
}
