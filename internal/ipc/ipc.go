package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	// PipeName is the named pipe path on Windows.
	// On non-Windows platforms, we use a Unix socket for testing.
	PipeName = `\\.\pipe\wslwatch`

	// UnixSocketPath is the socket path used on non-Windows platforms (for testing).
	UnixSocketPath = "/tmp/wslwatch.sock"
)

// Request represents an IPC request from a client.
type Request struct {
	Cmd    string `json:"cmd"`
	Distro string `json:"distro,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

// Response represents an IPC response from the server.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Handler processes IPC requests and returns responses.
type Handler func(req Request) Response

// Server listens on a named pipe (or unix socket) and handles IPC requests.
type Server struct {
	handler  Handler
	logger   *slog.Logger
	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.Mutex
	closed   bool
}

// NewServer creates a new IPC server with the given handler.
func NewServer(handler Handler, logger *slog.Logger) *Server {
	return &Server{
		handler: handler,
		logger:  logger,
	}
}

// ListenAndServe starts the IPC server. On non-Windows, it uses a Unix socket.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := listen()
	if err != nil {
		return fmt.Errorf("creating IPC listener: %w", err)
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	s.logger.Info("IPC server started")

	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			s.logger.Error("IPC accept error", "error", err)
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// Close shuts down the IPC server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return err
		}
	}
	s.wg.Wait()
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	// Set deadline for the entire interaction
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		s.logger.Error("failed to set connection deadline", "error", err)
		return
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := Response{OK: false, Error: "invalid request: " + err.Error()}
			writeResponse(conn, resp)
			continue
		}

		resp := s.handler(req)
		writeResponse(conn, resp)
	}
}

func writeResponse(w io.Writer, resp Response) {
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = w.Write(data)
}

// SendRequest sends a request to the IPC server and returns the response.
func SendRequest(req Request) (*Response, error) {
	conn, err := dial()
	if err != nil {
		return nil, fmt.Errorf("connecting to IPC server: %w (is wslwatch running?)", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, fmt.Errorf("setting connection deadline: %w", err)
	}

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}
		return nil, fmt.Errorf("empty response from server")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &resp, nil
}
