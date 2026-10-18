# WebAuthn without a browser path

## Decision

The browser never calls auth-gate. The client backend relays: it asks auth-gate for the options, passes them to its frontend, and sends the browser's response back.

## Context

`navigator.credentials.create/get` runs in a browser, so a passkey service usually exposes a public, CORS-enabled, cookie-bearing endpoint. auth-gate is called backend to backend with a per-client key, and a browser holding that key would defeat it.

## Alternatives rejected

- A browser-facing API with CORS: needs a public credential, rate limiting, session cookies, and a way to trust the subject the browser claims to be.
- A hosted login page: the service would own identity, which is a stated non-goal.

## Consequences

- No CORS, no cookies, no browser credential. The client already knows its subject.
- auth-gate checks `origin` and `rpId` against the client's registration, not against the request, so a client can only verify passkeys for its own domains.
- The relay is the client's work: two extra hops, and the client must forward the attestation or assertion untouched.
- The real round trip needs an authenticator, so `cmd/smoke` covers it; Bruno cannot.
