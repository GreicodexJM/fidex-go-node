# Partner Discovery and Registration

This package implements FideX partner discovery and registration as the specification describes it in §6.2–§6.4 (`fidex-protocol-specification.md`) and as `openapi.yaml` (`/api/v1/register`, `RegistrationRequest`, `RegistrationResponse`, `ErrorResponse`) defines it. A node can be the **initiator** (it registers with somebody else's node) or the **responder** (somebody registers with it).

## Model: registration by invitation

1. The **responder** mints a single-use token (`MintInvitation`): 256 bits from `crypto/rand`, unpadded base64url, open for 30 minutes (`InvitationTTL`). It shares its AS5 configuration URL with the token as a query parameter:
   `https://<public base URL>/.well-known/as5-configuration?token=<token>`
   The dashboard shows that URL as a QR (`GET /admin/dashboard/qr`, a new token per call; the URL is also in the `X-FideX-Discovery-URL` header) or returns it as JSON (`POST /admin/dashboard/invitations`).
2. The **initiator** receives the URL (QR, email, chat), reads the token from `?token=` (`TokenFromDiscoveryURL`) and echoes it as `security_token`. The initiator never mints a token.

The responder's AS5 configuration is public without `?token=`. With `?token=`, a token that is not open gets `401 INVALID_TOKEN`. Fetching the configuration does **not** spend the token; only a registration does.

## Handshake (spec §6.3)

```
┌─────────────┐                                    ┌─────────────┐
│   Node A    │                                    │   Node B    │
│  (Initiator)│                                    │ (Responder) │
└──────┬──────┘                                    └──────┬──────┘
       │ Phase 1                                          │
       │ GET /.well-known/as5-configuration?token=…       │
       ├─────────────────────────────────────────────────>│
       │ GET <endpoints.jwks>                             │
       ├─────────────────────────────────────────────────>│
       │                                                  │
       │ Phase 2                                          │
       │ POST <endpoints.register>                        │
       │ Content-Type: application/jose                   │
       │ JWS RS256 (kid = A's "sig" key) over             │
       │ {fidex_version, initiator_node_id,               │
       │  initiator_as5_config_url, security_token,       │
       │  timestamp}                                      │
       ├─────────────────────────────────────────────────>│
       │                                    Phase 3       │
       │        1. parse JWS, peek claims (unverified)    │
       │        2. timestamp within ±15 min               │
       │        3. SPEND the token                        │
       │        4. GET initiator_as5_config_url;          │
       │           node_id must = initiator_node_id       │
       │        5. GET A's JWKS; verify the JWS           │
       │        6. store A (409 if already a partner)     │
       │ 200 {status:"registered", responder_node_id,     │
       │      responder_as5_config_url, message,          │
       │      timestamp}                                  │
       │<─────────────────────────────────────────────────┤
       │ Phase 4: A stores B                              │
```

One handshake leaves both nodes knowing each other. A second, reverse handshake is not needed and would get `409 ALREADY_REGISTERED`.

### Why the responder's order matters

`initiator_as5_config_url` is text the initiator wrote. If the responder fetched it before checking the token, `/api/v1/register` would let anybody make the node issue HTTP requests (SSRF). So the token is checked and spent before any outbound request. The timestamp is checked before the token is spent, so a stale or replayed request does not burn an invitation. After the token is spent, every refusal leaves it spent: one token, one attempt.

## Responses

| Situation | HTTP | `error.code` |
|---|---|---|
| Registered | 200 | — (`RegistrationResponse`, `status: "registered"`) |
| Body not `application/jose`, not an RS256 JWS, missing/invalid field, unsupported `fidex_version`, non-RFC 3339 timestamp | 400 | `SIGNATURE_INVALID` (with `field` when it applies) |
| Timestamp outside ±15 min | 401 | `SIGNATURE_INVALID` (`field: "timestamp"`) |
| `security_token` missing, unknown, expired or already used | 401 | `INVALID_TOKEN` |
| Initiator AS5 configuration or JWKS cannot be fetched or is invalid | 400 | `CONFIG_UNREACHABLE` |
| AS5 configuration `node_id` ≠ `initiator_node_id` | 401 | `SIGNATURE_INVALID` |
| JWS header `kid` not an RS256 signing key in the initiator's JWKS | 401 | `UNKNOWN_KEY_ID` |
| Signature does not verify | 401 | `SIGNATURE_INVALID` |
| Initiator already a partner | 409 | `ALREADY_REGISTERED` |
| Storage failure | 500 | `INTERNAL_ERROR` |

Every error body is `ErrorResponse`: `{"error":{"code","message","timestamp","field"?}}`.

## Key selection

The signing `kid` is `crypto.SigningKeyID(nodeID)` (`<node_id>:enc-sig`), the kid of the `"use":"sig"` key this node already publishes in `/.well-known/jwks.json`. The responder accepts keys with `use` absent or `"sig"`, `alg` absent or `"RS256"`, and at least 2048 bits. When the JWS header has a `kid`, only that key qualifies. When it has none, every qualifying key is tried.

## Configuration

`FIDEX_ALLOW_PRIVATE_PEERS` (default `false`). Every request to a partner-chosen URL goes through `internal/netguard`: the AS5 configuration, the JWKS, `register`, and the queue worker's deliveries to `receive_message` / `receive_receipt`. netguard checks the resolved IP address at dial time and refuses loopback, private (RFC 1918 / ULA), link-local (including 169.254.169.254), CGNAT and reserved ranges. It never follows redirects, and with the guard on it ignores `HTTP(S)_PROXY`. Set the variable to `true` only on development or test networks where partners live on private addresses (docker bridge, localhost).

Endpoint URLs in an AS5 configuration must be absolute `http(s)` URLs without userinfo. They are NOT required to share the configuration's host: MFT publishes its per-tenant JWKS and its per-partner `receive_message` on different hosts, so that rule would break interop. The dial-time guard is what keeps them off internal addresses.

`FIDEX_PUBLIC_BASE_URL` is the URL partners use to reach the public API (for example `https://fidex.example.com`). It is used to build `initiator_as5_config_url` and the invitation URL. Default: `https://<FIDEX_PUBLIC_DOMAIN>:<FIDEX_PUBLIC_PORT>`.

## Files

- `as5_config.go` — the AS5 configuration document, `FetchAS5Config`, `FetchAndCachePartnerKeys`
- `tokens.go` — in-memory single-use token store (`GenerateToken`, `IsOpen`, `ValidateAndConsume`)
- `service.go` — protocol types, `MintInvitation`, the initiator (`InitiatePartnerHandshake`)
- `registration.go` — the responder (`HandleRegistration`), error codes
- `registration_test.go` — both halves: token from URL, JWS signing and verification, ±15 min window, token reuse, token checked before any fetch, node_id mismatch, unknown kid, tampering, 409, malformed bodies
- `../api/registration_e2e_test.go` — two nodes over the real routers

## Running the tests

```bash
go test ./internal/discovery/ ./internal/api/
```

## Access control

- `/admin/dashboard/*` (including `/qr` and `/invitations`, which mint tokens), `/admin/settings/*` and `/admin/auth/me` require a logged-in session (`RequireSessionAPI`); `/dashboard` redirects to `/login`. Without an `AuthService` these routers answer 503 instead of serving unauthenticated.
- The session cookie is `HttpOnly`, `SameSite=Lax`: a cross-site page cannot send it with a POST or an `<img>`/`fetch` subresource, which is the CSRF defence for the token-minting routes.
- The dashboard WebSocket accepts only same-origin upgrades.
- The internal router's IP allowlist judges the TCP peer address only; `X-Forwarded-For` / `X-Real-IP` are ignored (they let any client claim an allowlisted IP).
- The public router's access log prints `?token=REDACTED`; initiator errors and logs redact the token too.

## Known limits

- Tokens live in memory: a restart invalidates every open invitation.
- With `FIDEX_ALLOW_PRIVATE_PEERS=true` there is no SSRF guard beyond the token gate.
- No `INACTIVE` partner state yet (§6.5), so re-registration requires deleting the partner first.
- Behind a reverse proxy, the internal IP allowlist sees the proxy's address; put the allowlist on the proxy.
