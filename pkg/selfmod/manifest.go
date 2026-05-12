package selfmod

import (
	"encoding/json"
	"fmt"
	"strings"
)

type DynamicArtifactKind string

const (
	ArtifactTool     DynamicArtifactKind = "dynamic_tool"
	ArtifactSkill    DynamicArtifactKind = "generated_skill"
	ArtifactWorkflow DynamicArtifactKind = "generated_workflow"
	ArtifactHook     DynamicArtifactKind = "generated_hook"
	ArtifactScript   DynamicArtifactKind = "generated_script"
	ArtifactPuterApp DynamicArtifactKind = "generated_puter_app"
	ArtifactManifest DynamicArtifactKind = "tool_plugin_manifest"
	ArtifactDocs     DynamicArtifactKind = "passive_documentation"
)

var allowedKinds = map[DynamicArtifactKind]bool{
	ArtifactTool:     true,
	ArtifactSkill:    true,
	ArtifactWorkflow: true,
	ArtifactHook:     true,
	ArtifactScript:   true,
	ArtifactPuterApp: true,
	ArtifactManifest: true,
	ArtifactDocs:     true,
}

type DynamicArtifactName struct{ value string }

type DynamicArtifactPath struct{ value string }

func (n DynamicArtifactName) String() string { return n.value }
func (p DynamicArtifactPath) String() string { return p.value }

func newArtifactName(raw string) (DynamicArtifactName, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return DynamicArtifactName{}, fmt.Errorf("%s: name", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	return DynamicArtifactName{value: v}, nil
}

func newArtifactPath(raw string) (DynamicArtifactPath, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return DynamicArtifactPath{}, fmt.Errorf("%s: target_paths", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	return DynamicArtifactPath{value: v}, nil
}

type SelfModificationManifest struct {
	name            DynamicArtifactName
	version         string
	description     string
	kind            DynamicArtifactKind
	riskClass       string
	capabilities    []DynamicCapability
	targetPaths     []DynamicArtifactPath
	entrypoints     []string
	expectedEffects []string
	rollbackPlan    string
	tests           []string
	provenance      map[string]any
	raw             map[string]any
}

func (m SelfModificationManifest) Name() DynamicArtifactName { return m.name }
func (m SelfModificationManifest) Kind() DynamicArtifactKind { return m.kind }
func (m SelfModificationManifest) RiskClass() string         { return m.riskClass }
func (m SelfModificationManifest) Version() string           { return m.version }
func (m SelfModificationManifest) Description() string       { return m.description }
func (m SelfModificationManifest) RollbackPlan() string      { return m.rollbackPlan }

func (m SelfModificationManifest) Capabilities() []DynamicCapability {
	out := make([]DynamicCapability, len(m.capabilities))
	copy(out, m.capabilities)
	return out
}

func (m SelfModificationManifest) TargetPaths() []DynamicArtifactPath {
	out := make([]DynamicArtifactPath, len(m.targetPaths))
	copy(out, m.targetPaths)
	return out
}

func (m SelfModificationManifest) ExpectedEffects() []string {
	out := make([]string, len(m.expectedEffects))
	copy(out, m.expectedEffects)
	return out
}

func (m SelfModificationManifest) Raw() map[string]any {
	b, _ := json.Marshal(m.raw)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

func (m SelfModificationManifest) IsMutating() bool {
	return m.kind != ArtifactDocs
}

func ExtractManifest(params map[string]any) (SelfModificationManifest, error) {
	if params == nil {
		return SelfModificationManifest{}, fmt.Errorf(REASON_DYNAMIC_MANIFEST_MISSING)
	}
	for _, key := range []string{"manifest", "tool_manifest", "governance_manifest"} {
		if raw, ok := params[key]; ok {
			return ParseManifest(raw)
		}
	}
	return SelfModificationManifest{}, fmt.Errorf(REASON_DYNAMIC_MANIFEST_MISSING)
}

func ParseManifest(raw any) (SelfModificationManifest, error) {
	decoded, err := parseRawManifest(raw)
	if err != nil {
		return SelfModificationManifest{}, err
	}

	name, _ := decoded["name"].(string)
	n, err := newArtifactName(name)
	if err != nil {
		return SelfModificationManifest{}, err
	}

	kindStr, _ := decoded["artifact_kind"].(string)
	kind := DynamicArtifactKind(strings.TrimSpace(kindStr))
	if !allowedKinds[kind] {
		return SelfModificationManifest{}, fmt.Errorf("%s: artifact_kind", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}

	riskClass, _ := decoded["risk_class"].(string)
	if strings.TrimSpace(riskClass) == "" {
		return SelfModificationManifest{}, fmt.Errorf("%s: risk_class", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}

	capsRaw, ok := decoded["capabilities"]
	if !ok {
		return SelfModificationManifest{}, fmt.Errorf("%s: capabilities", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	caps := parseStringList(capsRaw)
	if len(caps) == 0 && kind != ArtifactDocs {
		return SelfModificationManifest{}, fmt.Errorf("%s: capabilities", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}

	pathsRaw, ok := decoded["target_paths"]
	if !ok {
		return SelfModificationManifest{}, fmt.Errorf("%s: target_paths", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	pathVals := parseStringList(pathsRaw)
	if len(pathVals) == 0 && kind != ArtifactDocs {
		return SelfModificationManifest{}, fmt.Errorf("%s: target_paths", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
	}
	targetPaths := make([]DynamicArtifactPath, 0, len(pathVals))
	for _, p := range pathVals {
		safePath, pErr := newArtifactPath(p)
		if pErr != nil {
			return SelfModificationManifest{}, pErr
		}
		targetPaths = append(targetPaths, safePath)
	}

	effects := parseStringList(decoded["expected_effects"])
	rollback, _ := decoded["rollback_plan"].(string)
	if kind != ArtifactDocs {
		if len(effects) == 0 {
			return SelfModificationManifest{}, fmt.Errorf("%s: expected_effects", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
		}
		if strings.TrimSpace(rollback) == "" {
			return SelfModificationManifest{}, fmt.Errorf("%s: rollback_plan", REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD)
		}
	}

	manifest := SelfModificationManifest{
		name:            n,
		kind:            kind,
		riskClass:       strings.TrimSpace(riskClass),
		capabilities:    toCapabilities(caps),
		targetPaths:     targetPaths,
		expectedEffects: effects,
		rollbackPlan:    strings.TrimSpace(rollback),
		entrypoints:     parseStringList(decoded["entrypoints"]),
		tests:           parseStringList(decoded["tests"]),
		raw:             decoded,
	}
	if v, _ := decoded["version"].(string); v != "" {
		manifest.version = strings.TrimSpace(v)
	}
	if d, _ := decoded["description"].(string); d != "" {
		manifest.description = strings.TrimSpace(d)
	}
	if p, ok := decoded["provenance"].(map[string]any); ok {
		manifest.provenance = copyMap(p)
	}
	return manifest, nil
}

func parseRawManifest(raw any) (map[string]any, error) {
	switch t := raw.(type) {
	case map[string]any:
		if len(t) == 0 {
			return nil, fmt.Errorf(REASON_DYNAMIC_MANIFEST_MISSING)
		}
		return copyMap(t), nil
	case string:
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			return nil, fmt.Errorf(REASON_DYNAMIC_MANIFEST_MISSING)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
			return nil, fmt.Errorf("%s: %v", REASON_DYNAMIC_MANIFEST_INVALID_JSON, err)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf(REASON_DYNAMIC_MANIFEST_MISSING)
		}
		return out, nil
	default:
		return nil, fmt.Errorf(REASON_DYNAMIC_MANIFEST_INVALID_JSON)
	}
}

func parseStringList(raw any) []string {
	switch t := raw.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, v := range t {
			if vv := strings.TrimSpace(v); vv != "" {
				out = append(out, vv)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if v, ok := item.(string); ok {
				if vv := strings.TrimSpace(v); vv != "" {
					out = append(out, vv)
				}
			}
		}
		return out
	case string:
		if v := strings.TrimSpace(t); v != "" {
			return []string{v}
		}
	}
	return nil
}

func toCapabilities(raw []string) []DynamicCapability {
	caps := make([]DynamicCapability, 0, len(raw))
	for _, c := range raw {
		caps = append(caps, normalizeCapability(c))
	}
	return caps
}

func copyMap(in map[string]any) map[string]any {
	b, _ := json.Marshal(in)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}
