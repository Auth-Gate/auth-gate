package challenge_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/huy090202/auth-gate/internal/auth/challenge"
	"github.com/huy090202/auth-gate/internal/platform/dynamo/dynamotest"
)

func newChallenge(id string, expires time.Time) challenge.Challenge {
	return challenge.Challenge{
		ID: id, ClientID: "c1", SubjectID: "u1", Purpose: "login",
		Factor: challenge.FactorPIN, Session: []byte("sess"), Status: challenge.StatusPending,
		MaxAttempts: 3, ExpiresAt: expires, CreatedAt: time.Now(),
	}
}

func TestAttemptExhaustsAndExpires(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	s := challenge.NewDynamoStore(c, table)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := s.Create(ctx, newChallenge("a", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		got, err := s.Attempt(ctx, "c1", "a", now)
		if err != nil || got.Attempts != i {
			t.Fatalf("attempt %d: attempts=%d err=%v", i, got.Attempts, err)
		}
	}
	if _, err := s.Attempt(ctx, "c1", "a", now); !errors.Is(err, challenge.ErrGone) {
		t.Fatalf("4th attempt err = %v, want ErrGone", err)
	}

	// Expired but not yet deleted by DynamoDB TTL.
	if err := s.Create(ctx, newChallenge("b", now.Add(-time.Second))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Attempt(ctx, "c1", "b", now); !errors.Is(err, challenge.ErrGone) {
		t.Fatalf("expired attempt err = %v, want ErrGone", err)
	}
}

func TestCompleteWinsExactlyOnce(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	s := challenge.NewDynamoStore(c, table)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.Create(ctx, newChallenge("a", now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.Complete(ctx, "c1", "a", now)
		}()
	}
	wg.Wait()

	wins, gone := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, challenge.ErrGone):
			gone++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || gone != n-1 {
		t.Fatalf("wins=%d gone=%d, want 1 and %d", wins, gone, n-1)
	}
}

func TestTakeRegistrationIsSingleUse(t *testing.T) {
	c, table := dynamotest.NewTable(t)
	s := challenge.NewDynamoStore(c, table)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	reg := challenge.Registration{ID: "r1", ClientID: "c1", SubjectID: "u1", DisplayName: "Laptop", Session: []byte("sess"), ExpiresAt: now.Add(time.Minute)}
	if err := s.PutRegistration(ctx, reg); err != nil {
		t.Fatal(err)
	}
	got, err := s.TakeRegistration(ctx, "c1", "r1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.SubjectID != "u1" || got.DisplayName != "Laptop" || string(got.Session) != "sess" || !got.ExpiresAt.Equal(reg.ExpiresAt) {
		t.Fatalf("got %+v, want %+v", got, reg)
	}
	if _, err := s.TakeRegistration(ctx, "c1", "r1", now); !errors.Is(err, challenge.ErrGone) {
		t.Fatalf("second take err = %v, want ErrGone", err)
	}
}
