package token

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Claims are the assertion token claims.
type Claims struct {
	Issuer    string
	Subject   string
	Audience  string
	Purpose   string
	JTI       string
	AMR       []string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// wireClaims fixes the claim order in the encoded payload.
type wireClaims struct {
	Iss     string   `json:"iss"`
	Sub     string   `json:"sub"`
	Aud     string   `json:"aud"`
	AMR     []string `json:"amr"`
	Purpose string   `json:"purpose"`
	JTI     string   `json:"jti"`
	Iat     int64    `json:"iat"`
	Exp     int64    `json:"exp"`
}

var b64 = base64.RawURLEncoding

// Issue returns a compact ES256 JWT for c, signed by s.
func Issue(ctx context.Context, s Signer, c Claims) (string, error) {
	h, err := json.Marshal(header{Alg: "ES256", Typ: "JWT", Kid: s.KeyID()})
	if err != nil {
		return "", fmt.Errorf("token: encode header: %w", err)
	}
	p, err := json.Marshal(wireClaims{
		Iss: c.Issuer, Sub: c.Subject, Aud: c.Audience, AMR: c.AMR,
		Purpose: c.Purpose, JTI: c.JTI,
		Iat: c.IssuedAt.Unix(), Exp: c.ExpiresAt.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("token: encode claims: %w", err)
	}
	signingInput := b64.EncodeToString(h) + "." + b64.EncodeToString(p)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := s.Sign(ctx, digest[:])
	if err != nil {
		return "", err
	}
	if len(sig) != 64 {
		return "", errors.New("token: signer returned a signature that is not 64 bytes")
	}
	return signingInput + "." + b64.EncodeToString(sig), nil
}
