# KMS signs the tokens, not a key in SSM

## Decision

Tokens are ES256 JWTs signed by an AWS KMS `ECC_NIST_P256` key (`kms:Sign`). The private key never leaves KMS. Public keys are served as JWKS from `kms:GetPublicKey`.

## Context

Consumers verify tokens offline against a JWKS, so auth-gate needs an asymmetric key pair. Something has to hold the private half.

## Alternatives rejected

- A private key in SSM SecureString: anything that can read the parameter can mint tokens, and it sits in Lambda memory. "Who can sign" becomes "who can read SSM".
- HS256 with a shared secret: every consumer could forge tokens, and there is no JWKS.
- A key in the function's environment or the zip: worse than SSM, and it ends up in logs and artifacts.

## Consequences

- "Who can sign" is one IAM line: `kms:Sign` on the `verify` role only.
- Cost is about $0.03 per 10k signatures and roughly 10 ms per successful verify.
- KMS returns DER; the signer converts it to the raw `r||s` JWS form.
- Rotation is a key-id list, not a secret swap: create a key, publish it in `KMS_KEY_IDS` on `jwks`, switch `KMS_KEY_ID` on `verify`, remove the old id after the longest token lifetime (15 minutes).
- Local dev and tests use an in-memory signer, enabled only by `SIGNER=local`. The `verify` role refuses to boot on AWS without `KMS_KEY_ID`.
