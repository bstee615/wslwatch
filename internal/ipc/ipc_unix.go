//go:build !windows

package ipc

import (
	"net"
	"os"
)

func listen() (net.Listener, error) {
	// Clean up old socket if it exists
	_ = os.Remove(UnixSocketPath)
	return net.Listen("unix", UnixSocketPath)
}

func dial() (net.Conn, error) {
	return net.Dial("unix", UnixSocketPath)
}
