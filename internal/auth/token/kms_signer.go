package token

import (
	"context"
	"crypto/ecdsa"
	"sync"

	"github.com/huy090202/auth-gate/internal/platform/kmsx"
)

// kmsSigner signs with an AWS KMS key; the private key never leaves KMS.
type kmsSigner struct {
	client *kmsx.Client
	keyID  string

	mu  sync.Mutex
	pub *ecdsa.PublicKey
}

// NewKMSSigner returns a Signer backed by the KMS key keyID.
func NewKMSSigner(c *kmsx.Client, keyID string) Signer {
	return &kmsSigner{client: c, keyID: keyID}
}

func (s *kmsSigner) KeyID() string { return s.keyID }

func (s *kmsSigner) Sign(ctx context.Context, digest []byte) ([]byte, error) {
	return s.client.Sign(ctx, s.keyID, digest)
}

// PublicKey fetches the key once and caches it.
func (s *kmsSigner) PublicKey(ctx context.Context) (*ecdsa.PublicKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pub != nil {
		return s.pub, nil
	}
	pub, err := s.client.PublicKey(ctx, s.keyID)
	if err != nil {
		return nil, err
	}
	s.pub = pub
	return pub, nil
}
