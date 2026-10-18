// Package challenge holds single-use challenges and pending registrations.
package challenge

import (
	"context"
	"errors"
	"time"
)

// Factor names the second factor a challenge is issued for.
type Factor string

const (
	FactorPIN      Factor = "pin"
	FactorWebAuthn Factor = "webauthn"
)

// Challenge statuses.
const (
	StatusPending  = "pending"
	StatusVerified = "verified"
)

// Challenge is a short-lived, single-use verification challenge.
type Challenge struct {
	ID, ClientID, SubjectID, Purpose string
	Factor                           Factor
	Session                          []byte
	Status                           string
	Attempts, MaxAttempts            int
	ExpiresAt, CreatedAt             time.Time
}

// Registration is a pending WebAuthn registration awaiting its attestation.
type Registration struct {
	ID, ClientID, SubjectID, DisplayName string
	Session                              []byte
	ExpiresAt                            time.Time
}

// ErrGone means the challenge or registration is missing, expired, exhausted or already used.
var ErrGone = errors.New("challenge: gone")

// Store persists challenges and pending registrations.
type Store interface {
	Create(ctx context.Context, c Challenge) error
	// Attempt counts one attempt and returns the post-increment challenge.
	// It returns ErrGone when the challenge is missing, not pending, out of
	// attempts or past expiresAt (the item may outlive expiresAt until TTL runs).
	Attempt(ctx context.Context, clientID, id string, now time.Time) (Challenge, error)
	// Complete marks a pending, unexpired challenge verified; it succeeds once.
	Complete(ctx context.Context, clientID, id string, now time.Time) error
	PutRegistration(ctx context.Context, r Registration) error
	// TakeRegistration returns and deletes the registration; ErrGone if missing or expired.
	TakeRegistration(ctx context.Context, clientID, id string, now time.Time) (Registration, error)
}
