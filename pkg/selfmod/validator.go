package selfmod

import (
	"encoding/json"
	"strings"
	"time"
)

type ValidateInput struct {
	OperationID    string
	ToolName       string
	Mode           string
	Parameters     map[string]any
	GeneratedGoSrc string
}

var forbiddenAuthorityKeys = map[string]bool{
	"verified":              true,
	"certified":             true,
	"trusted":               true,
	"approved":              true,
	"safe":                  true,
	"privileged":            true,
	"authorized":            true,
	"authority":             true,
	"root":                  true,
	"admin":                 true,
	"superuser":             true,
	"system":                true,
	"owner":                 true,
	"bypass":                true,
	"bypass_governance":     true,
	"governance_exempt":     true,
	"no_approval_required":  true,
	"approval_granted":      true,
	"user_approved":         true,
	"vcse_verified":         true,
	"vcse_certified":        true,
	"ledger_verified":       true,
	"render_verified":       true,
	"production_ready":      true,
	"auto_promote":          true,
	"promote_now":           true,
	"allow_credentials":     true,
	"allow_private_network": true,
	"allow_host_bridge":     true,
	"unrestricted_network":  true,
	"disable_sandbox":       true,
	"disable_audit":         true,
	"disable_logging":       true,
}

func ValidateDynamicProposal(input ValidateInput) DynamicValidationDecision {
	decision := DynamicValidationDecision{
		Allowed: true,
	}

	rawManifest, hasManifest := extractManifestRaw(input.Parameters)
	if !hasManifest {
		decision.Allowed = false
		decision.HardBlock = true
		decision.ReasonCode = REASON_DYNAMIC_MANIFEST_MISSING
		decision.Issues = []string{REASON_DYNAMIC_MANIFEST_MISSING}
		decision.Receipt = receiptFrom(input, nil, decision, "")
		return decision
	}
	if decoded, err := parseRawManifest(rawManifest); err == nil {
		if blocked, path := containsForbiddenAuthority(decoded, ""); blocked {
			decision.Allowed = false
			decision.HardBlock = true
			decision.ReasonCode = REASON_DYNAMIC_AUTHORITY_FIELD_FORBIDDEN
			decision.Issues = []string{"forbidden authority field: " + path}
			decision.Receipt = receiptFrom(input, nil, decision, "")
			return decision
		}
	}

	manifest, err := ExtractManifest(input.Parameters)
	if err != nil {
		decision.Allowed = false
		decision.HardBlock = true
		decision.ReasonCode = errorReasonCode(err.Error(), REASON_DYNAMIC_MANIFEST_MISSING)
		decision.Issues = []string{err.Error()}
		decision.Receipt = receiptFrom(input, nil, decision, "")
		return decision
	}

	requiresApproval, hardBlock, capReason, capIssues := classifyCapabilities(manifest.Capabilities())
	if hardBlock {
		decision.Allowed = false
		decision.HardBlock = true
		decision.ReasonCode = capReason
		decision.Issues = append(decision.Issues, capIssues...)
		decision.Manifest = &manifest
		decision.Receipt = receiptFrom(input, &manifest, decision, "")
		return decision
	}
	if requiresApproval {
		decision.RequiresApproval = true
		decision.ReasonCode = capReason
		decision.Issues = append(decision.Issues, capIssues...)
	}

	for _, p := range manifest.TargetPaths() {
		approval, blocked, reason, issue := validateTargetPath(p)
		if issue != "" {
			decision.Issues = append(decision.Issues, issue)
		}
		if blocked {
			decision.Allowed = false
			decision.HardBlock = true
			decision.ReasonCode = reason
			decision.Manifest = &manifest
			decision.Receipt = receiptFrom(input, &manifest, decision, "")
			return decision
		}
		if approval {
			decision.RequiresApproval = true
			if decision.ReasonCode == "" {
				decision.ReasonCode = REASON_DYNAMIC_PROMOTION_REQUIRES_APPROVAL
			}
		}
	}

	source := strings.TrimSpace(input.GeneratedGoSrc)
	if source == "" {
		source = extractGeneratedSource(input.Parameters)
	}
	if source != "" {
		scan := StaticScanGoSource(source, manifest.Capabilities())
		if !scan.Allowed {
			decision.Allowed = false
			decision.HardBlock = true
			decision.ReasonCode = scan.ReasonCode
			decision.Issues = append(decision.Issues, scan.Issues...)
			decision.Manifest = &manifest
			decision.Receipt = receiptFrom(input, &manifest, decision, hashAny(source))
			return decision
		}
	}

	if strings.EqualFold(strings.TrimSpace(input.Mode), "GOVERNANCE_CORRECTNESS") && manifest.IsMutating() {
		if len(manifest.ExpectedEffects()) == 0 || strings.TrimSpace(manifest.RollbackPlan()) == "" {
			decision.Allowed = false
			decision.HardBlock = true
			decision.ReasonCode = REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD
			decision.Issues = append(decision.Issues, "correctness mode requires expected_effects and rollback_plan")
			decision.Manifest = &manifest
			decision.Receipt = receiptFrom(input, &manifest, decision, hashAny(source))
			return decision
		}
	}

	decision.Manifest = &manifest
	if decision.ReasonCode == "" {
		decision.ReasonCode = "REASON_POLICY_ALLOWED"
	}
	decision.Receipt = receiptFrom(input, &manifest, decision, hashAny(source))
	return decision
}

func extractGeneratedSource(params map[string]any) string {
	if params == nil {
		return ""
	}
	for _, key := range []string{"go_source", "generated_source", "source", "tool_source"} {
		if v, ok := params[key].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func extractManifestRaw(params map[string]any) (any, bool) {
	if params == nil {
		return nil, false
	}
	for _, key := range []string{"manifest", "tool_manifest", "governance_manifest"} {
		if raw, ok := params[key]; ok && raw != nil {
			return raw, true
		}
	}
	return nil, false
}

func receiptFrom(input ValidateInput, manifest *SelfModificationManifest, decision DynamicValidationDecision, artifactHash string) DynamicValidationReceipt {
	receipt := DynamicValidationReceipt{
		OperationID:      input.OperationID,
		Allowed:          decision.Allowed,
		RequiresApproval: decision.RequiresApproval,
		ReasonCode:       decision.ReasonCode,
		IssuesCount:      len(decision.Issues),
		ArtifactHash:     artifactHash,
		CreatedAt:        time.Now().UTC(),
	}
	if manifest == nil {
		return receipt
	}
	receipt.ArtifactKind = string(manifest.Kind())
	receipt.ArtifactName = manifest.Name().String()
	receipt.RiskClass = manifest.RiskClass()
	for _, p := range manifest.TargetPaths() {
		receipt.TargetPaths = append(receipt.TargetPaths, p.String())
	}
	for _, c := range manifest.Capabilities() {
		receipt.Capabilities = append(receipt.Capabilities, string(c))
	}
	receipt.ManifestHash = hashAny(manifest.Raw())
	return receipt
}

func errorReasonCode(msg, fallback string) string {
	for _, reason := range []string{
		REASON_DYNAMIC_MANIFEST_MISSING_REQUIRED_FIELD,
		REASON_DYNAMIC_MANIFEST_INVALID_JSON,
		REASON_DYNAMIC_MANIFEST_MISSING,
	} {
		if strings.Contains(msg, reason) {
			return reason
		}
	}
	return fallback
}

func containsForbiddenAuthority(v any, prefix string) (bool, string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			norm := normalizeAuthorityKey(k)
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			if forbiddenAuthorityKeys[norm] {
				return true, next
			}
			if blocked, path := containsForbiddenAuthority(child, next); blocked {
				return true, path
			}
		}
	case []any:
		for i, child := range t {
			next := prefix + "[" + strconvI(i) + "]"
			if blocked, path := containsForbiddenAuthority(child, next); blocked {
				return true, path
			}
		}
	}
	return false, ""
}

func normalizeAuthorityKey(raw string) string {
	k := strings.ToLower(strings.TrimSpace(raw))
	repl := strings.NewReplacer("-", "_", " ", "_", ".", "_")
	k = repl.Replace(k)
	for strings.Contains(k, "__") {
		k = strings.ReplaceAll(k, "__", "_")
	}
	return strings.Trim(k, "_")
}

func strconvI(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
