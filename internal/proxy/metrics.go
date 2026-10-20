package proxy

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks real-time traffic statistics for the proxy.
type Metrics struct {
	TotalRequests    uint64
	ActiveConns      int64
	BytesIn          uint64
	BytesOut         uint64
	clientMu         sync.RWMutex
	ClientsLastSeen  map[string]time.Time
}

// NewMetrics initializes an empty metrics recorder.
func NewMetrics() *Metrics {
	return &Metrics{
		ClientsLastSeen: make(map[string]time.Time),
	}
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

// AddBytes adds transferred byte counts.
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
