package factor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/factor"
	"github.com/huy090202/auth-gate/internal/auth/pin"
	"github.com/huy090202/auth-gate/internal/platform/dynamo/dynamotest"
)

func TestPINLockoutAfterMaxAttempts(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	s := factor.NewDynamoStore(c, table)
	ctx := context.Background()
	rec := pin.Record{Hash: []byte("h"), Salt: []byte("s"), Params: pin.Params{Memory: 64, Time: 1, Threads: 1}}
	if err := s.PutPIN(ctx, "c1", "u1", factor.PIN{Record: rec, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetPIN(ctx, "c1", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.Params != rec.Params {
		t.Fatalf("params = %+v, want %+v", got.Record.Params, rec.Params)
	}

	now := time.Now().UTC().Truncate(time.Second)
	for i := 1; i <= 4; i++ {
		n, locked, err := s.RecordPINFailure(ctx, "c1", "u1", 5, 15*time.Minute, now)
		if err != nil || n != i || locked != nil {
			t.Fatalf("failure %d: n=%d locked=%v err=%v", i, n, locked, err)
		}
	}
	n, locked, err := s.RecordPINFailure(ctx, "c1", "u1", 5, 15*time.Minute, now)
	if err != nil || n != 5 || locked == nil || !locked.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("5th failure: n=%d locked=%v err=%v", n, locked, err)
	}
	got, err = s.GetPIN(ctx, "c1", "u1")
	if err != nil || got.FailedCount != 0 || got.LockedUntil == nil {
		t.Fatalf("after lock: %+v err=%v", got, err)
	}
	if err := s.ResetPINFailures(ctx, "c1", "u1"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetPIN(ctx, "c1", "u1")
	if err != nil || got.LockedUntil != nil || got.FailedCount != 0 {
		t.Fatalf("after reset: %+v err=%v", got, err)
	}
}

func TestPurgeSubjectRemovesPINAndCredentials(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	s := factor.NewDynamoStore(c, table)
	ctx := context.Background()
	rec := pin.Record{Hash: []byte("h"), Salt: []byte("s"), Params: pin.Params{Memory: 64, Time: 1, Threads: 1}}
	if err := s.PutPIN(ctx, "c1", "u1", factor.PIN{Record: rec, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"cred-a", "cred-b"} {
		cred := factor.Credential{ID: id, PublicKey: []byte("pk"), Transports: []string{"usb"}, CreatedAt: time.Now()}
		if err := s.PutCredential(ctx, "c1", "u1", cred); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PurgeSubject(ctx, "c1", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPIN(ctx, "c1", "u1"); !errors.Is(err, factor.ErrNotFound) {
		t.Fatalf("GetPIN err = %v, want ErrNotFound", err)
	}
	creds, err := s.ListCredentials(ctx, "c1", "u1")
	if err != nil || creds == nil || len(creds) != 0 {
		t.Fatalf("creds = %#v err=%v", creds, err)
	}
}
