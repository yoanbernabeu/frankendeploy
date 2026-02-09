package ssh

import (
	"testing"
	"time"
)

func TestNewClient_DefaultPort(t *testing.T) {
	client := NewClient("host", "user", 0, "/key")
	if client.Port != 22 {
		t.Errorf("expected default port 22, got %d", client.Port)
	}
}

func TestNewClient_CustomPort(t *testing.T) {
	client := NewClient("host", "user", 2222, "/key")
	if client.Port != 2222 {
		t.Errorf("expected port 2222, got %d", client.Port)
	}
}

func TestNewClient_DefaultOptions(t *testing.T) {
	client := NewClient("host", "user", 22, "/key")
	if client.opts.timeout != DefaultTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultTimeout, client.opts.timeout)
	}
	if client.opts.maxRetries != DefaultMaxRetries {
		t.Errorf("expected maxRetries %d, got %d", DefaultMaxRetries, client.opts.maxRetries)
	}
	if client.opts.initialDelay != DefaultInitialDelay {
		t.Errorf("expected initialDelay %v, got %v", DefaultInitialDelay, client.opts.initialDelay)
	}
	if client.opts.maxDelay != DefaultMaxDelay {
		t.Errorf("expected maxDelay %v, got %v", DefaultMaxDelay, client.opts.maxDelay)
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	client := NewClient("host", "user", 22, "/key",
		WithTimeout(10*time.Second),
		WithRetries(5),
		WithInitialDelay(2*time.Second),
		WithMaxDelay(30*time.Second),
	)

	if client.opts.timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", client.opts.timeout)
	}
	if client.opts.maxRetries != 5 {
		t.Errorf("expected maxRetries 5, got %d", client.opts.maxRetries)
	}
	if client.opts.initialDelay != 2*time.Second {
		t.Errorf("expected initialDelay 2s, got %v", client.opts.initialDelay)
	}
	if client.opts.maxDelay != 30*time.Second {
		t.Errorf("expected maxDelay 30s, got %v", client.opts.maxDelay)
	}
}

func TestIsConnected_NilClient(t *testing.T) {
	client := NewClient("host", "user", 22, "/key")
	if client.IsConnected() {
		t.Error("expected IsConnected() to return false for nil client")
	}
}

func TestReconnect_NoConfig(t *testing.T) {
	client := NewClient("host", "user", 22, "/key")
	err := client.Reconnect()
	if err == nil {
		t.Error("expected error when reconnecting without previous connection")
	}
}

func TestBackoffDelay(t *testing.T) {
	client := NewClient("host", "user", 22, "/key",
		WithInitialDelay(1*time.Second),
		WithMaxDelay(10*time.Second),
	)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},  // 1 * 2^0 = 1s
		{2, 2 * time.Second},  // 1 * 2^1 = 2s
		{3, 4 * time.Second},  // 1 * 2^2 = 4s
		{4, 8 * time.Second},  // 1 * 2^3 = 8s
		{5, 10 * time.Second}, // 1 * 2^4 = 16s, capped at 10s
	}

	for _, tt := range tests {
		got := client.backoffDelay(tt.attempt)
		if got != tt.expected {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestClose_NilClient(t *testing.T) {
	client := NewClient("host", "user", 22, "/key")
	err := client.Close()
	if err != nil {
		t.Errorf("expected nil error for Close on nil client, got %v", err)
	}
}

func TestNewSession_NilClient(t *testing.T) {
	client := NewClient("host", "user", 22, "/key")
	_, err := client.NewSession()
	if err == nil {
		t.Error("expected error when creating session on nil client")
	}
}

func TestNewClient_BackwardCompatible(t *testing.T) {
	// Ensure the old-style call without options still works
	client := NewClient("host", "user", 22, "/key")
	if client.Host != "host" || client.User != "user" || client.Port != 22 || client.KeyPath != "/key" {
		t.Error("backward-compatible NewClient call failed")
	}
}
