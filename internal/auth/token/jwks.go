package token

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// JWKS builds the JSON Web Key Set for the public keys of signers.
func JWKS(ctx context.Context, signers ...Signer) (map[string]any, error) {
	keys := make([]any, 0, len(signers))
	for _, s := range signers {
		pub, err := s.PublicKey(ctx)
		if err != nil {
			return nil, fmt.Errorf("token: public key %q: %w", s.KeyID(), err)
		}
		// Bytes is the uncompressed point: 0x04 || X (32) || Y (32).
		pt, err := pub.Bytes()
		if err != nil || len(pt) != 65 {
			return nil, fmt.Errorf("token: encode key %q", s.KeyID())
		}
		x, y := pt[1:33], pt[33:]
		keys = append(keys, map[string]any{
			"kty": "EC",
			"crv": "P-256",
			"x":   b64.EncodeToString(x),
			"y":   b64.EncodeToString(y),
			"kid": s.KeyID(),
			"alg": "ES256",
			"use": "sig",
		})
	}
	return map[string]any{"keys": keys}, nil
}

// VerifyWithJWKS checks the alg, kid, signature and expiry of token against
// jwks and returns its claims with times in UTC, truncated to seconds.
func VerifyWithJWKS(token string, jwks map[string]any) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("token: malformed token")
	}
	var h header
	if err := decodeJSON(parts[0], &h); err != nil {
		return Claims{}, fmt.Errorf("token: header: %w", err)
	}
	if h.Alg != "ES256" {
		return Claims{}, fmt.Errorf("token: unsupported alg %q", h.Alg)
	}
	pub, err := findKey(jwks, h.Kid)
	if err != nil {
		return Claims{}, err
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return Claims{}, errors.New("token: malformed signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return Claims{}, errors.New("token: signature mismatch")
	}
	var w wireClaims
	if err := decodeJSON(parts[1], &w); err != nil {
		return Claims{}, fmt.Errorf("token: claims: %w", err)
	}
	if !time.Now().Before(time.Unix(w.Exp, 0)) {
		return Claims{}, errors.New("token: expired")
	}
	return Claims{
		Issuer: w.Iss, Subject: w.Sub, Audience: w.Aud, Purpose: w.Purpose,
		JTI: w.JTI, AMR: w.AMR,
		IssuedAt:  time.Unix(w.Iat, 0).UTC(),
		ExpiresAt: time.Unix(w.Exp, 0).UTC(),
	}, nil
}

func decodeJSON(part string, v any) error {
	raw, err := b64.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func findKey(jwks map[string]any, kid string) (*ecdsa.PublicKey, error) {
	keys, _ := jwks["keys"].([]any)
	for _, k := range keys {
		m, ok := k.(map[string]any)
		if !ok || m["kid"] != kid {
			continue
		}
		xs, _ := m["x"].(string)
		ys, _ := m["y"].(string)
		x, errX := b64.DecodeString(xs)
		y, errY := b64.DecodeString(ys)
		if errX != nil || errY != nil || len(x) != 32 || len(y) != 32 {
			return nil, fmt.Errorf("token: malformed key %q", kid)
		}
		pt := append([]byte{0x04}, append(x, y...)...)
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pt)
		if err != nil {
			return nil, fmt.Errorf("token: key %q is not a valid P-256 point", kid)
		}
		return pub, nil
	}
	return nil, fmt.Errorf("token: kid %q not in key set", kid)
}
