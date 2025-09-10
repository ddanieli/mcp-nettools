# MCP Network Tools

A simple MCP (Model Context Protocol) server that provides network proxy debugging capabilities for Claude Code. This tool allows you to start TCP proxies that capture and inspect traffic between clients and servers.

## Features

- **Start multiple proxies** - Each proxy is identified by its listen port
- **Capture traffic** - Intercepts and logs all data passing through the proxy
- **Protocol detection** - Automatically detects HTTP/1.x, HTTP/2, gRPC, and TLS
- **Memory efficient** - Uses ring buffers to limit memory usage
- **Non-blocking** - All operations return immediately
- **Thread-safe** - Supports multiple concurrent connections

## Installation

### Build from source

```bash
cd /path/to/mcp-nettools
make deps    # Download dependencies
make build   # Build binary to bin/mcp-nettools
```

### Install to PATH

```bash
# Option 1: Install using Makefile (builds and installs to /usr/local/bin)
make install

# Option 2: Install to Go bin directory (builds from source)
go install

# Option 3: Add the bin directory to your PATH
export PATH=$PATH:/path/to/mcp-nettools/bin
```

## Makefile Targets

The project includes a Makefile with the following targets:

- `make build` - Build the binary to `bin/mcp-nettools`
- `make test` - Run all tests and validate MCP server functionality
- `make clean` - Remove build artifacts
- `make deps` - Download and tidy dependencies
- `make run` - Build and run the binary
- `make install` - Build and install to `/usr/local/bin`
- `make help` - Show help with all available targets

## Configuration in Claude Code

Add the following to your Claude Code MCP configuration:

```json
{
  "mcp-nettools": {
    "command": "mcp-nettools",
    "args": []
  }
}
```

Or if you're using the binary directly from the project directory:

```json
{
  "mcp-nettools": {
    "command": "/path/to/mcp-nettools/bin/mcp-nettools",
    "args": []
  }
}
```

## Available Tools

### 1. `start_proxy`

Starts a TCP proxy that captures traffic.

**Parameters:**
- `listen_port` (int, required) - Port to listen on
- `forward_host` (string, optional) - Host to forward to (default: "localhost")
- `forward_port` (int, required) - Port to forward to
- `capture_limit` (int, optional) - Max bytes to capture (default: 10485760 = 10MB)

**Example:**
```
Start a proxy on port 8080 forwarding to localhost:3000
```

### 2. `get_proxy_output`

Retrieves captured traffic from one or all proxies.

**Parameters:**
- `listen_port` (int, optional) - Specific proxy to get output from (omit for all)
- `clear_buffer` (bool, optional) - Whether to clear buffer after reading (default: true)

**Example:**
```
Show me the captured traffic from the proxy on port 8080
```

### 3. `stop_proxy`

Stops a running proxy.

**Parameters:**
- `listen_port` (int, required) - Port of the proxy to stop

**Example:**
```
Stop the proxy on port 8080
```

### 4. `list_proxies`

Lists all running proxies with their status.

**Parameters:** None

**Example:**
```
List all running proxies
```

## Use Cases

### Debugging HTTP APIs

```
1. Start a proxy on port 8080 forwarding to your API on port 3000
2. Configure your client to connect to localhost:8080
3. Make requests through the proxy
4. Get the captured output to see request/response details
```

### Debugging gRPC Services

```
1. Start a proxy between your gRPC client and server
2. The tool will detect gRPC traffic patterns
3. View the captured binary data and extracted strings
```

### Monitoring WebSocket Connections

```
1. Start a proxy for your WebSocket server
2. Connect clients through the proxy
3. Monitor the bidirectional traffic flow
```

## Output Format

The captured data includes:
- **Timestamp** - When the packet was captured
- **Direction** - Client->Server or Server->Client
- **Bytes** - Size of the captured data
- **Hex dump** - First 200 bytes in hexadecimal format
- **ASCII strings** - Extracted readable text
- **Protocol** - Detected protocol (HTTP/1.x, HTTP/2, gRPC, TLS, or Unknown)

## Limitations

- TCP only (no UDP support)
- Each proxy instance within an MCP server is limited by the capture buffer size
- Port conflicts are handled at the OS level - you cannot bind to an already-used port

## Troubleshooting

### Port already in use

If you get a "port already in use" error, either:
1. Another process is using that port
2. You already have a proxy running on that port (check with `list_proxies`)
3. Choose a different port

### No output captured

- Ensure traffic is actually flowing through the proxy
- Check that your client is configured to connect to the proxy port
- Verify the buffer hasn't been cleared (`clear_buffer: false` in `get_proxy_output`)

### Memory usage

Each proxy has a default 10MB capture limit. If you need more:
1. Increase the `capture_limit` when starting the proxy
2. Retrieve and clear buffers regularly to prevent data loss

## Development

### Running tests

```bash
make test
```

This runs both Go unit tests and validates that the MCP server has all expected tools registered.

### Building for different platforms

```bash
# Use make build for current platform
make build

# For cross-platform builds, use Go directly with environment variables:
# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/mcp-nettools-darwin-amd64 cmd/*.go
GOOS=darwin GOARCH=arm64 go build -o bin/mcp-nettools-darwin-arm64 cmd/*.go

# Linux
GOOS=linux GOARCH=amd64 go build -o bin/mcp-nettools-linux-amd64 cmd/*.go

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/mcp-nettools.exe cmd/*.go
```

## License

Apache 2.0
