// Package client is the registry of calling backends and their access keys.
package client

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/huy090202/auth-gate/internal/platform/httpx"
	"github.com/huy090202/auth-gate/internal/platform/id"
)

var (
	// ErrNotFound is returned when a client or key item does not exist.
	ErrNotFound = errors.New("client: not found")
	// ErrInvalid is returned for input that fails validation.
	ErrInvalid = errors.New("client: invalid input")
	// ErrUnauthorized is returned for an unknown, expired or malformed access key.
	ErrUnauthorized = errors.New("client: unauthorized")
)

const (
	defaultTokenTTLSec = 300
	maxTokenTTLSec     = 900
	keyGrace           = 24 * time.Hour
)

// PinPolicy is the per-client PIN rule set.
type PinPolicy struct{ Length, MaxAttempts, LockoutSec int }

// DefaultPinPolicy returns the policy used when a client does not set one.
func DefaultPinPolicy() PinPolicy { return PinPolicy{Length: 6, MaxAttempts: 5, LockoutSec: 900} }

// Client is a registered calling backend. KeyHashes holds the hashes of the
// currently active access keys; it is bookkeeping for rotation and must never
// be returned to callers.
type Client struct {
	ID          string
	Name        string
	RPID        string
	Origins     []string
	PinPolicy   PinPolicy
	TokenTTLSec int
	CreatedAt   time.Time
	KeyHashes   []string
}

// CreateInput is the admin request to register a client.
type CreateInput struct {
	Name        string
	RPID        string
	Origins     []string
	PinPolicy   *PinPolicy
	TokenTTLSec *int
}

// Validate checks the input and returns an error wrapping ErrInvalid.
func (in CreateInput) Validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	if strings.TrimSpace(in.Name) == "" {
		return bad("name is required")
	}
	rp := strings.ToLower(in.RPID)
	if rp == "" {
		return bad("rpId is required")
	}
	if !strings.Contains(rp, ".") && rp != "localhost" {
		return bad("rpId %q is not a registrable domain", in.RPID)
	}
	if len(in.Origins) == 0 {
		return bad("at least one origin is required")
	}
	for _, o := range in.Origins {
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return bad("origin %q must be http(s)://host[:port] with no path", o)
		}
		host := strings.ToLower(u.Hostname())
		if host != rp && !strings.HasSuffix(host, "."+rp) {
			return bad("rpId %q is not a suffix of origin host %q", in.RPID, host)
		}
	}
	if p := in.PinPolicy; p != nil && (p.Length < 4 || p.Length > 8) {
		return bad("pin length must be 4-8")
	}
	if in.TokenTTLSec != nil && (*in.TokenTTLSec < 1 || *in.TokenTTLSec > maxTokenTTLSec) {
		return bad("tokenTtlSec must be 1-%d", maxTokenTTLSec)
	}
	return nil
}

// Service implements the client use cases on top of a Store.
type Service struct {
	Store Store
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Create registers a client and returns it with its first raw access key,
// which is shown once.
func (s *Service) Create(ctx context.Context, in CreateInput) (Client, string, error) {
	if err := in.Validate(); err != nil {
		return Client{}, "", err
	}
	raw, hash := NewAccessKey()
	c := Client{
		ID:          id.New(),
		Name:        in.Name,
		RPID:        in.RPID,
		Origins:     in.Origins,
		PinPolicy:   DefaultPinPolicy(),
		TokenTTLSec: defaultTokenTTLSec,
		CreatedAt:   s.now().UTC().Truncate(time.Second),
		KeyHashes:   []string{hash},
	}
	if in.PinPolicy != nil {
		c.PinPolicy = *in.PinPolicy
	}
	if in.TokenTTLSec != nil {
		c.TokenTTLSec = *in.TokenTTLSec
	}
	// Key first: a client without a usable key is harmless, a key without a client is not reachable.
	if err := s.Store.PutKey(ctx, hash, c.ID, nil); err != nil {
		return Client{}, "", err
	}
	if err := s.Store.PutClient(ctx, c); err != nil {
		return Client{}, "", err
	}
	return c, raw, nil
}

// Get returns a client by id.
func (s *Service) Get(ctx context.Context, clientID string) (Client, error) {
	return s.Store.GetClient(ctx, clientID)
}

// RotateKey issues a new active key; every previously active key keeps working
// for 24 hours and then stops authenticating.
func (s *Service) RotateKey(ctx context.Context, clientID string) (string, error) {
	c, err := s.Store.GetClient(ctx, clientID)
	if err != nil {
		return "", err
	}
	raw, hash := NewAccessKey()
	if err := s.Store.PutKey(ctx, hash, c.ID, nil); err != nil {
		return "", err
	}
	expireAt := s.now().Add(keyGrace)
	for _, old := range c.KeyHashes {
		if err := s.Store.ExpireKey(ctx, old, expireAt); err != nil {
			return "", err
		}
	}
	c.KeyHashes = []string{hash}
	if err := s.Store.PutClient(ctx, c); err != nil {
		return "", err
	}
	return raw, nil
}

// Authenticate resolves a raw access key to its client. It returns
// ErrUnauthorized for unknown keys and for keys past their expiry, even while
// the key item still exists.
func (s *Service) Authenticate(ctx context.Context, rawKey string) (Client, error) {
	if rawKey == "" {
		return Client{}, ErrUnauthorized
	}
	clientID, expiresAt, err := s.Store.GetKey(ctx, HashKey(rawKey))
	if errors.Is(err, ErrNotFound) {
		return Client{}, ErrUnauthorized
	}
	if err != nil {
		return Client{}, err
	}
	if expiresAt != nil && expiresAt.Before(s.now()) {
		return Client{}, ErrUnauthorized
	}
	c, err := s.Store.GetClient(ctx, clientID)
	if errors.Is(err, ErrNotFound) {
		return Client{}, ErrUnauthorized
	}
	return c, err
}

// KeyFunc adapts Authenticate for httpx.RequireKey; the client is put in ctx.
// A store failure denies the request (fail closed).
func (s *Service) KeyFunc() httpx.KeyFunc {
	return func(ctx context.Context, key string) (context.Context, bool) {
		c, err := s.Authenticate(ctx, key)
		if err != nil {
			return ctx, false
		}
		return WithClient(ctx, c), true
	}
}
