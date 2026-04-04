// Package api provides Prometheus metrics for the API server.
package api

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics tracks API server and connector performance metrics.
type Metrics struct {
	// HTTP API metrics
	RequestsTotal          int64
	RequestsSuccess        int64
	RequestsError          int64
	TotalLatencyMs         int64 // Sum of all request latencies in milliseconds

	// Connector metrics
	ConnectorMessagesTotal int64
	ConnectorMessagesSent  int64
	ConnectorMessagesFailed int64
	ConnectorLatencyMs     int64 // Sum of all connector latencies

	// WebSocket metrics
	WebSocketConnectionsActive int64
	WebSocketConnectionsTotal  int64
	WebSocketMessagesTotal     int64

	// Per-endpoint tracking
	EndpointMetrics map[string]*EndpointMetrics
	mu              sync.RWMutex
}

// EndpointMetrics tracks metrics for a single endpoint.
type EndpointMetrics struct {
	Path            string
	Method          string
	RequestsTotal   int64
	RequestsSuccess int64
	RequestsError   int64
	TotalLatencyMs  int64
	MaxLatencyMs    int64
	MinLatencyMs    int64
}

// NewMetrics creates a new metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{
		EndpointMetrics: make(map[string]*EndpointMetrics),
	}
}

// RecordRequest records a completed HTTP request.
func (m *Metrics) RecordRequest(path, method string, latencyMs int64, success bool) {
	atomic.AddInt64(&m.RequestsTotal, 1)
	atomic.AddInt64(&m.TotalLatencyMs, latencyMs)

	if success {
		atomic.AddInt64(&m.RequestsSuccess, 1)
	} else {
		atomic.AddInt64(&m.RequestsError, 1)
	}

	// Update endpoint-specific metrics
	key := method + " " + path
	m.mu.Lock()
	if em, exists := m.EndpointMetrics[key]; exists {
		atomic.AddInt64(&em.RequestsTotal, 1)
		atomic.AddInt64(&em.TotalLatencyMs, latencyMs)
		if success {
			atomic.AddInt64(&em.RequestsSuccess, 1)
		} else {
			atomic.AddInt64(&em.RequestsError, 1)
		}
		// Update max/min latency
		if latencyMs > em.MaxLatencyMs {
			atomic.StoreInt64(&em.MaxLatencyMs, latencyMs)
		}
		if em.MinLatencyMs == 0 || latencyMs < em.MinLatencyMs {
			atomic.StoreInt64(&em.MinLatencyMs, latencyMs)
		}
	} else {
		m.EndpointMetrics[key] = &EndpointMetrics{
			Path:           path,
			Method:         method,
			RequestsTotal:  1,
			TotalLatencyMs: latencyMs,
			MaxLatencyMs:   latencyMs,
			MinLatencyMs:   latencyMs,
		}
		if success {
			m.EndpointMetrics[key].RequestsSuccess = 1
		} else {
			m.EndpointMetrics[key].RequestsError = 1
		}
	}
	m.mu.Unlock()
}

// RecordConnectorMessage records a message sent via a connector.
func (m *Metrics) RecordConnectorMessage(latencyMs int64, success bool) {
	atomic.AddInt64(&m.ConnectorMessagesTotal, 1)
	atomic.AddInt64(&m.ConnectorLatencyMs, latencyMs)

	if success {
		atomic.AddInt64(&m.ConnectorMessagesSent, 1)
	} else {
		atomic.AddInt64(&m.ConnectorMessagesFailed, 1)
	}
}

// RecordWebSocketConnection records a new WebSocket connection.
func (m *Metrics) RecordWebSocketConnection() {
	atomic.AddInt64(&m.WebSocketConnectionsActive, 1)
	atomic.AddInt64(&m.WebSocketConnectionsTotal, 1)
}

// RecordWebSocketDisconnection records a WebSocket disconnection.
func (m *Metrics) RecordWebSocketDisconnection() {
	active := atomic.AddInt64(&m.WebSocketConnectionsActive, -1)
	if active < 0 {
		atomic.StoreInt64(&m.WebSocketConnectionsActive, 0)
	}
}

// RecordWebSocketMessage records a message received via WebSocket.
func (m *Metrics) RecordWebSocketMessage() {
	atomic.AddInt64(&m.WebSocketMessagesTotal, 1)
}

// GetStats returns a snapshot of current metrics.
func (m *Metrics) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatency := int64(0)
	if atomic.LoadInt64(&m.RequestsTotal) > 0 {
		avgLatency = atomic.LoadInt64(&m.TotalLatencyMs) / atomic.LoadInt64(&m.RequestsTotal)
	}

	connectorAvgLatency := int64(0)
	if atomic.LoadInt64(&m.ConnectorMessagesTotal) > 0 {
		connectorAvgLatency = atomic.LoadInt64(&m.ConnectorLatencyMs) / atomic.LoadInt64(&m.ConnectorMessagesTotal)
	}

	successRate := float64(0)
	total := atomic.LoadInt64(&m.RequestsTotal)
	if total > 0 {
		successRate = float64(atomic.LoadInt64(&m.RequestsSuccess)) / float64(total) * 100
	}

	return map[string]interface{}{
		"http_requests_total":               atomic.LoadInt64(&m.RequestsTotal),
		"http_requests_success":             atomic.LoadInt64(&m.RequestsSuccess),
		"http_requests_error":               atomic.LoadInt64(&m.RequestsError),
		"http_success_rate_percent":         successRate,
		"http_avg_latency_ms":               avgLatency,
		"connector_messages_total":          atomic.LoadInt64(&m.ConnectorMessagesTotal),
		"connector_messages_sent":           atomic.LoadInt64(&m.ConnectorMessagesSent),
		"connector_messages_failed":         atomic.LoadInt64(&m.ConnectorMessagesFailed),
		"connector_avg_latency_ms":          connectorAvgLatency,
		"websocket_connections_active":     atomic.LoadInt64(&m.WebSocketConnectionsActive),
		"websocket_connections_total":      atomic.LoadInt64(&m.WebSocketConnectionsTotal),
		"websocket_messages_total":         atomic.LoadInt64(&m.WebSocketMessagesTotal),
		"uptime":                            time.Now().Unix(),
	}
}

// GetEndpointStats returns metrics for all endpoints.
func (m *Metrics) GetEndpointStats() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var stats []map[string]interface{}
	for _, em := range m.EndpointMetrics {
		avgLatency := int64(0)
		if em.RequestsTotal > 0 {
			avgLatency = em.TotalLatencyMs / em.RequestsTotal
		}

		successRate := float64(0)
		if em.RequestsTotal > 0 {
			successRate = float64(em.RequestsSuccess) / float64(em.RequestsTotal) * 100
		}

		stats = append(stats, map[string]interface{}{
			"endpoint":           em.Method + " " + em.Path,
			"requests_total":     em.RequestsTotal,
			"requests_success":   em.RequestsSuccess,
			"requests_error":     em.RequestsError,
			"success_rate_pct":   successRate,
			"avg_latency_ms":     avgLatency,
			"max_latency_ms":     em.MaxLatencyMs,
			"min_latency_ms":     em.MinLatencyMs,
		})
	}
	return stats
}
