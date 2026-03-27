//go:build windows

package lock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoubleLock verifies that a second Acquire call fails when the mutex is
// already held. This test only compiles and runs on Windows.
func TestDoubleLock(t *testing.T) {
	first, err := Acquire()
	require.NoError(t, err)
	require.NotNil(t, first)
	defer func() { _ = first.Release() }()

	// Second acquisition must fail.
	second, err := Acquire()
	assert.Error(t, err, "expected error when mutex is already held")
	assert.Nil(t, second)
}
