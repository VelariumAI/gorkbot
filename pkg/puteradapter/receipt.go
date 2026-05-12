package puteradapter

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Receipt is a bounded provenance record for a Puter operation.
type Receipt struct {
	OperationID string    `json:"operation_id"`
	Capability  string    `json:"capability"`
	Path        string    `json:"path,omitempty"`
	Key         string    `json:"key,omitempty"`
	Allowed     bool      `json:"allowed"`
	ReasonCode  string    `json:"reason_code"`
	Bytes       int64     `json:"bytes,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func buildReceipt(id string, capability Capability, decision Decision, path, key string, payload []byte, now time.Time) Receipt {
	receipt := Receipt{
		OperationID: boundedString(id, 128),
		Capability:  boundedString(string(capability), 128),
		Path:        boundedString(path, 512),
		Key:         boundedString(key, 256),
		Allowed:     decision.Allowed,
		ReasonCode:  boundedString(decision.ReasonCode, 128),
		CreatedAt:   now.UTC(),
	}
	if len(payload) > 0 {
		receipt.Bytes = int64(len(payload))
		sum := sha256.Sum256(payload)
		receipt.SHA256 = hex.EncodeToString(sum[:])
	}
	return receipt
}

func boundedString(raw string, max int) string {
	s := strings.TrimSpace(raw)
	if len(s) <= max {
		return s
	}
	return s[:max]
}
