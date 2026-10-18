package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/challenge"
	"github.com/huy090202/auth-gate/internal/auth/client"
	"github.com/huy090202/auth-gate/internal/auth/factor"
	"github.com/huy090202/auth-gate/internal/auth/service"
	"github.com/huy090202/auth-gate/internal/auth/token"
	"github.com/huy090202/auth-gate/internal/platform/dynamo/dynamotest"
)

type fixture struct {
	svc    *service.Service
	client client.Client
	signer *token.LocalSigner
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, table := dynamotest.NewTable(t)
	now := time.Now().UTC().Truncate(time.Second)
	clients := &client.Service{Store: client.NewDynamoStore(db, table), Now: func() time.Time { return now }}
	c, _, err := clients.Create(context.Background(), client.CreateInput{
		Name: "shop", RPID: "shop.example", Origins: []string{"https://shop.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewLocalSigner("test-kid")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{client: c, signer: signer, now: now}
	f.svc = &service.Service{
		Clients:    clients,
		Factors:    factor.NewDynamoStore(db, table),
		Challenges: challenge.NewDynamoStore(db, table),
		Signer:     signer,
		Issuer:     "https://auth.example",
		Now:        func() time.Time { return f.now },
	}
	return f
}

func TestPINVerifyIssuesTokenOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.EnrollPIN(ctx, f.client, "u1", "493817"); err != nil {
		t.Fatal(err)
	}
	ch, pk, err := f.svc.BeginChallenge(ctx, f.client, "u1", "transfer", challenge.FactorPIN)
	if err != nil || pk != nil {
		t.Fatalf("begin: pk=%v err=%v", pk, err)
	}
	tok, err := f.svc.VerifyPIN(ctx, f.client, ch.ID, "493817")
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := token.JWKS(ctx, f.signer)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := token.VerifyWithJWKS(tok.Token, jwks)
	if err != nil {
		t.Fatal(err)
	}
	if cl.Audience != f.client.ID || cl.Subject != "u1" || cl.Purpose != "transfer" || cl.JTI != ch.ID ||
		len(cl.AMR) != 1 || cl.AMR[0] != "pin" || cl.ExpiresAt.Sub(cl.IssuedAt) != 300*time.Second {
		t.Fatalf("claims = %+v", cl)
	}
	if _, err := f.svc.VerifyPIN(ctx, f.client, ch.ID, "493817"); !errors.Is(err, challenge.ErrGone) {
		t.Fatalf("second verify err = %v, want ErrGone", err)
	}
}

func TestPINLockoutThroughService(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.svc.EnrollPIN(ctx, f.client, "u1", "493817"); err != nil {
		t.Fatal(err)
	}
	ch, _, err := f.svc.BeginChallenge(ctx, f.client, "u1", "transfer", challenge.FactorPIN)
	if err != nil || ch.MaxAttempts != 5 {
		t.Fatalf("begin: max=%d err=%v", ch.MaxAttempts, err)
	}
	for want := 4; want >= 1; want-- {
		_, err := f.svc.VerifyPIN(ctx, f.client, ch.ID, "111222")
		var vf service.VerificationFailed
		if !errors.As(err, &vf) || vf.AttemptsRemaining != want {
			t.Fatalf("want remaining %d, got %v", want, err)
		}
	}
	_, err = f.svc.VerifyPIN(ctx, f.client, ch.ID, "111222")
	var locked service.Locked
	if !errors.As(err, &locked) || !locked.Until.Equal(f.now.Add(15*time.Minute)) {
		t.Fatalf("5th wrong: err = %v", err)
	}
	if _, _, err := f.svc.BeginChallenge(ctx, f.client, "u1", "transfer", challenge.FactorPIN); !errors.As(err, &locked) {
		t.Fatalf("begin while locked: err = %v, want Locked", err)
	}
	if err := f.svc.EnrollPIN(ctx, f.client, "u1", "493817"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.BeginChallenge(ctx, f.client, "u1", "transfer", challenge.FactorPIN); err != nil {
		t.Fatalf("begin after re-enroll: %v", err)
	}
}

func TestWrongFactorAndMissingFactor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, _, err := f.svc.BeginChallenge(ctx, f.client, "u1", "login", challenge.FactorWebAuthn); !errors.Is(err, service.ErrFactorNotFound) {
		t.Fatalf("webauthn without credentials: err = %v", err)
	}
	if err := f.svc.EnrollPIN(ctx, f.client, "u1", "493817"); err != nil {
		t.Fatal(err)
	}
	ch, _, err := f.svc.BeginChallenge(ctx, f.client, "u1", "login", challenge.FactorPIN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.VerifyWebAuthn(ctx, f.client, ch.ID, json.RawMessage(`{}`)); !errors.Is(err, service.ErrWrongFactor) {
		t.Fatalf("webauthn body on pin challenge: err = %v", err)
	}
}
