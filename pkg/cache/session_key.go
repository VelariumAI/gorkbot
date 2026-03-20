// Package cache provides a provider-agnostic prompt-caching layer for the
// Gorkbot orchestrator. It augments (never replaces) existing provider logic
// with the correct caching strategy for each backend:
//
//   - Anthropic / MiniMax / OpenRouter: cache_control breakpoints (explicit)
//   - Gemini: cachedContents REST lifecycle (explicit)
//   - xAI/Grok: automatic + x-grok-conv-id sticky routing header
//   - OpenAI: structural prefix optimizer (automatic, no headers needed)
//   - Moonshot: best-effort upload/tag model with graceful fallback
//   - All providers: application-layer TTL response cache as universal fallback
package cache

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

// SessionKey is a 32-byte random key minted once per orchestrator session.
// All provider-specific cache IDs are BLAKE2b-keyed-hash derivatives of this
// key, ensuring strict per-session isolation — a cache entry created in one
// session is cryptographically unreachable from another.
type SessionKey struct {
	key [32]byte
}

// NewSessionKey generates a cryptographically random session key.
func NewSessionKey() (*SessionKey, error) {
	sk := &SessionKey{}
	if _, err := rand.Read(sk.key[:]); err != nil {
		return nil, fmt.Errorf("cache: session key: %w", err)
	}
	return sk, nil
}

// Sign returns a 64-char hex keyed-BLAKE2b-256 MAC of data, scoped to this
// session. Two sessions never produce the same tag for the same input.
func (sk *SessionKey) Sign(data []byte) string {
	h, _ := blake2b.New256(sk.key[:]) // keyed hash: key[:] is 32 bytes
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// ContentHash returns an unkeyed BLAKE2b-256 fingerprint of content.
// Used to detect whether the system prompt or tool schema has changed between
// turns; a change invalidates existing Tier-1 explicit cache entries.
func ContentHash(content []byte) string {
	h, _ := blake2b.New256(nil) // nil key = unkeyed; same input → same hash
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}
