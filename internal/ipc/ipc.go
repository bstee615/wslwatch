package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

const PipeName = `\\.\pipe\wslwatch`
const PipeNameUnix = "/tmp/wslwatch.sock" // fallback for non-Windows

// Request is sent by CLI clients.
type Request struct {
	Cmd    string `json:"cmd"`
	Distro string `json:"distro,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}

// Response is sent back by the server.
type Response struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// StatusData is the data field in a status response.
type StatusData struct {
	Running   bool       `json:"running"`
	Uptime    string     `json:"uptime"`     // formatted duration
	StartedAt time.Time  `json:"started_at"`
	Distros   []DistroData `json:"distros"`
}

// DistroData holds per-distro status information.
type DistroData struct {
	Name         string      `json:"name"`
	State        string      `json:"state"`
	Uptime       string      `json:"uptime"`
	RestartCount int         `json:"restart_count"`
	InBackoff    bool        `json:"in_backoff"`
	BackoffUntil time.Time   `json:"backoff_until,omitempty"`
	Exhausted    bool        `json:"exhausted"`
	FailureTimes []time.Time `json:"failure_times,omitempty"`
}

// Handler is called by the server to handle incoming requests.
type Handler func(req Request) Response

// Server listens on the named pipe and dispatches requests to a Handler.
type Server struct {
	handler Handler
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewServer creates a new Server with the given handler.
func NewServer(handler Handler) *Server {
	return &Server{
		handler: handler,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start begins listening on the named pipe and serving requests in goroutines.
// Returns error if the pipe cannot be created.
func (s *Server) Start() error {
	ln, err := createListener()
	if err != nil {
		return err
	}

	go func() {
		defer close(s.doneCh)
		defer ln.Close()

		// Close the listener when stop is signalled so Accept unblocks.
		go func() {
			<-s.stopCh
			ln.Close()
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-s.stopCh:
					return
				default:
					// transient error – keep going
					continue
				}
			}
			go s.handleConn(conn)
		}
	}()

	return nil
}

// Stop signals the server to stop accepting new connections and waits until
// the accept loop exits.
func (s *Server) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

// handleConn handles a single connection: read one request, call handler,
// write one response, then close.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	line := scanner.Bytes()

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return
	}

	resp := s.handler(req)

	enc := json.NewEncoder(conn)
	_ = enc.Encode(resp) // writes JSON followed by '\n'
}

// Client sends requests to a running wslwatch service.
type Client struct {
	timeout time.Duration
}

// NewClient returns a Client with a default 5-second timeout.
func NewClient() *Client {
	return &Client{timeout: 5 * time.Second}
}

// NewClientWithTimeout returns a Client with the specified timeout.
func NewClientWithTimeout(timeout time.Duration) *Client {
	return &Client{timeout: timeout}
}

// Send sends a request and returns the response.
// Returns error if the service is not running or the connection fails.
func (c *Client) Send(req Request) (*Response, error) {
	conn, err := dialPipe(c.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		return nil, err
	}

	var resp Response
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IsRunning checks if the watchdog service is reachable.
func (c *Client) IsRunning() bool {
	conn, err := dialPipe(c.timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
