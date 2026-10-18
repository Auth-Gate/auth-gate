# auth-gate module — design

Date: 2026-10-18. Status: draft for review. Lands in `main` as
`docs/superpowers/specs/2026-10-18-auth-gate-module-design.md` through the first
round-2 worktree task.

## 1. Purpose

auth-gate is a backend-to-backend verification service. A calling backend (a
*client*) already knows who its user (a *subject*) is. It asks auth-gate to enroll
and later verify a second factor for that subject — a PIN or a WebAuthn passkey —
and gets back a short-lived signed assertion token it can attach to whatever the
verification was for: a step-up before a sensitive action, the second step of a
login, or a passkey-only login. auth-gate does not care which; the client does.

It is the third sibling of `activity-logger-v2` and `notification-manager-v2`
(Go, Lambda Function URLs, one DynamoDB table, shared-secret header, Bruno
OpenCollection as the integration handoff) and copies their conventions unless a
section below says otherwise and why.

## 2. Decisions already made

- Go, AWS Lambda behind Function URLs (no API Gateway), one DynamoDB table.
- One binary deployed as three functions selected by `ROLE` (like activity-logger),
  all behind **one CloudFront distribution** so integrators get a single base URL
  (optionally a custom domain). Decided 2026-10-18 after the first draft: the
  service is meant to be dropped into many projects, and a handoff with three
  raw Function URLs is not "one bundle like the siblings".
- Browsers and mobile apps never call auth-gate. Only backends do.
- Clients are registered, each with its own access key. `clientId` is derived
  from the key, never trusted from the body.
- WebAuthn is verified server-side with `go-webauthn/webauthn`.
- A successful verification yields a JWT (ES256) signed by AWS KMS; public keys
  are served as JWKS.
- v1 factors: PIN and WebAuthn. Nothing else (no TOTP, no SMS, no recovery codes).
- Open source, CV quality: the tree shows the layering, memos explain the why.

## 3. Non-goals (v1)

- User management, sessions, passwords, SSO. The client owns identity.
- A browser-facing path, CORS, cookies.
- Multi-region, usage metering, WAF rate limiting (CloudFront makes it a later
  add-on, not a v1 feature).
- Recovery flows. If a subject loses all factors the client deletes them via the
  admin role and re-enrolls.

## 4. Concepts

| Term | Meaning |
|---|---|
| Client | A calling backend. Has `clientId`, an access key, `rpId`, `origins`, `pinPolicy`, `tokenTtlSec`. |
| Subject | A user of a client. `subjectId` is chosen by the client; auth-gate treats it as an opaque string. |
| Factor | A PIN (at most one per subject) or a WebAuthn credential (many per subject). |
| Registration | A pending WebAuthn enrollment: options were issued, the attestation has not arrived yet. |
| Challenge | A pending verification attempt bound to a `purpose` and a factor. Used once. |
| Purpose | Free-form string chosen by the client (`transfer`, `login`, `change-email`). Copied into the token. |

## 5. Deployment shape: three roles from one binary

`cmd/auth-gate/main.go` reads `ROLE` at cold start and installs one of three muxes.
Each function has its own IAM role with only what its mux needs.

| ROLE | Routes | Auth | IAM (beyond logs) |
|---|---|---|---|
| `verify` | §6.1 | client access key | `dynamodb:GetItem/PutItem/UpdateItem/Query/DeleteItem` on the table; `kms:Sign` |
| `admin` | §6.2 | admin key from SSM | `dynamodb:*Item/Query` on the table; **no KMS** |
| `jwks` | §6.3 | none | `kms:GetPublicKey` only; **no DynamoDB** |

Why not one function: the thing that signs tokens must not be the thing that
edits clients, and the only public route must live in a binary path that has no
secrets. Three functions make that an IAM statement instead of a code review.
Memo: `three-roles-one-binary`.

### 5.1 One door: CloudFront in front of the three functions

A single CloudFront distribution is the only URL integrators see. Behaviours by
path, most specific first:

| Path pattern | Origin | Caching |
|---|---|---|
| `/.well-known/*` | `jwks` Function URL | honour origin `Cache-Control` (300 s) |
| `/api/clients*` | `admin` Function URL | disabled |
| `/*` (default) | `verify` Function URL | disabled |

All viewer headers except `Host` are forwarded (managed policy
`AllViewerExceptHostHeader`); only HTTPS; HTTP/2 and 3.

Direct calls to the Function URLs are refused: CloudFront adds a custom origin
header `x-origin-verify: <secret>` and the `verify` and `admin` muxes answer
`401 {"error":"unauthorized"}` when it is missing or wrong (constant-time
compare, same body as a bad access key). The secret lives in SSM
`/credo/auth-gate/<stage>/origin-verify` and is read at cold start. `jwks` does
not check it: it is public anyway.

Why a secret header rather than Lambda OAC with `AuthType: AWS_IAM`: OAC makes
CloudFront sign origin requests, but for `POST`/`PUT` the *viewer* must then
send `x-amz-content-sha256` with the body hash. Every integrator would have to
hash request bodies, which breaks "one base URL, one header". The secret header
is invisible to callers. Memo: `one-door-cloudfront`.

Custom domain is optional: parameters `DomainName` and `CertificateArn` (ACM in
`us-east-1`, created by `00-cert.sh`) add an alias and, when `HostedZoneId` is
given, a Route 53 alias record. Without them the distribution's
`*.cloudfront.net` name is the base URL. Either way `03-deploy.sh` prints exactly
one URL, and the token `iss` is `https://<that host>`.

Unknown path → 404; known path, wrong verb → 405. The access-key check wraps the
whole `verify` and `admin` muxes, so an unknown path without a key is 401 before
404 (same as notification-manager). The 401 body never contains the word "key"
or the header name.

## 6. API

All routes are under `/api` except JWKS. Request and response bodies are JSON.
Any id (`subjectId`, `clientId`, `purpose`, `credentialId`) containing `#` is
rejected with 400 before it reaches storage. Timestamps are RFC 3339 UTC.

### 6.1 `verify` role — header `x-internal-access-key: <client key>`

| Method, path | Body | Success | Notes |
|---|---|---|---|
| `POST /factors/pin` | `{subjectId, pin}` | `201 {}` | Sets or replaces the PIN. Policy violation → 400 `pin_rejected`. Resets lockout. |
| `DELETE /factors/pin/{subjectId}` | — | `204` | Idempotent. |
| `GET /factors/{subjectId}` | — | `200 {pin: bool, webauthn: [{credentialId, createdAt, lastUsedAt}]}` | Arrays are never null. |
| `POST /webauthn/registrations` | `{subjectId, displayName?}` | `201 {registrationId, publicKey}` | `publicKey` is the `PublicKeyCredentialCreationOptions` the client forwards to its frontend. Existing credentials go in `excludeCredentials`. |
| `POST /webauthn/registrations/{registrationId}` | `{credential}` (the attestation response as the browser produced it) | `201 {credentialId}` | Verified against the *client's* `rpId`/`origins`. Expired or used → 410. Bad attestation → 400 `registration_failed`. |
| `DELETE /webauthn/credentials/{credentialId}?subjectId=` | — | `204` | `subjectId` required: the key is `S#client#subject`. |
| `POST /challenges` | `{subjectId, purpose, factor: "pin"\|"webauthn"}` | `201 {challengeId, expiresAt, publicKey?}` | `publicKey` (the `PublicKeyCredentialRequestOptions`) only for `webauthn`. Subject has no such factor → 404 `factor_not_found`. PIN subject locked → 423. |
| `POST /challenges/{challengeId}/verify` | `{pin}` or `{credential}` | `200 {token, expiresAt, factor}` | See error table below. |

Verify errors:

| Case | Status, `error` | Extra |
|---|---|---|
| wrong PIN / invalid assertion | 400 `verification_failed` | `attemptsRemaining` (on the challenge) |
| subject locked (PIN) | 423 `locked` | `lockedUntil` |
| challenge expired, already verified, or attempts exhausted | 410 `challenge_gone` | — |
| body factor does not match challenge factor | 400 `wrong_factor` | — |

Why PIN goes through a challenge too: both factors get the same shape, every
verification carries a purpose, is single-use, and is counted. A client never
sends a PIN "for nothing". Memo: `pin-goes-through-a-challenge`.

### 6.2 `admin` role — header `x-internal-access-key: <admin key>`

| Method, path | Body | Success | Notes |
|---|---|---|---|
| `POST /clients` | `{name, rpId, origins: [..], pinPolicy?, tokenTtlSec?}` | `201 {clientId, accessKey}` | The key is returned **once**. Only its SHA-256 is stored. |
| `GET /clients/{clientId}` | — | `200 {clientId, name, rpId, origins, pinPolicy, tokenTtlSec, createdAt}` | Never returns key material. |
| `POST /clients/{clientId}/keys` | `{}` | `201 {accessKey}` | New key active now; previous key keeps working for 24 h, then its item expires via TTL. |
| `DELETE /clients/{clientId}/subjects/{subjectId}` | — | `204` | Deletes PIN and all credentials of one subject (right to be forgotten). Pending challenges die by TTL. |

Deleting a client is out of v1; `99-purge.sh` handles stage teardown.

### 6.3 `jwks` role — no auth

`GET /.well-known/jwks.json` → `200 {keys: [...]}`, `Cache-Control: public,
max-age=300`. Keys come from `kms:GetPublicKey` on every key id listed in
`KMS_KEY_IDS` (comma-separated; more than one only during a signing-key
rotation).

There is no `/health` or `/version` on any role (a sibling's health endpoint once
leaked its environment). Memo: `no-health-endpoint`.

### 6.4 The WebAuthn flow without a browser path

The browser still runs `navigator.credentials.create/get`, but it talks only to
the client's own backend, which relays to auth-gate:

```
browser ──► client backend ──► POST /webauthn/registrations ──► options
browser ◄── client backend ◄──────────────────────────────────┘
browser (navigator.credentials.create) ──► attestation ──► client backend
                        client backend ──► POST /webauthn/registrations/{id} ──► credentialId
```

Verification is the same with `POST /challenges` and `/challenges/{id}/verify`.
auth-gate checks `origin` and `rpId` against the *client's* registration, so a
client can only verify passkeys for its own domains. Memo: `webauthn-without-a-browser`.

## 7. Data model

One table, string `pk`/`sk`, TTL attribute `ttl`, on-demand capacity, PITR on,
default AWS encryption. No GSI in v1. The tenant (`clientId`) is always in `pk`.

| Item | `pk` | `sk` | Attributes |
|---|---|---|---|
| Client | `C#<clientId>` | `META` | name, rpId, origins, pinPolicy, tokenTtlSec, createdAt |
| Access key | `K#<sha256(key)>` | `META` | clientId, createdAt, `ttl` (only set on a superseded key) |
| PIN | `S#<clientId>#<subjectId>` | `PIN` | hash, salt, argon2 params, failedCount, lockedUntil, updatedAt |
| Credential | `S#<clientId>#<subjectId>` | `WA#<credentialId>` | publicKey (COSE), signCount, aaguid, transports, createdAt, lastUsedAt |
| Registration | `R#<clientId>#<registrationId>` | `META` | subjectId, sessionData (go-webauthn), `ttl` = now + 5 min |
| Challenge | `CH#<clientId>#<challengeId>` | `META` | subjectId, purpose, factor, sessionData (webauthn only), status, attempts, `ttl` = now + 5 min |

`clientId`, `registrationId`, `challengeId`, `credentialId` are ULIDs (credentialId
is the authenticator's, base64url). Access keys are `ag_` + 32 random bytes
base64url.

Decisions:

- **Client lookup is a hash lookup, not a scan or a compare.** `sha256(header)`
  → `GetItem K#…` → `clientId`. Raw keys are never stored. Rotation writes a new
  `K#` item and sets `ttl` on the old one.
- **Registrations and challenges are client-scoped, not subject-scoped.** The
  verify route only has the id, and a client-B key cannot see `CH#A#…`. They live
  five minutes, so subject purge only has to `Query S#client#subject`. Memo:
  `challenge-is-client-scoped`.
- **Single use is a condition, not a transaction.** Verify does
  `UpdateItem` with `ConditionExpression: #status = :pending AND attempts < :max`
  (and, for success, sets `status = verified`) before any token is issued. Two
  concurrent verifies cannot both win.
- **Lockout lives on the PIN item only.** PINs are guessable, passkeys are not.
  Wrong PIN: conditional increment of `failedCount` *before* responding; at
  `pinPolicy.maxAttempts` set `lockedUntil`. Correct PIN resets it. WebAuthn is
  bounded by 3 attempts per challenge and the 5-minute TTL. `signCount` is
  stored and checked (clone detection) by go-webauthn.

## 8. PIN

- `pinPolicy` per client, defaults: `length: 6` (allowed 4–8), `maxAttempts: 5`,
  `lockoutSec: 900`.
- Rejected at enrollment: wrong length, non-digits, all one digit (`000000`),
  ascending or descending runs (`123456`, `654321`). Nothing else — the defence
  is lockout, not complexity.
- Hash: argon2id, 16-byte random salt, parameters stored on the item
  (`m=64 MiB, t=3, p=1`, sized for a 256 MB Lambda). Old PINs verify with their
  stored parameters; re-enrolling uses the current ones. No migration needed to
  raise cost later.
- Comparison of the derived key uses `crypto/subtle.ConstantTimeCompare`.

## 9. Token and KMS

- JWT, `alg: ES256`, `kid` = KMS key id. Signed by `kms:Sign` with
  `ECDSA_SHA_256` on an `ECC_NIST_P256` `SIGN_VERIFY` key created by the SAM
  template. KMS returns DER; the signer converts to the raw `r||s` JWS form.
- Claims: `iss` = `https://<base host>` (the CloudFront or custom domain, so
  `<iss>/.well-known/jwks.json` is the key set), `sub` = subjectId, `aud` = clientId,
  `amr` = `["pin"]` or `["webauthn"]`, `purpose`, `jti` = challengeId, `iat`,
  `exp` = `iat + tokenTtlSec` (client default 300, max 900).
- Consumers verify signature via JWKS, then check `aud` is themselves and
  `purpose` matches the action. A step-up token for `transfer` does not authorise
  `change-email`.
- Signing-key rotation: create a new KMS key, add it to `KMS_KEY_IDS` on the
  `jwks` function first, switch `KMS_KEY_ID` on `verify`, remove the old id after
  15 minutes (max `exp`). JWKS is cached for 300 s.
- `token.Signer` is an interface. `kmsSigner` is used on AWS; `localSigner`
  (in-memory ECDSA) is used by `make dev` and tests and only loads when
  `SIGNER=local`. The `verify` role refuses to boot on AWS without `KMS_KEY_ID`.

Why KMS rather than a private key in SSM: anything that can read SSM could sign;
with KMS "who can sign" is one IAM line in `template.yaml`. Cost ≈ $0.03 per
10k signatures and ~10 ms per successful verify. Memo: `kms-signs-not-ssm`.

## 10. Repository layout

Go lives at the repository root, as in the siblings. The empty `apps/` and
`packages/` from round 1 are removed; pnpm stays for root tooling only
(shellcheck, Bruno CLI).

```
cmd/auth-gate/main.go            ROLE → mux → Lambda adapter (aws-lambda-go + Function URL events)
cmd/smoke/main.go                full WebAuthn round-trip against a deployed stage
internal/platform/               foundation: dynamo, kms, ssm, httpx (mux, envelope, key check), log, ulid
internal/auth/                   domain: client, pin, webauthn, challenge, token
infra/template.yaml              3 functions, 3 roles, table, KMS key, Function URLs, CloudFront door
scripts/setup-and-deploy/        00-cert.sh 01-ssm.sh 02-build.sh 03-deploy.sh 04-smoke.sh 99-purge.sh
bruno/                           opencollection.yml, folders, environments/local.yml.example
docs/memo/                       one decision per file
Makefile                         dev, build, test, lint, check
```

`internal/platform` must not import `internal/auth`. `internal/auth` talks to
storage through small interfaces it defines (`ClientStore`, `FactorStore`,
`ChallengeStore`, `Signer`) so the domain is testable without DynamoDB.

Config (env set by SAM): `ROLE`, `STAGE`, `TABLE_NAME`, `KMS_KEY_ID` (verify),
`KMS_KEY_IDS` (jwks), `ADMIN_KEY_PARAM` (admin; SSM SecureString path
`/credo/auth-gate/<stage>/admin-key`, read once at cold start, boot fails if
empty), `ORIGIN_VERIFY_PARAM` (verify and admin; SSM path
`/credo/auth-gate/<stage>/origin-verify`, same rules), `BASE_URL` (verify; the
token `iss`). `SIGNER=local` and `DYNAMO_ENDPOINT` are for local only; the
origin-verify check is skipped only in `ROLE=dev`.

Gate: `pnpm run check` calls `make check` = `go vet ./...` +
`golangci-lint run` + `go test ./...`, in addition to the round-1 shellcheck.
CI adds a DynamoDB Local service container for the store tests.

Local dev: `make dev` runs the three muxes on one `net/http` server
(`/verify/…`, `/admin/…`, `/jwks/…` prefixes) against DynamoDB Local from
`docker compose`.

## 11. Testing — only where it earns its keep

Tests go where a bug would be silent or expensive, not one per function.

| Area | Test | Why |
|---|---|---|
| PIN policy and hash | table test for accept/reject; one round-trip hash→verify; parameters stored and honoured | silent weakness otherwise |
| Challenge single-use and PIN lockout | store test against DynamoDB Local with two concurrent verifies and with N wrong PINs | the two conditional updates are the security core |
| WebAuthn | one happy path and three failures (wrong origin, wrong rpId, regressed signCount) via `descope/virtualwebauthn` | cannot be exercised from Bruno |
| Token | claims and `kid` shape; DER→raw conversion; JWKS round-trip with `localSigner` | consumers depend on exact shape |
| Key check | 401 before 404; constant-time compare; body text; missing `x-origin-verify` is 401 even with a valid access key | the boundary |
| Smoke | `04-smoke.sh`: Bruno CLI runs the collection on stg, then `cmd/smoke` does a real WebAuthn round-trip | proves the deployed path |

No tests for handlers that only marshal JSON, no mocks of the AWS SDK, no
coverage targets.

## 12. Bruno collection

`bruno/opencollection.yml`, name `[External] Auth Gate (Lambda)`. Variables:
`baseUrl`, `adminKey`, `clientKey`, `subjectId` — one URL, like
notification-manager.
`environments/local.yml.example` is tracked; real environments are gitignored.

| Folder | Content |
|---|---|
| `01-the-boundary/` | all expected to fail: no key, wrong key, client key on an admin route, `#` in subjectId, replayed challenge, challenge id of another client, `/health` → 404, a raw Function URL (from stack outputs, variable `rawVerifyUrl`) with a valid key → 401 |
| `02-clients/` | create client (captures `clientKey`, `clientId`), get, rotate key |
| `03-pin/` | enroll, challenge, verify ok, verify wrong ×N → 423, delete |
| `04-webauthn/` | begin registration (asserts options shape and `rp.id`); finish with garbage → 400; docs point to `cmd/smoke` for the real round-trip |
| `05-tokens/` | decode the token from `03-pin`, fetch JWKS, verify signature and claims in an `after-response` script |

Request docs follow the siblings: why, expected status in bold, what breaks
on prd.

## 13. Memos (`docs/memo/`)

`three-roles-one-binary`, `pin-goes-through-a-challenge`, `kms-signs-not-ssm`,
`challenge-is-client-scoped`, `webauthn-without-a-browser`, `no-health-endpoint`,
`client-key-is-a-hash-lookup`, `one-door-cloudfront`.

## 14. Definition of done

- `pnpm run check` green locally and in CI.
- `03-deploy.sh` to stg prints one base URL (CloudFront or custom domain);
  `04-smoke.sh` passes through it; the raw Function URLs refuse direct calls.
- Bruno collection runs clean against stg with the CLI; `01-the-boundary/`
  all fail as asserted.
- README (collection `docs:` and repo README) explain the credential model, the
  relay flow for WebAuthn, and how a consumer verifies a token.
- Every memo listed in §13 exists.

## 15. Small decisions

- `displayName` is optional on registration begin (defaults to `subjectId`), is
  used for `user.name`/`user.displayName` in the creation options, and is stored
  on the credential item only. It is never shown back to callers.
- `origins` are exact-match strings and may include a port
  (`http://localhost:3000`), which is what go-webauthn compares against.
