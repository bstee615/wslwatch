package ipc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestServerClient_Roundtrip(t *testing.T) {
	handler := func(req Request) Response {
		switch req.Cmd {
		case "status":
			data, _ := json.Marshal(map[string]string{"state": "running"})
			return Response{OK: true, Data: data}
		case "pause":
			return Response{OK: true}
		default:
			return Response{OK: false, Error: "unknown command"}
		}
	}

	server := NewServer(handler, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe(ctx)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test status command
	resp, err := SendRequest(Request{Cmd: "status"})
	require.NoError(t, err)
	assert.True(t, resp.OK)
	assert.NotEmpty(t, resp.Data)

	// Test pause command
	resp, err = SendRequest(Request{Cmd: "pause", Distro: "Ubuntu"})
	require.NoError(t, err)
	assert.True(t, resp.OK)

	// Test unknown command
	resp, err = SendRequest(Request{Cmd: "unknown"})
	require.NoError(t, err)
	assert.False(t, resp.OK)
	assert.Contains(t, resp.Error, "unknown command")

	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestServerClient_ServerNotRunning(t *testing.T) {
	// Remove the socket if it exists
	os.Remove(UnixSocketPath)

	_, err := SendRequest(Request{Cmd: "status"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connecting to IPC server")
}

func TestServer_Close(t *testing.T) {
	handler := func(req Request) Response {
		return Response{OK: true}
	}

	server := NewServer(handler, testLogger())
	ctx, cancel := context.WithCancel(context.Background())

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()
	err := server.Close()
	assert.NoError(t, err)
}
