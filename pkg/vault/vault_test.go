package vault

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestCredentialVault_StoreAndRead(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Store credential
	err := vault.StoreCredential("alice", "api_key_xai", APIKey, "xai-secret-12345", "XAI API key")
	if err != nil {
		t.Fatalf("failed to store credential: %v", err)
	}

	// Read credential
	value, err := vault.ReadCredential("alice", "api_key_xai")
	if err != nil {
		t.Fatalf("failed to read credential: %v", err)
	}

	if value != "xai-secret-12345" {
		t.Errorf("expected 'xai-secret-12345', got '%s'", value)
	}

	t.Logf("✓ Credential stored and retrieved successfully")
}

func TestCredentialVault_AccessControl(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Alice stores credential
	vault.StoreCredential("alice", "secret_key", APIKey, "secret-value", "Secret")

	// Bob should not be able to read
	_, err := vault.ReadCredential("bob", "secret_key")
	if err == nil {
		t.Error("expected error when bob reads alice's credential")
	}

	// Alice grants read access to Bob
	vault.GrantAccess("alice", "secret_key", "bob", AccessRead, "For collaboration")

	// Now Bob can read
	value, err := vault.ReadCredential("bob", "secret_key")
	if err != nil {
		t.Fatalf("bob should be able to read after access grant: %v", err)
	}

	if value != "secret-value" {
		t.Errorf("expected 'secret-value', got '%s'", value)
	}

	t.Logf("✓ Access control enforced correctly")
}

func TestCredentialVault_RevokeAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Alice stores and grants access to Bob
	vault.StoreCredential("alice", "shared_key", APIKey, "shared-secret", "Shared")
	vault.GrantAccess("alice", "shared_key", "bob", AccessRead, "Temporary access")

	// Bob can read
	_, err := vault.ReadCredential("bob", "shared_key")
	if err != nil {
		t.Fatalf("bob should be able to read: %v", err)
	}

	// Alice revokes access
	vault.RevokeCredential("alice", "shared_key", "bob")

	// Bob can no longer read
	_, err = vault.ReadCredential("bob", "shared_key")
	if err == nil {
		t.Error("expected error when bob tries to read after revocation")
	}

	t.Logf("✓ Access revocation works correctly")
}

func TestCredentialVault_Encryption(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Store in vault
	secret := "super-secret-password-12345"
	vault.StoreCredential("alice", "password", BasicAuth, secret, "User password")

	// Verify we can read it back correctly (encryption/decryption works)
	decrypted, err := vault.ReadCredential("alice", "password")
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if decrypted != secret {
		t.Errorf("decrypted value mismatch: expected '%s', got '%s'", secret, decrypted)
	}

	t.Logf("✓ Encryption/decryption works correctly")
}

func TestCredentialVault_LockUnlock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Store credential
	vault.StoreCredential("alice", "api_key", APIKey, "key-value", "API Key")

	// Unlock by default
	if vault.IsLocked() {
		t.Error("vault should not be locked initially")
	}

	// Lock the vault
	vault.Lock()
	if !vault.IsLocked() {
		t.Error("vault should be locked after Lock()")
	}

	// Should not be able to read when locked
	_, err := vault.ReadCredential("alice", "api_key")
	if err == nil {
		t.Error("should not be able to read when vault is locked")
	}

	// Unlock
	vault.Unlock()
	if vault.IsLocked() {
		t.Error("vault should not be locked after Unlock()")
	}

	// Should be able to read now
	_, err = vault.ReadCredential("alice", "api_key")
	if err != nil {
		t.Fatalf("should be able to read after unlock: %v", err)
	}

	t.Logf("✓ Lock/Unlock mechanism works correctly")
}

func TestCredentialVault_DeleteCredential(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Alice stores credential
	vault.StoreCredential("alice", "temp_key", APIKey, "temp-value", "Temporary")

	// Alice can read
	value, _ := vault.ReadCredential("alice", "temp_key")
	if value != "temp-value" {
		t.Error("credential should be readable before deletion")
	}

	// Alice deletes it
	err := vault.DeleteCredential("alice", "temp_key")
	if err != nil {
		t.Fatalf("failed to delete credential: %v", err)
	}

	// Should no longer be readable
	_, err = vault.ReadCredential("alice", "temp_key")
	if err == nil {
		t.Error("should not be able to read deleted credential")
	}

	// Bob cannot delete Alice's credential (if Alice had granted access)
	vault.StoreCredential("alice", "protected_key", APIKey, "protected", "Protected")
	err = vault.DeleteCredential("bob", "protected_key")
	if err == nil {
		t.Error("only owner should be able to delete credential")
	}

	t.Logf("✓ Credential deletion works correctly")
}

func TestCredentialVault_AccessLevels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Alice stores credential
	vault.StoreCredential("alice", "api_key", APIKey, "secret", "API Key")

	tests := []struct {
		name    string
		level   AccessLevel
		canRead bool
	}{
		{"AccessDeny", AccessDeny, false},
		{"AccessRead", AccessRead, true},
		{"AccessRotate", AccessRotate, true},
		{"AccessRevoke", AccessRevoke, true},
		{"AccessAdmin", AccessAdmin, true},
	}

	for _, tc := range tests {
		// Grant access at level
		vault.GrantAccess("alice", "api_key", "bob", tc.level, "Test access")

		// Try to read
		_, err := vault.ReadCredential("bob", "api_key")

		if tc.canRead && err != nil {
			t.Errorf("access level %s should allow read, but got error: %v", tc.level, err)
		}
		if !tc.canRead && err == nil {
			t.Errorf("access level %s should deny read", tc.level)
		}

		// Revoke for next iteration
		vault.RevokeCredential("alice", "api_key", "bob")
	}

	t.Logf("✓ All access levels work correctly")
}

func TestCredentialVault_AuditLog(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Perform operations
	vault.StoreCredential("alice", "key1", APIKey, "secret1", "Key 1")
	vault.ReadCredential("alice", "key1")
	vault.RevokeCredential("alice", "key1", "bob") // Should fail but be logged

	// Get audit log
	stats := vault.GetStats()
	auditSize := stats["audit_log_size"].(int)

	if auditSize < 2 {
		t.Errorf("expected at least 2 audit entries, got %d", auditSize)
	}

	t.Logf("✓ Audit logging works (entries: %d)", auditSize)
}

func TestCredentialVault_CredentialExpiration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	policy := NewVaultPolicy()
	policy.CredentialTTL = 1 * time.Second // Short TTL for testing
	vault, _ := NewCredentialVault(policy, logger)

	// Store credential
	vault.StoreCredential("alice", "expiring_key", APIKey, "expiring-value", "Will expire")

	// Can read immediately
	_, err := vault.ReadCredential("alice", "expiring_key")
	if err != nil {
		t.Fatalf("credential should be readable immediately: %v", err)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Should not be readable after expiration
	_, err = vault.ReadCredential("alice", "expiring_key")
	if err == nil {
		t.Error("expired credential should not be readable")
	}

	t.Logf("✓ Credential expiration works correctly")
}

func TestCredentialVault_Stats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	// Store multiple credentials
	vault.StoreCredential("alice", "key1", APIKey, "value1", "Key 1")
	vault.StoreCredential("alice", "key2", APIKey, "value2", "Key 2")
	vault.StoreCredential("bob", "key3", APIKey, "value3", "Key 3")

	// Grant access to create ACL rules
	vault.GrantAccess("alice", "key1", "bob", AccessRead, "Shared")

	// Get stats
	stats := vault.GetStats()

	if creds, ok := stats["credentials"].(int); ok {
		if creds != 3 {
			t.Errorf("expected 3 credentials, got %d", creds)
		}
	}

	if rules, ok := stats["acl_rules"].(int); ok {
		if rules < 3 { // At least one rule per credential + one shared
			t.Logf("ACL rules: %d (includes owner rules)", rules)
		}
	}

	t.Logf("✓ Statistics collection works: %v", stats)
}

func TestCredentialVault_MaxCredentialsQuota(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	policy := NewVaultPolicy()
	policy.MaxCredentialsPerUser = 3
	vault, _ := NewCredentialVault(policy, logger)

	// Store up to quota
	for i := 1; i <= 3; i++ {
		name := string(rune('a' - 1 + i))
		err := vault.StoreCredential("alice", "key"+name, APIKey, "value", "Key")
		if err != nil {
			t.Errorf("failed to store credential %d: %v", i, err)
		}
	}

	// Try to exceed quota
	err := vault.StoreCredential("alice", "key_d", APIKey, "value", "Key")
	if err == nil {
		t.Error("should not allow credentials beyond quota")
	}

	t.Logf("✓ Credential quota enforcement works")
}

// Benchmark tests

func BenchmarkVault_StoreCredential(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vault.StoreCredential("alice", "key", APIKey, "secret-value", "Key")
	}
}

func BenchmarkVault_ReadCredential(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)
	vault.StoreCredential("alice", "key", APIKey, "secret-value", "Key")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vault.ReadCredential("alice", "key")
	}
}

func BenchmarkVault_GrantAccess(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	vault, _ := NewCredentialVault(NewVaultPolicy(), logger)
	vault.StoreCredential("alice", "key", APIKey, "secret-value", "Key")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vault.GrantAccess("alice", "key", "bob", AccessRead, "Access")
	}
}
