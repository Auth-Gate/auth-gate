// Package factor stores a subject's second factors: one PIN and any number of
// WebAuthn credentials, all under the subject's partition.
package factor

import (
	"context"
	"errors"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/pin"
)

// ErrNotFound is returned when a PIN or credential item does not exist.
var ErrNotFound = errors.New("factor: not found")

// PIN is a subject's stored PIN with its lockout state.
type PIN struct {
	Record      pin.Record
	FailedCount int
	LockedUntil *time.Time
	UpdatedAt   time.Time
}

// Credential is a registered WebAuthn passkey.
type Credential struct {
	ID          string
	PublicKey   []byte
	SignCount   uint32
	AAGUID      []byte
	Transports  []string
	DisplayName string
	// BackupEligible and BackupState are the authenticator flags go-webauthn checks on login.
	BackupEligible bool
	BackupState    bool
	CreatedAt      time.Time
	LastUsedAt     *time.Time
}

// Store persists PIN and credential items.
type Store interface {
	PutPIN(ctx context.Context, clientID, subjectID string, p PIN) error
	// GetPIN returns ErrNotFound when the subject has no PIN.
	GetPIN(ctx context.Context, clientID, subjectID string) (PIN, error)
	// DeletePIN is idempotent.
	DeletePIN(ctx context.Context, clientID, subjectID string) error
	// RecordPINFailure counts a failed attempt and returns the post-increment
	// count. Reaching maxAttempts sets lockedUntil to now+lockout and resets
	// the stored count to 0; lockedUntil is nil otherwise. It returns
	// ErrNotFound when the subject has no PIN.
	RecordPINFailure(ctx context.Context, clientID, subjectID string, maxAttempts int, lockout time.Duration, now time.Time) (failedCount int, lockedUntil *time.Time, err error)
	// ResetPINFailures zeroes the count and clears the lock.
	ResetPINFailures(ctx context.Context, clientID, subjectID string) error
	PutCredential(ctx context.Context, clientID, subjectID string, c Credential) error
	// ListCredentials never returns a nil slice on success.
	ListCredentials(ctx context.Context, clientID, subjectID string) ([]Credential, error)
	GetCredential(ctx context.Context, clientID, subjectID, credentialID string) (Credential, error)
	// UpdateCredentialUse records a login: the new sign count, backup state and time.
	UpdateCredentialUse(ctx context.Context, clientID, subjectID, credentialID string, signCount uint32, backupState bool, now time.Time) error
	DeleteCredential(ctx context.Context, clientID, subjectID, credentialID string) error
	// PurgeSubject deletes every item under the subject's partition.
	PurgeSubject(ctx context.Context, clientID, subjectID string) error
}
