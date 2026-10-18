# No health or version endpoint

## Decision

None of the three roles exposes `/health`, `/version` or anything like them. An unknown path is 404 (401 first if the key is missing).

## Context

A sibling service once shipped a health endpoint that printed its environment. A Lambda Function URL behind CloudFront has no load balancer to probe it, so nothing needs one.

## Alternatives rejected

- A bare `/health` that returns `ok`: it adds a route that needs no key and can drift into returning more. The first version is never the last.
- A keyed `/health`: it proves the key check works, which the Bruno boundary folder already proves.

## Consequences

- Nothing unauthenticated answers except the JWKS, which is public by design.
- Liveness is observed from the deployed path: `04-smoke.sh` runs the real flow.
- `01-the-boundary/` asserts that `/health` is a 404, so adding one fails the collection.
- Operators use CloudWatch metrics and logs instead of a probe.
