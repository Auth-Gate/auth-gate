// Package id generates and validates opaque identifiers.
package id

import (
	"errors"
	"strings"

	"github.com/oklog/ulid/v2"
)

// maxLen is the maximum identifier length in bytes.
const maxLen = 256

// ErrInvalid is returned for identifiers that are empty, too long or contain '#'.
var ErrInvalid = errors.New("invalid id")

// New returns a new ULID string.
func New() string { return ulid.Make().String() }

// Validate checks that s is 1-256 bytes and contains no '#'.
func Validate(s string) error {
	if s == "" || len(s) > maxLen || strings.Contains(s, "#") {
		return ErrInvalid
	}
	return nil
}
