# FideX AS5 Quick Start Guide (5 Minutes)

> **Document Status: INFORMATIVE** — See `fidex-protocol-specification.md` for the normative specification.

## What is FideX?

FideX (AS5) is the modern replacement for AS2/AS4 B2B messaging. It uses **REST + JSON + JOSE** instead of S/MIME or SOAP. If you can call an API, you can use FideX.

## The 3 Core Concepts

```
1. SIGN → 2. ENCRYPT → 3. SEND → 4. GET RECEIPT
```

| Concept | What | How |
|---------|------|-----|
| **Envelope** | JSON wrapper with routing metadata + encrypted payload | `routing_header` (cleartext) + `encrypted_payload` (JWE) |
| **Crypto** | Sign-then-encrypt: `JWE(JWS(payload))` | RS256 signature + RSA-OAEP/A256GCM encryption |
| **Receipt** | Asynchronous J-MDN proving delivery | Signed JSON posted to sender's webhook |

## Step 1: Send a Message (2 min)

Your ERP calls your local FideX node:

```bash
curl -X POST http://localhost:8080/api/v1/transmit \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "destination_partner_id": "urn:gln:0614141000012",
    "document_type": "GS1_ORDER_JSON",
    "receipt_webhook": "https://your-erp.com/fidex/receipt",
    "payload": {
      "order_id": "PO-2026-001",
      "total_amount": 2499.00
    }
  }'
```

**Response (HTTP 202):**
```json
{
  "message_id": "fdx-a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6",
  "status": "QUEUED",
  "timestamp": "2026-02-20T18:30:00.000Z"
}
```

Your node automatically: signs → encrypts → transmits to partner.

## Step 2: Receive a Receipt (1 min)

Your partner's node processes the message and POSTs a J-MDN to your `receipt_webhook`:

```json
{
  "original_message_id": "fdx-a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6",
  "status": "DELIVERED",
  "receiver_id": "urn:gln:0614141000012",
  "hash_verification": "sha256:9f86d081884c7d659a...",
  "timestamp": "2026-02-20T18:30:02.000Z",
  "error_log": null,
  "signature": "eyJhbGciOiJSUzI1NiIs..."
}
```

The `signature` is a JWS signed by your partner — **legal proof of delivery**.

## Step 3: Register a Partner (2 min)

### Option A: Scan QR Code (Recommended)
1. Partner shares their discovery QR code
2. You scan it in the FideX dashboard
3. Automated: fetch config → fetch keys → sign registration → mutual trust established

### Option B: Manual URL
```bash
# 1. Fetch partner's config
curl https://fidex.partner.com/as5/config?token=xyz789

# 2. Register via your node
curl -X POST http://localhost:8080/api/v1/partners/register \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{"as5_config_url": "https://fidex.partner.com/as5/config?token=xyz789"}'
```

## Message Flow Diagram

```
YOUR ERP                YOUR NODE              PARTNER NODE           PARTNER ERP
   |                       |                       |                      |
   |-- POST /transmit ---->|                       |                      |
   |<-- 202 QUEUED --------|                       |                      |
   |                       |-- Sign (JWS) -------->|                      |
   |                       |-- Encrypt (JWE) ----->|                      |
   |                       |-- POST /receive ----->|                      |
   |                       |<-- 202 Accepted ------|                      |
   |                       |                       |-- Decrypt ---------->|
   |                       |                       |-- Verify Sig ------->|
   |                       |                       |-- Deliver to ERP --->|
   |                       |<-- POST J-MDN --------|                      |
   |<-- Webhook receipt ----|                       |                      |
```

## Key Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/transmit` | POST | Send a message (internal, from ERP) |
| `/api/v1/receive` | POST | Receive a message (public, from partners) |
| `/api/v1/receipt` | POST | Receive a J-MDN receipt (public) |
| `/.well-known/jwks.json` | GET | Public keys (public, no auth) |
| `/as5/config` | GET | Node discovery config (public) |
| `/api/v1/register` | POST | Partner registration (public) |
| `/health` | GET | Liveness check |

## Required Algorithms

| Purpose | Algorithm | Standard |
|---------|-----------|----------|
| Signing | RS256 | RFC 7518 |
| Key Encryption | RSA-OAEP | RFC 7518 |
| Content Encryption | A256GCM | RFC 7518 |
| Hashing | SHA-256 | FIPS 180-4 |
| Min Key Size | 2048-bit RSA | (4096 recommended) |

## What Makes FideX Different from AS2/AS4?

| | AS2 (2005) | AS4 (2013) | **FideX AS5 (2026)** |
|--|-----------|-----------|---------------------|
| Format | S/MIME | SOAP/XML | **REST/JSON** |
| Crypto | CMS | WS-Security | **JOSE (JWS+JWE)** |
| Keys | Manual email | Manual | **Auto-discovery** |
| Receipt | MDN (email-style) | ebMS Receipt | **J-MDN (JSON webhook)** |
| Learning curve | Weeks | Weeks | **Hours** |

## Next Steps

- 📖 Read the full spec: `fidex-protocol-specification.md`
- 🔒 Security guide: `fidex-security-guide.md`
- 💻 Implementation examples: `fidex-implementation-guide.md`
- 📋 OpenAPI contract: `openapi.yaml`
