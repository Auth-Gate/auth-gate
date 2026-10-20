package pin

import (
	"errors"
	"testing"

	"github.com/huy090202/auth-gate/internal/auth/client"
)

// The policy is the only guard against guessable PINs, so each rule needs a case.
func TestValidate(t *testing.T) {
	six := client.PinPolicy{Length: 6}
	four := client.PinPolicy{Length: 4}
	cases := []struct {
		pin    string
		policy client.PinPolicy
		ok     bool
	}{
		{"493817", six, true},
		{"12345", six, false},
		{"1234567", six, false},
		{"12a456", six, false},
		{"000000", six, false},
		{"123456", six, false},
		{"654321", six, false},
		{"987654", six, false},
		{"1357", four, true},
	}
	for _, c := range cases {
		err := Validate(c.pin, c.policy)
		if c.ok && err != nil {
			t.Errorf("Validate(%q) = %v, want nil", c.pin, err)
		}
		if !c.ok && !errors.Is(err, ErrRejected) {
			t.Errorf("Validate(%q) = %v, want ErrRejected", c.pin, err)
		}
	}
}

// Stored records must stay verifiable after Current changes, so Verify has to
// use the params saved in the record rather than the package default.
func TestHashVerifyHonoursStoredParams(t *testing.T) {
	rec, err := Hash("493817")
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Verify("493817") {
		t.Error("correct pin rejected")
	}
	if rec.Verify("493818") {
		t.Error("wrong pin accepted")
	}

	weak := Params{Memory: 8192, Time: 1, Threads: 1}
	rec.Params = weak
	rec.Hash = hashWith("493817", rec.Salt, weak)
	if !rec.Verify("493817") {
		t.Error("record with weaker stored params not verified")
	}
}
