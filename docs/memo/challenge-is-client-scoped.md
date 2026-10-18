# Challenges and registrations are client-scoped

## Decision

Pending registrations and challenges are keyed by client and id only: `R#<clientId>#<registrationId>` and `CH#<clientId>#<challengeId>`. The subject is an attribute, not part of the key.

## Context

The verify route receives only a challenge id. The server must find the item from the caller's key (which gives `clientId`) and that id. Meanwhile, the right to be forgotten needs to remove everything about one subject.

## Alternatives rejected

- Subject-scoped keys (`S#client#subject` with `CH#...` sort keys): the verify route would need the subject in the path, or a GSI to find the item.
- A global key by id alone: a client B key could probe or consume client A challenges. Putting `clientId` in the key makes cross-tenant access a miss, not a check.

## Consequences

- A challenge id from another client returns the same answer as an unknown one.
- The subject purge only has to `Query S#client#subject`: pending items live five minutes and expire through the `ttl` attribute.
- There is no GSI in v1.
- Cost: a subject cannot list its own pending challenges. Nothing needs that.
