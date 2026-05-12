# ADR-0003: Dual encryption algorithm support (RSA-OAEP + RSA-OAEP-256)

- **Status:** Accepted
- **Date:** 2026-05-12
- **Accepted:** 2026-05-12
- **Deciders:** CTO, Lead Dev Orion (Cristiam)
- **Related ticket:** FID-4

## Context

FideX spec §5 mandates `RSA-OAEP` as the JWE key-encryption algorithm and §5.2 explicitly admits `RSA-OAEP-256` as a peer-of-equals. Until this commit FideXNode advertised and emitted only the former: AS5 published a single string `encryption_algorithm: "RSA-OAEP"` and the worker called `SignAndEncrypt` with a hard-coded `jose.RSA_OAEP` constant.

Two forces push us off this floor:

1. **NIST timetable.** SP 800-131A formally deprecates SHA-1 inside cryptographic primitives, and the JWA RSA-OAEP variant (RFC 7518 §4.2) builds on MGF1+SHA-1. Banks and government peers we're courting (SERDIMPRE on the SENIAT integration roadmap, several Cleo trading partners) will fail compliance audits if they keep accepting SHA-1-based key wrapping in 2026. `RSA-OAEP-256` (RFC 7518 §4.3, MGF1+SHA-256) is the standard remediation.
2. **Conformance suite alignment.** The fidex-protocol conformance bucket already accepts either algorithm. Peers that have moved to SHA-256-only will silently fail handshake with us today — not because the spec rejects us, but because we never tell them we can speak the newer variant.

We need to ship dual support without breaking the dozens of existing partner rows whose AS5 advertisements still publish only `encryption_algorithm: "RSA-OAEP"`. Wire compatibility with `fidex-php` (the reference peer) and with any partner that registered before this PR must be 100% — additive-only changes to the AS5 document, no schema migrations, no forced renegotiation.

## Decision

We adopt **dual algorithm advertisement + per-message capability negotiation**, layered on top of the existing AS5 contract:

- AS5 `Security` block keeps the legacy single field `encryption_algorithm: "RSA-OAEP"` unchanged (back-compat anchor) and gains an additive optional array `supported_encryption_algorithms: ["RSA-OAEP", "RSA-OAEP-256"]`.
- AS5 consumers read the array first; if absent, fall back to the single field. The resolver lives in `discovery.ResolveSupportedEncryptionAlgorithms`.
- The crypto engine gains `SignAndEncryptWithAlg(payload, key, alg string)` alongside the original `SignAndEncrypt`. The latter is preserved as a thin wrapper that pins `alg = "RSA-OAEP"` so every existing caller stays bit-for-bit identical.
- The decrypt path additively whitelists `RSA-OAEP-256` in the go-jose `ParseEncrypted` accept list. We accept anything a peer might legitimately send us; we are stricter on emit than on receive.
- The outbound queue worker calls `crypto.NegotiateEncryptionAlgorithm(ours, theirs)` before encryption and logs the negotiated alg per message. Selection rules, in order:
  1. Both peers advertise `RSA-OAEP-256` → use it (prefer the stronger algorithm).
  2. Both advertise `RSA-OAEP` → use it.
  3. Peer advertised nothing (legacy partner row with `nil` array) → fall back to `RSA-OAEP`.
  4. No overlap → return an error; the worker marks the row FAILED (validation, non-retryable).

The partner-side capability list is hydrated in-memory on `Partner.SupportedEncryptionAlgorithms` at handshake / webhook-registration time. It is **not** persisted in the `trading_partners` SQL table: the schema is owned by the FID-6 column-promotion work and is locked for this PR. Pre-FID-4 partner rows therefore present an empty list and route through fallback rule #3, which keeps them at parity with their original behaviour.

## Consequences

**Positive**

- Forward compatible with SHA-256-only peers without breaking SHA-1-only peers.
- No wire-format break: the AS5 array is optional and the single field never changed value.
- Deterministic negotiation: the same (ours, theirs) input always yields the same alg, which makes incident triage and reproducibility cheap.
- Decrypt path tolerates both algs so we never refuse a stronger inbound; eases interop with peers that switched first.
- The negotiation function and constants (`AlgRSAOAEP`, `AlgRSAOAEP256`) live in `internal/crypto` and are reusable by future paths (e.g. signed J-MDN if we ever generalize beyond RS256).

**Negative / costs**

- `Partner.SupportedEncryptionAlgorithms` is non-persisted today. A partner registered before a node restart that bypasses the discovery handshake loses the field until the next handshake, and silently downgrades to `RSA-OAEP`. Acceptable trade-off for now (the downgrade is to the spec-default algorithm) but tracked as a follow-up: persist the array on the trading_partners table once the FID-6 schema settles, probably as `supported_encryption_algorithms TEXT` (JSON-encoded slice) with a backfill that reads from any cached AS5 JSON. Suggested ticket: **FID-4b — persist partner encryption capability**.
- The decrypt accept-list change crosses into territory that FID-7 (interop / inbound error paths) also touches. The change is purely additive — appending one element to a slice literal — but if FID-7 lands first and rewrites the parse call site, this line will need to be re-merged. Coordinated with Agent A via the file-ownership ledger.
- One extra log line per outbound message (negotiated alg). Trivial cost; the operational signal is worth it.

## Alternatives considered

- **Option B: always-array, deprecate the single field.** Cleaner long term and aligns better with how the JWA registry talks about algorithm sets. Rejected because every existing partner row and the reference peer `fidex-php` both read `encryption_algorithm` directly; promoting the array to the source of truth would require coordinated migration on the peer side. Revisit when the protocol spec itself drops the single field.
- **Option C: hard-coded "always RSA-OAEP-256, take it or leave it".** Cleanest implementation, zero negotiation logic. Rejected because every partner who registered before this PR would immediately stop receiving messages — exactly the back-compat break we are trying to avoid.
- **Option D: per-partner override via config flag.** Push the choice down to operators; let them flip a partner from OAEP to OAEP-256 manually. Rejected as ops toil — the AS5 advertisement already carries this information; making operators replicate it in a config file is a regression. Negotiation is the right primitive.
- **Do nothing.** Rejected. The NIST timetable is non-negotiable; we want to be the peer that already supports the new alg when the conformance bar moves, not the one scrambling to ship a fix the week before.

## References

- FID-4 — Publish dual algorithm advertisement (RSA-OAEP-256 support)
- ADR-0001 — ADR template (numbering convention)
- ADR-0002 — Worker job_type column (preceding ADR; sets the precedent that this ADR follows for "Accepted on commit" status)
- RFC 7518 §4.2 — `RSA-OAEP` (MGF1+SHA-1)
- RFC 7518 §4.3 — `RSA-OAEP-256` (MGF1+SHA-256)
- NIST SP 800-131A Rev. 2 — SHA-1 deprecation timetable
- FideX Protocol Specification §5 / §5.2 — algorithm registry
- Code paths:
  - `internal/discovery/as5_config.go` — `AS5SecurityConfig.SupportedEncryptionAlgorithms`, `ResolveSupportedEncryptionAlgorithms`
  - `internal/crypto/as5_engine.go` — `SignAndEncryptWithAlg`, `NegotiateEncryptionAlgorithm`, `joseKeyAlgorithm`, `AlgRSAOAEP*` constants
  - `internal/queue/worker.go` — negotiation call site in `deliverBusinessDocument`
  - `internal/domain/repositories.go` — `Partner.SupportedEncryptionAlgorithms` (non-persisted)
  - Tests: `as5_engine_oaep256_test.go`, `as5_oaep256_test.go`, `worker_oaep256_test.go`

## Notes for next visitor

When the FID-6 schema dust settles, the right follow-up is to persist `SupportedEncryptionAlgorithms` on the `trading_partners` row. The minimal-impact migration is a single `ALTER TABLE trading_partners ADD COLUMN supported_encryption_algorithms TEXT` with a default of `'["RSA-OAEP"]'` and an opportunistic backfill from any cached AS5 config JSON we still have on disk. That removes the "downgrades to RSA-OAEP on restart" caveat in the Consequences section without touching the negotiator at all.

If we ever extend the protocol to add a third algorithm (RSA-OAEP-384, post-quantum hybrids, etc.), the `NegotiateEncryptionAlgorithm` selection-order block is the one place to update — keep the preference order explicit there rather than relying on slice ordering.
