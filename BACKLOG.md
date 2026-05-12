# FideXNode Backlog — Conformance Gaps

These items are blocking `enhanced` and `edge` profile certification
against the [FideX-conformance](https://github.com/GreicodexJM/fidex-conformance) (Jira project [FID](https://greicodex.atlassian.net/jira/software/c/projects/FID/boards/386))
suite. Each entry is structured for direct import into Jira (or any
issue tracker) once a `FIDEX` project key is provisioned.

## FID-1 · Inbound handler must declare rejection on bad envelope

**Type:** Bug
**Priority:** High
**Conformance bucket:** 06.02, 06.04
**Spec reference:** §4.4 (Quarantine semantics), §8 (Error codes)

**Summary**
Inbound `/api/v1/inbound` returns HTTP 202 with an empty body when given
an envelope whose `sender_id` is unknown or whose JWS cannot be verified.
The spec requires explicit rejection, either as 4xx with `error_code`, or
2xx with `status=REJECTED` and `error_code` in the response body.

**Acceptance criteria**
- [ ] Unknown `sender_id` → HTTP 401 with `{"error_code":"UNKNOWN_PARTNER"}`
      or HTTP 202 with `{"status":"REJECTED","error_code":"UNKNOWN_PARTNER"}`
- [ ] Tampered JWS → HTTP 401 with `error_code=SIGNATURE_INVALID`
      or 202 with explicit REJECTED status
- [ ] Decrypt failure → 4xx OR 202 with `status=QUARANTINED`
- [ ] Bucket 06.02 + 06.04 pass against the conformance suite

**Touch points**
- `internal/api/external_handlers.go::inboundHandler`
- `internal/crypto/as5_engine.go::Decrypt/Verify` — surface typed errors
- `internal/domain/messages.go` — `QUARANTINED` status exists, wire it through

---

## FID-2 · Duplicate `message_id` returns 500 instead of 409

**Type:** Bug
**Priority:** High
**Conformance bucket:** 06.03, 07.04
**Spec reference:** §9.3 (Replay protection)

**Summary**
Submitting the same envelope twice produces HTTP 202 on the first call
and HTTP 500 on replay. The 500 comes from a SQLite UNIQUE constraint
violation bubbling up unhandled in the persistence layer. Spec §9.3
requires deterministic 409 with `error_code=DUPLICATE_MESSAGE`.

**Acceptance criteria**
- [ ] Replay of identical envelope → HTTP 409 with `error_code=DUPLICATE_MESSAGE`
- [ ] No SQL error in logs on replay
- [ ] Replay of envelope whose first delivery failed → still rejected
      (not silently accepted)
- [ ] Bucket 06.03 + 07.04 pass

**Touch points**
- `internal/repository/sqlite/message_repo.go::Create` — detect and map
  `sqlite3.ErrConstraint` (errno 19, extended 2067) to a domain error
- `internal/api/external_handlers.go::inboundHandler` — branch on the
  domain error type, return 409 with structured body

---

## FID-3 · J-MDN receipt emission not implemented

**Type:** Story
**Priority:** Critical (blocks `enhanced` profile certification)
**Conformance bucket:** 05.01, 05.02, 05.03
**Spec reference:** §7 (Message disposition notifications)

**Summary**
After a successful inbound decrypt+verify, FideXNode does not generate
or deliver a signed J-MDN receipt back to the sender. The sender's
outbound row therefore stays in `SENT` indefinitely with no acknowledgment.

**Acceptance criteria**
- [ ] On successful `DELIVERED` of inbound, enqueue `send_jmdn` job with
      `original_message_id`, status `processed`, timestamp, and
      `hash_verification` over the original encrypted payload (spec §7.2)
- [ ] J-MDN is signed by the NUT's private signing key (JWS, alg=RS256)
- [ ] Worker POSTs J-MDN to the sender partner's `receive_receipt` URL
      from cached AS5 config
- [ ] Bucket 05.01 + 05.02 + 05.03 pass against peer
- [ ] The peer's outbound row flips from `SENT` to `DELIVERED` within 30s

**Touch points**
- `internal/queue/worker.go` — add `send_jmdn` handler
- `internal/crypto/as5_engine.go` — `BuildJMDN(originalMessage)` method
- `internal/domain/jmdn.go` (new) — disposition notification struct
- `internal/api/external_handlers.go::receiptHandler` — confirm it
  already reconciles outbound status on incoming receipts (needed for
  bucket 05 when FideXNode is the *sender*, not just receiver)

---

## FID-4 · Publish dual algorithm advertisement (nice-to-have)

**Type:** Improvement
**Priority:** Low
**Conformance bucket:** N/A (suite already accepts `RSA-OAEP`)
**Spec reference:** §5.2 (Algorithm registry)

**Summary**
AS5 config advertises `encryption_algorithm: "RSA-OAEP"`, which matches
spec §5 verbatim and was accepted by the conformance suite after a
whitelist patch. To ease future migration off SHA-1 OAEP, also support
and optionally advertise `RSA-OAEP-256` (SHA-256 variant, JWA RFC 7518
§4.3). Decide algo at session setup based on partner's published
capability.

**Acceptance criteria**
- [ ] `crypto.Encrypt()` accepts an `alg` parameter, dispatches to
      `RSA-OAEP` (SHA-1) or `RSA-OAEP-256` (SHA-256)
- [ ] AS5 config can advertise both as `supported_encryption_algorithms`
- [ ] Backwards compatible with existing partners
