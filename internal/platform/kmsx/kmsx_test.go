package kmsx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"testing"
)

// KMS returns DER signatures; the token layer needs fixed-width r||s, so the
// conversion must keep 32-byte width and reject malformed input.
func TestDERToRaw(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("payload"))

	sawHighBit := false
	for i := 0; i < 200 && !sawHighBit; i++ {
		der, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		raw, err := DERToRaw(der)
		if err != nil {
			t.Fatalf("DERToRaw: %v", err)
		}
		if len(raw) != 64 {
			t.Fatalf("len = %d, want 64", len(raw))
		}
		r := new(big.Int).SetBytes(raw[:32])
		s := new(big.Int).SetBytes(raw[32:])
		if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
			t.Fatal("signature does not verify")
		}
		// DER encodes r with a leading 0x00 when its high bit is set.
		if raw[0]&0x80 != 0 {
			sawHighBit = true
		}
	}
	if !sawHighBit {
		t.Fatal("never produced an r with the high bit set")
	}

	der, _ := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if _, err := DERToRaw(der[:len(der)-3]); err == nil {
		t.Fatal("truncated DER: want error")
	}
}
