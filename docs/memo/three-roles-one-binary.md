# Three roles, one binary

## Decision

One Go binary, deployed as three Lambda functions. `ROLE` (`verify`, `admin`, `jwks`) picks the mux at cold start, and each function has its own IAM role.

## Context

The code that signs tokens must not be the code that edits clients, and the only public route (JWKS) should live on a path that holds no secrets. One build and one zip stay the norm, as in the siblings. IAM beyond logs: `verify` gets DynamoDB item access and `kms:Sign`; `admin` gets DynamoDB and no KMS; `jwks` gets `kms:GetPublicKey` only and no DynamoDB.

## Alternatives rejected

- One function with every route: one IAM role holds `kms:Sign` and the admin key, so a bug in any route reaches both. The separation would be a code-review promise.
- Three separate modules: three builds and three copies of the shared platform code for no extra isolation, since IAM is already per function.

## Consequences

- "Who can sign" and "who can edit clients" are IAM statements in `infra/template.yaml`.
- A mux can only serve its own routes; a verify key on an admin path is a 404/401, never a code path that was forgotten.
- Cost: three functions to configure. `internal/platform/config` refuses to boot a role that lacks what it needs.
