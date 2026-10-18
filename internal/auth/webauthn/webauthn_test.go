package webauthn_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

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
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, table := dynamotest.NewTable(t)
	now := time.Now().UTC().Truncate(time.Second)
	clients := &client.Service{Store: client.NewDynamoStore(db, table), Now: func() time.Time { return now }}
	c, _, err := clients.Create(context.Background(), client.CreateInput{
		Name: "Demo", RPID: "app.example.com", Origins: []string{"https://app.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := token.NewLocalSigner("test-kid")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{client: c, now: now}
	f.svc = &service.Service{
		Clients: clients, Factors: factor.NewDynamoStore(db, table), Challenges: challenge.NewDynamoStore(db, table),
		Signer: signer, Issuer: "https://auth.example", Now: func() time.Time { return f.now },
	}
	return f
}

var goodRP = virtualwebauthn.RelyingParty{Name: "Demo", ID: "app.example.com", Origin: "https://app.example.com"}

// register runs the relay registration flow with rp and returns FinishRegistration's result.
func (f *fixture) register(t *testing.T, rp virtualwebauthn.RelyingParty, auth *virtualwebauthn.Authenticator, cred virtualwebauthn.Credential) (string, error) {
	t.Helper()
	ctx := context.Background()
	reg, opts, err := f.svc.BeginRegistration(ctx, f.client, "u1", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	ao, err := virtualwebauthn.ParseAttestationOptions(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	resp := virtualwebauthn.CreateAttestationResponse(rp, *auth, cred, *ao)
	return f.svc.FinishRegistration(ctx, f.client, reg.ID, []byte(resp))
}

// assert opens a webauthn challenge and answers it with cred.
func (f *fixture) assert(t *testing.T, auth virtualwebauthn.Authenticator, cred virtualwebauthn.Credential) (service.Token, error) {
	t.Helper()
	ctx := context.Background()
	ch, opts, err := f.svc.BeginChallenge(ctx, f.client, "u1", "login", challenge.FactorWebAuthn)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	ao, err := virtualwebauthn.ParseAssertionOptions(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	resp := virtualwebauthn.CreateAssertionResponse(goodRP, auth, cred, *ao)
	return f.svc.VerifyWebAuthn(ctx, f.client, ch.ID, []byte(resp))
}

// Why: the whole relay flow must work end to end against go-webauthn, and the
// stored credential must track use.
func TestWebAuthnRegisterThenVerify(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	credID, err := f.register(t, goodRP, &auth, cred)
	if err != nil {
		t.Fatal(err)
	}
	auth.AddCredential(cred)
	view, err := f.svc.ListFactors(ctx, f.client, "u1")
	if err != nil || len(view.WebAuthn) != 1 || view.WebAuthn[0].CredentialID != credID {
		t.Fatalf("factors = %+v err=%v", view, err)
	}
	cred.Counter = 1
	tok, err := f.assert(t, auth, cred)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Factor != challenge.FactorWebAuthn || tok.Token == "" {
		t.Fatalf("token = %+v", tok)
	}
	stored, err := f.svc.Factors.GetCredential(ctx, f.client.ID, "u1", credID)
	if err != nil || stored.LastUsedAt == nil || stored.SignCount == 0 {
		t.Fatalf("stored = %+v err=%v", stored, err)
	}
}

// Why: a client may only register passkeys for its own rpId and origins.
func TestWebAuthnRejectsWrongOriginAndRPID(t *testing.T) {
	f := newFixture(t)
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	badOrigin := virtualwebauthn.RelyingParty{Name: "Demo", ID: "app.example.com", Origin: "https://evil.example.com"}
	if _, err := f.register(t, badOrigin, &auth, cred); !errors.Is(err, service.ErrRegistrationFailed) {
		t.Fatalf("wrong origin: err = %v", err)
	}
	badRPID := virtualwebauthn.RelyingParty{Name: "Demo", ID: "evil.example.com", Origin: "https://app.example.com"}
	if _, err := f.register(t, badRPID, &auth, cred); !errors.Is(err, service.ErrRegistrationFailed) {
		t.Fatalf("wrong rpId: err = %v", err)
	}
}

// Why: a sign count that goes backwards signals a cloned authenticator.
func TestWebAuthnRejectsRegressedSignCount(t *testing.T) {
	f := newFixture(t)
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	if _, err := f.register(t, goodRP, &auth, cred); err != nil {
		t.Fatal(err)
	}
	auth.AddCredential(cred)
	cred.Counter = 1
	if _, err := f.assert(t, auth, cred); err != nil {
		t.Fatal(err)
	}
	cred.Counter = 0
	_, err := f.assert(t, auth, cred)
	var vf service.VerificationFailed
	if !errors.As(err, &vf) || vf.AttemptsRemaining != 2 {
		t.Fatalf("regressed count: err = %v", err)
	}
}
