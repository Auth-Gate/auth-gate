// Package pin validates PINs against a client policy and stores them as
// argon2id records.
package pin

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"

	"golang.org/x/crypto/argon2"

	"github.com/huy090202/auth-gate/internal/auth/client"
)

// ErrRejected is returned when a PIN does not satisfy the client policy.
var ErrRejected = errors.New("pin: rejected by policy")

const (
	saltLen = 16
	keyLen  = 32
)

// Validate checks pin against the policy: exact length, digits only, not a
// single repeated digit and not a strictly ascending or descending run.
func Validate(pin string, p client.PinPolicy) error {
	if len(pin) != p.Length || p.Length <= 0 {
		return ErrRejected
	}
	for i := 0; i < len(pin); i++ {
		if pin[i] < '0' || pin[i] > '9' {
			return ErrRejected
		}
	}
	same, asc, desc := true, true, true
	for i := 1; i < len(pin); i++ {
		d := int(pin[i]) - int(pin[i-1])
		same = same && d == 0
		asc = asc && d == 1
		desc = desc && d == -1
	}
	if same || asc || desc {
		return ErrRejected
	}
	return nil
}

// Params are the argon2id cost parameters, stored with each record so that
// Current can be raised without invalidating existing records.
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
}

// Current is the cost used for new records.
var Current = Params{Memory: 65536, Time: 3, Threads: 1}

// Record is a stored PIN hash.
type Record struct {
	Hash, Salt []byte
	Params     Params
}

func hashWith(pin string, salt []byte, p Params) []byte {
	return argon2.IDKey([]byte(pin), salt, p.Time, p.Memory, p.Threads, keyLen)
}

// Hash derives a new record for pin with a random salt and the Current params.
func Hash(pin string) (Record, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return Record{}, err
	}
	return Record{Hash: hashWith(pin, salt, Current), Salt: salt, Params: Current}, nil
}

// Verify reports whether pin matches the record, using the record's own params.
func (r Record) Verify(pin string) bool {
	return subtle.ConstantTimeCompare(r.Hash, hashWith(pin, r.Salt, r.Params)) == 1
}
