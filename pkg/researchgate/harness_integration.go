package researchgate

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/harness"
	"github.com/velariumai/gorkbot/pkg/trace"
)

func (g *Gateway) runHarnessAudit(req ResearchRequest, decision ResearchDecision, result ResearchResult, dur time.Duration) *harness.AuditSummary {
	if g == nil || g.HarnessRuntime == nil || g.HarnessRuntime.Mode() == harness.ModeOff {
		return nil
	}

	artifact := buildResearchHarnessArtifact(req, decision, result, dur)
	report, _ := g.HarnessRuntime.Validate(nil, artifact)
	summary := harness.SummarizeReport(g.HarnessRuntime.Mode(), report)
	return &summary
}

func buildResearchHarnessArtifact(req ResearchRequest, decision ResearchDecision, result ResearchResult, dur time.Duration) harness.Artifact {
	host := ""
	if decision.NormalizedURL != "" {
		if u, err := url.Parse(decision.NormalizedURL); err == nil {
			host = normalizedHost(u.Host)
		}
	}
	if host == "" && req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			host = normalizedHost(u.Host)
		}
	}

	urlHash := trace.StableHash(strings.TrimSpace(decision.NormalizedURL))
	artifactID := trace.StableHash(
		"researchgate-audit",
		strings.TrimSpace(req.ID),
		urlHash,
		strings.TrimSpace(decision.ReasonCode),
		fmt.Sprintf("%d", result.StatusCode),
	)

	metadata := map[string]string{
		"surface":        "researchgate",
		"request_kind":   string(req.Kind),
		"method":         strings.ToUpper(strings.TrimSpace(req.Method)),
		"host":           host,
		"allowed":        fmt.Sprintf("%t", decision.Allowed),
		"final_status":   decision.FinalStatus,
		"reason_code":    decision.ReasonCode,
		"risk_class":     decision.RiskClass,
		"status_code":    fmt.Sprintf("%d", result.StatusCode),
		"bytes_read":     fmt.Sprintf("%d", result.BytesRead),
		"from_cache":     fmt.Sprintf("%t", result.FromCache),
		"duration_ms":    fmt.Sprintf("%d", dur.Milliseconds()),
		"url_hash":       urlHash,
		"content_sha256": result.SHA256,
	}

	return harness.Artifact{
		ID:          artifactID,
		Kind:        harness.ArtifactKindResearchClaim,
		Name:        "researchgate.egress_decision",
		ContentHash: result.SHA256,
		Refs: []trace.Ref{
			trace.NewRef("request_id", req.ID, "", 0),
			trace.NewRef("url_hash", urlHash, urlHash, 0),
		},
		Metadata: metadata,
	}
}
