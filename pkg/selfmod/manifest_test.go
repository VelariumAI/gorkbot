package selfmod

import "testing"

func TestParseManifestValid(t *testing.T) {
	m, err := ParseManifest(map[string]any{
		"name":             "safe_tool",
		"artifact_kind":    "dynamic_tool",
		"risk_class":       "moderate",
		"capabilities":     []any{"dynamic.skill.stage"},
		"target_paths":     []any{".gorkbot/staging/tools/safe_tool.go"},
		"expected_effects": []any{"staged tool file"},
		"rollback_plan":    "delete staged file",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if m.Name().String() != "safe_tool" {
		t.Fatalf("unexpected name: %s", m.Name().String())
	}
}

func TestParseManifestEmptyFails(t *testing.T) {
	if _, err := ParseManifest(""); err == nil {
		t.Fatal("expected empty manifest failure")
	}
}

func TestParseManifestInvalidJSONFails(t *testing.T) {
	if _, err := ParseManifest("{not-json"); err == nil {
		t.Fatal("expected invalid JSON failure")
	}
}

func TestParseManifestMissingRequiredFieldFails(t *testing.T) {
	if _, err := ParseManifest(map[string]any{"name": "x"}); err == nil {
		t.Fatal("expected required field failure")
	}
}

func TestParseManifestUnknownKindFails(t *testing.T) {
	if _, err := ParseManifest(map[string]any{
		"name": "x", "artifact_kind": "unknown", "risk_class": "low",
		"capabilities": []any{"dynamic.skill.stage"}, "target_paths": []any{".gorkbot/staging/a"},
		"expected_effects": []any{"x"}, "rollback_plan": "y",
	}); err == nil {
		t.Fatal("expected unknown kind failure")
	}
}

func TestParseManifestEmptyCapabilitiesFailsForMutating(t *testing.T) {
	if _, err := ParseManifest(map[string]any{
		"name": "x", "artifact_kind": "dynamic_tool", "risk_class": "low",
		"capabilities": []any{}, "target_paths": []any{".gorkbot/staging/a"},
		"expected_effects": []any{"x"}, "rollback_plan": "y",
	}); err == nil {
		t.Fatal("expected capabilities failure")
	}
}

func TestParseManifestExpectedEffectsRequired(t *testing.T) {
	if _, err := ParseManifest(map[string]any{
		"name": "x", "artifact_kind": "dynamic_tool", "risk_class": "low",
		"capabilities": []any{"dynamic.skill.stage"}, "target_paths": []any{".gorkbot/staging/a"},
		"rollback_plan": "y",
	}); err == nil {
		t.Fatal("expected expected_effects failure")
	}
}

func TestParseManifestRollbackRequired(t *testing.T) {
	if _, err := ParseManifest(map[string]any{
		"name": "x", "artifact_kind": "dynamic_tool", "risk_class": "low",
		"capabilities": []any{"dynamic.skill.stage"}, "target_paths": []any{".gorkbot/staging/a"},
		"expected_effects": []any{"x"},
	}); err == nil {
		t.Fatal("expected rollback_plan failure")
	}
}
