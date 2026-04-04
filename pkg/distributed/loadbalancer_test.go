package distributed

import (
	"testing"
)

// TestRoundRobinSelector_Rotates verifies round-robin rotation through nodes.
func TestRoundRobinSelector_Rotates(t *testing.T) {
	rr := NewRoundRobinSelector()
	nodes := []string{"a", "b", "c"}

	// Should rotate a, b, c, a, b, c
	expected := []string{"a", "b", "c", "a", "b", "c"}

	for i, exp := range expected {
		selected := rr.Select("TestEvent", nodes)
		if len(selected) != 1 || selected[0] != exp {
			t.Errorf("iteration %d: expected [%s], got %v", i, exp, selected)
		}
	}
}

// TestRoundRobinSelector_SingleNode verifies behavior with single node.
func TestRoundRobinSelector_SingleNode(t *testing.T) {
	rr := NewRoundRobinSelector()
	nodes := []string{"only"}

	for i := 0; i < 5; i++ {
		selected := rr.Select("TestEvent", nodes)
		if len(selected) != 1 || selected[0] != "only" {
			t.Errorf("iteration %d: expected [only], got %v", i, selected)
		}
	}
}

// TestLeastLoadedSelector_PrefersUnknown verifies unknown nodes are preferred.
func TestLeastLoadedSelector_PrefersUnknown(t *testing.T) {
	ll := NewLeastLoadedSelector()
	nodes := []string{"known", "unknown"}

	// Record latency for "known" node
	ll.RecordLatency("known", 100, false)

	// Select should prefer "unknown" (lower latency)
	selected := ll.Select("TestEvent", nodes)
	if len(selected) != 1 || selected[0] != "unknown" {
		t.Errorf("expected [unknown], got %v", selected)
	}
}

// TestLeastLoadedSelector_PrefersLowest verifies lowest latency is preferred.
func TestLeastLoadedSelector_PrefersLowest(t *testing.T) {
	ll := NewLeastLoadedSelector()
	nodes := []string{"fast", "slow"}

	// Record latencies
	ll.RecordLatency("fast", 20, false)
	ll.RecordLatency("fast", 30, false)  // avg: 25ms
	ll.RecordLatency("slow", 80, false)
	ll.RecordLatency("slow", 120, false) // avg: 100ms

	// Select should prefer "fast"
	selected := ll.Select("TestEvent", nodes)
	if len(selected) != 1 || selected[0] != "fast" {
		t.Errorf("expected [fast], got %v", selected)
	}
}

// TestCapabilitySelector_FiltersNodes verifies capability-based filtering.
func TestCapabilitySelector_FiltersNodes(t *testing.T) {
	// Create mock service discovery function
	nodeInfo := map[string]*ServiceDiscoveryEntry{
		"provider_node": {
			NodeId: "provider_node",
			Capabilities: []string{"provider_coordinator", "tool_coordinator"},
		},
		"memory_node": {
			NodeId: "memory_node",
			Capabilities: []string{"memory_coordinator"},
		},
	}

	infoFn := func(nodeID string) *ServiceDiscoveryEntry {
		return nodeInfo[nodeID]
	}

	baseSelector := NewRoundRobinSelector()
	capSelector := NewCapabilitySelector("memory_coordinator", baseSelector, infoFn)

	nodes := []string{"provider_node", "memory_node"}
	selected := capSelector.Select("MemoryQueryEvent", nodes)

	// Should only include memory_node
	if len(selected) != 1 || selected[0] != "memory_node" {
		t.Errorf("expected [memory_node], got %v", selected)
	}
}

// TestCapabilitySelector_FallsBackToAll verifies fallback when no match.
func TestCapabilitySelector_FallsBackToAll(t *testing.T) {
	// No nodes have the required capability
	nodeInfo := map[string]*ServiceDiscoveryEntry{
		"node1": {
			NodeId: "node1",
			Capabilities: []string{"tool_coordinator"},
		},
		"node2": {
			NodeId: "node2",
			Capabilities: []string{"tool_coordinator"},
		},
	}

	infoFn := func(nodeID string) *ServiceDiscoveryEntry {
		return nodeInfo[nodeID]
	}

	baseSelector := NewRoundRobinSelector()
	capSelector := NewCapabilitySelector("memory_coordinator", baseSelector, infoFn)

	nodes := []string{"node1", "node2"}
	selected := capSelector.Select("MemoryQueryEvent", nodes)

	// Should fall back to all nodes (first one via round-robin)
	if len(selected) != 1 || (selected[0] != "node1" && selected[0] != "node2") {
		t.Errorf("expected one of [node1, node2], got %v", selected)
	}
}
