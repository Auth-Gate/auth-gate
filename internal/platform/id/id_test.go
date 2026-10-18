package id

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateRejectsHashEmptyAndLong(t *testing.T) {
	for _, s := range []string{"a#b", "", strings.Repeat("x", 257)} {
		if err := Validate(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Validate(%q) = %v, want ErrInvalid", s, err)
		}
	}
	if err := Validate("user_1"); err != nil {
		t.Errorf("Validate(user_1) = %v, want nil", err)
	}
}
