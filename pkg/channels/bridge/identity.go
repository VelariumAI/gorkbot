// Package bridge provides cross-channel user identity resolution and session
// routing. It lets a user who interacts via Discord, Telegram, and the TUI be
// treated as a single canonical identity with a shared conversation history.
package bridge

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

const linkCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 chars, no ambiguous O/0/I/1

// Registry maps (platform, platformUserID) pairs to a canonical UUID identity.
type Registry struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewRegistry creates a Registry backed by db and initialises its schema.
func NewRegistry(db *sql.DB) (*Registry, error) {
	r := &Registry{db: db}
	if err := r.init(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) init() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS channel_identities (
			canonical_id     TEXT NOT NULL,
			platform         TEXT NOT NULL,
			platform_user_id TEXT NOT NULL,
			username         TEXT,
			created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (platform, platform_user_id)
		);
		CREATE TABLE IF NOT EXISTS channel_link_codes (
			code         TEXT PRIMARY KEY,
			canonical_id TEXT NOT NULL,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at   DATETIME NOT NULL
		);
	`)
	return err
}

// GetOrCreate returns the canonical ID for the given platform/user, creating one
// if it does not exist yet.
func (r *Registry) GetOrCreate(platform, platformUserID, username string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Try to fetch existing.
	var id string
	err := r.db.QueryRow(
		`SELECT canonical_id FROM channel_identities WHERE platform=? AND platform_user_id=?`,
		platform, platformUserID,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("bridge: query identity: %w", err)
	}

	// Create new.
	newID := newUUID()
	_, err = r.db.Exec(
		`INSERT INTO channel_identities(canonical_id, platform, platform_user_id, username) VALUES(?,?,?,?)`,
		newID, platform, platformUserID, username,
	)
	if err != nil {
		// Race condition: another goroutine may have inserted — retry read.
		if retryErr := r.db.QueryRow(
			`SELECT canonical_id FROM channel_identities WHERE platform=? AND platform_user_id=?`,
			platform, platformUserID,
		).Scan(&id); retryErr == nil {
			return id, nil
		}
		return "", fmt.Errorf("bridge: insert identity: %w", err)
	}
	return newID, nil
}

// GenerateLinkCode creates a 6-char link code valid for 10 minutes.
func (r *Registry) GenerateLinkCode(canonicalID string) (string, error) {
	code, err := randomCode(6)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(10 * time.Minute)
	_, err = r.db.Exec(
		`INSERT OR REPLACE INTO channel_link_codes(code, canonical_id, expires_at) VALUES(?,?,?)`,
		code, canonicalID, expires.UTC().Format(time.RFC3339),
	)
	return code, err
}

// ConsumeLink validates a code and merges the given platform user into the
// target canonical identity. The code is consumed (deleted) on success.
func (r *Registry) ConsumeLink(code, platform, platformUserID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var targetID, expiresStr string
	err := r.db.QueryRow(
		`SELECT canonical_id, expires_at FROM channel_link_codes WHERE code=?`, code,
	).Scan(&targetID, &expiresStr)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("bridge: link code not found or already used")
	}
	if err != nil {
		return "", fmt.Errorf("bridge: query link code: %w", err)
	}

	expires, _ := time.Parse(time.RFC3339, expiresStr)
	if time.Now().After(expires) {
		return "", fmt.Errorf("bridge: link code expired")
	}

	// Upsert: assign the target canonical ID to this platform user.
	_, err = r.db.Exec(
		`INSERT OR REPLACE INTO channel_identities(canonical_id, platform, platform_user_id) VALUES(?,?,?)`,
		targetID, platform, platformUserID,
	)
	if err != nil {
		return "", fmt.Errorf("bridge: update identity: %w", err)
	}

	// Consume the code.
	_, _ = r.db.Exec(`DELETE FROM channel_link_codes WHERE code=?`, code)
	return targetID, nil
}

// LinkedPlatforms returns the (platform, username) pairs linked to a canonical ID.
func (r *Registry) LinkedPlatforms(canonicalID string) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT platform, username FROM channel_identities WHERE canonical_id=?`, canonicalID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var plat, uname string
		if err := rows.Scan(&plat, &uname); err != nil {
			continue
		}
		result = append(result, plat+":"+uname)
	}
	return result, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func randomCode(n int) (string, error) {
	b := make([]byte, n)
	alphabet := []byte(linkCodeAlphabet)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b), nil
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// PlatformPrefix returns a short platform prefix for display.
func PlatformPrefix(s string) string {
	if idx := strings.Index(s, ":"); idx >= 0 {
		return s[:idx]
	}
	return s
}
