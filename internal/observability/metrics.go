package observability

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// MetricsCollector collects and aggregates system metrics
type MetricsCollector struct {
	logger         *slog.Logger
	metrics        map[string]Metric
	mu             sync.RWMutex
	startTime      time.Time
}

// Metric is the base interface for all metrics
type Metric interface {
	Name() string
	Type() string
	Value() interface{}
	Record(value float64)
}

// CounterMetric tracks event counts
type CounterMetric struct {
	name  string
	value int64
	mu    sync.Mutex
}

func (c *CounterMetric) Name() string             { return c.name }
func (c *CounterMetric) Type() string             { return "counter" }
func (c *CounterMetric) Value() interface{}       { return c.value }
func (c *CounterMetric) Record(value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += int64(value)
}

// GaugeMetric tracks current value
type GaugeMetric struct {
	name  string
	value float64
	mu    sync.Mutex
}

func (g *GaugeMetric) Name() string             { return g.name }
func (g *GaugeMetric) Type() string             { return "gauge" }
func (g *GaugeMetric) Value() interface{}       { return g.value }
func (g *GaugeMetric) Record(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
}

// HistogramMetric tracks value distribution
type HistogramMetric struct {
	name    string
	buckets []int64
	mu      sync.Mutex
}

func (h *HistogramMetric) Name() string       { return h.name }
func (h *HistogramMetric) Type() string       { return "histogram" }
func (h *HistogramMetric) Value() interface{} { return h.buckets }
func (h *HistogramMetric) Record(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if int(value) < len(h.buckets) {
		h.buckets[int(value)]++
	}
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(logger *slog.Logger) *MetricsCollector {
	if logger == nil {
		logger = slog.Default()
	}

	return &MetricsCollector{
		logger:    logger,
		metrics:   make(map[string]Metric),
		startTime: time.Now(),
	}
}

// RegisterCounter registers a new counter metric
func (mc *MetricsCollector) RegisterCounter(name string) *CounterMetric {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	counter := &CounterMetric{name: name}
	mc.metrics[name] = counter

	mc.logger.Debug("registered counter metric", slog.String("name", name))

	return counter
}

// RegisterGauge registers a new gauge metric
func (mc *MetricsCollector) RegisterGauge(name string) *GaugeMetric {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	gauge := &GaugeMetric{name: name}
	mc.metrics[name] = gauge

	mc.logger.Debug("registered gauge metric", slog.String("name", name))

	return gauge
}

// RecordToolInvocation records tool execution
func (mc *MetricsCollector) RecordToolInvocation(toolName string, success bool, duration time.Duration) {
	metricName := fmt.Sprintf("tool_invocations{tool=%s,status=%s}", toolName, boolToStatus(success))

	counter := mc.getOrCreateCounter(metricName)
	counter.Record(1)

	durationMs := float64(duration.Milliseconds())
	durationMetric := mc.getOrCreateGauge(fmt.Sprintf("tool_duration_ms{tool=%s}", toolName))
	durationMetric.Record(durationMs)

	mc.logger.Debug("recorded tool invocation",
		slog.String("tool", toolName),
		slog.Bool("success", success),
		slog.Duration("duration", duration),
	)
}

// RecordProviderLatency records provider response time
func (mc *MetricsCollector) RecordProviderLatency(provider string, model string, latency time.Duration) {
	metricName := fmt.Sprintf("provider_latency_ms{provider=%s,model=%s}", provider, model)
	gauge := mc.getOrCreateGauge(metricName)
	gauge.Record(float64(latency.Milliseconds()))

	mc.logger.Debug("recorded provider latency",
		slog.String("provider", provider),
		slog.String("model", model),
		slog.Duration("latency", latency),
	)
}

// RecordCost records operation cost
func (mc *MetricsCollector) RecordCost(provider string, user string, amount float64) {
	metricName := fmt.Sprintf("operation_cost_usd{provider=%s,user=%s}", provider, user)
	counter := mc.getOrCreateCounter(metricName)
	counter.Record(amount)

	sessionCostMetric := mc.getOrCreateGauge(fmt.Sprintf("session_cost_usd{user=%s}", user))
	sessionCostMetric.Record(amount)

	mc.logger.Debug("recorded cost",
		slog.String("provider", provider),
		slog.String("user", user),
		slog.Float64("amount", amount),
	)
}

// RecordMemorySearchQuality records memory search effectiveness
func (mc *MetricsCollector) RecordMemorySearchQuality(query string, precision float64, recall float64) {
	precisionMetric := mc.getOrCreateGauge(fmt.Sprintf("memory_search_precision{query=%s}", query))
	precisionMetric.Record(precision)

	recallMetric := mc.getOrCreateGauge(fmt.Sprintf("memory_search_recall{query=%s}", query))
	recallMetric.Record(recall)

	mc.logger.Debug("recorded memory search quality",
		slog.String("query", query),
		slog.Float64("precision", precision),
		slog.Float64("recall", recall),
	)
}

// GetMetric retrieves a metric by name
func (mc *MetricsCollector) GetMetric(name string) Metric {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.metrics[name]
}

// ListMetrics returns all registered metrics
func (mc *MetricsCollector) ListMetrics() []Metric {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	metrics := make([]Metric, 0, len(mc.metrics))
	for _, m := range mc.metrics {
		metrics = append(metrics, m)
	}
	return metrics
}

// ExportPrometheus exports metrics in Prometheus format
func (mc *MetricsCollector) ExportPrometheus() string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	output := "# HELP gorkbot_metrics Gorkbot system metrics\n"
	output += "# TYPE gorkbot_metrics gauge\n"

	for _, metric := range mc.metrics {
		output += fmt.Sprintf("%s %v\n", metric.Name(), metric.Value())
	}

	uptime := time.Since(mc.startTime)
	output += fmt.Sprintf("uptime_seconds %.0f\n", uptime.Seconds())

	return output
}

// Helper methods

func (mc *MetricsCollector) getOrCreateCounter(name string) *CounterMetric {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if existing, ok := mc.metrics[name]; ok {
		if counter, ok := existing.(*CounterMetric); ok {
			return counter
		}
	}

	counter := &CounterMetric{name: name}
	mc.metrics[name] = counter
	return counter
}

func (mc *MetricsCollector) getOrCreateGauge(name string) *GaugeMetric {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if existing, ok := mc.metrics[name]; ok {
		if gauge, ok := existing.(*GaugeMetric); ok {
			return gauge
		}
	}

	gauge := &GaugeMetric{name: name}
	mc.metrics[name] = gauge
	return gauge
}

func boolToStatus(b bool) string {
	if b {
		return "success"
	}
	return "error"
}

// HealthChecker monitors system health
type HealthChecker struct {
	logger  *slog.Logger
	checks  map[string]HealthCheck
	mu      sync.RWMutex
}

// HealthCheck represents a health check result
type HealthCheck struct {
	Name   string
	Status bool
	Error  string
	LastCheck time.Time
}

// NewHealthChecker creates a health checker
func NewHealthChecker(logger *slog.Logger) *HealthChecker {
	if logger == nil {
		logger = slog.Default()
	}

	return &HealthChecker{
		logger: logger,
		checks: make(map[string]HealthCheck),
	}
}

// Check runs a health check
func (hc *HealthChecker) Check(name string, checkFunc func() error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	err := checkFunc()
	status := err == nil
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	hc.checks[name] = HealthCheck{
		Name:      name,
		Status:    status,
		Error:     errMsg,
		LastCheck: time.Now(),
	}

	hc.logger.Debug("health check",
		slog.String("component", name),
		slog.Bool("status", status),
	)
}

// IsHealthy returns true if all checks pass
func (hc *HealthChecker) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	for _, check := range hc.checks {
		if !check.Status {
			return false
		}
	}
	return true
}

// GetStatus returns health status
func (hc *HealthChecker) GetStatus() map[string]interface{} {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	return map[string]interface{}{
		"healthy": hc.IsHealthy(),
		"checks":  hc.checks,
	}
}
