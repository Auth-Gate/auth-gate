# A PIN goes through a challenge

## Decision

Verifying a PIN is two calls, like a passkey: `POST /api/challenges` with `factor: "pin"`, then `POST /api/challenges/{id}/verify` with the PIN.

## Context

A client could send `{subjectId, pin}` to one endpoint and get a token. That is shorter, but it makes PIN and WebAuthn two different shapes.

## Alternatives rejected

- A single `verify-pin` call: no `purpose` is forced on the caller, nothing is single-use, and "check this PIN for nothing" becomes possible. Every later feature (purpose in the token, attempt counting, replay protection) would exist twice.
- A challenge for WebAuthn only: the token code would branch on the factor.

## Consequences

- Both factors share one lifecycle: the client names a `purpose`, the challenge lives five minutes, is used once, and is counted.
- The token's `purpose` and `jti` (the challenge id) come from the same place for both.
- A PIN verification costs one extra round trip.
- Single use is a DynamoDB condition (`status = pending AND attempts < max`), so two concurrent verifies cannot both win.
- Lockout still lives on the PIN item, because PINs are guessable and passkeys are not.
