# The client key is a hash lookup

## Decision

An access key is `ag_` plus 32 random bytes. The server stores only `sha256(key)` as `K#<hash>` and finds the client with one `GetItem`.

## Context

Every verify and admin request carries a key. The service must map it to a `clientId` without trusting the body, and a stolen table must not yield usable keys.

## Alternatives rejected

- Scan or query all clients and compare: cost grows with clients, and it is a variable-time loop over secrets.
- Store the raw key and compare: a table read becomes a credential leak.
- A slow hash such as argon2 for keys: keys are 256 bits of randomness, so a fast hash is enough, and a slow one would add milliseconds to every request.
- A shared key for all clients with `clientId` in the body: the body would decide the tenant.

## Consequences

- Lookup is O(1). A missing item is 401 with the same body as a missing header.
- The key is shown once, at creation or rotation; it cannot be recovered.
- Rotation writes a new `K#` item and sets `ttl` on the old one, so the old key works for 24 hours and then disappears by itself.
- `clientId` comes from the lookup; the body is never trusted for it.
