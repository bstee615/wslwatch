//go:build windows

package ipc

import (
	"context"
	"net"
	"time"

	winio "github.com/Microsoft/go-winio"
)

func createListener() (net.Listener, error) {
	return winio.ListenPipe(PipeName, &winio.PipeConfig{
		// Allow Administrators (BA), SYSTEM (SY), and all users (WD) to connect
		// so that non-elevated CLI invocations can reach the service.
		SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;WD)",
		MessageMode:        false,
		InputBufferSize:    4096,
		OutputBufferSize:   4096,
	})
}

func dialPipe(timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return winio.DialPipeContext(ctx, PipeName)
}
