package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// StartProxyHandler handles the start_proxy tool
type StartProxyHandler struct {
	manager *ProxyManager
}

// NewStartProxyHandler creates a new start proxy handler
func NewStartProxyHandler(manager *ProxyManager) *StartProxyHandler {
	return &StartProxyHandler{manager: manager}
}

// Execute implements the tool handler
func (h *StartProxyHandler) Execute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse arguments
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid arguments format")
	}

	// Get listen port (required)
	listenPort, ok := getInt(args, "listen_port")
	if !ok {
		return nil, fmt.Errorf("listen_port is required")
	}

	// Get forward host (optional, default: localhost)
	forwardHost, _ := getString(args, "forward_host")
	if forwardHost == "" {
		forwardHost = "localhost"
	}

	// Get forward port (required)
	forwardPort, ok := getInt(args, "forward_port")
	if !ok {
		return nil, fmt.Errorf("forward_port is required")
	}

	// Get capture limit (optional, default: 10MB)
	captureLimit, _ := getInt(args, "capture_limit")
	if captureLimit <= 0 {
		captureLimit = 10 * 1024 * 1024 // 10MB default
	}

	// Start the proxy
	err := h.manager.StartProxy(listenPort, forwardHost, forwardPort, captureLimit)
	if err != nil {
		// Return error as JSON result
		result := map[string]interface{}{
			"error": err.Error(),
		}
		jsonBytes, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}

	// Return success result
	result := map[string]interface{}{
		"status":      "started",
		"listen_port": listenPort,
		"forward_to":  fmt.Sprintf("%s:%d", forwardHost, forwardPort),
	}
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// GetProxyOutputHandler handles the get_proxy_output tool
type GetProxyOutputHandler struct {
	manager *ProxyManager
}

// NewGetProxyOutputHandler creates a new get proxy output handler
func NewGetProxyOutputHandler(manager *ProxyManager) *GetProxyOutputHandler {
	return &GetProxyOutputHandler{manager: manager}
}

// Execute implements the tool handler
func (h *GetProxyOutputHandler) Execute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse arguments
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		args = make(map[string]interface{}) // Empty args is valid
	}

	// Get listen port (optional)
	listenPort, hasPort := getInt(args, "listen_port")

	// Get clear_buffer flag (optional, default: true)
	clearBuffer := true
	if cb, ok := args["clear_buffer"].(bool); ok {
		clearBuffer = cb
	}

	// Collect proxy data
	var proxies []*ProxyInstance
	if hasPort {
		proxy, exists := h.manager.GetProxy(listenPort)
		if !exists {
			result := map[string]interface{}{
				"error": fmt.Sprintf("no proxy running on port %d", listenPort),
			}
			jsonBytes, _ := json.Marshal(result)
			return mcp.NewToolResultText(string(jsonBytes)), nil
		}
		proxies = []*ProxyInstance{proxy}
	} else {
		proxies = h.manager.GetAllProxies()
	}

	// Build response
	proxyResults := make([]map[string]interface{}, 0, len(proxies))

	for _, proxy := range proxies {
		// Get captures
		captures := proxy.Buffer.GetAll()
		captureData := make([]map[string]interface{}, 0, len(captures))

		for _, capture := range captures {
			captureData = append(captureData, map[string]interface{}{
				"timestamp":         capture.Timestamp.Format("2006-01-02T15:04:05.000Z"),
				"direction":         capture.Direction,
				"bytes":             capture.Bytes,
				"hex_dump":          capture.HexDump,
				"ascii_strings":     capture.AsciiStrings,
				"detected_protocol": capture.DetectedProtocol,
			})
		}

		// Get buffer stats
		_, totalBytes, usage := proxy.Buffer.GetStats()

		// Get connection stats
		proxy.Stats.mu.RLock()
		bytesCaptured := proxy.Stats.BytesCaptured
		proxy.Stats.mu.RUnlock()

		proxyResult := map[string]interface{}{
			"listen_port":          proxy.ListenPort,
			"forward_to":           fmt.Sprintf("%s:%d", proxy.ForwardHost, proxy.ForwardPort),
			"captures":             captureData,
			"total_bytes_captured": bytesCaptured,
			"buffer_usage":         fmt.Sprintf("%.1f%%", usage),
			"buffer_bytes":         totalBytes,
		}

		proxyResults = append(proxyResults, proxyResult)

		// Clear buffer if requested
		if clearBuffer {
			proxy.Buffer.Clear()
		}
	}

	result := map[string]interface{}{
		"proxies": proxyResults,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// StopProxyHandler handles the stop_proxy tool
type StopProxyHandler struct {
	manager *ProxyManager
}

// NewStopProxyHandler creates a new stop proxy handler
func NewStopProxyHandler(manager *ProxyManager) *StopProxyHandler {
	return &StopProxyHandler{manager: manager}
}

// Execute implements the tool handler
func (h *StopProxyHandler) Execute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse arguments
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid arguments format")
	}

	// Get listen port (required)
	listenPort, ok := getInt(args, "listen_port")
	if !ok {
		return nil, fmt.Errorf("listen_port is required")
	}

	// Stop the proxy
	bytesCaptured, err := h.manager.StopProxy(listenPort)
	if err != nil {
		result := map[string]interface{}{
			"error": err.Error(),
		}
		jsonBytes, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonBytes)), nil
	}

	// Return success result
	result := map[string]interface{}{
		"status":         "stopped",
		"listen_port":    listenPort,
		"bytes_captured": bytesCaptured,
	}
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// ListProxiesHandler handles the list_proxies tool
type ListProxiesHandler struct {
	manager *ProxyManager
}

// NewListProxiesHandler creates a new list proxies handler
func NewListProxiesHandler(manager *ProxyManager) *ListProxiesHandler {
	return &ListProxiesHandler{manager: manager}
}

// Execute implements the tool handler
func (h *ListProxiesHandler) Execute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	proxies := h.manager.GetAllProxies()

	proxyList := make([]map[string]interface{}, 0, len(proxies))

	for _, proxy := range proxies {
		// Get stats
		proxy.Stats.mu.RLock()
		bytesCaptured := proxy.Stats.BytesCaptured
		totalConnections := proxy.Stats.Connections
		proxy.Stats.mu.RUnlock()

		activeConnections := proxy.GetConnectionCount()
		_, _, usage := proxy.Buffer.GetStats()

		proxyInfo := map[string]interface{}{
			"listen_port":        proxy.ListenPort,
			"forward_to":         fmt.Sprintf("%s:%d", proxy.ForwardHost, proxy.ForwardPort),
			"status":             "running",
			"active_connections": activeConnections,
			"total_connections":  totalConnections,
			"bytes_captured":     bytesCaptured,
			"buffer_usage":       fmt.Sprintf("%.1f%%", usage),
			"started_at":         proxy.StartedAt.Format("2006-01-02T15:04:05.000Z"),
		}

		proxyList = append(proxyList, proxyInfo)
	}

	result := map[string]interface{}{
		"proxies": proxyList,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// Helper functions to extract typed values from arguments

func getInt(args map[string]interface{}, key string) (int, bool) {
	val, exists := args[key]
	if !exists {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	case int64:
		return int(v), true
	default:
		return 0, false
	}
}

func getString(args map[string]interface{}, key string) (string, bool) {
	val, exists := args[key]
	if !exists {
		return "", false
	}

	str, ok := val.(string)
	return str, ok
}
