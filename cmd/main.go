package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	Version = "1.0.0"
)

func main() {
	// Configure logging to stderr to avoid interfering with stdio
	log.SetOutput(os.Stderr)
	log.SetPrefix("[mcp-nettools] ")

	// Create the proxy manager
	manager := NewProxyManager()

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"mcp-nettools",
		fmt.Sprintf("Network proxy debugging tools for MCP (v%s)", Version),
	)

	// Register start_proxy tool
	mcpServer.AddTool(
		mcp.NewTool(
			"start_proxy",
			mcp.WithDescription("Start a network proxy that captures traffic between a client and server"),
			mcp.WithNumber("listen_port",
				mcp.Required(),
				mcp.Description("Port to listen on for incoming connections"),
			),
			mcp.WithString("forward_host",
				mcp.Description("Host to forward connections to (default: localhost)"),
			),
			mcp.WithNumber("forward_port",
				mcp.Required(),
				mcp.Description("Port to forward connections to"),
			),
			mcp.WithNumber("capture_limit",
				mcp.Description("Maximum bytes to capture (default: 10MB)"),
			),
		),
		NewStartProxyHandler(manager).Execute,
	)

	// Register get_proxy_output tool
	mcpServer.AddTool(
		mcp.NewTool(
			"get_proxy_output",
			mcp.WithDescription("Get captured traffic from one or all proxies and optionally clear the buffer"),
			mcp.WithNumber("listen_port",
				mcp.Description("Specific proxy port to get output from (omit for all proxies)"),
			),
			mcp.WithBoolean("clear_buffer",
				mcp.Description("Whether to clear the buffer after reading (default: true)"),
			),
		),
		NewGetProxyOutputHandler(manager).Execute,
	)

	// Register stop_proxy tool
	mcpServer.AddTool(
		mcp.NewTool(
			"stop_proxy",
			mcp.WithDescription("Stop a running proxy"),
			mcp.WithNumber("listen_port",
				mcp.Required(),
				mcp.Description("Port of the proxy to stop"),
			),
		),
		NewStopProxyHandler(manager).Execute,
	)

	// Register list_proxies tool
	mcpServer.AddTool(
		mcp.NewTool(
			"list_proxies",
			mcp.WithDescription("List all running proxies with their status"),
		),
		NewListProxiesHandler(manager).Execute,
	)

	// Handle graceful shutdown
	go func() {
		<-context.Background().Done()
		manager.StopAll()
	}()

	// Start the server using stdio transport
	if err := server.ServeStdio(mcpServer); err != nil {
		log.Fatalf("Failed to start MCP server: %v", err)
	}
}
