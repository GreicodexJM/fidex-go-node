# FideXNode Conformance Status

Tested by [FideX-conformance](https://github.com/GreicodexJM/fidex-conformance)
suite, Drummond-style certification model.

## Current verdict

| Profile    | Status         | Pass / Total |
| ---------- | -------------- | ------------ |
| `core`     | **CERTIFIED**  | 17 / 17      |
| `enhanced` | NOT CERTIFIED  | 18 / 21      |
| `edge`     | NOT CERTIFIED  | 21 / 25      |

`core` covers discovery, registration, outbound transmit, inbound receive.
This is the spec-required floor for B2B interop. `enhanced` adds receipts
(J-MDN) and error semantics; `edge` further adds active security probes.

## Open gaps blocking `enhanced` / `edge`

### G1 · Inbound silently 202s on rejection

**Bucket 06.02 / 06.04** — when the inbound handler receives an envelope
with an unknown `sender_id`, an unverifiable JWS, or a malformed JWE, it
currently persists the row as `DELIVERED` (or no row) and returns HTTP 202
with an empty body. Spec §4.4 and §8 require either a 4xx with
`error_code=UNKNOWN_PARTNER`/`SIGNATURE_INVALID`, or a 2xx with explicit
`status=REJECTED` and `error_code` in the response body.

**Fix path:** `internal/api/external_handlers.go` `inboundHandler` —
branch on decrypt/verify outcome, persist with `QUARANTINED` status, return
either a `{status, error_code, message}` body or a proper 4xx.

### G2 · Duplicate `message_id` returns 500

**Bucket 06.03 / 07.04** — submitting the same envelope twice returns HTTP
202 first, then HTTP 500 on replay (UNIQUE constraint bubbles up
unhandled). Spec §9.3 requires deterministic 409 with
`error_code=DUPLICATE_MESSAGE`, or idempotent 2xx if the first delivery
succeeded.

**Fix path:** detect `sqlite3.ErrConstraint` (or repository-level
duplicate-key error) in the inbound persistence path; map to 409.

### G3 · J-MDN emission missing

**Bucket 05.02 / 05.03** — after a successful inbound decrypt + verify, no
signed J-MDN receipt is generated or delivered back to the sender. The
sender's outbound row stays in `SENT` indefinitely.

**Fix path:** `internal/queue/worker.go` — after successful inbound
delivery, enqueue a `send_jmdn` job. New worker handler generates the
signed disposition notification (spec §7) and POSTs it to the sender's
`receive_receipt` URL. The sender's `/api/v1/receipt` handler must accept
and reconcile against outbound message status.

### G4 · Algorithm name in AS5 config

(NB: not a blocker — `RSA-OAEP` matches spec §5 verbatim; the suite was
patched to accept this.) Consider also publishing the SHA-2 variant
`RSA-OAEP-256` once peers support it, to ease future migration off SHA-1.

## Running the suite locally

```bash
# 1. Boot FideXNode on test ports
FIDEX_NODE_ID=urn:custom:fidexnode-dev \
FIDEX_PUBLIC_PORT=18443 FIDEX_INTERNAL_PORT=18444 \
FIDEX_API_KEY=$(openssl rand -hex 16) \
FIDEX_DB_PATH=/tmp/fidexnode-dev.sqlite \
  ./fidex-node &

# 2. Clone the conformance suite next to this repo
git clone https://github.com/GreicodexJM/fidex-conformance.git ../FideX-conformance

# 3. Run
cd ../FideX-conformance
./runner.sh \
  --node-name "FideXNode (dev)" \
  --node-as5-url http://localhost:18443/.well-known/as5-configuration \
  --node-id urn:custom:fidexnode-dev \
  --node-transmit-url http://localhost:18444/api/v1/transmit \
  --node-transmit-api-key $FIDEX_API_KEY \
  --node-db-path /tmp/fidexnode-dev.sqlite \
  --profile core
```

CI runs the `core` profile as a required check on every PR
(`.github/workflows/conformance.yml`). The `edge` profile runs as an
advisory job in the same workflow.
