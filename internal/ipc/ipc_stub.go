//go:build !windows

package ipc

import (
	"net"
	"os"
	"time"
)

const unixSocketPath = "/tmp/wslwatch.sock"

func createListener() (net.Listener, error) {
	// Remove stale socket from a previous run.
	_ = os.Remove(unixSocketPath)
	return net.Listen("unix", unixSocketPath)
}

func dialPipe(timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", unixSocketPath, timeout)
}
