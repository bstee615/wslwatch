package ipc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bstee615/wslwatch/internal/ipc"
)

// TestServerClientRoundtrip starts a server, sends a status request, and
// verifies a valid response is returned.
func TestServerClientRoundtrip(t *testing.T) {
	handler := func(req ipc.Request) ipc.Response {
		if req.Cmd != "status" {
			return ipc.Response{OK: false, Error: "unknown command"}
		}
		raw, _ := json.Marshal(ipc.StatusData{
			Running:   true,
			Uptime:    "1h 0m",
			StartedAt: time.Now(),
			Distros: []ipc.DistroData{
				{Name: "Ubuntu-22.04", State: "healthy"},
			},
		})
		return ipc.Response{OK: true, Data: raw}
	}

	srv := ipc.NewServer(handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("server Start: %v", err)
	}
	defer srv.Stop()

	// Give the goroutine a moment to start listening.
	time.Sleep(50 * time.Millisecond)

	client := ipc.NewClientWithTimeout(3 * time.Second)
	resp, err := client.Send(ipc.Request{Cmd: "status"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected OK=true, got error: %s", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("expected non-nil Data in response")
	}
}

// TestClientNotRunning verifies that Send returns an error when no server is
// listening on the expected address.
func TestClientNotRunning(t *testing.T) {
	client := ipc.NewClientWithTimeout(200 * time.Millisecond)
	_, err := client.Send(ipc.Request{Cmd: "status"})
	if err == nil {
		t.Fatal("expected error when no server is running, got nil")
	}
}

// TestRequestResponse verifies JSON encoding and decoding of Request/Response.
func TestRequestResponse(t *testing.T) {
	req := ipc.Request{
		Cmd:    "set",
		Distro: "Ubuntu-22.04",
		Key:    "enabled",
		Value:  "true",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal Request: %v", err)
	}

	var decoded ipc.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal Request: %v", err)
	}
	if decoded.Cmd != req.Cmd {
		t.Errorf("Cmd: got %q, want %q", decoded.Cmd, req.Cmd)
	}
	if decoded.Distro != req.Distro {
		t.Errorf("Distro: got %q, want %q", decoded.Distro, req.Distro)
	}
	if decoded.Key != req.Key {
		t.Errorf("Key: got %q, want %q", decoded.Key, req.Key)
	}
	if decoded.Value != req.Value {
		t.Errorf("Value: got %q, want %q", decoded.Value, req.Value)
	}

	resp := ipc.Response{OK: true, Error: ""}
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal Response: %v", err)
	}

	var decodedResp ipc.Response
	if err := json.Unmarshal(data, &decodedResp); err != nil {
		t.Fatalf("json.Unmarshal Response: %v", err)
	}
	if !decodedResp.OK {
		t.Errorf("OK: got false, want true")
	}

	// omitempty: Error field should be absent when empty
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal raw: %v", err)
	}
	if _, present := raw["error"]; present {
		t.Errorf("expected 'error' field to be omitted when empty")
	}
}
