package wsl

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWSLRunnerParseOutput tests that ParseListVerbose handles the output
// from wsl --list --verbose correctly (does not invoke wsl.exe).
func TestWSLRunnerParseOutput(t *testing.T) {
	// Simulate the output that wsl.exe --list --verbose would produce,
	// including Windows CRLF line endings.
	output := "  NAME                   STATE           VERSION\r\n" +
		"* Ubuntu-22.04           Running         2\r\n" +
		"  Ubuntu-20.04           Stopped         2\r\n" +
		"  docker-desktop         Running         2\r\n" +
		"  docker-desktop-data    Stopped         2\r\n"

	distros, err := ParseListVerbose(output)
	assert.NoError(t, err)
	assert.Len(t, distros, 4)

	assert.Equal(t, "Ubuntu-22.04", distros[0].Name)
	assert.Equal(t, StateRunning, distros[0].State)
	assert.True(t, distros[0].Default)
	assert.Equal(t, 2, distros[0].Version)

	assert.Equal(t, "Ubuntu-20.04", distros[1].Name)
	assert.Equal(t, StateStopped, distros[1].State)
	assert.False(t, distros[1].Default)
	assert.Equal(t, 2, distros[1].Version)

	assert.Equal(t, "docker-desktop", distros[2].Name)
	assert.Equal(t, StateRunning, distros[2].State)
	assert.False(t, distros[2].Default)

	assert.Equal(t, "docker-desktop-data", distros[3].Name)
	assert.Equal(t, StateStopped, distros[3].State)
	assert.False(t, distros[3].Default)
}

// TestMockRunnerImplementsInterface verifies MockRunner implements Runner interface.
func TestMockRunnerImplementsInterface(t *testing.T) {
	var _ Runner = (*MockRunner)(nil)
	var _ Runner = NewMockRunner()
}

// TestMockRunnerProbe tests probe success and failure scenarios.
func TestMockRunnerProbe(t *testing.T) {
	t.Run("probe success", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		err := mock.Probe(ctx, "Ubuntu-22.04")
		assert.NoError(t, err)
		assert.Equal(t, 1, mock.ProbeCalls["Ubuntu-22.04"])
	})

	t.Run("probe failure", func(t *testing.T) {
		mock := NewMockRunner()
		expectedErr := errors.New("distro not responding")
		mock.ProbeErr["Ubuntu-22.04"] = expectedErr
		ctx := context.Background()

		err := mock.Probe(ctx, "Ubuntu-22.04")
		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, 1, mock.ProbeCalls["Ubuntu-22.04"])
	})

	t.Run("probe different distros independently", func(t *testing.T) {
		mock := NewMockRunner()
		probeErr := errors.New("connection refused")
		mock.ProbeErr["broken-distro"] = probeErr
		ctx := context.Background()

		err1 := mock.Probe(ctx, "Ubuntu-22.04")
		err2 := mock.Probe(ctx, "broken-distro")

		assert.NoError(t, err1)
		assert.ErrorIs(t, err2, probeErr)
		assert.Equal(t, 1, mock.ProbeCalls["Ubuntu-22.04"])
		assert.Equal(t, 1, mock.ProbeCalls["broken-distro"])
	})

	t.Run("probe call count increments", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			_ = mock.Probe(ctx, "Ubuntu-22.04")
		}
		assert.Equal(t, 3, mock.ProbeCalls["Ubuntu-22.04"])
	})
}

// TestMockRunnerListDistros tests listing distros with the mock runner.
func TestMockRunnerListDistros(t *testing.T) {
	t.Run("list distros returns configured distros", func(t *testing.T) {
		mock := NewMockRunner()
		mock.Distros = []DistroInfo{
			{Name: "Ubuntu-22.04", State: StateRunning, Default: true, Version: 2},
			{Name: "Ubuntu-20.04", State: StateStopped, Default: false, Version: 2},
		}
		ctx := context.Background()

		distros, err := mock.ListDistros(ctx)
		assert.NoError(t, err)
		assert.Len(t, distros, 2)
		assert.Equal(t, "Ubuntu-22.04", distros[0].Name)
		assert.Equal(t, StateRunning, distros[0].State)
		assert.True(t, distros[0].Default)
		assert.Equal(t, "Ubuntu-20.04", distros[1].Name)
		assert.Equal(t, StateStopped, distros[1].State)
	})

	t.Run("list call count increments", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		assert.Equal(t, 0, mock.ListCalls)
		_, _ = mock.ListDistros(ctx)
		_, _ = mock.ListDistros(ctx)
		assert.Equal(t, 2, mock.ListCalls)
	})

	t.Run("empty distro list", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		distros, err := mock.ListDistros(ctx)
		assert.NoError(t, err)
		assert.Empty(t, distros)
	})
}

// TestMockRunnerTerminate tests Terminate call tracking.
func TestMockRunnerTerminate(t *testing.T) {
	t.Run("terminate success", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		err := mock.Terminate(ctx, "Ubuntu-22.04")
		assert.NoError(t, err)
		assert.Equal(t, 1, mock.TerminateCalls["Ubuntu-22.04"])
	})

	t.Run("terminate failure", func(t *testing.T) {
		mock := NewMockRunner()
		expectedErr := errors.New("terminate failed")
		mock.TerminateErr["Ubuntu-22.04"] = expectedErr
		ctx := context.Background()

		err := mock.Terminate(ctx, "Ubuntu-22.04")
		assert.ErrorIs(t, err, expectedErr)
	})
}

// TestMockRunnerStart tests Start call tracking.
func TestMockRunnerStart(t *testing.T) {
	t.Run("start success", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		err := mock.Start(ctx, "Ubuntu-22.04")
		assert.NoError(t, err)
		assert.Equal(t, 1, mock.StartCalls["Ubuntu-22.04"])
	})

	t.Run("start failure", func(t *testing.T) {
		mock := NewMockRunner()
		expectedErr := errors.New("start failed")
		mock.StartErr["Ubuntu-22.04"] = expectedErr
		ctx := context.Background()

		err := mock.Start(ctx, "Ubuntu-22.04")
		assert.ErrorIs(t, err, expectedErr)
	})
}

// TestMockRunnerExec tests Exec with pre-configured results.
func TestMockRunnerExec(t *testing.T) {
	t.Run("exec returns configured result", func(t *testing.T) {
		mock := NewMockRunner()
		mock.ExecResults["Ubuntu-22.04:uname -r"] = "5.15.0-91-generic\n"
		ctx := context.Background()

		result, err := mock.Exec(ctx, "Ubuntu-22.04", "uname", "-r")
		assert.NoError(t, err)
		assert.Equal(t, "5.15.0-91-generic\n", result)
	})

	t.Run("exec returns empty string for unknown command", func(t *testing.T) {
		mock := NewMockRunner()
		ctx := context.Background()

		result, err := mock.Exec(ctx, "Ubuntu-22.04", "unknown-cmd")
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})
}

// encodeUTF16LE is a test helper that encodes a Go string to UTF-16LE bytes.
func encodeUTF16LE(s string) []byte {
	var buf []byte
	for _, r := range s {
		var lo [2]byte
		binary.LittleEndian.PutUint16(lo[:], uint16(r))
		buf = append(buf, lo[:]...)
	}
	return buf
}

func TestDecodeUTF16LE(t *testing.T) {
	t.Run("plain UTF-8 passthrough", func(t *testing.T) {
		input := "hello world"
		assert.Equal(t, input, decodeUTF16LE([]byte(input)))
	})

	t.Run("empty input", func(t *testing.T) {
		assert.Equal(t, "", decodeUTF16LE(nil))
	})

	t.Run("single byte", func(t *testing.T) {
		assert.Equal(t, "x", decodeUTF16LE([]byte("x")))
	})

	t.Run("UTF-16LE without BOM", func(t *testing.T) {
		encoded := encodeUTF16LE("Hello\n")
		result := decodeUTF16LE(encoded)
		assert.Equal(t, "Hello\n", result)
	})

	t.Run("UTF-16LE with BOM", func(t *testing.T) {
		bom := []byte{0xFF, 0xFE}
		encoded := append(bom, encodeUTF16LE("Hello\n")...)
		result := decodeUTF16LE(encoded)
		assert.Equal(t, "Hello\n", result)
	})

	t.Run("real wsl.exe output simulation", func(t *testing.T) {
		// Simulate wsl.exe --list --verbose output in UTF-16LE
		raw := "  NAME                   STATE           VERSION\r\n" +
			"* Debian                 Running         2\r\n" +
			"  docker-desktop         Running         2\r\n" +
			"  Alpine                 Stopped         2\r\n"
		encoded := encodeUTF16LE(raw)

		decoded := decodeUTF16LE(encoded)
		assert.Equal(t, raw, decoded)

		// Verify it parses correctly
		distros, err := ParseListVerbose(decoded)
		assert.NoError(t, err)
		assert.Len(t, distros, 3)
		assert.Equal(t, "Debian", distros[0].Name)
		assert.True(t, distros[0].Default)
		assert.Equal(t, "docker-desktop", distros[1].Name)
		assert.Equal(t, "Alpine", distros[2].Name)
		assert.Equal(t, StateStopped, distros[2].State)
	})
}
