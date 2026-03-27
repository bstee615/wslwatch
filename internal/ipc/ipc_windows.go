//go:build windows

package ipc

import (
	"net"
	"time"

	winio "github.com/Microsoft/go-winio"
)

func listen() (net.Listener, error) {
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;WD)",
		InputBufferSize:    4096,
		OutputBufferSize:   4096,
	}
	return winio.ListenPipe(PipeName, cfg)
}

func dial() (net.Conn, error) {
	return winio.DialPipe(PipeName, (*time.Duration)(nil))
}
