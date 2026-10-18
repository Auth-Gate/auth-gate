// Package webauthn adapts go-webauthn to auth-gate: one relying party per
// client, and conversion between stored and library credentials.
package webauthn

import (
	"encoding/base64"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	gowa "github.com/go-webauthn/webauthn/webauthn"

	"github.com/huy090202/auth-gate/internal/auth/client"
	"github.com/huy090202/auth-gate/internal/auth/factor"
)

// For builds the relying party for c; origin and rpId checks use the client's values.
func For(c client.Client) (*gowa.WebAuthn, error) {
	return gowa.New(&gowa.Config{RPID: c.RPID, RPDisplayName: c.Name, RPOrigins: c.Origins})
}

// EncodeID is the stored and public form of a raw credential ID.
func EncodeID(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

// ToCredential converts a freshly created credential for storage.
func ToCredential(wc *gowa.Credential, displayName string, now time.Time) factor.Credential {
	transports := make([]string, 0, len(wc.Transport))
	for _, t := range wc.Transport {
		transports = append(transports, string(t))
	}
	return factor.Credential{
		ID: EncodeID(wc.ID), PublicKey: wc.PublicKey, SignCount: wc.Authenticator.SignCount,
		AAGUID: wc.Authenticator.AAGUID, Transports: transports, DisplayName: displayName, CreatedAt: now,
	}
}

// FromCredential converts a stored credential for go-webauthn. A stored ID
// that is not valid base64url yields an empty ID, which never matches.
func FromCredential(fc factor.Credential) gowa.Credential {
	raw, _ := base64.RawURLEncoding.DecodeString(fc.ID)
	transports := make([]protocol.AuthenticatorTransport, 0, len(fc.Transports))
	for _, t := range fc.Transports {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}
	return gowa.Credential{
		ID: raw, PublicKey: fc.PublicKey, Transport: transports,
		Authenticator: gowa.Authenticator{AAGUID: fc.AAGUID, SignCount: fc.SignCount},
	}
}
