package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyManager manages all proxy instances
type ProxyManager struct {
	proxies map[int]*ProxyInstance
	mu      sync.RWMutex
}

// ProxyInstance represents a single proxy
type ProxyInstance struct {
	ListenPort  int
	ForwardHost string
	ForwardPort int
	Listener    net.Listener
	Buffer      *RingBuffer
	Stats       *ProxyStats
	Done        chan struct{}
	StartedAt   time.Time
	connections int32 // atomic counter
}

// ProxyStats tracks proxy statistics
type ProxyStats struct {
	BytesCaptured int64
	Connections   int64
	mu            sync.RWMutex
}

// NewProxyManager creates a new proxy manager
func NewProxyManager() *ProxyManager {
	return &ProxyManager{
		proxies: make(map[int]*ProxyInstance),
	}
}

// StartProxy starts a new proxy instance
func (pm *ProxyManager) StartProxy(listenPort int, forwardHost string, forwardPort int, captureLimit int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if proxy already exists on this port
	if _, exists := pm.proxies[listenPort]; exists {
		return fmt.Errorf("proxy already running on port %d", listenPort)
	}

	// Try to create listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", listenPort))
	if err != nil {
		return fmt.Errorf("failed to bind to port %d: %v", listenPort, err)
	}

	// Create proxy instance
	proxy := &ProxyInstance{
		ListenPort:  listenPort,
		ForwardHost: forwardHost,
		ForwardPort: forwardPort,
		Listener:    listener,
		Buffer:      NewRingBuffer(captureLimit),
		Stats:       &ProxyStats{},
		Done:        make(chan struct{}),
		StartedAt:   time.Now(),
	}

	// Start proxy goroutine
	go proxy.run()

	// Store proxy
	pm.proxies[listenPort] = proxy

	log.Printf("Started proxy on port %d forwarding to %s:%d", listenPort, forwardHost, forwardPort)
	return nil
}

// StopProxy stops a proxy instance
func (pm *ProxyManager) StopProxy(listenPort int) (int64, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proxy, exists := pm.proxies[listenPort]
	if !exists {
		return 0, fmt.Errorf("no proxy running on port %d", listenPort)
	}

	// Signal shutdown
	close(proxy.Done)

	// Close listener
	proxy.Listener.Close()

	// Get final stats
	proxy.Stats.mu.RLock()
	bytesCaptured := proxy.Stats.BytesCaptured
	proxy.Stats.mu.RUnlock()

	// Remove from map
	delete(pm.proxies, listenPort)

	log.Printf("Stopped proxy on port %d (captured %d bytes)", listenPort, bytesCaptured)
	return bytesCaptured, nil
}

// GetProxy returns a proxy instance by port
func (pm *ProxyManager) GetProxy(listenPort int) (*ProxyInstance, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	proxy, exists := pm.proxies[listenPort]
	return proxy, exists
}

// GetAllProxies returns all proxy instances
func (pm *ProxyManager) GetAllProxies() []*ProxyInstance {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*ProxyInstance, 0, len(pm.proxies))
	for _, proxy := range pm.proxies {
		result = append(result, proxy)
	}
	return result
}

// StopAll stops all proxies
func (pm *ProxyManager) StopAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port, proxy := range pm.proxies {
		close(proxy.Done)
		proxy.Listener.Close()
		log.Printf("Stopped proxy on port %d", port)
	}
	pm.proxies = make(map[int]*ProxyInstance)
}

// run is the main proxy loop
func (p *ProxyInstance) run() {
	log.Printf("Proxy listening on :%d, forwarding to %s:%d", p.ListenPort, p.ForwardHost, p.ForwardPort)

	for {
		select {
		case <-p.Done:
			return
		default:
			// Set accept deadline to check for shutdown periodically
			p.Listener.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))

			clientConn, err := p.Listener.Accept()
			if err != nil {
				if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
					continue // Timeout is expected, check for shutdown
				}
				if !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Accept error on port %d: %v", p.ListenPort, err)
				}
				continue
			}

			// Increment connection counter
			atomic.AddInt32(&p.connections, 1)
			p.Stats.mu.Lock()
			p.Stats.Connections++
			p.Stats.mu.Unlock()

			// Handle connection in goroutine
			go p.handleConnection(clientConn)
		}
	}
}

// handleConnection handles a single client connection
func (p *ProxyInstance) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()
	defer atomic.AddInt32(&p.connections, -1)

	// Connect to target server
	serverConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", p.ForwardHost, p.ForwardPort))
	if err != nil {
		log.Printf("Failed to connect to %s:%d: %v", p.ForwardHost, p.ForwardPort, err)
		return
	}
	defer serverConn.Close()

	log.Printf("New connection from %s -> %s:%d", clientConn.RemoteAddr(), p.ForwardHost, p.ForwardPort)

	// Create done channel for this connection
	connDone := make(chan struct{})

	// Proxy data in both directions
	go p.copyWithCapture(serverConn, clientConn, "Client->Server", connDone)
	p.copyWithCapture(clientConn, serverConn, "Server->Client", connDone)

	log.Printf("Connection closed: %s", clientConn.RemoteAddr())
}

// copyWithCapture copies data between connections while capturing to buffer
func (p *ProxyInstance) copyWithCapture(dst, src net.Conn, direction string, done chan struct{}) {
	buf := make([]byte, 4096)

	for {
		// Check for shutdown first, without holding any locks
		select {
		case <-p.Done:
			return
		case <-done:
			return
		default:
		}

		// Set read deadline to check for shutdown periodically
		src.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, err := src.Read(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue // Timeout is expected, check for shutdown
			}
			if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("%s read error: %v", direction, err)
			}
			// Only close done once
			select {
			case <-done:
				// Already closed
			default:
				close(done)
			}
			return
		}

		if n > 0 {
			data := buf[:n]

			// Capture to buffer
			p.captureData(data, direction)

			// Forward the data
			_, err = dst.Write(data)
			if err != nil {
				log.Printf("%s write error: %v", direction, err)
				// Only close done once
				select {
				case <-done:
					// Already closed
				default:
					close(done)
				}
				return
			}
		}
	}
}

// captureData captures data to the ring buffer
func (p *ProxyInstance) captureData(data []byte, direction string) {
	// Update stats
	p.Stats.mu.Lock()
	p.Stats.BytesCaptured += int64(len(data))
	p.Stats.mu.Unlock()

	// Detect protocol
	protocol := detectProtocol(data)

	// Extract ASCII strings
	asciiStrings := extractAsciiStrings(data)

	// Create hex dump (limit to first 200 bytes for display)
	hexDumpData := data
	if len(data) > 200 {
		hexDumpData = data[:200]
	}
	hexDump := hex.Dump(hexDumpData)

	// Add to buffer
	capture := &CapturedPacket{
		Timestamp:        time.Now(),
		Direction:        direction,
		Bytes:            len(data),
		HexDump:          hexDump,
		AsciiStrings:     asciiStrings,
		DetectedProtocol: protocol,
		RawData:          append([]byte(nil), data...), // Copy data
	}

	p.Buffer.Add(capture)
}

// detectProtocol attempts to detect the protocol from packet data
func detectProtocol(data []byte) string {
	dataStr := string(data)

	// HTTP/1.x detection
	if strings.HasPrefix(dataStr, "GET ") || strings.HasPrefix(dataStr, "POST ") ||
		strings.HasPrefix(dataStr, "PUT ") || strings.HasPrefix(dataStr, "DELETE ") ||
		strings.HasPrefix(dataStr, "HEAD ") || strings.HasPrefix(dataStr, "OPTIONS ") ||
		strings.HasPrefix(dataStr, "HTTP/1.") {
		return "HTTP/1.x"
	}

	// HTTP/2 preface
	if strings.HasPrefix(dataStr, "PRI * HTTP/2.0") {
		return "HTTP/2"
	}

	// gRPC paths (common pattern)
	if strings.Contains(dataStr, "/grpc.") || strings.Contains(dataStr, ".proto.") {
		return "gRPC"
	}

	// TLS handshake (simplified detection)
	if len(data) > 5 && data[0] == 0x16 && data[1] == 0x03 {
		return "TLS"
	}

	return "Unknown"
}

// extractAsciiStrings extracts readable ASCII strings from binary data
func extractAsciiStrings(data []byte) []string {
	var strings []string
	var current []byte

	for _, b := range data {
		if b >= 32 && b <= 126 { // Printable ASCII
			current = append(current, b)
		} else {
			if len(current) > 4 { // Minimum string length
				strings = append(strings, string(current))
			}
			current = nil
		}
	}

	// Don't forget the last string
	if len(current) > 4 {
		strings = append(strings, string(current))
	}

	// Limit to first 10 strings
	if len(strings) > 10 {
		strings = strings[:10]
	}

	return strings
}

// GetConnectionCount returns the current number of active connections
func (p *ProxyInstance) GetConnectionCount() int {
	return int(atomic.LoadInt32(&p.connections))
}
