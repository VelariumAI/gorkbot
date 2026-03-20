package hitl

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// HITLMemory manages persistent HITL approval/rejection history for learning
// and auto-decision making. It uses SQLite as the backing store.
type HITLMemory struct {
	db            *sql.DB
	sessionID     string
	fuzzyMatcher  *FuzzyMatcher
	cache         map[string]*DecisionRecord
	cacheMu       sync.RWMutex
	lastPruneTime time.Time
}

// DecisionRecord represents a single HITL approval/rejection decision
type DecisionRecord struct {
	ID              int64
	SessionID       string
	Timestamp       time.Time
	ToolName        string
	ParamsHash      string
	ParamsJSON      string
	Approval        bool
	Rejection       bool
	AmendmentNotes  string
	RiskLevel       int
	Confidence      int
	ExecutionResult string
	SimilarCount    int
	TTLDays         int
}

// ApprovalStats tracks approval/rejection statistics for a tool
type ApprovalStats struct {
	ToolName        string
	TotalCount      int
	ApprovedCount   int
	RejectedCount   int
	AmendedCount    int
	ApprovalRate    float64 // 0.0-1.0
	LastDecision    time.Time
	AverageRiskLevel float64
	AverageConfidence float64
}

// NewHITLMemory creates a new HITL memory store
func NewHITLMemory(db *sql.DB, sessionID string) (*HITLMemory, error) {
	hm := &HITLMemory{
		db:           db,
		sessionID:    sessionID,
		fuzzyMatcher: NewFuzzyMatcher(),
		cache:        make(map[string]*DecisionRecord),
	}

	// Ensure table exists
	if err := hm.ensureSchema(); err != nil {
		return nil, err
	}

	// Load recent decisions into cache
	if err := hm.loadRecentDecisionsIntoCache(); err != nil {
		return nil, err
	}

	return hm, nil
}

// ensureSchema creates the HITL decisions table if it doesn't exist
func (hm *HITLMemory) ensureSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS hitl_decisions (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id      TEXT NOT NULL,
		timestamp       DATETIME DEFAULT CURRENT_TIMESTAMP,
		tool_name       TEXT NOT NULL,
		params_hash     TEXT NOT NULL,
		params_json     TEXT,
		approval        INTEGER DEFAULT 0,
		rejection       INTEGER DEFAULT 0,
		amendment_notes TEXT,
		risk_level      INTEGER,
		confidence      INTEGER,
		execution_result TEXT,
		similar_count   INTEGER DEFAULT 0,
		ttl_days        INTEGER DEFAULT 30
	);

	CREATE INDEX IF NOT EXISTS idx_hitl_tool ON hitl_decisions(tool_name);
	CREATE INDEX IF NOT EXISTS idx_hitl_hash ON hitl_decisions(params_hash);
	CREATE INDEX IF NOT EXISTS idx_hitl_session ON hitl_decisions(session_id);
	CREATE INDEX IF NOT EXISTS idx_hitl_approval ON hitl_decisions(approval, rejection);
	CREATE INDEX IF NOT EXISTS idx_hitl_timestamp ON hitl_decisions(timestamp);
	`

	_, err := hm.db.Exec(schema)
	return err
}

// loadRecentDecisionsIntoCache loads the most recent decisions from the database
func (hm *HITLMemory) loadRecentDecisionsIntoCache() error {
	// Load last 100 decisions for quick lookups
	query := `
	SELECT id, session_id, timestamp, tool_name, params_hash, params_json,
	       approval, rejection, amendment_notes, risk_level, confidence,
	       execution_result, similar_count
	FROM hitl_decisions
	ORDER BY timestamp DESC
	LIMIT 100
	`

	rows, err := hm.db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var rec DecisionRecord
		var approval, rejection int
		var timestamp string

		err := rows.Scan(
			&rec.ID, &rec.SessionID, &timestamp, &rec.ToolName, &rec.ParamsHash,
			&rec.ParamsJSON, &approval, &rejection, &rec.AmendmentNotes,
			&rec.RiskLevel, &rec.Confidence, &rec.ExecutionResult, &rec.SimilarCount,
		)
		if err != nil {
			return err
		}

		rec.Approval = approval != 0
		rec.Rejection = rejection != 0

		// Parse timestamp
		if parsedTime, err := time.Parse(time.RFC3339, timestamp); err == nil {
			rec.Timestamp = parsedTime
		}

		// Cache by hash
		hm.cache[rec.ParamsHash] = &rec
	}

	return rows.Err()
}

// RecordDecision stores an HITL approval/rejection decision
func (hm *HITLMemory) RecordDecision(
	toolName string,
	params map[string]interface{},
	approved bool,
	rejected bool,
	notes string,
	riskLevel int,
	confidence int,
) error {
	// Hash parameters for matching
	paramsHash := hm.hashParameters(params)
	paramsJSON, _ := json.Marshal(params)

	query := `
	INSERT INTO hitl_decisions
	(session_id, tool_name, params_hash, params_json, approval, rejection,
	 amendment_notes, risk_level, confidence)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	approval := 0
	if approved {
		approval = 1
	}
	rejection := 0
	if rejected {
		rejection = 1
	}

	result, err := hm.db.Exec(
		query,
		hm.sessionID, toolName, paramsHash, string(paramsJSON),
		approval, rejection, notes, riskLevel, confidence,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()

	// Update cache
	rec := &DecisionRecord{
		ID:             id,
		SessionID:      hm.sessionID,
		Timestamp:      time.Now(),
		ToolName:       toolName,
		ParamsHash:     paramsHash,
		ParamsJSON:     string(paramsJSON),
		Approval:       approved,
		Rejection:      rejected,
		AmendmentNotes: notes,
		RiskLevel:      riskLevel,
		Confidence:     confidence,
	}

	hm.cacheMu.Lock()
	hm.cache[paramsHash] = rec
	hm.cacheMu.Unlock()

	return nil
}

// CountApprovedExecutions returns how many times a similar operation was approved
func (hm *HITLMemory) CountApprovedExecutions(toolName string, params map[string]interface{}) int {
	paramsHash := hm.hashParameters(params)

	// Check exact match first
	query := `
	SELECT COUNT(*)
	FROM hitl_decisions
	WHERE tool_name = ? AND params_hash = ? AND approval = 1
	`

	var count int
	err := hm.db.QueryRow(query, toolName, paramsHash).Scan(&count)
	if err == nil && count > 0 {
		return count
	}

	// Check for similar approved operations
	similar := hm.FindSimilarApproved(toolName, params, 0.8)
	return len(similar)
}

// WasRecentlyRejected checks if a similar operation was recently rejected
func (hm *HITLMemory) WasRecentlyRejected(toolName string, params map[string]interface{}, withinDuration time.Duration) bool {
	paramsHash := hm.hashParameters(params)
	cutoffTime := time.Now().Add(-withinDuration)

	query := `
	SELECT COUNT(*)
	FROM hitl_decisions
	WHERE tool_name = ? AND params_hash = ? AND rejection = 1
	AND timestamp > ?
	`

	var count int
	err := hm.db.QueryRow(query, toolName, paramsHash, cutoffTime).Scan(&count)
	if err == nil && count > 0 {
		return true
	}

	// Check for similar recent rejections
	similar := hm.FindSimilarRejected(toolName, params, 0.75, withinDuration)
	return len(similar) > 0
}

// FindSimilarApproved finds previously approved decisions similar to the given parameters
func (hm *HITLMemory) FindSimilarApproved(toolName string, params map[string]interface{}, similarity float64) []DecisionRecord {
	query := `
	SELECT id, session_id, timestamp, tool_name, params_hash, params_json,
	       approval, rejection, amendment_notes, risk_level, confidence,
	       execution_result, similar_count
	FROM hitl_decisions
	WHERE tool_name = ? AND approval = 1
	ORDER BY timestamp DESC
	LIMIT 50
	`

	rows, err := hm.db.Query(query, toolName)
	if err != nil {
		return []DecisionRecord{}
	}
	defer rows.Close()

	paramsJSON, _ := json.Marshal(params)
	var results []DecisionRecord

	for rows.Next() {
		var rec DecisionRecord
		var approval, rejection int
		var timestamp string

		err := rows.Scan(
			&rec.ID, &rec.SessionID, &timestamp, &rec.ToolName, &rec.ParamsHash,
			&rec.ParamsJSON, &approval, &rejection, &rec.AmendmentNotes,
			&rec.RiskLevel, &rec.Confidence, &rec.ExecutionResult, &rec.SimilarCount,
		)
		if err != nil {
			continue
		}

		// Compute similarity
		score := hm.fuzzyMatcher.ComputeSimilarity(string(paramsJSON), rec.ParamsJSON)
		if score >= similarity {
			rec.Approval = approval != 0
			rec.Rejection = rejection != 0
			results = append(results, rec)
		}
	}

	return results
}

// FindSimilarRejected finds previously rejected decisions similar to the given parameters
func (hm *HITLMemory) FindSimilarRejected(toolName string, params map[string]interface{}, similarity float64, withinDuration time.Duration) []DecisionRecord {
	cutoffTime := time.Now().Add(-withinDuration)

	query := `
	SELECT id, session_id, timestamp, tool_name, params_hash, params_json,
	       approval, rejection, amendment_notes, risk_level, confidence,
	       execution_result, similar_count
	FROM hitl_decisions
	WHERE tool_name = ? AND rejection = 1 AND timestamp > ?
	ORDER BY timestamp DESC
	LIMIT 50
	`

	rows, err := hm.db.Query(query, toolName, cutoffTime)
	if err != nil {
		return []DecisionRecord{}
	}
	defer rows.Close()

	paramsJSON, _ := json.Marshal(params)
	var results []DecisionRecord

	for rows.Next() {
		var rec DecisionRecord
		var approval, rejection int
		var timestamp string

		err := rows.Scan(
			&rec.ID, &rec.SessionID, &timestamp, &rec.ToolName, &rec.ParamsHash,
			&rec.ParamsJSON, &approval, &rejection, &rec.AmendmentNotes,
			&rec.RiskLevel, &rec.Confidence, &rec.ExecutionResult, &rec.SimilarCount,
		)
		if err != nil {
			continue
		}

		// Compute similarity
		score := hm.fuzzyMatcher.ComputeSimilarity(string(paramsJSON), rec.ParamsJSON)
		if score >= similarity {
			rec.Approval = approval != 0
			rec.Rejection = rejection != 0
			results = append(results, rec)
		}
	}

	return results
}

// GetApprovalStats returns statistics about approvals for a specific tool
func (hm *HITLMemory) GetApprovalStats(toolName string) ApprovalStats {
	query := `
	SELECT COUNT(*), SUM(CASE WHEN approval = 1 THEN 1 ELSE 0 END),
	       SUM(CASE WHEN rejection = 1 THEN 1 ELSE 0 END),
	       SUM(CASE WHEN (approval = 0 AND rejection = 0) THEN 1 ELSE 0 END),
	       MAX(timestamp),
	       AVG(CASE WHEN risk_level > 0 THEN risk_level ELSE NULL END),
	       AVG(CASE WHEN confidence > 0 THEN confidence ELSE NULL END)
	FROM hitl_decisions
	WHERE tool_name = ?
	`

	stats := ApprovalStats{ToolName: toolName}

	var totalCount, approvedCount, rejectedCount, amendedCount sql.NullInt64
	var lastTime sql.NullString
	var avgRisk, avgConf sql.NullFloat64

	err := hm.db.QueryRow(query, toolName).Scan(
		&totalCount, &approvedCount, &rejectedCount, &amendedCount,
		&lastTime, &avgRisk, &avgConf,
	)

	if err != nil || !totalCount.Valid {
		return stats
	}

	stats.TotalCount = int(totalCount.Int64)
	if approvedCount.Valid {
		stats.ApprovedCount = int(approvedCount.Int64)
	}
	if rejectedCount.Valid {
		stats.RejectedCount = int(rejectedCount.Int64)
	}
	if amendedCount.Valid {
		stats.AmendedCount = int(amendedCount.Int64)
	}

	if stats.TotalCount > 0 {
		stats.ApprovalRate = float64(stats.ApprovedCount) / float64(stats.TotalCount)
	}

	if lastTime.Valid {
		if parsedTime, err := time.Parse(time.RFC3339, lastTime.String); err == nil {
			stats.LastDecision = parsedTime
		}
	}

	if avgRisk.Valid {
		stats.AverageRiskLevel = avgRisk.Float64
	}
	if avgConf.Valid {
		stats.AverageConfidence = avgConf.Float64
	}

	return stats
}

// UpdateExecutionResult records the result of executing a previously approved tool
func (hm *HITLMemory) UpdateExecutionResult(toolName string, params map[string]interface{}, result string) error {
	paramsHash := hm.hashParameters(params)

	query := `
	UPDATE hitl_decisions
	SET execution_result = ?
	WHERE tool_name = ? AND params_hash = ?
	ORDER BY timestamp DESC
	LIMIT 1
	`

	_, err := hm.db.Exec(query, result, toolName, paramsHash)
	return err
}

// PruneOldDecisions removes decisions older than the specified number of days
func (hm *HITLMemory) PruneOldDecisions(olderThanDays int) error {
	// Avoid frequent pruning
	if time.Since(hm.lastPruneTime) < 24*time.Hour {
		return nil
	}

	cutoffTime := time.Now().AddDate(0, 0, -olderThanDays)

	query := `
	DELETE FROM hitl_decisions
	WHERE timestamp < ?
	`

	_, err := hm.db.Exec(query, cutoffTime)
	if err == nil {
		hm.lastPruneTime = time.Now()
	}

	return err
}

// GetDecisionHistory returns the decision history for a tool
func (hm *HITLMemory) GetDecisionHistory(toolName string, limit int) []DecisionRecord {
	query := `
	SELECT id, session_id, timestamp, tool_name, params_hash, params_json,
	       approval, rejection, amendment_notes, risk_level, confidence,
	       execution_result, similar_count
	FROM hitl_decisions
	WHERE tool_name = ?
	ORDER BY timestamp DESC
	LIMIT ?
	`

	rows, err := hm.db.Query(query, toolName, limit)
	if err != nil {
		return []DecisionRecord{}
	}
	defer rows.Close()

	var results []DecisionRecord

	for rows.Next() {
		var rec DecisionRecord
		var approval, rejection int
		var timestamp string

		err := rows.Scan(
			&rec.ID, &rec.SessionID, &timestamp, &rec.ToolName, &rec.ParamsHash,
			&rec.ParamsJSON, &approval, &rejection, &rec.AmendmentNotes,
			&rec.RiskLevel, &rec.Confidence, &rec.ExecutionResult, &rec.SimilarCount,
		)
		if err != nil {
			continue
		}

		rec.Approval = approval != 0
		rec.Rejection = rejection != 0

		if parsedTime, err := time.Parse(time.RFC3339, timestamp); err == nil {
			rec.Timestamp = parsedTime
		}

		results = append(results, rec)
	}

	return results
}

// hashParameters creates a deterministic hash of parameters for comparison
func (hm *HITLMemory) hashParameters(params map[string]interface{}) string {
	jsonBytes, _ := json.Marshal(params)
	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:])
}

// ClearSessionHistory removes all decisions for the current session
func (hm *HITLMemory) ClearSessionHistory() error {
	query := `DELETE FROM hitl_decisions WHERE session_id = ?`
	_, err := hm.db.Exec(query, hm.sessionID)
	return err
}

// GetGlobalStats returns approval statistics across all tools
func (hm *HITLMemory) GetGlobalStats() map[string]interface{} {
	query := `
	SELECT COUNT(*),
	       SUM(CASE WHEN approval = 1 THEN 1 ELSE 0 END),
	       SUM(CASE WHEN rejection = 1 THEN 1 ELSE 0 END),
	       COUNT(DISTINCT tool_name)
	FROM hitl_decisions
	`

	var totalCount, approvedCount, rejectedCount, toolCount sql.NullInt64

	err := hm.db.QueryRow(query).Scan(&totalCount, &approvedCount, &rejectedCount, &toolCount)
	if err != nil || !totalCount.Valid {
		return map[string]interface{}{
			"total_decisions": 0,
			"approved":        0,
			"rejected":        0,
			"unique_tools":    0,
			"approval_rate":   0.0,
		}
	}

	total := totalCount.Int64
	approved := int64(0)
	rejected := int64(0)
	tools := int64(0)

	if approvedCount.Valid {
		approved = approvedCount.Int64
	}
	if rejectedCount.Valid {
		rejected = rejectedCount.Int64
	}
	if toolCount.Valid {
		tools = toolCount.Int64
	}

	approvalRate := 0.0
	if total > 0 {
		approvalRate = float64(approved) / float64(total)
	}

	return map[string]interface{}{
		"total_decisions": total,
		"approved":        approved,
		"rejected":        rejected,
		"unique_tools":    tools,
		"approval_rate":   approvalRate,
	}
}
