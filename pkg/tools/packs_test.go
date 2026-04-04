package tools

import "testing"

func TestCommPackExcludesStubTools(t *testing.T) {
	packs := GetToolPacks()
	comm, ok := packs["comm"]
	if !ok {
		t.Fatal("comm pack not found")
	}

	for _, tool := range comm {
		name := tool.Name()
		if name == "send_email" || name == "slack_post" {
			t.Fatalf("stub tool %q must not be registered in comm pack", name)
		}
	}
}
