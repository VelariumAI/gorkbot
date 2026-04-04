package distributed

import (
	"math"
	"sync"
	"time"
)

// NodeStats tracks per-node latency and failure metrics.
type NodeStats struct {
	NodeID          string
	TotalRequests   int64
	TotalLatencyMS  int64
	FailureCount    int64
	LastSeenAt      time.Time
	mu              sync.RWMutex
}

// AvgLatencyMS returns the average latency in milliseconds for this node.
func (ns *NodeStats) AvgLatencyMS() int64 {
	ns.mu.RLock()
	defer ns.mu.RUnlock()
	if ns.TotalRequests == 0 {
		return 0
	}
	return ns.TotalLatencyMS / ns.TotalRequests
}

// Record records a request's latency and failure status.
func (ns *NodeStats) Record(latencyMS int64, failed bool) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.TotalRequests++
	ns.TotalLatencyMS += latencyMS
	ns.LastSeenAt = time.Now()
	if failed {
		ns.FailureCount++
	}
}

// NodeSelector is the interface for pluggable node routing strategies.
type NodeSelector interface {
	// Select returns the nodes to target for the given event type.
	// Returns a slice of node IDs (never nil, may be empty).
	Select(eventType string, allNodes []string) []string
}

// RoundRobinSelector rotates through nodes on each call.
type RoundRobinSelector struct {
	cursor int
	mu     sync.Mutex
}

// NewRoundRobinSelector creates a new round-robin selector.
func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{cursor: 0}
}

// Select returns the next node in rotation (single node).
func (rr *RoundRobinSelector) Select(eventType string, allNodes []string) []string {
	if len(allNodes) == 0 {
		return []string{}
	}

	rr.mu.Lock()
	defer rr.mu.Unlock()

	selected := allNodes[rr.cursor%len(allNodes)]
	rr.cursor++

	return []string{selected}
}

// LeastLoadedSelector selects the node with the lowest average latency.
// Unknown nodes are treated as having 0ms latency (preferred).
type LeastLoadedSelector struct {
	stats map[string]*NodeStats
	mu    sync.RWMutex
}

// NewLeastLoadedSelector creates a new least-loaded selector.
func NewLeastLoadedSelector() *LeastLoadedSelector {
	return &LeastLoadedSelector{
		stats: make(map[string]*NodeStats),
	}
}

// Select returns the node with the lowest average latency.
func (ll *LeastLoadedSelector) Select(eventType string, allNodes []string) []string {
	if len(allNodes) == 0 {
		return []string{}
	}

	ll.mu.RLock()
	defer ll.mu.RUnlock()

	var best string
	var bestLatency int64 = math.MaxInt64

	for _, nodeID := range allNodes {
		stats, ok := ll.stats[nodeID]
		latency := int64(0)

		if ok {
			latency = stats.AvgLatencyMS()
		}
		// Unknown nodes (latency == 0) are preferred

		if latency < bestLatency {
			bestLatency = latency
			best = nodeID
		}
	}

	if best == "" && len(allNodes) > 0 {
		best = allNodes[0]
	}

	return []string{best}
}

// RecordLatency records latency for a node.
func (ll *LeastLoadedSelector) RecordLatency(nodeID string, latencyMS int64, failed bool) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	stats, ok := ll.stats[nodeID]
	if !ok {
		stats = &NodeStats{NodeID: nodeID}
		ll.stats[nodeID] = stats
	}

	stats.Record(latencyMS, failed)
}

// GetStats returns the current stats for a node.
func (ll *LeastLoadedSelector) GetStats(nodeID string) *NodeStats {
	ll.mu.RLock()
	defer ll.mu.RUnlock()
	return ll.stats[nodeID]
}

// CapabilitySelector filters nodes by required capability and delegates final selection to a base selector.
// Falls back to all nodes if no matching capability found.
type CapabilitySelector struct {
	required string
	base     NodeSelector
	infoFn   func(string) *ServiceDiscoveryEntry
	mu       sync.RWMutex
}

// NewCapabilitySelector creates a new capability-aware selector.
// required: the required capability (e.g., "memory_coordinator")
// base: the base selector to use for final node pick
// infoFn: function to get ServiceDiscoveryEntry for a node (may return nil)
func NewCapabilitySelector(required string, base NodeSelector, infoFn func(string) *ServiceDiscoveryEntry) *CapabilitySelector {
	return &CapabilitySelector{
		required: required,
		base:     base,
		infoFn:   infoFn,
	}
}

// Select filters by capability and delegates to base selector.
func (cs *CapabilitySelector) Select(eventType string, allNodes []string) []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Filter nodes by capability
	var capable []string
	for _, nodeID := range allNodes {
		if cs.infoFn == nil {
			// No capability info available; include all
			capable = append(capable, nodeID)
			continue
		}

		entry := cs.infoFn(nodeID)
		if entry == nil {
			continue
		}

		// Check if node has required capability
		for _, cap := range entry.Capabilities {
			if cap == cs.required {
				capable = append(capable, nodeID)
				break
			}
		}
	}

	// If no matching capabilities, fall back to all nodes
	if len(capable) == 0 {
		capable = allNodes
	}

	// Delegate final selection to base selector
	if cs.base != nil {
		return cs.base.Select(eventType, capable)
	}

	// No base selector; return first capable node
	if len(capable) > 0 {
		return []string{capable[0]}
	}

	return []string{}
}
