package wsl

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockRunner_Handler(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "--list" {
				return "Ubuntu-22.04", nil
			}
			return "", fmt.Errorf("unexpected args: %v", args)
		},
	}

	out, err := mock.Run(context.Background(), "--list")
	require.NoError(t, err)
	assert.Equal(t, "Ubuntu-22.04", out)

	_, err = mock.Run(context.Background(), "--bad")
	assert.Error(t, err)
}

func TestMockRunner_NoHandler(t *testing.T) {
	mock := &MockRunner{}
	_, err := mock.Run(context.Background(), "--list")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler set")
}

func TestRunWithTimeout_MockSuccess(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			return "ok", nil
		},
	}

	out, err := RunWithTimeout(mock, 5e9, "-e", "echo", "ok")
	require.NoError(t, err)
	assert.Equal(t, "ok", out)
}

func TestRunWithTimeout_MockError(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			return "", fmt.Errorf("mock error")
		},
	}

	_, err := RunWithTimeout(mock, 5e9, "-e", "echo", "ok")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock error")
}

func TestProbeDistro_Success(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			// Expect: -d <distro> -e echo ok
			if len(args) >= 5 && args[0] == "-d" && args[2] == "-e" && args[3] == "echo" && args[4] == "ok" {
				return "ok", nil
			}
			return "", fmt.Errorf("unexpected args: %v", args)
		},
	}

	ok, err := ProbeDistro(mock, "Ubuntu", 5e9)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestProbeDistro_BadOutput(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			return "not ok", nil
		},
	}

	ok, err := ProbeDistro(mock, "Ubuntu", 5e9)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestProbeDistro_Error(t *testing.T) {
	mock := &MockRunner{
		Handler: func(args []string) (string, error) {
			return "", fmt.Errorf("connection refused")
		},
	}

	ok, err := ProbeDistro(mock, "Ubuntu", 5e9)
	assert.Error(t, err)
	assert.False(t, ok)
}
