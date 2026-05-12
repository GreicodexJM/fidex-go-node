# FideXNode Conformance Status

Tested by [FideX-conformance](https://github.com/GreicodexJM/fidex-conformance)
suite, Drummond-style certification model.

## Current verdict

| Profile    | Status         | Pass / Total |
| ---------- | -------------- | ------------ |
| `core`     | **CERTIFIED**  | 17 / 17      |
| `enhanced` | NOT CERTIFIED  | 20 / 21      |
| `edge`     | NOT CERTIFIED  | 24 / 25      |

`core` covers discovery, registration, outbound transmit, inbound receive.
This is the spec-required floor for B2B interop. `enhanced` adds receipts
(J-MDN) and error semantics; `edge` further adds active security probes.

Bucket 06 (errors) and 07 (security) are now fully green after FID-1 +
FID-2. The signed-J-MDN emission path (FID-3) reaches the peer
successfully and the peer's `ProcessReceipt` use case accepts the
receipt, but the conformance suite's bucket 05.03 assertion does not
recognise the peer's terminal outbound status (`ACKNOWLEDGED`) — see
remaining gap G1 below.

## Remaining gaps

### G1 · Bucket 05.03 — status-string match too narrow

The reference peer (`fidex-php`) transitions an outbound message
through `QUEUED → SENT → ACKNOWLEDGED` once a valid J-MDN arrives. The
conformance suite's bucket 05.03 only accepts `delivered|DELIVERED|
completed|COMPLETED`, so the test marks the round-trip as FAIL despite
the peer correctly reconciling the row.

This is a conformance-suite gap (the test should accept `ACKNOWLEDGED`
since that is the canonical PHP-peer terminal success state). The
J-MDN emission and reconciliation themselves are spec-compliant — the
NUT delivers a signed receipt within 2s of inbound processing and the
peer returns 200.

**Resolution path:** update `tests/05-receipts.sh` in the conformance
suite to add `acknowledged|ACKNOWLEDGED` to the success-status case
list. Out of scope for this repo; tracked separately.

### G2 · RSA-OAEP-256 algorithm advertisement (FID-4)

Optional spec §5.2 enhancement: advertise both RSA-OAEP and
RSA-OAEP-256 in the AS5 config, with the encryption path choosing
based on the peer's published capability. Not a conformance blocker.

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
