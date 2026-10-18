// Package kmsx wraps AWS KMS for ECDSA P-256 signing.
package kmsx

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

const componentLen = 32

// Client signs with KMS-held keys.
type Client struct {
	api *kms.Client
}

// New returns a KMS client built from cfg.
func New(cfg aws.Config) *Client {
	return &Client{api: kms.NewFromConfig(cfg)}
}

// Sign signs a SHA-256 digest with keyID and returns the 64-byte raw r||s.
func (c *Client) Sign(ctx context.Context, keyID string, digest []byte) ([]byte, error) {
	out, err := c.api.Sign(ctx, &kms.SignInput{
		KeyId:            aws.String(keyID),
		Message:          digest,
		MessageType:      types.MessageTypeDigest,
		SigningAlgorithm: types.SigningAlgorithmSpecEcdsaSha256,
	})
	if err != nil {
		return nil, fmt.Errorf("kmsx: sign: %w", err)
	}
	return DERToRaw(out.Signature)
}

// PublicKey fetches the ECDSA public key of keyID.
func (c *Client) PublicKey(ctx context.Context, keyID string) (*ecdsa.PublicKey, error) {
	out, err := c.api.GetPublicKey(ctx, &kms.GetPublicKeyInput{KeyId: aws.String(keyID)})
	if err != nil {
		return nil, fmt.Errorf("kmsx: get public key: %w", err)
	}
	parsed, err := x509.ParsePKIXPublicKey(out.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("kmsx: parse public key: %w", err)
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("kmsx: key is not an ECDSA public key")
	}
	return pub, nil
}

// DERToRaw converts an ASN.1 SEQUENCE{r INTEGER, s INTEGER} signature into
// 64 bytes of r||s, each left-padded to 32 bytes.
func DERToRaw(der []byte) ([]byte, error) {
	in := cryptobyte.String(der)
	var seq, rb, sb cryptobyte.String
	if !in.ReadASN1(&seq, asn1.SEQUENCE) || !in.Empty() ||
		!seq.ReadASN1(&rb, asn1.INTEGER) ||
		!seq.ReadASN1(&sb, asn1.INTEGER) || !seq.Empty() {
		return nil, errors.New("kmsx: malformed DER signature")
	}
	out := make([]byte, 2*componentLen)
	if err := fill(out[:componentLen], rb); err != nil {
		return nil, err
	}
	if err := fill(out[componentLen:], sb); err != nil {
		return nil, err
	}
	return out, nil
}

// fill right-aligns the unsigned integer v in dst, dropping DER's sign byte.
func fill(dst, v []byte) error {
	if len(v) == 0 || v[0]&0x80 != 0 {
		return errors.New("kmsx: invalid DER integer")
	}
	for len(v) > 1 && v[0] == 0 {
		v = v[1:]
	}
	if len(v) > len(dst) {
		return errors.New("kmsx: DER integer too large")
	}
	copy(dst[len(dst)-len(v):], v)
	return nil
}
