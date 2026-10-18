// Package service implements the verify use cases: PIN enrollment, factor
// listing, challenges and assertion tokens.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"

	"github.com/huy090202/auth-gate/internal/auth/challenge"
	"github.com/huy090202/auth-gate/internal/auth/client"
	"github.com/huy090202/auth-gate/internal/auth/factor"
	"github.com/huy090202/auth-gate/internal/auth/pin"
	"github.com/huy090202/auth-gate/internal/auth/token"
	"github.com/huy090202/auth-gate/internal/platform/id"
)

const challengeTTL = 5 * time.Minute

// Service wires the stores and the signer behind the verify API.
type Service struct {
	Clients    *client.Service
	Factors    factor.Store
	Challenges challenge.Store
	Signer     token.Signer
	Issuer     string
	Now        func() time.Time
}

// Token is an issued assertion token.
type Token struct {
	Token     string
	ExpiresAt time.Time
	Factor    challenge.Factor
}

// FactorsView lists what a subject has enrolled. WebAuthn is never nil.
type FactorsView struct {
	PIN      bool             `json:"pin"`
	WebAuthn []CredentialView `json:"webauthn"`
}

// CredentialView is the public view of a passkey. Transports is never nil.
type CredentialView struct {
	CredentialID string     `json:"credentialId"`
	Transports   []string   `json:"transports"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt"`
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func validateIDs(ids ...string) error {
	for _, v := range ids {
		if err := id.Validate(v); err != nil {
			return err
		}
	}
	return nil
}

// EnrollPIN sets or replaces the subject's PIN and clears any lockout.
func (s *Service) EnrollPIN(ctx context.Context, c client.Client, subjectID, p string) error {
	if err := validateIDs(subjectID); err != nil {
		return err
	}
	if err := pin.Validate(p, c.PinPolicy); err != nil {
		return ErrPINRejected
	}
	rec, err := pin.Hash(p)
	if err != nil {
		return err
	}
	return s.Factors.PutPIN(ctx, c.ID, subjectID, factor.PIN{Record: rec, FailedCount: 0, LockedUntil: nil, UpdatedAt: s.now()})
}

// DeletePIN removes the subject's PIN; it is idempotent.
func (s *Service) DeletePIN(ctx context.Context, c client.Client, subjectID string) error {
	if err := validateIDs(subjectID); err != nil {
		return err
	}
	return s.Factors.DeletePIN(ctx, c.ID, subjectID)
}

// ListFactors reports the subject's enrolled factors.
func (s *Service) ListFactors(ctx context.Context, c client.Client, subjectID string) (FactorsView, error) {
	if err := validateIDs(subjectID); err != nil {
		return FactorsView{}, err
	}
	view := FactorsView{WebAuthn: []CredentialView{}}
	_, err := s.Factors.GetPIN(ctx, c.ID, subjectID)
	switch {
	case err == nil:
		view.PIN = true
	case !errors.Is(err, factor.ErrNotFound):
		return FactorsView{}, err
	}
	creds, err := s.Factors.ListCredentials(ctx, c.ID, subjectID)
	if err != nil {
		return FactorsView{}, err
	}
	for _, cr := range creds {
		tr := cr.Transports
		if tr == nil {
			tr = []string{}
		}
		view.WebAuthn = append(view.WebAuthn, CredentialView{
			CredentialID: cr.ID, Transports: tr, CreatedAt: cr.CreatedAt, LastUsedAt: cr.LastUsedAt,
		})
	}
	return view, nil
}

// lockedPIN loads the subject's PIN and reports an active lock as Locked.
func (s *Service) lockedPIN(ctx context.Context, clientID, subjectID string, now time.Time) error {
	p, err := s.Factors.GetPIN(ctx, clientID, subjectID)
	if errors.Is(err, factor.ErrNotFound) {
		return ErrFactorNotFound
	}
	if err != nil {
		return err
	}
	if p.LockedUntil != nil && p.LockedUntil.After(now) {
		return Locked{Until: *p.LockedUntil}
	}
	return nil
}

// BeginChallenge opens a single-use challenge for purpose. The assertion
// options are returned only for webauthn; they are nil for pin.
func (s *Service) BeginChallenge(ctx context.Context, c client.Client, subjectID, purpose string, f challenge.Factor) (challenge.Challenge, *protocol.CredentialAssertion, error) {
	if err := validateIDs(subjectID, purpose); err != nil {
		return challenge.Challenge{}, nil, err
	}
	now := s.now()
	ch := challenge.Challenge{
		ID: id.New(), ClientID: c.ID, SubjectID: subjectID, Purpose: purpose, Factor: f,
		Status: challenge.StatusPending, Attempts: 0, ExpiresAt: now.Add(challengeTTL), CreatedAt: now,
	}
	switch f {
	case challenge.FactorPIN:
		if err := s.lockedPIN(ctx, c.ID, subjectID, now); err != nil {
			return challenge.Challenge{}, nil, err
		}
		ch.MaxAttempts = c.PinPolicy.MaxAttempts
	case challenge.FactorWebAuthn:
		creds, err := s.Factors.ListCredentials(ctx, c.ID, subjectID)
		if err != nil {
			return challenge.Challenge{}, nil, err
		}
		if len(creds) == 0 {
			return challenge.Challenge{}, nil, ErrFactorNotFound
		}
		return challenge.Challenge{}, nil, fmt.Errorf("webauthn challenge: %w", errNotImplemented)
	default:
		return challenge.Challenge{}, nil, ErrWrongFactor
	}
	if err := s.Challenges.Create(ctx, ch); err != nil {
		return challenge.Challenge{}, nil, err
	}
	return ch, nil, nil
}

// VerifyPIN spends one attempt on the challenge and, on a correct PIN,
// completes it and issues the assertion token.
func (s *Service) VerifyPIN(ctx context.Context, c client.Client, challengeID, p string) (Token, error) {
	if err := validateIDs(challengeID); err != nil {
		return Token{}, err
	}
	now := s.now()
	ch, err := s.Challenges.Attempt(ctx, c.ID, challengeID, now)
	if err != nil {
		return Token{}, err
	}
	if ch.Factor != challenge.FactorPIN {
		return Token{}, ErrWrongFactor
	}
	rec, err := s.Factors.GetPIN(ctx, c.ID, ch.SubjectID)
	if errors.Is(err, factor.ErrNotFound) {
		return Token{}, ErrFactorNotFound
	}
	if err != nil {
		return Token{}, err
	}
	if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
		return Token{}, Locked{Until: *rec.LockedUntil}
	}
	if !rec.Record.Verify(p) {
		lockout := time.Duration(c.PinPolicy.LockoutSec) * time.Second
		_, until, err := s.Factors.RecordPINFailure(ctx, c.ID, ch.SubjectID, c.PinPolicy.MaxAttempts, lockout, now)
		if err != nil {
			return Token{}, err
		}
		if until != nil {
			return Token{}, Locked{Until: *until}
		}
		return Token{}, VerificationFailed{AttemptsRemaining: ch.MaxAttempts - ch.Attempts}
	}
	if err := s.Challenges.Complete(ctx, c.ID, ch.ID, now); err != nil {
		return Token{}, err
	}
	if err := s.Factors.ResetPINFailures(ctx, c.ID, ch.SubjectID); err != nil {
		return Token{}, err
	}
	return s.issue(ctx, c, ch, now)
}

// VerifyWebAuthn spends one attempt and checks the factor. The assertion
// check itself is added in Task 10.
func (s *Service) VerifyWebAuthn(ctx context.Context, c client.Client, challengeID string, _ json.RawMessage) (Token, error) {
	if err := validateIDs(challengeID); err != nil {
		return Token{}, err
	}
	ch, err := s.Challenges.Attempt(ctx, c.ID, challengeID, s.now())
	if err != nil {
		return Token{}, err
	}
	if ch.Factor != challenge.FactorWebAuthn {
		return Token{}, ErrWrongFactor
	}
	return Token{}, fmt.Errorf("webauthn verify: %w", errNotImplemented)
}

func (s *Service) issue(ctx context.Context, c client.Client, ch challenge.Challenge, now time.Time) (Token, error) {
	exp := now.Add(time.Duration(c.TokenTTLSec) * time.Second)
	tok, err := token.Issue(ctx, s.Signer, token.Claims{
		Issuer: s.Issuer, Subject: ch.SubjectID, Audience: c.ID, Purpose: ch.Purpose, JTI: ch.ID,
		AMR: []string{string(ch.Factor)}, IssuedAt: now, ExpiresAt: exp,
	})
	if err != nil {
		return Token{}, err
	}
	return Token{Token: tok, ExpiresAt: exp, Factor: ch.Factor}, nil
}
