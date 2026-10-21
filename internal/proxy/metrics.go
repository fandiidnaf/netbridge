package proxy

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks real-time traffic statistics for the proxy.
type Metrics struct {
	TotalRequests   uint64
	ActiveConns     int64
	BytesIn         uint64
	BytesOut        uint64
	clientMu        sync.RWMutex
	ClientsLastSeen map[string]time.Time

	speedMu         sync.RWMutex
	lastCalcTime    time.Time
	lastBytesIn     uint64
	lastBytesOut    uint64
	CurrentSpeedIn  uint64
	CurrentSpeedOut uint64
}

// NewMetrics initializes an empty metrics recorder.
func NewMetrics() *Metrics {
	m := &Metrics{
		ClientsLastSeen: make(map[string]time.Time),
		lastCalcTime:    time.Now(),
	}
	return m
}

// RecordConnection increments the active connection counter and registers client activity.
func (m *Metrics) RecordConnection(clientIP string) {
	atomic.AddInt64(&m.ActiveConns, 1)
	atomic.AddUint64(&m.TotalRequests, 1)

	m.clientMu.Lock()
	m.ClientsLastSeen[clientIP] = time.Now()
	m.clientMu.Unlock()
}

// ReleaseConnection decrements the active connection counter.
func (m *Metrics) ReleaseConnection() {
	atomic.AddInt64(&m.ActiveConns, -1)
}

// AddBytes atomically adds transferred byte counts without acquiring locks.
func (m *Metrics) AddBytes(in, out uint64) {
	atomic.AddUint64(&m.BytesIn, in)
	atomic.AddUint64(&m.BytesOut, out)
}

// ActiveClientCount returns the number of distinct clients seen within the active duration.
func (m *Metrics) ActiveClientCount(window time.Duration) int {
	m.clientMu.RLock()
	defer m.clientMu.RUnlock()

	threshold := time.Now().Add(-window)
	count := 0
	for _, lastSeen := range m.ClientsLastSeen {
		if lastSeen.After(threshold) {
			count++
		}
	}
	return count
}

// GetSpeed returns the current transfer speed in bytes per second (in, out).
func (m *Metrics) GetSpeed() (uint64, uint64) {
	m.calculateSpeed()
	m.speedMu.RLock()
	defer m.speedMu.RUnlock()
	return m.CurrentSpeedIn, m.CurrentSpeedOut
}

func (m *Metrics) calculateSpeed() {
	m.speedMu.Lock()
	defer m.speedMu.Unlock()

	now := time.Now()
	elapsed := now.Sub(m.lastCalcTime).Seconds()
	if elapsed < 0.5 {
		return
	}

	totalIn := atomic.LoadUint64(&m.BytesIn)
	totalOut := atomic.LoadUint64(&m.BytesOut)

	deltaIn := totalIn - m.lastBytesIn
	deltaOut := totalOut - m.lastBytesOut

	m.CurrentSpeedIn = uint64(float64(deltaIn) / elapsed)
	m.CurrentSpeedOut = uint64(float64(deltaOut) / elapsed)

	m.lastCalcTime = now
	m.lastBytesIn = totalIn
	m.lastBytesOut = totalOut
}

// FormatBytes formats byte counts into human-readable binary sizes.
func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
