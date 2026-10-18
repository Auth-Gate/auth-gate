# auth-gate Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship v1 of auth-gate: a Go Lambda service that enrolls and verifies PINs and WebAuthn passkeys for registered backend clients and returns KMS-signed assertion tokens, with a Bruno collection as the integration handoff.

**Architecture:** One Go binary, three `ROLE`s (`verify`, `admin`, `jwks`) each deployed as its own Lambda Function URL with its own IAM role, all behind one CloudFront distribution (optional custom domain) so integrators see a single base URL; raw Function URLs refuse calls without CloudFront's `x-origin-verify` header. One DynamoDB table (`pk`/`sk`, tenant in `pk`). `internal/platform` is foundation (http, dynamo, kms, ssm, ids); `internal/auth` is domain (client, pin, factor, challenge, token, webauthn, service, handler) and talks to storage through interfaces it defines.

**Tech Stack:** Go 1.26, `aws-lambda-go` + `aws-lambda-go-api-proxy/httpadapter`, `aws-sdk-go-v2` (dynamodb, kms, ssm), `go-webauthn/webauthn`, `oklog/ulid/v2`, `x/crypto/argon2`, `descope/virtualwebauthn` (tests), AWS SAM, DynamoDB Local (docker compose), golangci-lint v2, Bruno OpenCollection.

**Spec:** `docs/superpowers/specs/2026-10-18-auth-gate-module-design.md` (landed by Task 0 from `handoffs/feat-auth-gate/spec.md` on the `orchestrator` branch).

## Global Constraints

- Module path `github.com/huy090202/auth-gate`; `go 1.26` in `go.mod`.
- Code, comments, docs, commit messages in English.
- Any id containing `#` → `400 invalid_id` before storage. Ids are 1–256 bytes.
- The 401 body is exactly `{"error":"unauthorized"}`; no `details`, never the word "key" or the header name.
- Error envelope: `{"error": "<snake_case_code>", ...extras}`. Arrays in success bodies are never `null`.
- Header name `x-internal-access-key`. CloudFront origin header `x-origin-verify`. SSM paths `/credo/auth-gate/<stage>/admin-key` and `/credo/auth-gate/<stage>/origin-verify`. Region `ap-southeast-1` (ACM certificate for CloudFront in `us-east-1`).
- Integrators see one base URL (CloudFront, optionally a custom domain). Raw Function URLs answer 401 without the origin header; `jwks` is exempt.
- TTLs: registrations and challenges 5 min; superseded access keys 24 h. Token `tokenTtlSec` default 300, max 900. PIN defaults `length 6` (4–8), `maxAttempts 5`, `lockoutSec 900`. WebAuthn max 3 attempts per challenge.
- `internal/platform/**` must not import `internal/auth/**`.
- Tests only where the spec's §11 table puts them; no handler-marshalling tests, no AWS SDK mocks, no coverage targets.
- Commit after every task with a conventional message. Steps marked **HUMAN** (AWS deploys, SSM writes, pushes) are run by the human, never by Claude.
- No `/health`, no `/version`, no `GET` challenge by id.

## Review Focus

1. A challenge whose `expiresAt` passed but whose item DynamoDB TTL has not yet deleted (TTL lags up to 48 h) must still be `410 challenge_gone` — `Attempt` checks `expiresAt` itself (test in Task 6).
2. A superseded access key past its 24 h grace must stop authenticating even if the `K#` item still exists — `Authenticate` checks `ttl` itself (test in Task 3).
3. A body with the right field but wrong JSON type (`{"pin": 123456}`) or malformed JSON must be `400 invalid_json`, never a 500 (test in Task 1).
4. `subjectId` empty or longer than 256 bytes must be `400 invalid_id` (test in Task 1).
5. `POST /clients` with `tokenTtlSec > 900`, an origin without a scheme, or an `rpId` that is not a registrable suffix of every origin host must be `400 invalid_client` (test in Task 3).

---

## File structure

```
go.mod, go.sum, Makefile, docker-compose.yml, .golangci.yml
cmd/auth-gate/main.go                     ROLE → mux → Function URL adapter / local server
cmd/smoke/main.go                         WebAuthn round-trip against a deployed stage
internal/platform/id/id.go                New() ULID, Validate(id) error
internal/platform/httpx/json.go           DecodeJSON, WriteJSON, WriteError
internal/platform/httpx/keycheck.go       KeyFunc, RequireKey, StaticKey
internal/platform/dynamo/dynamo.go        New(ctx, endpoint) *dynamodb.Client, EnsureTable
internal/platform/dynamo/dynamotest/      NewTable(t) test helper (skips without DYNAMO_ENDPOINT)
internal/platform/kmsx/kmsx.go            Sign (DER→raw), PublicKey
internal/platform/ssmx/ssmx.go            GetSecureString
internal/platform/config/config.go        Load(getenv) (Config, error) with per-role boot checks
internal/auth/client/                     Client, PinPolicy, access keys, Store, Service, context
internal/auth/pin/                        Policy validation, argon2id Hash/Verify
internal/auth/factor/                     PIN + credential items, lockout, purge
internal/auth/challenge/                  Challenge + Registration items, Attempt/Complete
internal/auth/token/                      Signer, LocalSigner, KMSSigner, Issue, JWKS
internal/auth/webauthn/                   go-webauthn adapter per client
internal/auth/service/                    Service: the use cases (enroll, challenge, verify, admin)
internal/auth/handler/                    verify.go, admin.go, jwks.go: thin HTTP → Service
infra/template.yaml
scripts/setup-and-deploy/01-ssm.sh 02-build.sh 03-deploy.sh 04-smoke.sh 99-purge.sh
bruno/opencollection.yml, bruno/01-…/, bruno/environments/local.example.yml
docs/memo/*.md
```

---

### Task 0: Land the docs and convert the repo to a Go module

**Files:**
- Create: `docs/superpowers/specs/2026-10-18-auth-gate-module-design.md` (copy of `orchestrator:handoffs/feat-auth-gate/spec.md`), `docs/superpowers/plans/2026-10-18-auth-gate-module.md` (this file)
- Create: `go.mod`, `Makefile`, `.golangci.yml`, `docker-compose.yml`, `cmd/auth-gate/main.go` (placeholder `func main() {}`)
- Modify: `package.json` (`check` → add `make check`; drop `typecheck`), `.github/workflows/ci.yml`, `.gitignore`, `README.md` (layout block)
- Delete: `pnpm-workspace.yaml`, `apps/`, `packages/`

**Interfaces:**
- Produces: `make check` = `go vet ./... && golangci-lint run && go test ./...`; `make test-store` = `docker compose up -d dynamodb && DYNAMO_ENDPOINT=http://localhost:8000 go test ./...`; `make dev` (filled in Task 11).

- [x] **Step 1: Copy spec and plan into `docs/superpowers/`**

```bash
git show orchestrator:handoffs/feat-auth-gate/spec.md > docs/superpowers/specs/2026-10-18-auth-gate-module-design.md
git show orchestrator:handoffs/feat-auth-gate/plan.md > docs/superpowers/plans/2026-10-18-auth-gate-module.md
```

- [x] **Step 2: `go mod init github.com/huy090202/auth-gate`; write `Makefile` with targets `build`, `test`, `test-store`, `lint`, `check`, `dev`; `.golangci.yml` v2 with default linters plus `errcheck`, `gosec`, `staticcheck`**

- [x] **Step 3: `docker-compose.yml` with one service `dynamodb` (`amazon/dynamodb-local`, `-jar DynamoDBLocal.jar -inMemory -sharedDb`, port 8000)**

- [x] **Step 4: Remove the empty pnpm workspace; `package.json` scripts: `lint` unchanged, `test` = round-1 shell tests, `check` = `pnpm run lint && pnpm run test && make check`**

- [x] **Step 5: CI: add `actions/setup-go@v5` (`go-version-file: go.mod`), `golangci/golangci-lint-action@v8`, and a `services: dynamodb` container (`amazon/dynamodb-local`, port 8000) with `DYNAMO_ENDPOINT=http://localhost:8000` on the check step. `.gitignore`: add `bin/`, `.aws-sam/`**

- [x] **Step 6: Run `pnpm run check`** — Expected: green (shellcheck, shell tests, empty Go module).

- [x] **Step 7: Commit** — `chore: land auth-gate spec and plan; convert repo to a Go module`

---

### Task 1: platform/id and platform/httpx

**Files:**
- Create: `internal/platform/id/id.go`, `internal/platform/id/id_test.go`
- Create: `internal/platform/httpx/json.go`, `internal/platform/httpx/keycheck.go`, `internal/platform/httpx/httpx_test.go`

**Interfaces:**
- Produces:
  - `id.New() string` (ULID), `id.Validate(s string) error` (empty, >256 bytes, or contains `#` → `id.ErrInvalid`).
  - `httpx.DecodeJSON(r *http.Request, dst any) error` (1 MiB limit, `DisallowUnknownFields` off, type errors wrapped as `httpx.ErrBadJSON`).
  - `httpx.WriteJSON(w, status int, v any)`; `httpx.WriteError(w, status int, code string, extra map[string]any)`.
  - `type KeyFunc func(ctx context.Context, key string) (context.Context, bool)`; `httpx.RequireKey(header string, fn KeyFunc) func(http.Handler) http.Handler` reading the named header, 401 on missing/false; `httpx.StaticKey(expected string) KeyFunc` using `crypto/subtle.ConstantTimeCompare`. The same middleware serves both the access key (`x-internal-access-key`) and the CloudFront origin check (`x-origin-verify`, outermost).
  - Routing uses Go's `http.ServeMux` method patterns (`"POST /api/factors/pin"`), which gives 404/405 for free; `RequireKey` wraps the whole mux so 401 precedes 404.

- [x] **Step 1: Write failing tests**

```go
func TestValidateRejectsHashEmptyAndLong(t *testing.T)   // "a#b", "", strings.Repeat("x",257) → ErrInvalid; "user_1" → nil
func TestRequireKeyAnswers401BeforeRoutingAndNeverSaysKey(t *testing.T)
// mux with "GET /api/x"; wrapped := RequireKey("x-origin-verify", StaticKey("cf"))(RequireKey("x-internal-access-key", StaticKey("s3cret"))(mux))
// GET /nope without headers → 401, body == `{"error":"unauthorized"}`+"\n"
// GET /nope with both headers → 404 ; GET /api/x with origin ok, key "wrong" → 401 ; POST /api/x with both → 405
// GET /api/x with valid key but no x-origin-verify → 401 (direct Function URL call)
func TestDecodeJSONRejectsMalformedAndWrongType(t *testing.T)
// body `{"pin":123456}` into struct{Pin string} → errors.Is(err, ErrBadJSON); body `{` → ErrBadJSON
```

- [x] **Step 2: Run `go test ./internal/platform/...`** — Expected: FAIL, undefined symbols.
- [x] **Step 3: Implement the four files to the signatures above.**
- [x] **Step 4: Run `go test ./internal/platform/...`** — Expected: PASS.
- [x] **Step 5: Commit** — `feat(platform): ids, json envelope and access-key middleware`

---

### Task 2: platform/dynamo and the test table helper

**Files:**
- Create: `internal/platform/dynamo/dynamo.go`, `internal/platform/dynamo/dynamotest/table.go`

**Interfaces:**
- Produces:
  - `dynamo.New(ctx context.Context, endpoint string) (*dynamodb.Client, error)` — empty endpoint = real AWS; non-empty = `BaseEndpoint` + static dummy credentials.
  - `dynamo.EnsureTable(ctx, c *dynamodb.Client, name string) error` — creates `pk`/`sk` string keys, on-demand, TTL attribute `ttl`; no-op if it exists.
  - `dynamo.Key(pk, sk string) map[string]types.AttributeValue`.
  - `dynamotest.NewTable(t *testing.T) (*dynamodb.Client, string)` — `t.Skip` when `DYNAMO_ENDPOINT` is unset; otherwise creates `authgate-test-<ulid>` and deletes it in `t.Cleanup`.

- [x] **Step 1: Implement both files.** No unit test of its own: the helper is exercised by every store test from Task 3 on.
- [x] **Step 2: `go vet ./... && go build ./...`** — Expected: clean.
- [x] **Step 3: Commit** — `feat(platform): dynamodb client and test table helper`

---

### Task 3: auth/client — registry, access keys, authentication

**Files:**
- Create: `internal/auth/client/client.go`, `key.go`, `store.go`, `context.go`, `client_test.go`

**Interfaces:**
- Produces:
  ```go
  type PinPolicy struct{ Length, MaxAttempts, LockoutSec int }     // Default(): 6, 5, 900
  type Client struct{ ID, Name, RPID string; Origins []string; PinPolicy PinPolicy; TokenTTLSec int; CreatedAt time.Time }
  type CreateInput struct{ Name, RPID string; Origins []string; PinPolicy *PinPolicy; TokenTTLSec *int }
  func (in CreateInput) Validate() error       // ErrInvalid: empty name/rpId, no origins, origin without http(s) scheme or with path, rpId not equal to or a registrable suffix of each origin host, Length∉[4,8], TokenTTLSec∉[1,900]
  func NewAccessKey() (raw string, hash string) // raw = "ag_"+base64url(32 random bytes); hash = hex(sha256(raw))
  func HashKey(raw string) string
  type Store interface {
      PutClient(ctx, Client) error; GetClient(ctx, id string) (Client, error)       // ErrNotFound
      PutKey(ctx, hash, clientID string, expiresAt *time.Time) error               // expiresAt nil = active; non-nil sets ttl
      GetKey(ctx, hash string) (clientID string, expiresAt *time.Time, err error) // ErrNotFound
      ExpireKey(ctx, hash string, at time.Time) error
  }
  func NewDynamoStore(c *dynamodb.Client, table string) Store
  type Service struct{ Store Store; Now func() time.Time }
  func (s *Service) Create(ctx, in CreateInput) (Client, rawKey string, err error)
  func (s *Service) Get(ctx, id string) (Client, error)
  func (s *Service) RotateKey(ctx, id string) (rawKey string, err error) // all currently active keys get expiresAt = now+24h
  func (s *Service) Authenticate(ctx, rawKey string) (Client, error)   // ErrUnauthorized; also when expiresAt != nil && expiresAt < now
  func (s *Service) KeyFunc() httpx.KeyFunc                              // puts Client in ctx
  func FromContext(ctx) (Client, bool)
  ```
  Items: `C#<id>/META`, `K#<hash>/META` with `clientId`, `createdAt`, optional `ttl` (unix seconds). Rotation needs the active hashes: store them on the client item as `keyHashes []string`.

- [x] **Step 1: Write failing tests**

```go
func TestCreateInputValidate(t *testing.T) // table: ok; tokenTtlSec 901 → ErrInvalid; origin "app.example.com" (no scheme) → ErrInvalid; rpId "other.com" for origin https://app.example.com → ErrInvalid; rpId "example.com" for https://app.example.com → ok
func TestAuthenticateByHashAndRotation(t *testing.T)   // dynamotest; create → Authenticate(raw) ok; Authenticate("ag_nope") → ErrUnauthorized
// RotateKey → new raw works; old raw still works; svc.Now = +25h → old raw ErrUnauthorized although the item exists; new still works
```

- [x] **Step 2: Run `DYNAMO_ENDPOINT=http://localhost:8000 go test ./internal/auth/client/`** (after `docker compose up -d`) — Expected: FAIL.
- [x] **Step 3: Implement to the signatures above.**
- [x] **Step 4: Run again** — Expected: PASS. Also `go test ./internal/auth/client/` without the endpoint — Expected: validate test PASS, store test SKIP.
- [x] **Step 5: Commit** — `feat(client): registry with hashed per-client access keys`

---

### Task 4: auth/pin — policy and argon2id

**Files:**
- Create: `internal/auth/pin/pin.go`, `internal/auth/pin/pin_test.go`

**Interfaces:**
- Produces:
  ```go
  var ErrRejected error
  func Validate(pin string, p client.PinPolicy) error // length == p.Length, digits only, not all one digit, not a strictly ascending or descending run (step ±1)
  type Params struct{ Memory uint32; Time uint32; Threads uint8 } // Current = {65536, 3, 1}
  type Record struct{ Hash, Salt []byte; Params Params }
  func Hash(pin string) (Record, error)                 // 16-byte random salt, 32-byte key, Current params
  func (r Record) Verify(pin string) bool               // argon2.IDKey with r.Params, subtle.ConstantTimeCompare
  ```

- [x] **Step 1: Write failing tests**

```go
func TestValidate(t *testing.T) // 6-digit policy: "493817" ok; "12345" / "1234567" / "12a456" / "000000" / "123456" / "654321" / "987654" → ErrRejected; 4-digit policy: "1357" ok
func TestHashVerifyHonoursStoredParams(t *testing.T) // rec := Hash("493817"); rec.Verify("493817") true; rec.Verify("493818") false
// rec.Params = Params{8192,1,1}; re-derive via a helper hashWith(pin, salt, params) → Verify true with the weaker params, proving params are read from the record
```

- [x] **Step 2: Run `go test ./internal/auth/pin/`** — Expected: FAIL.
- [x] **Step 3: Implement.** Expose `hashWith(pin string, salt []byte, p Params) []byte` unexported and used by both `Hash` and `Verify`.
- [x] **Step 4: Run** — Expected: PASS.
- [x] **Step 5: Commit** — `feat(pin): policy validation and argon2id records`

---

### Task 5: auth/factor — PIN and credential items, lockout, purge

**Files:**
- Create: `internal/auth/factor/factor.go`, `store.go`, `factor_test.go`

**Interfaces:**
- Produces:
  ```go
  type PIN struct{ Record pin.Record; FailedCount int; LockedUntil *time.Time; UpdatedAt time.Time }
  type Credential struct{ ID string; PublicKey []byte; SignCount uint32; AAGUID []byte; Transports []string; DisplayName string; CreatedAt time.Time; LastUsedAt *time.Time }
  type Store interface {
      PutPIN(ctx, clientID, subjectID string, p PIN) error
      GetPIN(ctx, clientID, subjectID string) (PIN, error)           // ErrNotFound
      DeletePIN(ctx, clientID, subjectID string) error               // idempotent
      RecordPINFailure(ctx, clientID, subjectID string, maxAttempts int, lockout time.Duration, now time.Time) (failedCount int, lockedUntil *time.Time, err error)
      ResetPINFailures(ctx, clientID, subjectID string) error
      PutCredential(ctx, clientID, subjectID string, c Credential) error
      ListCredentials(ctx, clientID, subjectID string) ([]Credential, error)  // never nil
      GetCredential(ctx, clientID, subjectID, credentialID string) (Credential, error)
      UpdateCredentialUse(ctx, clientID, subjectID, credentialID string, signCount uint32, now time.Time) error
      DeleteCredential(ctx, clientID, subjectID, credentialID string) error
      PurgeSubject(ctx, clientID, subjectID string) error            // Query S#client#subject, BatchWrite deletes in chunks of 25
  }
  func NewDynamoStore(c *dynamodb.Client, table string) Store
  ```
  `RecordPINFailure` is one `UpdateItem`: `ADD failedCount :one`; when the returned count ≥ maxAttempts, a second `UpdateItem` sets `lockedUntil` and resets `failedCount` to 0. Keys: `S#<clientId>#<subjectId>` / `PIN` and `WA#<credentialId>`.

- [x] **Step 1: Write failing test**

```go
func TestPINLockoutAfterMaxAttempts(t *testing.T) // dynamotest; PutPIN; 4× RecordPINFailure(max 5) → lockedUntil nil, counts 1..4
// 5th → lockedUntil = now+15m, failedCount reset 0; ResetPINFailures → GetPIN shows LockedUntil nil
func TestPurgeSubjectRemovesPINAndCredentials(t *testing.T) // PutPIN + 2 PutCredential; PurgeSubject; GetPIN ErrNotFound; ListCredentials len 0, not nil
```

- [x] **Step 2: Run with endpoint** — Expected: FAIL.
- [x] **Step 3: Implement.**
- [x] **Step 4: Run** — Expected: PASS.
- [x] **Step 5: Commit** — `feat(factor): pin and credential storage with lockout and purge`

---

### Task 6: auth/challenge — single-use challenges and pending registrations

**Files:**
- Create: `internal/auth/challenge/challenge.go`, `store.go`, `challenge_test.go`

**Interfaces:**
- Produces:
  ```go
  type Factor string // "pin" | "webauthn"
  type Challenge struct{ ID, ClientID, SubjectID, Purpose string; Factor Factor; Session []byte; Status string; Attempts, MaxAttempts int; ExpiresAt, CreatedAt time.Time }
  type Registration struct{ ID, ClientID, SubjectID, DisplayName string; Session []byte; ExpiresAt time.Time }
  var ErrGone error
  type Store interface {
      Create(ctx, Challenge) error
      Attempt(ctx, clientID, id string, now time.Time) (Challenge, error)
      // UpdateItem ADD attempts 1 with ConditionExpression "#status = :pending AND attempts < maxAttempts AND expiresAt > :now"; condition failure or missing → ErrGone; returns the post-increment item
      Complete(ctx, clientID, id string, now time.Time) error
      // SET status = verified with ConditionExpression "#status = :pending AND expiresAt > :now" → ErrGone on failure
      PutRegistration(ctx, Registration) error
      TakeRegistration(ctx, clientID, id string, now time.Time) (Registration, error) // Get + conditional Delete; ErrGone if missing or expired
  }
  func NewDynamoStore(c *dynamodb.Client, table string) Store
  ```
  Keys `CH#<clientId>#<id>/META`, `R#<clientId>#<id>/META`; both set `ttl = expiresAt.Unix()`.

- [x] **Step 1: Write failing tests**

```go
func TestAttemptExhaustsAndExpires(t *testing.T) // MaxAttempts 3: Attempt ×3 ok (Attempts 1,2,3), 4th ErrGone; another challenge with ExpiresAt = now-1s and no TTL deletion yet → Attempt ErrGone
func TestCompleteWinsExactlyOnce(t *testing.T)   // 8 goroutines call Complete on the same pending challenge; exactly one nil, seven ErrGone
func TestTakeRegistrationIsSingleUse(t *testing.T) // Take ok, second Take ErrGone
```

- [x] **Step 2: Run with endpoint** — Expected: FAIL.
- [x] **Step 3: Implement.**
- [x] **Step 4: Run** — Expected: PASS.
- [x] **Step 5: Commit** — `feat(challenge): single-use challenges and pending registrations`

---

### Task 7: auth/token — Signer, LocalSigner, Issue, JWKS

**Files:**
- Create: `internal/auth/token/signer.go`, `token.go`, `jwks.go`, `token_test.go`

**Interfaces:**
- Produces:
  ```go
  type Signer interface {
      KeyID() string
      Sign(ctx context.Context, digest []byte) (rawRS []byte, error) // 64 bytes r||s
      PublicKey(ctx context.Context) (*ecdsa.PublicKey, error)
  }
  func NewLocalSigner(kid string) (*LocalSigner, error) // P-256 key in memory
  type Claims struct{ Issuer, Subject, Audience, Purpose, JTI string; AMR []string; IssuedAt, ExpiresAt time.Time }
  func Issue(ctx, s Signer, c Claims) (string, error) // header {"alg":"ES256","typ":"JWT","kid":s.KeyID()}; JSON claims iss sub aud amr purpose jti iat exp (iat/exp unix ints); sha256 over "h.p"; base64url no padding
  func JWKS(ctx, signers ...Signer) (map[string]any, error) // {"keys":[{kty EC, crv P-256, x, y, kid, alg ES256, use sig}]}
  func VerifyWithJWKS(token string, jwks map[string]any) (Claims, error) // test-only helper; also used by cmd/smoke
  ```

- [x] **Step 1: Write failing tests**

```go
func TestIssueProducesExpectedHeaderAndClaims(t *testing.T) // decode header: alg ES256, kid "local-1"; payload: aud "cl_1", amr ["pin"], purpose "transfer", exp-iat == 300
func TestJWKSRoundTrip(t *testing.T) // Issue with LocalSigner; JWKS(); VerifyWithJWKS → same Claims; flip one signature byte → error; kid not in set → error
```

- [x] **Step 2: Run `go test ./internal/auth/token/`** — Expected: FAIL.
- [x] **Step 3: Implement.**
- [x] **Step 4: Run** — Expected: PASS.
- [x] **Step 5: Commit** — `feat(token): ES256 assertion tokens with a pluggable signer and JWKS`

---

### Task 8: platform/kmsx and platform/ssmx, token.KMSSigner

**Files:**
- Create: `internal/platform/kmsx/kmsx.go`, `kmsx_test.go`, `internal/platform/ssmx/ssmx.go`, `internal/auth/token/kms_signer.go`

**Interfaces:**
- Produces:
  - `kmsx.DERToRaw(der []byte) ([]byte, error)` — ASN.1 `SEQUENCE{r INTEGER, s INTEGER}` → 64 bytes, left-padded.
  - `kmsx.New(cfg aws.Config) *Client`; `(*Client).Sign(ctx, keyID string, digest []byte) ([]byte, error)` (`SigningAlgorithm ECDSA_SHA_256`, `MessageType DIGEST`, returns `DERToRaw`); `(*Client).PublicKey(ctx, keyID string) (*ecdsa.PublicKey, error)` (`x509.ParsePKIXPublicKey`).
  - `ssmx.GetSecureString(ctx, cfg aws.Config, name string) (string, error)` (`WithDecryption`).
  - `token.NewKMSSigner(c *kmsx.Client, keyID string) Signer` (`KeyID()` = keyID).

- [x] **Step 1: Write failing test**

```go
func TestDERToRaw(t *testing.T) // sign a digest with crypto/ecdsa.SignASN1 → DERToRaw → ecdsa.Verify(pub, digest, r, s) true; r with a leading 0x00 (high bit set) still 32 bytes; truncated DER → error
```

- [x] **Step 2: Run** — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** — PASS; `go build ./...` clean.
- [x] **Step 5: Commit** — `feat(platform): kms signing, ssm parameters, kms-backed token signer`

---

### Task 9: auth/service and verify handlers — the PIN path end to end

**Files:**
- Create: `internal/auth/service/service.go`, `errors.go`, `service_test.go`
- Create: `internal/auth/handler/verify.go`, `internal/auth/handler/respond.go`

**Interfaces:**
- Produces:
  ```go
  // service
  type Service struct{ Clients *client.Service; Factors factor.Store; Challenges challenge.Store; Signer token.Signer; Issuer string; Now func() time.Time }
  var ErrLocked, ErrFactorNotFound, ErrWrongFactor, ErrPINRejected error // plus challenge.ErrGone, id.ErrInvalid passed through
  type VerificationFailed struct{ AttemptsRemaining int }  // implements error
  type Locked struct{ Until time.Time }                    // implements error
  func (s *Service) EnrollPIN(ctx, c client.Client, subjectID, pin string) error      // Validate id+pin, Hash, PutPIN with FailedCount 0 / LockedUntil nil
  func (s *Service) DeletePIN(ctx, c client.Client, subjectID string) error
  func (s *Service) Factors(ctx, c client.Client, subjectID string) (FactorsView, error) // {PIN bool; WebAuthn []CredentialView}
  func (s *Service) BeginChallenge(ctx, c client.Client, subjectID, purpose string, f challenge.Factor) (challenge.Challenge, *protocol.CredentialAssertion, error)
  // pin: GetPIN (ErrFactorNotFound / Locked); MaxAttempts = PinPolicy.MaxAttempts; webauthn: Task 10
  func (s *Service) VerifyPIN(ctx, c client.Client, challengeID, pin string) (Token, error)
  // Attempt → (wrong factor → ErrWrongFactor) → GetPIN → Locked? → Verify → fail: RecordPINFailure then VerificationFailed{Remaining: MaxAttempts-Attempts} or Locked → ok: Complete (ErrGone if lost the race) → ResetPINFailures → Issue
  type Token struct{ Token string; ExpiresAt time.Time; Factor challenge.Factor }
  // handler
  func NewVerify(svc *service.Service, clients *client.Service) http.Handler // ServeMux with the 6 PIN/challenge routes + GET /factors; RequireKey("x-internal-access-key", clients.KeyFunc()) around it; WebAuthn routes added in Task 10. The origin-verify wrapper is applied in main (Task 11), not here, so service tests and ROLE=dev stay simple.
  ```
  `respond.go`: `writeServiceError(w, err)` maps: `id.ErrInvalid` → 400 `invalid_id`; `httpx.ErrBadJSON` → 400 `invalid_json`; `ErrPINRejected` → 400 `pin_rejected`; `VerificationFailed` → 400 `verification_failed` + `attemptsRemaining`; `Locked` → 423 `locked` + `lockedUntil`; `challenge.ErrGone` → 410 `challenge_gone`; `ErrWrongFactor` → 400 `wrong_factor`; `ErrFactorNotFound` → 404 `factor_not_found`; else 500 `internal` (logged).

- [x] **Step 1: Write failing tests (service level, real stores on dynamotest, LocalSigner)**

```go
func TestPINVerifyIssuesTokenOnce(t *testing.T) // EnrollPIN "493817"; BeginChallenge(pin,"transfer"); VerifyPIN ok → Claims aud==client.ID, sub==subject, amr ["pin"], purpose "transfer", jti==challengeID, exp-iat==300; VerifyPIN again same challenge → ErrGone
func TestPINLockoutThroughService(t *testing.T)  // policy max 5 (challenge MaxAttempts = 5): 4 wrong → VerificationFailed{Remaining 4,3,2,1}; 5th wrong → Locked{Until now+15m}; correct PIN on a new challenge → BeginChallenge already returns Locked; EnrollPIN again → BeginChallenge ok
func TestWrongFactorAndMissingFactor(t *testing.T) // BeginChallenge(webauthn) for subject without credentials → ErrFactorNotFound; BeginChallenge(pin) then VerifyWebAuthn body → ErrWrongFactor (VerifyWebAuthn stubbed to the factor check in this task)
```

- [x] **Step 2: Run with endpoint** — Expected: FAIL. **Step 3: Implement service, handlers, respond.** **Step 4: Run** — PASS.
- [x] **Step 5: Commit** — `feat(service): pin enrollment, challenges and assertion tokens`

---

### Task 10: WebAuthn — adapter, registration, assertion, handlers

**Files:**
- Create: `internal/auth/webauthn/webauthn.go`, `user.go`, `webauthn_test.go`
- Modify: `internal/auth/service/service.go` (BeginRegistration, FinishRegistration, BeginChallenge webauthn branch, VerifyWebAuthn, DeleteCredential), `internal/auth/handler/verify.go` (3 webauthn routes)

**Interfaces:**
- Produces:
  ```go
  // webauthn package: one go-webauthn instance per client, built on demand
  func For(c client.Client) (*webauthn.WebAuthn, error) // Config{RPID: c.RPID, RPDisplayName: c.Name, RPOrigins: c.Origins}
  func NewUser(subjectID, displayName string, creds []factor.Credential) webauthn.User // WebAuthnID = []byte(subjectID)
  func ToCredential(wc *webauthn.Credential, displayName string, now time.Time) factor.Credential
  func FromCredential(fc factor.Credential) webauthn.Credential
  // service additions
  func (s *Service) BeginRegistration(ctx, c client.Client, subjectID, displayName string) (challenge.Registration, *protocol.CredentialCreation, error) // excludeCredentials from ListCredentials; Session = json(SessionData)
  func (s *Service) FinishRegistration(ctx, c client.Client, registrationID string, body []byte) (credentialID string, err error) // TakeRegistration; protocol.ParseCredentialCreationResponseBody; w.CreateCredential; PutCredential; ErrRegistrationFailed → 400 registration_failed
  func (s *Service) VerifyWebAuthn(ctx, c client.Client, challengeID string, body []byte) (Token, error)
  // Attempt → factor check → ParseCredentialRequestResponseBody → ValidateLogin; error OR cred.Authenticator.CloneWarning → VerificationFailed; ok → Complete → UpdateCredentialUse(signCount) → Issue amr ["webauthn"]
  func (s *Service) DeleteCredential(ctx, c client.Client, subjectID, credentialID string) error
  var ErrRegistrationFailed error // service/errors.go; respond.go maps it to 400 registration_failed
  ```
  Routes added: `POST /api/webauthn/registrations`, `POST /api/webauthn/registrations/{id}`, `DELETE /api/webauthn/credentials/{id}` (query `subjectId`). `POST /api/challenges/{id}/verify` dispatches on which of `pin`/`credential` is present in the body; both or neither → 400 `invalid_json`.

- [x] **Step 1: Write failing tests with `github.com/descope/virtualwebauthn`**

```go
// setup: rp := virtualwebauthn.NewRelyingParty("Demo","app.example.com","https://app.example.com"); auth := NewAuthenticator(); cred := NewCredential(KeyTypeEC2); client with RPID app.example.com, Origins [https://app.example.com]
func TestWebAuthnRegisterThenVerify(t *testing.T) // BeginRegistration → options JSON → virtualwebauthn.ParseAttestationOptions → CreateAttestationResponse → FinishRegistration ok → Factors shows 1 credential; BeginChallenge(webauthn) → ParseAssertionOptions → CreateAssertionResponse → VerifyWebAuthn → amr ["webauthn"]; credential LastUsedAt set, SignCount > 0
func TestWebAuthnRejectsWrongOriginAndRPID(t *testing.T) // same flow but rp built with origin https://evil.example.com → FinishRegistration ErrRegistrationFailed; rp with id "evil.example.com" → ErrRegistrationFailed
func TestWebAuthnRejectsRegressedSignCount(t *testing.T) // after one good assertion, set cred.Counter back to 0 → VerifyWebAuthn → VerificationFailed
```

- [x] **Step 2: Run with endpoint** — Expected: FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [x] **Step 5: Commit** — `feat(webauthn): passkey registration and assertion through the client relay`

---

### Task 11: config, admin and jwks handlers, `cmd/auth-gate`, `make dev`

**Files:**
- Create: `internal/platform/config/config.go`, `config_test.go`, `internal/auth/handler/admin.go`, `internal/auth/handler/jwks.go`, `cmd/auth-gate/main.go` (replace placeholder)
- Modify: `Makefile` (`dev`), `internal/auth/service/service.go` (admin use cases)

**Interfaces:**
- Produces:
  ```go
  type Config struct{ Role, Stage, Table, Region, KMSKeyID string; KMSKeyIDs []string; AdminKeyParam, OriginVerifyParam, BaseURL, DynamoEndpoint string; LocalSigner bool; Port string }
  func Load(getenv func(string) string) (Config, error)
  // ROLE ∉ {verify,admin,jwks,dev} → error; verify: Table, BaseURL, OriginVerifyParam and (KMSKeyID or LocalSigner) required; admin: Table, AdminKeyParam, OriginVerifyParam required; jwks: KMSKeyIDs or LocalSigner required; dev: none of the SSM params
  func NewAdmin(svc *service.Service, adminKey string) http.Handler // RequireKey("x-internal-access-key", StaticKey(adminKey)); POST /api/clients, GET /api/clients/{id}, POST /api/clients/{id}/keys, DELETE /api/clients/{id}/subjects/{subjectId}
  func NewJWKS(signers ...token.Signer) http.Handler              // GET /.well-known/jwks.json, Cache-Control: public, max-age=300
  func (s *Service) CreateClient(ctx, in client.CreateInput) (client.Client, string, error)  // CreateInput.Validate error → 400 invalid_client
  func (s *Service) RotateClientKey(ctx, id string) (string, error)   // ErrNotFound → 404 client_not_found
  func (s *Service) PurgeSubject(ctx, clientID, subjectID string) error
  ```
  `main.go`: `config.Load(os.Getenv)`; build deps; `ROLE=dev` serves all three under `/verify`, `/admin`, `/jwks` prefixes on `:PORT` with `LocalSigner`, `ADMIN_KEY` from env and no origin check (dev only); otherwise wrap `verify`/`admin` in `RequireKey("x-origin-verify", StaticKey(originSecret))` and `lambda.Start(httpadapter.NewFunctionURL(h).ProxyWithContext)`. Secrets on AWS: `ssmx.GetSecureString` for `AdminKeyParam` and `OriginVerifyParam` at cold start; empty → `log.Fatal`. `service.Issuer = cfg.BaseURL`.
  `make dev`: `docker compose up -d && ROLE=dev TABLE_NAME=authgate-dev DYNAMO_ENDPOINT=http://localhost:8000 SIGNER=local ADMIN_KEY=dev-admin PORT=8080 go run ./cmd/auth-gate` (main calls `dynamo.EnsureTable` when `DynamoEndpoint != ""`).

- [x] **Step 1: Write failing test**

```go
func TestLoadRefusesIncompleteRoles(t *testing.T) // table: verify without KMS_KEY_ID and without SIGNER=local → error; verify without ORIGIN_VERIFY_PARAM or BASE_URL → error; admin without ADMIN_KEY_PARAM → error; jwks without KMS_KEY_IDS → error; ROLE=nope → error; ROLE=dev with nothing but TABLE_NAME → ok
```

- [x] **Step 2: Run** — FAIL. **Step 3: Implement config, admin, jwks, main, Makefile.** **Step 4: Run `make check`** — PASS.
- [x] **Step 5: Manual check: `make dev`, then**

```bash
curl -s -XPOST localhost:8080/admin/api/clients -H 'x-internal-access-key: dev-admin' -d '{"name":"demo","rpId":"localhost","origins":["http://localhost:3000"]}'
# → 201 {clientId, accessKey}; then with that key:
curl -s -XPOST localhost:8080/verify/api/factors/pin -H "x-internal-access-key: $KEY" -d '{"subjectId":"u1","pin":"493817"}'   # 201
curl -s localhost:8080/jwks/.well-known/jwks.json   # 200 one EC key
curl -s -i localhost:8080/verify/api/nope           # 401 {"error":"unauthorized"}
```

- [x] **Step 6: Commit** — `feat: role-selected binary, admin and jwks roles, local dev server`

---

### Task 12: SAM template (functions + CloudFront) and deploy scripts

**Files:**
- Create: `infra/template.yaml`, `scripts/setup-and-deploy/00-cert.sh`, `01-ssm.sh`, `02-build.sh`, `03-deploy.sh`, `99-purge.sh`

**Interfaces:**
- Produces: stack `auth-gate-<stage>` with:
  - `Table` (pk/sk, `ttl`, PITR, on-demand); `SigningKey` (`KeySpec ECC_NIST_P256`, `KeyUsage SIGN_VERIFY`) + alias `alias/auth-gate-<stage>`.
  - Three `AWS::Serverless::Function` (`provided.al2023`, arm64, 256 MB, 10 s, `FunctionUrlConfig AuthType NONE`) `Verify`, `Admin`, `Jwks` sharing one `CodeUri` (`02-build.sh` → `bin/bootstrap`), each with its own `Policies` exactly as spec §5 (plus `ssm:GetParameter` on its own parameters), env per Task 11; `BASE_URL` is `https://<DomainName>` when given, else the distribution domain (`!GetAtt Distribution.DomainName` — resolved through a `!If`).
  - `Distribution` (`AWS::CloudFront::Distribution`): three origins = the Function URL hosts (`!Select [2, !Split ["/", !GetAtt VerifyUrl.FunctionUrl]]`), each with `OriginCustomHeaders: x-origin-verify = {{resolve:ssm:/credo/auth-gate/<stage>/origin-verify}}`; behaviours per spec §5.1 with managed `CachingDisabled` + `AllViewerExceptHostHeader` for `/*` and `/api/clients*`, and a cache policy honouring origin headers for `/.well-known/*`; `ViewerProtocolPolicy redirect-to-https`; `HttpVersion http2and3`; `PriceClass_200`.
  - Parameters `Stage`, `DomainName` (default `""`), `CertificateArn` (default `""`), `HostedZoneId` (default `""`); conditions `HasDomain`, `HasZone`; `Aliases`/`ViewerCertificate` under `HasDomain`; `AWS::Route53::RecordSet` alias under `HasZone`.
  - Outputs: `BaseUrl` (one), and `RawVerifyUrl`/`RawAdminUrl`/`RawJwksUrl` for the boundary test and debugging.
- Scripts take `STAGE` (default `stg`) and `AWS_PROFILE` (default `credo-stg-ops`): `00-cert.sh DOMAIN` requests an ACM certificate in `us-east-1` with DNS validation and prints the CNAME to add and the ARN (optional step); `01-ssm.sh` writes random `admin-key` and `origin-verify` SecureStrings (refuses if they exist); `02-build.sh` cross-compiles `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`; `03-deploy.sh` runs `sam deploy` with the optional domain parameters from env and prints `BaseUrl`; `99-purge.sh` deletes the stack and both SSM parameters after a typed confirmation.

- [x] **Step 1: Write the template and scripts; shellcheck is in the gate.**
- [x] **Step 2: `sam validate --lint -t infra/template.yaml`** — Expected: valid. `pnpm run check` — green.
- [ ] **Step 3: HUMAN — optionally `00-cert.sh`, then `01-ssm.sh`, `02-build.sh`, `03-deploy.sh` to stg (CloudFront takes 5–10 min). Record `BaseUrl` and `RawVerifyUrl` for the Bruno `stg` environment (Task 13). Check: `curl -i $RawVerifyUrl/api/nope` → 401; `curl -i $BaseUrl/.well-known/jwks.json` → 200.**
- [x] **Step 4: Commit** — `feat(infra): sam template with three least-privilege functions behind one cloudfront door`

---

### Task 13: Bruno collection, `cmd/smoke`, `04-smoke.sh`

**Files:**
- Create: `bruno/opencollection.yml`, `bruno/environments/local.example.yml`, `bruno/01-the-boundary/`, `02-clients/`, `03-pin/`, `04-webauthn/`, `05-tokens/` (folder.yml + numbered requests), `cmd/smoke/main.go`, `scripts/setup-and-deploy/04-smoke.sh`

**Interfaces:**
- Collection `[External] Auth Gate (Lambda)`; variables `baseUrl`, `rawVerifyUrl`, `adminKey`, `clientKey`, `clientId`, `subjectId`, `challengeId`, `token`. Requests per spec §12; every request has `docs:` (why, **expected status**, prd note) and `tests` assertions; `02-clients/01-create` and `03-pin/02-challenge` capture vars in `after-response`. `01-the-boundary/` includes the `#`-in-id, the cross-client challenge (`CH` id from a second client created in the same folder) and the direct-Function-URL (`{{rawVerifyUrl}}` with `{{clientKey}}` → 401) requests. In `local.example.yml`, `baseUrl` is `http://localhost:8080` and the dev server's role prefixes are handled by a collection-level `pre-request` script that rewrites `/api/clients*` → `/admin/api/clients*`, `/.well-known/*` → `/jwks/...`, else `/verify/...` when `baseUrl` is localhost.
- `cmd/smoke`: flags `-base-url -admin-key`; creates a client with `rpId smoke.local`, origin `https://smoke.local`; runs the full virtualwebauthn register→challenge→verify loop; verifies the token with `token.VerifyWithJWKS` fetched from `<base-url>/.well-known/jwks.json` and checks `iss == base-url`; exits 0 only if claims match. Prints one line per step.
- `04-smoke.sh`: reads `BaseUrl` and `RawVerifyUrl` from the stack outputs and `admin-key` from SSM, runs `bru run bruno --env stg`, then `go run ./cmd/smoke …`; fails if `adminKey` equals `clientKey`.

- [ ] **Step 1: Write the collection; run it against `make dev` with `local.example.yml` copied to `local.yml`** — `npx -y @usebruno/cli run bruno --env local` — Expected: all boundary requests fail as asserted, all others pass. If the CLI cannot read the OpenCollection YAML, record the exact error in the request docs and keep the collection for the Bruno app; the smoke then relies on `cmd/smoke` and a note in `04-smoke.sh`.
- [ ] **Step 2: Write `cmd/smoke` and run it against `make dev`** — Expected: `ok token verified` as last line, exit 0.
- [ ] **Step 3: HUMAN — `04-smoke.sh` against stg.**
- [ ] **Step 4: Commit** — `feat(bruno): integration collection and webauthn smoke`

---

### Task 14: Memos, README, CONTRIBUTING

**Files:**
- Create: `docs/memo/three-roles-one-binary.md`, `pin-goes-through-a-challenge.md`, `kms-signs-not-ssm.md`, `challenge-is-client-scoped.md`, `webauthn-without-a-browser.md`, `no-health-endpoint.md`, `client-key-is-a-hash-lookup.md`, `one-door-cloudfront.md` (includes why not OAC)
- Modify: `README.md` (replace the round-1 intro and layout; add "one base URL" + optional custom domain, credential model, relay flow diagram from spec §6.4, "how a consumer verifies a token", signing-key rotation order, local dev, deploy), `CONTRIBUTING.md` (Go toolchain, `make check`, DynamoDB Local), `bruno/opencollection.yml` `docs:` (collection README per sibling order)

- [ ] **Step 1: Write each memo as: decision, context, alternatives rejected, consequences — ≤ 25 lines each.**
- [ ] **Step 2: Update README/CONTRIBUTING; `pnpm run check`** — green.
- [ ] **Step 3: Commit** — `docs: memos, readme and contributing for the auth-gate module`
- [ ] **Step 4: Write the handoff (`/handoff`), set the BOARD row to `review`.**
