//go:build !windows

package ipc

import (
	"net"
	"time"
)

const localAddr = "127.0.0.1:47392"

func createListener() (net.Listener, error) {
	return net.Listen("tcp", localAddr)
}

func dialPipe(timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", localAddr, timeout)
}
