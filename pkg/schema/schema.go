package schema

import (
	"encoding/json"
	"time"

	"github.com/velariumai/gorkbot/pkg/sense"
)

// GetSchema returns the canonical discovery document for Gorkbot.
// It includes all registered tools and known CLI flags.
func GetSchema(tools []sense.ToolDescriptor) string {
	doc := sense.DiscoveryDoc{
		SchemaVersion:  sense.DiscoveryVersion,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Application:    "Gorkbot",
		ToolCount:      len(tools),
		CategoryCounts: make(map[string]int),
		Tools:          tools,
		Flags:          sense.KnownCLIFlags(),
		SENSEVersion:   "1.0.0",
	}

	for _, t := range tools {
		doc.CategoryCounts[t.Category]++
	}

	bytes, _ := json.MarshalIndent(doc, "", "  ")
	return string(bytes)
}
