package client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/client"
	"github.com/huy090202/auth-gate/internal/platform/dynamo/dynamotest"
)

func TestCreateInputValidate(t *testing.T) {
	ttl := func(n int) *int { return &n }
	base := func() client.CreateInput {
		return client.CreateInput{Name: "shop", RPID: "example.com", Origins: []string{"https://app.example.com"}}
	}
	tests := []struct {
		name    string
		mutate  func(*client.CreateInput)
		wantErr bool
	}{
		{"ok", func(*client.CreateInput) {}, false},
		{"ttl 901", func(in *client.CreateInput) { in.TokenTTLSec = ttl(901) }, true},
		{"origin without scheme", func(in *client.CreateInput) { in.Origins = []string{"app.example.com"} }, true},
		{"rpId not a suffix of origin host", func(in *client.CreateInput) { in.RPID = "other.com" }, true},
		{"duplicate origin", func(in *client.CreateInput) {
			in.Origins = []string{"https://app.example.com", "https://app.example.com"}
		}, true},
		{"rpId equals origin host", func(in *client.CreateInput) { in.RPID = "app.example.com" }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mutate(&in)
			err := in.Validate()
			if tc.wantErr && !errors.Is(err, client.ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want ok, got %v", err)
			}
		})
	}
}

func TestAuthenticateByHashAndRotation(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	ctx := context.Background()
	now := time.Now()
	svc := &client.Service{Store: client.NewDynamoStore(c, table), Now: func() time.Time { return now }}

	cl, raw, err := svc.Create(ctx, client.CreateInput{Name: "shop", RPID: "example.com", Origins: []string{"https://app.example.com"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.Authenticate(ctx, raw)
	if err != nil || got.ID != cl.ID {
		t.Fatalf("authenticate: %v %v", got.ID, err)
	}
	if _, err := svc.Authenticate(ctx, "ag_nope"); !errors.Is(err, client.ErrUnauthorized) {
		t.Fatalf("unknown key: want ErrUnauthorized, got %v", err)
	}

	newRaw, err := svc.RotateKey(ctx, cl.ID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := svc.Authenticate(ctx, newRaw); err != nil {
		t.Fatalf("new key: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("old key within grace: %v", err)
	}

	now = now.Add(25 * time.Hour)
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, client.ErrUnauthorized) {
		t.Fatalf("old key after grace: want ErrUnauthorized, got %v", err)
	}
	if _, err := svc.Authenticate(ctx, newRaw); err != nil {
		t.Fatalf("new key after grace: %v", err)
	}
}
