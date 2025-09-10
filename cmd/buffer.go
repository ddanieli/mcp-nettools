package main

import (
	"sync"
	"time"
)

// CapturedPacket represents a single captured packet
type CapturedPacket struct {
	Timestamp        time.Time `json:"timestamp"`
	Direction        string    `json:"direction"`
	Bytes            int       `json:"bytes"`
	HexDump          string    `json:"hex_dump"`
	AsciiStrings     []string  `json:"ascii_strings"`
	DetectedProtocol string    `json:"detected_protocol"`
	RawData          []byte    `json:"-"` // Not included in JSON output
}

// RingBuffer is a thread-safe circular buffer for captured packets
type RingBuffer struct {
	data        []*CapturedPacket
	maxSize     int // Max size in bytes
	currentSize int // Current size in bytes
	head        int
	tail        int
	count       int
	mu          sync.Mutex
}

// NewRingBuffer creates a new ring buffer with specified max size in bytes
func NewRingBuffer(maxSize int) *RingBuffer {
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // Default 10MB
	}

	// Pre-allocate with reasonable capacity
	initialCapacity := 1000

	return &RingBuffer{
		data:    make([]*CapturedPacket, initialCapacity),
		maxSize: maxSize,
	}
}

// Add adds a packet to the buffer
func (rb *RingBuffer) Add(packet *CapturedPacket) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	packetSize := len(packet.RawData)

	// If this single packet exceeds max size, truncate it
	if packetSize > rb.maxSize {
		packet.RawData = packet.RawData[:rb.maxSize]
		packetSize = rb.maxSize
	}

	// Remove old packets if necessary to make room
	for rb.currentSize+packetSize > rb.maxSize && rb.count > 0 {
		oldPacket := rb.data[rb.tail]
		rb.currentSize -= len(oldPacket.RawData)
		rb.tail = (rb.tail + 1) % len(rb.data)
		rb.count--
	}

	// Grow the buffer if needed
	if rb.count == len(rb.data) {
		rb.grow()
	}

	// Add the new packet
	rb.data[rb.head] = packet
	rb.head = (rb.head + 1) % len(rb.data)
	rb.count++
	rb.currentSize += packetSize
}

// grow doubles the buffer capacity
func (rb *RingBuffer) grow() {
	newData := make([]*CapturedPacket, len(rb.data)*2)

	// Copy existing data in order
	if rb.tail < rb.head {
		copy(newData, rb.data[rb.tail:rb.head])
	} else if rb.count > 0 {
		n := copy(newData, rb.data[rb.tail:])
		copy(newData[n:], rb.data[:rb.head])
	}

	rb.data = newData
	rb.tail = 0
	rb.head = rb.count
}

// GetAll returns all packets in the buffer
func (rb *RingBuffer) GetAll() []*CapturedPacket {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]*CapturedPacket, rb.count)

	if rb.tail < rb.head {
		copy(result, rb.data[rb.tail:rb.head])
	} else {
		n := copy(result, rb.data[rb.tail:])
		copy(result[n:], rb.data[:rb.head])
	}

	return result
}

// Clear removes all packets from the buffer
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.tail = 0
	rb.count = 0
	rb.currentSize = 0

	// Clear references to allow GC
	for i := range rb.data {
		rb.data[i] = nil
	}
}

// getUsagePercentLocked returns the buffer usage as a percentage
// IMPORTANT: This assumes the mutex is already held by the caller
func (rb *RingBuffer) getUsagePercentLocked() float64 {
	if rb.maxSize == 0 {
		return 0
	}
	return float64(rb.currentSize) * 100 / float64(rb.maxSize)
}

// GetUsagePercent returns the buffer usage as a percentage
func (rb *RingBuffer) GetUsagePercent() float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.getUsagePercentLocked()
}

// GetStats returns buffer statistics
func (rb *RingBuffer) GetStats() (packets int, bytes int, usage float64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	// Use the internal helper to avoid deadlock
	return rb.count, rb.currentSize, rb.getUsagePercentLocked()
}
