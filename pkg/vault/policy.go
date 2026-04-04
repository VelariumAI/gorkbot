package vault

import "time"

// AccessLevel defines permission levels for credentials
type AccessLevel string

const (
	// AccessDeny blocks all access
	AccessDeny AccessLevel = "deny"
	// AccessRead allows reading credential values
	AccessRead AccessLevel = "read"
	// AccessRotate allows reading and rotating credentials
	AccessRotate AccessLevel = "rotate"
	// AccessRevoke allows reading, rotating, and revoking credentials
	AccessRevoke AccessLevel = "revoke"
	// AccessAdmin allows all operations including ACL management
	AccessAdmin AccessLevel = "admin"
)

// CredentialType defines the type of credential
type CredentialType string

const (
	// APIKey for API key credentials (e.g., XAI_API_KEY, GEMINI_API_KEY)
	APIKey CredentialType = "api_key"
	// OAuthToken for OAuth tokens with expiration
	OAuthToken CredentialType = "oauth_token"
	// BasicAuth for username/password pairs
	BasicAuth CredentialType = "basic_auth"
	// Custom for custom credential types
	Custom CredentialType = "custom"
)

// VaultPolicy defines access control and credential rules
type VaultPolicy struct {
	// Encryption algorithm (AES-256-GCM)
	EncryptionAlgorithm string

	// Master key derivation method (PBKDF2, Scrypt, etc.)
	KeyDerivationMethod string

	// Credentials require confirmation for rotation
	RotationRequiresConfirmation bool

	// Auto-lock vault after this duration of inactivity
	AutoLockDuration time.Duration

	// Audit log retention (e.g., 90 days)
	AuditLogRetention time.Duration

	// Maximum credentials per user
	MaxCredentialsPerUser int

	// Allow credentials to be read multiple times (vs one-time read)
	AllowMultipleReads bool

	// Credentials expire after this duration
	CredentialTTL time.Duration

	// Enable secret rotation reminders
	EnableRotationReminders bool

	// Rotation reminder interval
	RotationReminderInterval time.Duration
}

// NewVaultPolicy creates a default vault policy
func NewVaultPolicy() *VaultPolicy {
	return &VaultPolicy{
		EncryptionAlgorithm:         "AES-256-GCM",
		KeyDerivationMethod:         "PBKDF2",
		RotationRequiresConfirmation: true,
		AutoLockDuration:            15 * time.Minute,
		AuditLogRetention:           90 * 24 * time.Hour,
		MaxCredentialsPerUser:       20,
		AllowMultipleReads:          true,
		CredentialTTL:               365 * 24 * time.Hour, // 1 year default
		EnableRotationReminders:     true,
		RotationReminderInterval:    30 * 24 * time.Hour, // Monthly reminders
	}
}

// AccessRule defines granular access control for a credential
type AccessRule struct {
	// Subject (userID) who this rule applies to
	Subject string

	// Credential name this rule applies to
	CredentialName string

	// Level of access allowed
	Level AccessLevel

	// Created timestamp
	CreatedAt time.Time

	// Expires after this duration (0 = no expiration)
	ExpiresAt time.Time

	// Reason for access (for audit trail)
	Reason string

	// Approved by (userID who granted this rule)
	ApprovedBy string
}

// CredentialMetadata holds non-secret metadata about a credential
type CredentialMetadata struct {
	// Unique identifier for this credential
	ID string

	// Human-readable name
	Name string

	// Type of credential (APIKey, OAuthToken, etc.)
	Type CredentialType

	// Owner (userID who created it)
	Owner string

	// Created timestamp
	CreatedAt time.Time

	// Last accessed timestamp
	LastAccessedAt time.Time

	// Last rotated timestamp
	LastRotatedAt time.Time

	// Last access count
	AccessCount int64

	// Credential is marked for rotation
	NeedsRotation bool

	// Expiration timestamp
	ExpiresAt time.Time

	// Description/notes about this credential
	Description string

	// Provider name (e.g., "xai", "google", "anthropic")
	Provider string

	// ACL for this credential
	ACL []AccessRule
}

// RotationPolicy defines rotation requirements
type RotationPolicy struct {
	// Require rotation every N days (0 = no automatic requirement)
	DaysBeforeRotation int

	// Grace period before enforcement (days)
	GracePeriod int

	// Notify before rotation is required (days before deadline)
	NotificationDays int

	// Number of previous rotations to keep
	KeepPreviousCount int
}
