package lock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAcquireRelease runs on all platforms (uses the stub on non-Windows).
func TestAcquireRelease(t *testing.T) {
	l, err := Acquire()
	require.NoError(t, err)
	require.NotNil(t, l)

	err = l.Release()
	assert.NoError(t, err)
}
