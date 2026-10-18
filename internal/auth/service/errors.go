package service

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrLocked is matched by errors.Is against any Locked value.
	ErrLocked = errors.New("service: locked")
	// ErrFactorNotFound means the subject has no such factor enrolled.
	ErrFactorNotFound = errors.New("service: factor not found")
	// ErrWrongFactor means the verify body does not match the challenge factor.
	ErrWrongFactor = errors.New("service: wrong factor")
	// ErrPINRejected means the PIN violates the client's policy.
	ErrPINRejected = errors.New("service: pin rejected")
	// errNotImplemented marks the WebAuthn paths filled in by Task 10.
	errNotImplemented = errors.New("service: not implemented")
)

// VerificationFailed is a wrong PIN or invalid assertion on a live challenge.
type VerificationFailed struct{ AttemptsRemaining int }

func (e VerificationFailed) Error() string {
	return fmt.Sprintf("service: verification failed (%d attempts remaining)", e.AttemptsRemaining)
}

// Locked means the subject's PIN is locked until Until.
type Locked struct{ Until time.Time }

func (e Locked) Error() string { return "service: locked until " + e.Until.UTC().Format(time.RFC3339) }

// Is lets errors.Is(err, ErrLocked) match a Locked value.
func (e Locked) Is(target error) bool { return target == ErrLocked }
