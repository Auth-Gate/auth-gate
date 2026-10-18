# One door: CloudFront in front of three functions

## Decision

One CloudFront distribution is the only URL integrators see. It routes by path to the three Function URLs and adds a secret `x-origin-verify` header. `verify` and `admin` answer 401 when it is missing or wrong. A custom domain is optional.

## Context

A service meant for many projects cannot hand each of them three raw URLs. Function URLs are public, so the door must also prove a request came through CloudFront. Routes: `/.well-known/*` to jwks (cached per origin `Cache-Control`), `/api/clients*` to admin, everything else to verify (no caching).

## Alternatives rejected

- Three Function URLs: three base URLs and three hosts to allow-list.
- API Gateway: a fourth moving part and a per-request cost; Function URLs suffice.
- Lambda OAC with `AWS_IAM`: for POST and PUT the viewer must send `x-amz-content-sha256` with the body hash, so every integrator would hash request bodies.

## Consequences

- One base URL, one header. The token `iss` is `https://<that host>`.
- The secret lives in SSM as a plain String: CloudFront cannot resolve a SecureString, and the value is visible in the distribution config anyway. It is compared in constant time and read at cold start.
- Without a custom domain the first deploy takes two passes (see `03-deploy.sh`).
- CloudFront is the place to add WAF rate limiting later.
