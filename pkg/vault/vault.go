package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CredentialVault provides encrypted storage for sensitive credentials
type CredentialVault struct {
	policy *VaultPolicy

	// Encrypted credentials storage (credentialID -> encrypted value)
	credentials map[string]*encryptedCredential
	credMu      sync.RWMutex

	// Metadata index (credentialName -> metadata)
	metadata map[string]*CredentialMetadata
	metaMu   sync.RWMutex

	// ACL rules (userID + credentialName -> access rules)
	acl map[string][]AccessRule
	aclMu sync.RWMutex

	// Audit log for compliance
	auditLog []AuditLogEntry
	auditMu  sync.RWMutex

	// Master key for encryption/decryption
	masterKey []byte

	// Locked state
	locked bool
	lockMu sync.RWMutex

	// Last activity timestamp
	lastActivity time.Time
	activityMu   sync.RWMutex

	logger *slog.Logger
}

// encryptedCredential stores encrypted credential data
type encryptedCredential struct {
	// Encrypted credential value
	EncryptedValue []byte
	// Initialization vector for decryption
	IV []byte
	// Auth tag for GCM
	AuthTag []byte
	// Timestamp when encrypted
	EncryptedAt time.Time
	// Credential metadata ID
	MetadataID string
}

// AuditLogEntry records all vault operations for compliance
type AuditLogEntry struct {
	Timestamp     time.Time
	Operation     string // "store", "read", "rotate", "revoke", "revoke_user"
	Subject       string // userID performing operation
	CredentialID  string
	ResourceName  string
	Success       bool
	ErrorMessage  string
	Details       map[string]string
}

// NewCredentialVault creates a new credential vault
func NewCredentialVault(policy *VaultPolicy, logger *slog.Logger) (*CredentialVault, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if policy == nil {
		policy = NewVaultPolicy()
	}

	// Generate master key (in production, would use HSM or KMS)
	masterKey := make([]byte, 32) // 256 bits for AES-256
	if _, err := rand.Read(masterKey); err != nil {
		return nil, fmt.Errorf("failed to generate master key: %w", err)
	}

	vault := &CredentialVault{
		policy:       policy,
		credentials:  make(map[string]*encryptedCredential),
		metadata:     make(map[string]*CredentialMetadata),
		acl:          make(map[string][]AccessRule),
		auditLog:     make([]AuditLogEntry, 0),
		masterKey:    masterKey,
		locked:       false,
		lastActivity: time.Now(),
		logger:       logger,
	}

	logger.Info("Credential vault initialized",
		slog.String("encryption", policy.EncryptionAlgorithm),
		slog.String("key_derivation", policy.KeyDerivationMethod),
	)

	return vault, nil
}

// StoreCredential stores an encrypted credential
func (cv *CredentialVault) StoreCredential(userID string, name string, credType CredentialType, secretValue string, description string) error {
	cv.lockMu.RLock()
	if cv.locked {
		cv.lockMu.RUnlock()
		return fmt.Errorf("vault is locked")
	}
	cv.lockMu.RUnlock()

	cv.updateActivity()

	// Check user credentials quota
	cv.metaMu.RLock()
	userCredCount := 0
	for _, meta := range cv.metadata {
		if meta.Owner == userID {
			userCredCount++
		}
	}
	cv.metaMu.RUnlock()

	if userCredCount >= cv.policy.MaxCredentialsPerUser {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "store",
			Subject:      userID,
			ResourceName: name,
			Success:      false,
			ErrorMessage: fmt.Sprintf("exceeded max credentials per user: %d", cv.policy.MaxCredentialsPerUser),
		})
		return fmt.Errorf("credential quota exceeded for user %s", userID)
	}

	// Encrypt credential value
	encryptedValue, iv, authTag, err := cv.encryptValue(secretValue)
	if err != nil {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "store",
			Subject:      userID,
			ResourceName: name,
			Success:      false,
			ErrorMessage: fmt.Sprintf("encryption failed: %v", err),
		})
		return err
	}

	// Generate unique ID
	credID := fmt.Sprintf("cred_%d_%s", time.Now().UnixNano(), name)

	// Create metadata
	meta := &CredentialMetadata{
		ID:          credID,
		Name:        name,
		Type:        credType,
		Owner:       userID,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(cv.policy.CredentialTTL),
		Description: description,
		ACL:         []AccessRule{},
	}

	// Store encrypted credential
	cv.credMu.Lock()
	cv.credentials[credID] = &encryptedCredential{
		EncryptedValue: encryptedValue,
		IV:             iv,
		AuthTag:        authTag,
		EncryptedAt:    time.Now(),
		MetadataID:     credID,
	}
	cv.credMu.Unlock()

	// Store metadata
	cv.metaMu.Lock()
	cv.metadata[name] = meta
	cv.metaMu.Unlock()

	// Grant owner full access
	cv.grantAccess(userID, name, userID, AccessAdmin, "Owner access")

	cv.auditLogEntry(AuditLogEntry{
		Timestamp:    time.Now(),
		Operation:    "store",
		Subject:      userID,
		CredentialID: credID,
		ResourceName: name,
		Success:      true,
		Details: map[string]string{
			"type":     string(credType),
			"provider": meta.Provider,
		},
	})

	cv.logger.Info("credential stored",
		slog.String("credential_id", credID),
		slog.String("name", name),
		slog.String("type", string(credType)),
		slog.String("owner", userID),
	)

	return nil
}

// ReadCredential reads and decrypts a credential value
func (cv *CredentialVault) ReadCredential(requestingUserID string, credentialName string) (string, error) {
	cv.lockMu.RLock()
	if cv.locked {
		cv.lockMu.RUnlock()
		return "", fmt.Errorf("vault is locked")
	}
	cv.lockMu.RUnlock()

	cv.updateActivity()

	// Check access permission
	if !cv.hasAccess(requestingUserID, credentialName, AccessRead) {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "read",
			Subject:      requestingUserID,
			ResourceName: credentialName,
			Success:      false,
			ErrorMessage: "access denied",
		})
		return "", fmt.Errorf("access denied for credential: %s", credentialName)
	}

	// Get metadata
	cv.metaMu.RLock()
	meta, exists := cv.metadata[credentialName]
	cv.metaMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("credential not found: %s", credentialName)
	}

	// Check expiration
	if !meta.ExpiresAt.IsZero() && time.Now().After(meta.ExpiresAt) {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "read",
			Subject:      requestingUserID,
			ResourceName: credentialName,
			Success:      false,
			ErrorMessage: "credential expired",
		})
		return "", fmt.Errorf("credential expired: %s", credentialName)
	}

	// Get encrypted credential
	cv.credMu.RLock()
	encCred, exists := cv.credentials[meta.ID]
	cv.credMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("encrypted credential not found: %s", credentialName)
	}

	// Decrypt value
	decryptedValue, err := cv.decryptValue(encCred.EncryptedValue, encCred.IV, encCred.AuthTag)
	if err != nil {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "read",
			Subject:      requestingUserID,
			ResourceName: credentialName,
			Success:      false,
			ErrorMessage: fmt.Sprintf("decryption failed: %v", err),
		})
		return "", err
	}

	// Update metadata
	cv.metaMu.Lock()
	meta.LastAccessedAt = time.Now()
	meta.AccessCount++
	cv.metaMu.Unlock()

	cv.auditLogEntry(AuditLogEntry{
		Timestamp:    time.Now(),
		Operation:    "read",
		Subject:      requestingUserID,
		ResourceName: credentialName,
		Success:      true,
	})

	cv.logger.Info("credential read",
		slog.String("name", credentialName),
		slog.String("reader", requestingUserID),
		slog.Int64("access_count", meta.AccessCount),
	)

	return decryptedValue, nil
}

// RevokeCredential revokes access to a credential for a user
func (cv *CredentialVault) RevokeCredential(requestingUserID string, credentialName string, targetUserID string) error {
	cv.lockMu.RLock()
	if cv.locked {
		cv.lockMu.RUnlock()
		return fmt.Errorf("vault is locked")
	}
	cv.lockMu.RUnlock()

	cv.updateActivity()

	// Check if requestor has revoke permission
	if !cv.hasAccess(requestingUserID, credentialName, AccessRevoke) {
		cv.auditLogEntry(AuditLogEntry{
			Timestamp:    time.Now(),
			Operation:    "revoke_user",
			Subject:      requestingUserID,
			ResourceName: credentialName,
			Success:      false,
			ErrorMessage: "insufficient permissions for revocation",
		})
		return fmt.Errorf("insufficient permissions for revocation: %s", credentialName)
	}

	// Remove ACL rules for target user
	key := fmt.Sprintf("%s:%s", targetUserID, credentialName)
	cv.aclMu.Lock()
	delete(cv.acl, key)
	cv.aclMu.Unlock()

	cv.auditLogEntry(AuditLogEntry{
		Timestamp:    time.Now(),
		Operation:    "revoke_user",
		Subject:      requestingUserID,
		ResourceName: credentialName,
		Details: map[string]string{
			"target_user": targetUserID,
		},
		Success: true,
	})

	cv.logger.Info("credential access revoked",
		slog.String("name", credentialName),
		slog.String("revoked_for", targetUserID),
		slog.String("revoked_by", requestingUserID),
	)

	return nil
}

// DeleteCredential permanently deletes a credential (owner only)
func (cv *CredentialVault) DeleteCredential(requestingUserID string, credentialName string) error {
	cv.lockMu.RLock()
	if cv.locked {
		cv.lockMu.RUnlock()
		return fmt.Errorf("vault is locked")
	}
	cv.lockMu.RUnlock()

	cv.updateActivity()

	// Get metadata
	cv.metaMu.RLock()
	meta, exists := cv.metadata[credentialName]
	cv.metaMu.RUnlock()

	if !exists {
		return fmt.Errorf("credential not found: %s", credentialName)
	}

	// Only owner can delete
	if meta.Owner != requestingUserID {
		return fmt.Errorf("only owner can delete credential: %s", credentialName)
	}

	// Delete credential and metadata
	cv.credMu.Lock()
	delete(cv.credentials, meta.ID)
	cv.credMu.Unlock()

	cv.metaMu.Lock()
	delete(cv.metadata, credentialName)
	cv.metaMu.Unlock()

	// Delete ACL rules
	cv.aclMu.Lock()
	for k := range cv.acl {
		if k[len(k)-len(credentialName):] == credentialName {
			delete(cv.acl, k)
		}
	}
	cv.aclMu.Unlock()

	cv.auditLogEntry(AuditLogEntry{
		Timestamp:    time.Now(),
		Operation:    "revoke",
		Subject:      requestingUserID,
		ResourceName: credentialName,
		Success:      true,
		Details: map[string]string{
			"action": "delete",
		},
	})

	cv.logger.Info("credential deleted",
		slog.String("name", credentialName),
		slog.String("owner", requestingUserID),
	)

	return nil
}

// GrantAccess grants credential access to a user
func (cv *CredentialVault) GrantAccess(approverID string, credentialName string, targetUserID string, level AccessLevel, reason string) error {
	cv.lockMu.RLock()
	if cv.locked {
		cv.lockMu.RUnlock()
		return fmt.Errorf("vault is locked")
	}
	cv.lockMu.RUnlock()

	cv.updateActivity()

	// Check if approver has admin permission
	if !cv.hasAccess(approverID, credentialName, AccessAdmin) {
		return fmt.Errorf("insufficient permissions: user %s cannot grant access to %s", approverID, credentialName)
	}

	return cv.grantAccess(targetUserID, credentialName, approverID, level, reason)
}

// grantAccess (internal) grants access without permission check
func (cv *CredentialVault) grantAccess(targetUserID string, credentialName string, approverID string, level AccessLevel, reason string) error {
	rule := AccessRule{
		Subject:        targetUserID,
		CredentialName: credentialName,
		Level:          level,
		CreatedAt:      time.Now(),
		Reason:         reason,
		ApprovedBy:     approverID,
	}

	key := fmt.Sprintf("%s:%s", targetUserID, credentialName)
	cv.aclMu.Lock()
	cv.acl[key] = []AccessRule{rule}
	cv.aclMu.Unlock()

	return nil
}

// hasAccess checks if a user has at least the required access level
func (cv *CredentialVault) hasAccess(userID string, credentialName string, requiredLevel AccessLevel) bool {
	key := fmt.Sprintf("%s:%s", userID, credentialName)

	cv.aclMu.RLock()
	rules, exists := cv.acl[key]
	cv.aclMu.RUnlock()

	if !exists {
		return false
	}

	for _, rule := range rules {
		if cv.isAccessSufficient(rule.Level, requiredLevel) {
			return true
		}
	}

	return false
}

// isAccessSufficient checks if available level meets required level
func (cv *CredentialVault) isAccessSufficient(available AccessLevel, required AccessLevel) bool {
	levels := map[AccessLevel]int{
		AccessDeny:   0,
		AccessRead:   1,
		AccessRotate: 2,
		AccessRevoke: 3,
		AccessAdmin:  4,
	}

	return levels[available] >= levels[required]
}

// Lock locks the vault
func (cv *CredentialVault) Lock() {
	cv.lockMu.Lock()
	cv.locked = true
	cv.lockMu.Unlock()

	cv.logger.Info("vault locked")
}

// Unlock unlocks the vault
func (cv *CredentialVault) Unlock() {
	cv.lockMu.Lock()
	cv.locked = false
	cv.lockMu.Unlock()

	cv.logger.Info("vault unlocked")
}

// IsLocked returns lock status
func (cv *CredentialVault) IsLocked() bool {
	cv.lockMu.RLock()
	defer cv.lockMu.RUnlock()
	return cv.locked
}

// encryptValue encrypts a credential value using AES-256-GCM
func (cv *CredentialVault) encryptValue(plaintext string) ([]byte, []byte, []byte, error) {
	block, err := aes.NewCipher(cv.masterKey)
	if err != nil {
		return nil, nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	return ciphertext, nonce, nil, nil
}

// decryptValue decrypts a credential value using AES-256-GCM
func (cv *CredentialVault) decryptValue(ciphertext []byte, nonce []byte, _ []byte) (string, error) {
	block, err := aes.NewCipher(cv.masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// updateActivity updates last activity timestamp
func (cv *CredentialVault) updateActivity() {
	cv.activityMu.Lock()
	cv.lastActivity = time.Now()
	cv.activityMu.Unlock()
}

// auditLogEntry adds an entry to the audit log
func (cv *CredentialVault) auditLogEntry(entry AuditLogEntry) {
	cv.auditMu.Lock()
	defer cv.auditMu.Unlock()

	cv.auditLog = append(cv.auditLog, entry)

	// Prune old entries
	if len(cv.auditLog) > 10000 {
		cutoff := time.Now().Add(-cv.policy.AuditLogRetention)
		var filtered []AuditLogEntry
		for _, e := range cv.auditLog {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		cv.auditLog = filtered
	}
}

// GetAuditLog returns audit log entries for a credential
func (cv *CredentialVault) GetAuditLog(credentialName string, limit int) []AuditLogEntry {
	cv.auditMu.RLock()
	defer cv.auditMu.RUnlock()

	var entries []AuditLogEntry
	for _, entry := range cv.auditLog {
		if entry.ResourceName == credentialName {
			entries = append(entries, entry)
		}
		if len(entries) >= limit {
			break
		}
	}

	return entries
}

// GetStats returns vault statistics
func (cv *CredentialVault) GetStats() map[string]interface{} {
	cv.metaMu.RLock()
	credentialCount := len(cv.metadata)
	cv.metaMu.RUnlock()

	cv.auditMu.RLock()
	auditLogSize := len(cv.auditLog)
	cv.auditMu.RUnlock()

	cv.aclMu.RLock()
	aclRulesCount := len(cv.acl)
	cv.aclMu.RUnlock()

	return map[string]interface{}{
		"locked":          cv.IsLocked(),
		"credentials":     credentialCount,
		"acl_rules":       aclRulesCount,
		"audit_log_size":  auditLogSize,
		"last_activity":   cv.lastActivity,
	}
}

// ExportAuditLog exports audit log as formatted strings (for compliance)
func (cv *CredentialVault) ExportAuditLog() []string {
	cv.auditMu.RLock()
	defer cv.auditMu.RUnlock()

	var exported []string
	for _, entry := range cv.auditLog {
		status := "success"
		if !entry.Success {
			status = "failure"
		}
		line := fmt.Sprintf(
			"%s | %s | %s | %s | %s | %s",
			entry.Timestamp.Format(time.RFC3339),
			entry.Operation,
			entry.Subject,
			entry.ResourceName,
			status,
			entry.ErrorMessage,
		)
		exported = append(exported, line)
	}

	return exported
}
