package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestBufferGetStatsDeadlock tests that GetStats doesn't deadlock
func TestBufferGetStatsDeadlock(t *testing.T) {
	// Create a buffer with 1MB limit
	buffer := NewRingBuffer(1024 * 1024)

	// Add some test data
	packet := &CapturedPacket{
		Timestamp:        time.Now(),
		Direction:        "Test",
		Bytes:            100,
		HexDump:          "test hex",
		AsciiStrings:     []string{"test"},
		DetectedProtocol: "Test",
		RawData:          make([]byte, 100),
	}
	buffer.Add(packet)

	// This should complete without deadlock
	done := make(chan bool)
	go func() {
		packets, bytes, usage := buffer.GetStats()
		if packets != 1 {
			t.Errorf("Expected 1 packet, got %d", packets)
		}
		if bytes != 100 {
			t.Errorf("Expected 100 bytes, got %d", bytes)
		}
		if usage == 0 {
			t.Errorf("Expected non-zero usage, got %f", usage)
		}
		done <- true
	}()

	select {
	case <-done:
		// Test passed
	case <-time.After(1 * time.Second):
		t.Fatal("GetStats deadlocked - timeout after 1 second")
	}
}

// TestProxyLifecycle tests the full proxy lifecycle without deadlock
func TestProxyLifecycle(t *testing.T) {
	manager := NewProxyManager()

	// Start a proxy
	err := manager.StartProxy(19090, "localhost", 18080, 1024*1024)
	if err != nil {
		t.Fatalf("Failed to start proxy: %v", err)
	}

	// Wait a bit for proxy to initialize
	time.Sleep(100 * time.Millisecond)

	// This should not deadlock - test list_proxies equivalent
	done := make(chan bool)
	go func() {
		proxies := manager.GetAllProxies()
		if len(proxies) != 1 {
			t.Errorf("Expected 1 proxy, got %d", len(proxies))
		}

		// Simulate what list_proxies handler does
		for _, proxy := range proxies {
			proxy.Stats.mu.RLock()
			bytesCaptured := proxy.Stats.BytesCaptured
			totalConnections := proxy.Stats.Connections
			proxy.Stats.mu.RUnlock()

			activeConnections := proxy.GetConnectionCount()
			// This is where the deadlock occurs!
			_, _, usage := proxy.Buffer.GetStats()

			// Basic checks
			if bytesCaptured != 0 {
				t.Errorf("Expected 0 bytes captured initially, got %d", bytesCaptured)
			}
			if totalConnections != 0 {
				t.Errorf("Expected 0 connections initially, got %d", totalConnections)
			}
			if activeConnections != 0 {
				t.Errorf("Expected 0 active connections initially, got %d", activeConnections)
			}
			if usage != 0 {
				t.Errorf("Expected 0%% usage initially, got %f", usage)
			}
		}
		done <- true
	}()

	select {
	case <-done:
		// Test passed
	case <-time.After(1 * time.Second):
		t.Fatal("List proxies operation deadlocked - timeout after 1 second")
	}

	// Clean up
	_, err = manager.StopProxy(19090)
	if err != nil {
		t.Fatalf("Failed to stop proxy: %v", err)
	}
}

// TestListProxiesHandler tests the actual handler doesn't timeout
func TestListProxiesHandler(t *testing.T) {
	manager := NewProxyManager()

	// Start a proxy
	err := manager.StartProxy(19091, "localhost", 18081, 1024*1024)
	if err != nil {
		t.Fatalf("Failed to start proxy: %v", err)
	}
	defer manager.StopProxy(19091)

	// Create the handler
	handler := NewListProxiesHandler(manager)

	// Create a mock request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_proxies",
			Arguments: map[string]interface{}{},
		},
	}

	// This should complete without timeout
	done := make(chan bool)
	var result *mcp.CallToolResult
	var handlerErr error

	go func() {
		result, handlerErr = handler.Execute(context.Background(), request)
		done <- true
	}()

	select {
	case <-done:
		if handlerErr != nil {
			t.Fatalf("Handler returned error: %v", handlerErr)
		}
		if result == nil {
			t.Fatal("Handler returned nil result")
		}

		// Parse the result
		var response map[string]interface{}
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp.TextContent); ok {
				err := json.Unmarshal([]byte(textContent.Text), &response)
				if err != nil {
					t.Fatalf("Failed to parse response: %v", err)
				}
			}
		}

		// Check we got proxies in response
		if proxies, ok := response["proxies"].([]interface{}); ok {
			if len(proxies) != 1 {
				t.Errorf("Expected 1 proxy in response, got %d", len(proxies))
			}
		} else {
			t.Error("No proxies array in response")
		}

	case <-time.After(2 * time.Second):
		t.Fatal("ListProxiesHandler deadlocked - timeout after 2 seconds")
	}
}

// TestConcurrentBufferOperations tests concurrent access doesn't deadlock
func TestConcurrentBufferOperations(t *testing.T) {
	buffer := NewRingBuffer(1024 * 1024)
	done := make(chan bool, 3)

	// Goroutine 1: Add packets
	go func() {
		for i := 0; i < 10; i++ {
			packet := &CapturedPacket{
				Timestamp: time.Now(),
				Direction: fmt.Sprintf("Test-%d", i),
				Bytes:     100,
				RawData:   make([]byte, 100),
			}
			buffer.Add(packet)
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2: Get stats repeatedly
	go func() {
		for i := 0; i < 10; i++ {
			_, _, _ = buffer.GetStats()
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 3: Get usage percent repeatedly
	go func() {
		for i := 0; i < 10; i++ {
			_ = buffer.GetUsagePercent()
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines with timeout
	for i := 0; i < 3; i++ {
		select {
		case <-done:
			// Good
		case <-time.After(2 * time.Second):
			t.Fatal("Concurrent operations deadlocked")
		}
	}
}
