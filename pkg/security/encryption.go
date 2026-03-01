package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// KeyManager handles encryption key generation and storage
type KeyManager struct {
	keyPath string
}

// NewKeyManager creates a new key manager
func NewKeyManager(configDir string) (*KeyManager, error) {
	return &KeyManager{
		keyPath: filepath.Join(configDir, ".grokster.key"),
	}, nil
}

// GetOrCreateKey retrieves the encryption key or creates a new one
func (km *KeyManager) GetOrCreateKey() ([]byte, error) {
	// Check if key exists
	if _, err := os.Stat(km.keyPath); os.IsNotExist(err) {
		// Generate new key
		key := make([]byte, 32) // AES-256
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("failed to generate random key: %w", err)
		}
		
		// Save key securely
		if err := os.WriteFile(km.keyPath, key, 0600); err != nil {
			return nil, fmt.Errorf("failed to save key: %w", err)
		}
		return key, nil
	}

	// Read existing key
	return os.ReadFile(km.keyPath)
}

// Encrypt encrypts plain text using AES-GCM
func (km *KeyManager) Encrypt(plaintext string) (string, error) {
	key, err := km.GetOrCreateKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-GCM
func (km *KeyManager) Decrypt(ciphertextHex string) (string, error) {
	key, err := km.GetOrCreateKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
