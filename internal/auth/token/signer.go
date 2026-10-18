// Package token issues ES256 assertion tokens and publishes the matching JWKS.
package token

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
)

// Signer signs SHA-256 digests with an ECDSA P-256 key.
type Signer interface {
	KeyID() string
	// Sign returns the 64-byte raw r||s signature of digest.
	Sign(ctx context.Context, digest []byte) ([]byte, error)
	PublicKey(ctx context.Context) (*ecdsa.PublicKey, error)
}

// LocalSigner signs with an in-memory P-256 key. It is for local runs and tests.
type LocalSigner struct {
	kid string
	key *ecdsa.PrivateKey
}

// NewLocalSigner generates a fresh in-memory P-256 key identified by kid.
func NewLocalSigner(kid string) (*LocalSigner, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("token: generate key: %w", err)
	}
	return &LocalSigner{kid: kid, key: key}, nil
}

// KeyID returns the key id placed in the JWT header and the JWKS.
func (s *LocalSigner) KeyID() string { return s.kid }

// Sign returns r||s, each left-padded to 32 bytes.
func (s *LocalSigner) Sign(_ context.Context, digest []byte) ([]byte, error) {
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, digest)
	if err != nil {
		return nil, fmt.Errorf("token: sign: %w", err)
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	ss.FillBytes(out[32:])
	return out, nil
}

// PublicKey returns the verification key.
func (s *LocalSigner) PublicKey(context.Context) (*ecdsa.PublicKey, error) {
	return &s.key.PublicKey, nil
}
