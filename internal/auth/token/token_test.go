package token

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testClaims() Claims {
	iat := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	return Claims{
		Issuer:    "https://auth.example.test",
		Subject:   "sub_1",
		Audience:  "cl_1",
		AMR:       []string{"pin"},
		Purpose:   "transfer",
		JTI:       "ch_1",
		IssuedAt:  iat,
		ExpiresAt: iat.Add(300 * time.Second),
	}
}

func decodePart(t *testing.T, s string) map[string]any {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("decode part: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal part: %v", err)
	}
	return m
}

func TestIssueProducesExpectedHeaderAndClaims(t *testing.T) {
	ctx := context.Background()
	s, err := NewLocalSigner("local-1")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := Issue(ctx, s, testClaims())
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("want 3 parts, got %d", len(parts))
	}
	h := decodePart(t, parts[0])
	if h["alg"] != "ES256" || h["typ"] != "JWT" || h["kid"] != "local-1" {
		t.Fatalf("header = %v", h)
	}
	p := decodePart(t, parts[1])
	if p["aud"] != "cl_1" || p["purpose"] != "transfer" {
		t.Fatalf("payload = %v", p)
	}
	if !reflect.DeepEqual(p["amr"], []any{"pin"}) {
		t.Fatalf("amr = %v", p["amr"])
	}
	if p["exp"].(float64)-p["iat"].(float64) != 300 {
		t.Fatalf("exp-iat = %v", p["exp"].(float64)-p["iat"].(float64))
	}
}

func TestJWKSRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := NewLocalSigner("local-1")
	if err != nil {
		t.Fatal(err)
	}
	want := testClaims()
	// Verification checks expiry, so issue a token that is currently valid.
	want.IssuedAt = time.Now().UTC().Truncate(time.Second)
	want.ExpiresAt = want.IssuedAt.Add(300 * time.Second)
	tok, err := Issue(ctx, s, want)
	if err != nil {
		t.Fatal(err)
	}
	set, err := JWKS(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyWithJWKS(tok, set)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("claims = %+v, want %+v", got, want)
	}

	parts := strings.Split(tok, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[10] ^= 0x01
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	if _, err := VerifyWithJWKS(strings.Join(parts, "."), set); err == nil {
		t.Fatal("tampered signature accepted")
	}

	other, err := NewLocalSigner("local-2")
	if err != nil {
		t.Fatal(err)
	}
	otherSet, err := JWKS(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyWithJWKS(tok, otherSet); err == nil {
		t.Fatal("unknown kid accepted")
	}
}
