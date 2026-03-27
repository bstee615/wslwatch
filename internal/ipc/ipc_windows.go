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
		SecurityDescriptor: "",
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
