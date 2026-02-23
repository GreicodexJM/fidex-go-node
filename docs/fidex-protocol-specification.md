# FideX Protocol Specification (AS5)

**Version:** 1.0 (Draft)  
**Status:** Proposed Standard  
**Date:** February 23, 2026  
**Authors:** FideX Protocol Working Group

---

> **Document Status: NORMATIVE**
>
> This document is the **authoritative, normative specification** of the FideX Protocol (AS5).
> All conforming implementations MUST satisfy the requirements stated herein.
>
> **Related Documents:**
> - `fidex-annotated-specification.md` — INFORMATIVE companion with rationale, examples, and code samples
> - `fidex-security-guide.md` — INFORMATIVE operational security best practices
> - `fidex-implementation-guide.md` — INFORMATIVE multi-language implementation examples
> - `openapi.yaml` — NORMATIVE OpenAPI 3.0 machine-readable contract (MUST match this specification)
>
> In case of conflict between documents, THIS specification takes precedence.

> **Formal Designation: AS5 (Application Statement 5)**
>
> FideX adopts the "AS" naming lineage from established B2B interchange standards:
> - **AS2** (RFC 4130) — MIME/S/MIME over HTTP (2005)
> - **AS4** (OASIS ebMS 3.0) — SOAP/WS-Security (2013)
> - **AS5** (FideX) — REST/JOSE over HTTPS (2026)
>
> The designation signals evolutionary continuity while marking a generational leap
> to modern web-native architecture.

---

## Abstract

This document specifies the FideX Protocol (AS5), a modern application-layer protocol for secure Business-to-Business (B2B) message exchange. FideX provides cryptographic non-repudiation, data integrity, and confidentiality using JOSE (JSON Object Signing and Encryption) over HTTPS, replacing legacy AS2 and AS4 standards with a REST-oriented approach accessible to modern web developers.

## Table of Contents

1. [Introduction](#1-introduction)
2. [Transport Layer](#2-transport-layer)
3. [Message Structure](#3-message-structure)
4. [Cryptographic Requirements](#4-cryptographic-requirements)
5. [Key Distribution](#5-key-distribution)
6. [Partner Discovery](#6-partner-discovery)
7. [Message States and Receipts](#7-message-states-and-receipts)
8. [Error Handling](#8-error-handling)
9. [Security Considerations](#9-security-considerations)
10. [References](#10-references)

---

## 1. Introduction

### 1.1 Purpose

FideX (Fast Integration for Digital Enterprises eXchange) defines a secure, reliable message exchange protocol for B2B electronic data interchange. The protocol ensures:

- **Non-repudiation:** Cryptographic proof of message origin
- **Integrity:** Detection of message tampering
- **Confidentiality:** End-to-end encryption
- **Reliability:** Asynchronous acknowledgments with retry semantics

### 1.2 Scope

This specification defines:
- Message format and structure
- Cryptographic operations
- Partner discovery and registration
- State management and acknowledgments
- Error codes and handling

This specification does NOT define:
- Business document formats (payload-agnostic)
- Application-specific processing logic
- Implementation details or code examples
- Deployment or operational procedures

### 1.3 Terminology

**MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are interpreted as described in RFC 2119.

**Additional Terms:**
- **Node:** A FideX-compliant server capable of sending and receiving messages
- **Partner:** A trading partner with established trust relationship
- **Message:** A business document wrapped in FideX envelope
- **J-MDN:** JSON Message Disposition Notification (receipt)

---

## 2. Transport Layer

### 2.1 Required Transport

Nodes MUST support HTTP/1.1 over TLS 1.3 as defined in RFC 8446. TLS 1.2 (RFC 5246) MAY be supported with Perfect Forward Secrecy (ECDHE key exchange) as fallback.

**Requirements:**
- Port 443 (HTTPS)
- Valid certificate from trusted CA
- Server Name Indication (SNI)
- Full certificate chain validation

### 2.2 Optional Transports

Nodes MAY support:
- **HTTP/2 (RFC 7540):** For multiplexed connections
- **HTTP/3 (RFC 9114):** For connection resilience (QUIC)

### 2.3 Request Method

All FideX message transmissions use HTTP POST method to the receiving endpoint specified in partner configuration.

### 2.4 Content Type

Messages MUST use `Content-Type: application/json`.

---

## 3. Message Structure

### 3.1 Message Envelope

A FideX message consists of two parts:

```json
{
  "routing_header": { ... },
  "encrypted_payload": "eyJhbGc..."
}
```

### 3.2 Routing Header

The routing header is cleartext JSON containing message metadata:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fidex_version` | string | YES | Protocol version (semantic versioning "major.minor", e.g., "1.0") |
| `message_id` | string | YES | Globally unique identifier (UUID v4, "fdx-" prefix RECOMMENDED) |
| `sender_id` | string | YES | URN of sending organization |
| `receiver_id` | string | YES | URN of receiving organization |
| `document_type` | string | YES | Business document type identifier (uppercase alphanumeric with underscores) |
| `timestamp` | string | YES | ISO 8601 UTC timestamp. Format: `YYYY-MM-DDTHH:mm:ss.SSSZ` (millisecond precision, always UTC `Z`) |
| `receipt_webhook` | string | YES | HTTPS URL where J-MDN receipt MUST be delivered. HTTP (non-TLS) is NOT allowed. REQUIRED for non-repudiation chain integrity. |
| `payload_digest` | string | NO | SHA-256 digest of the encrypted_payload string. Format: `"sha256:{hex}"`. Enables routing-layer integrity checks WITHOUT decryption. |

**Identifier Format (sender_id / receiver_id):**
- `urn:gln:{gln}` - GS1 Global Location Number
- `urn:duns:{duns}` - D-U-N-S Number
- `urn:lei:{lei}` - Legal Entity Identifier
- `urn:tin:{tin}` - Tax Identification Number
- `urn:custom:{identifier}` - Custom scheme

**Extension Fields:**
Implementations MAY include additional fields prefixed with `x-`. Standard processors MUST ignore unknown extension fields.

### 3.3 Document Type Registry

The `document_type` field uses a two-tier naming system:

**Tier 1 — Standard Types (managed by FideX Working Group):**

| Type Identifier | Standard | Description |
|-----------------|----------|-------------|
| `GS1_ORDER_JSON` | GS1 | Purchase order (JSON binding) |
| `GS1_INVOICE_JSON` | GS1 | Commercial invoice (JSON binding) |
| `GS1_DESADV_JSON` | GS1 | Despatch advice (JSON binding) |
| `GS1_RECADV_JSON` | GS1 | Receiving advice (JSON binding) |
| `GS1_CATALOG_JSON` | GS1 | Product catalog (JSON binding) |
| `X12_850` | ANSI X12 | Purchase order |
| `X12_810` | ANSI X12 | Invoice |
| `X12_856` | ANSI X12 | Advance ship notice |
| `EDIFACT_ORDERS` | UN/EDIFACT | Purchase order message |
| `EDIFACT_INVOIC` | UN/EDIFACT | Invoice message |
| `EDIFACT_DESADV` | UN/EDIFACT | Despatch advice message |
| `UBL_ORDER_21` | OASIS UBL 2.1 | Order document |
| `UBL_INVOICE_21` | OASIS UBL 2.1 | Invoice document |

Standard types use uppercase alphanumeric characters and underscores. Pattern: `^[A-Z0-9_]+$`.

**Tier 2 — Custom Types (organization-defined):**

Custom types MUST use reverse domain notation to avoid collisions:

| Pattern | Example | Description |
|---------|---------|-------------|
| `{TLD}_{ORG}_{DOCTYPE}_{VERSION}` | `COM_ACME_WAREHOUSE_RECEIPT_V2` | Custom warehouse receipt |
| `{TLD}_{ORG}_{DOCTYPE}` | `ORG_MYCOMPANY_INTERNAL_MEMO` | Internal memo type |

Custom types MUST NOT start with a standard prefix (`GS1_`, `X12_`, `EDIFACT_`, `UBL_`).

**Receiver Behavior:**
- Receivers MUST accept messages with any syntactically valid `document_type`
- Receivers SHOULD return J-MDN with `UNKNOWN_DOCUMENT_TYPE` error for types they cannot process
- Receivers MUST NOT reject messages at the HTTP level solely because of unknown `document_type` (use J-MDN instead)

### 3.4 Encrypted Payload

The `encrypted_payload` field contains a JWE (JSON Web Encryption) token as defined in RFC 7516. The JWE encrypts a JWS (JSON Web Signature) token as defined in RFC 7515, creating a nested structure:

```
JWE( JWS( business_document ) )
```

This sign-then-encrypt pattern ensures both authenticity and confidentiality.

---

## 4. Cryptographic Requirements

### 4.1 Signature (JWS)

Messages MUST be signed using the sender's private key before encryption.

**Required Algorithm:**
- **RS256** (RSASSA-PKCS1-v1_5 with SHA-256) per RFC 7518

**Optional Algorithms:**
- RS384, RS512, PS256, PS384, PS512

**Minimum Key Size:** 2048 bits (4096 bits RECOMMENDED)

**JWS Header:**
```json
{
  "alg": "RS256",
  "kid": "{sender-key-id}"
}
```

### 4.2 Encryption (JWE)

Signed messages MUST be encrypted using the receiver's public key.

**Required Algorithms:**
- **Key Encryption:** RSA-OAEP (RFC 7518)
- **Content Encryption:** A256GCM (AES-256-GCM)

**JWE Header:**
```json
{
  "alg": "RSA-OAEP",
  "enc": "A256GCM",
  "kid": "{receiver-key-id}"
}
```

### 4.3 Prohibited Algorithms

Implementations MUST NOT use:
- `none` algorithm (no signature/encryption)
- Symmetric signature algorithms (HS256, HS384, HS512)
- RSA keys smaller than 2048 bits
- Deprecated algorithms (MD5, SHA1-based)

---

## 5. Key Distribution

### 5.1 JSON Web Key Set (JWKS)

Nodes MUST publish public keys via a JWKS endpoint at:

```
https://{public_domain}/.well-known/jwks.json
```

This endpoint:
- MUST be publicly accessible (no authentication)
- MUST return `Content-Type: application/json`
- SHOULD include `Cache-Control` headers (recommended: 1 hour)

### 5.2 JWKS Format

Per RFC 7517, keys must include:

```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "kid": "{unique-key-id}",
      "alg": "RS256",
      "n": "{base64url-modulus}",
      "e": "{base64url-exponent}"
    }
  ]
}
```

**Fields:**
- `kty`: Key type (MUST be "RSA")
- `use`: "sig" for signing, "enc" for encryption
- `kid`: Unique key identifier within JWKS
- `alg`: Algorithm for this key
- `n`: RSA modulus (base64url-encoded)
- `e`: RSA exponent (base64url-encoded, typically "AQAB")

### 5.3 Key Rotation

Implementations SHOULD rotate keys annually. During rotation:
1. Publish new key alongside old key
2. Maintain both keys for transition period (30-60 days RECOMMENDED)
3. Begin signing with new key after partners have cached it
4. Remove old key after transition period

---

## 6. Partner Discovery

### 6.1 Discovery Overview

Partner discovery enables automated onboarding without manual certificate exchange. The process consists of:
1. Configuration discovery
2. Key retrieval
3. Signed registration
4. Mutual confirmation

### 6.2 AS5 Configuration Endpoint

Nodes MUST expose an AS5 configuration document at an HTTPS URL. The URL:
- MAY be at any path (not required to be well-known)
- MAY include a single-use security token as query parameter
- SHOULD be shareable via QR code for ease of use

**Configuration Structure:**
```json
{
  "fidex_version": "1.0",
  "supported_versions": ["1.0"],
  "conformance_profile": "core",
  "node_id": "urn:gln:1234567890123",
  "organization_name": "Example Corp",
  "public_domain": "fidex.example.com",
  "supported_document_types": ["GS1_ORDER_JSON", "GS1_INVOICE_JSON"],
  "endpoints": {
    "receive_message": "https://fidex.example.com/api/v1/receive",
    "receive_receipt": "https://fidex.example.com/api/v1/receipt",
    "register": "https://fidex.example.com/api/v1/register",
    "jwks": "https://fidex.example.com/.well-known/jwks.json"
  },
  "security": {
    "signature_algorithm": "RS256",
    "encryption_algorithm": "RSA-OAEP",
    "content_encryption": "A256GCM",
    "minimum_key_size": 2048
  }
}
```

**AS5 Configuration Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `fidex_version` | YES | Current active protocol version |
| `supported_versions` | YES | Array of all protocol versions this node supports (e.g., `["1.0", "1.1"]`) |
| `conformance_profile` | NO | Conformance level: `"core"`, `"enhanced"`, or `"edge"` (see Appendix C) |
| `node_id` | YES | URN identifier for this node |
| `organization_name` | YES | Human-readable organization name |
| `public_domain` | YES | Public-facing domain name |
| `supported_document_types` | NO | Array of `document_type` identifiers this node can process (see Section 3.3) |
| `endpoints` | YES | Service endpoint URLs (see below) |
| `security` | YES | Cryptographic algorithm declarations |

### 6.2.1 Version Negotiation

When two nodes with different `supported_versions` attempt to communicate:

1. **Sender** checks receiver's `supported_versions` from AS5 config
2. **Sender** selects the highest version present in BOTH `supported_versions` arrays
3. **Sender** sets `fidex_version` in routing header to the negotiated version
4. **Receiver** MUST reject messages with `fidex_version` not in its `supported_versions`

**Example:** Sender supports `["1.0", "1.1"]`, receiver supports `["1.0"]` → negotiated version is `"1.0"`.

If no common version exists, the sender MUST NOT transmit and SHOULD report an error to the local ERP.

### 6.3 Discovery Handshake

**Phase 1: Initiator Discovers Responder**
1. Initiator obtains AS5 configuration URL (via QR code, email, etc.)
2. Initiator fetches responder's AS5 configuration
3. Initiator fetches responder's JWKS from well-known endpoint

**Phase 2: Initiator Registers**
1. Initiator builds registration payload:
   ```json
   {
     "fidex_version": "1.0",
     "initiator_node_id": "urn:gln:...",
     "initiator_as5_config_url": "https://...",
     "security_token": "...",
     "timestamp": "2026-02-20T19:00:00Z"
   }
   ```
2. Initiator signs payload with private key (JWS)
3. Initiator posts signed request to responder's register endpoint

**Phase 3: Responder Validates**
1. Responder validates security token (if provided)
2. Responder fetches initiator's AS5 configuration from URL in request
3. Responder fetches initiator's JWKS
4. Responder verifies JWS signature using initiator's public key
5. Responder stores initiator's details
6. Responder returns success confirmation

**Phase 4: Completion**
1. Initiator receives confirmation
2. Initiator stores responder's details
3. Both parties can exchange messages immediately

### 6.4 Registration Security

The registration request MUST:
- Be signed with initiator's private key (RS256)
- Include timestamp within ±15 minutes of current time
- Include security token if responder requires it

Responders MUST:
- Validate signature before trusting payload
- Reject expired timestamps
- Reject invalid or reused tokens

### 6.5 Partner De-Registration

Partners MAY terminate their trust relationship through a de-registration process.

**De-registration is a local operation.** There is no protocol-level de-registration handshake. Each party independently manages its own partner database.

**De-registration Process:**

1. **Initiating Party** sets partner status to `INACTIVE` in its local database
2. **Initiating Party** SHOULD notify the other party via out-of-band channel (email, phone, portal)
3. **Initiating Party** MUST continue to accept J-MDNs for messages already in flight
4. **Initiating Party** MUST reject new inbound messages from de-registered partner with HTTP 401
5. **Initiating Party** SHOULD retain partner records for audit purposes (7 years RECOMMENDED)

**Partner States:**

| State | Description |
|-------|-------------|
| `ACTIVE` | Normal operation — messages accepted |
| `SUSPENDED` | Temporarily paused — messages rejected with HTTP 503 |
| `INACTIVE` | De-registered — messages rejected with HTTP 401 |

**Re-Registration:** A de-registered partner MAY re-register using the standard discovery handshake (Section 6.3). The responder MAY require a new security token.

**Grace Period:** After de-registration, implementations SHOULD maintain a 24-hour grace period during which in-flight J-MDNs are still accepted.

---

## 7. Message States and Receipts

### 7.1 Message States

Messages transition through the following states:

| State | Description |
|-------|-------------|
| `QUEUED` | Created, awaiting transmission |
| `SENT` | Transmitted, awaiting receipt |
| `DELIVERED` | J-MDN received and verified |
| `FAILED` | Permanent failure or max retries exceeded |

### 7.2 Synchronous Response

Upon receiving a message, nodes MUST perform structural validation and return:
- **HTTP 202 Accepted:** Message structurally valid, queued for processing
- **HTTP 4xx/5xx:** Immediate rejection (see Error Handling)

HTTP 202 indicates structural acceptance only, NOT successful decryption or processing.

### 7.3 Asynchronous Receipt (J-MDN)

The J-MDN (JSON Message Disposition Notification) is the **most legally important artifact** in the FideX protocol. It provides cryptographic proof that a specific message was received, decrypted, and either accepted or rejected by the trading partner.

After processing a message (whether successfully or not), the receiver MUST send a J-MDN to the sender's `receipt_webhook`.

#### 7.3.1 J-MDN Payload Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `original_message_id` | string | YES | The `message_id` from the original FideX envelope's `routing_header`. |
| `status` | string | YES | `"DELIVERED"` or `"FAILED"`. |
| `receiver_id` | string | YES | URN of the receiver generating this J-MDN. MUST match `receiver_id` in original `routing_header`. |
| `hash_verification` | string | YES | SHA-256 hash of the raw business payload bytes BEFORE JWS signing. Format: `"sha256:{hex_encoded_hash}"`. |
| `timestamp` | string | YES | ISO 8601 UTC timestamp when J-MDN was created. Format: `YYYY-MM-DDTHH:mm:ss.SSSZ`. |
| `error_log` | object/null | YES | MUST be `null` when `status` is `"DELIVERED"`. MUST be an error object when `status` is `"FAILED"`. |
| `signature` | string | YES | JWS compact serialization of all other J-MDN fields (see 7.3.3). |

**Positive J-MDN Example (DELIVERED):**
```json
{
  "original_message_id": "fdx-a1b2c3d4-e5f6-g7h8",
  "status": "DELIVERED",
  "receiver_id": "urn:gln:9876543210987",
  "hash_verification": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
  "timestamp": "2026-02-20T18:30:02.000Z",
  "error_log": null,
  "signature": "eyJhbGciOiJSUzI1NiIsImtpZCI6InJlY2VpdmVyLXNpZ24tMjAyNi0wMi1wcmltYXJ5In0..."
}
```

**Negative J-MDN Example (FAILED):**
```json
{
  "original_message_id": "fdx-a1b2c3d4-e5f6-g7h8",
  "status": "FAILED",
  "receiver_id": "urn:gln:9876543210987",
  "hash_verification": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
  "timestamp": "2026-02-20T18:30:02.000Z",
  "error_log": {
    "error_code": "DECRYPTION_FAILED",
    "error_message": "Unable to decrypt payload with provided key",
    "details": "Key ID mismatch"
  },
  "signature": "eyJhbGciOiJSUzI1NiIsImtpZCI6InJlY2VpdmVyLXNpZ24tMjAyNi0wMi1wcmltYXJ5In0..."
}
```

#### 7.3.2 Hash Verification Definition

The `hash_verification` field provides proof that the receiver decrypted the exact payload the sender signed.

**Definition:** `hash_verification = "sha256:" + hex(SHA-256(raw_business_payload_bytes))`

Where `raw_business_payload_bytes` is the UTF-8 encoded byte representation of the business JSON payload **before** JWS signing (i.e., the original cleartext payload that was the input to the JWS sign operation).

When `status` is `"FAILED"` and the receiver could NOT decrypt the payload, the `hash_verification` field MUST be set to `"sha256:0000000000000000000000000000000000000000000000000000000000000000"` (64 zero hex digits).

#### 7.3.3 J-MDN Signature Requirements

The `signature` field MUST contain a JWS compact serialization that covers all other J-MDN fields.

**JWS Protected Header:**
```json
{
  "alg": "RS256",
  "kid": "{receiver-signing-key-id}"
}
```

**JWS Payload:** The canonical JSON serialization of the J-MDN fields (excluding the `signature` field itself). Fields MUST be serialized in the order defined in Section 7.3.1.

**Signing process:**
1. Construct a JSON object with all J-MDN fields EXCEPT `signature`
2. Serialize to canonical UTF-8 JSON (no extra whitespace, keys in schema order)
3. Sign using RS256 with receiver's private signing key
4. Set `signature` to the resulting JWS compact serialization

**Verification process (sender side):**
1. Extract `signature` from J-MDN
2. Parse JWS, extract `kid` from header
3. Lookup receiver's public key from their JWKS using `kid`
4. Verify JWS signature
5. Compare JWS payload with remaining J-MDN fields for consistency

#### 7.3.4 J-MDN Error Codes

When `status` is `"FAILED"`, the `error_log` object MUST contain:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `error_code` | string | YES | Machine-readable error code from the list below. |
| `error_message` | string | YES | Human-readable error description. |
| `details` | string | NO | Additional diagnostic information. MUST NOT contain sensitive data. |

**Standard J-MDN Error Codes:**

| Code | Description |
|------|-------------|
| `DECRYPTION_FAILED` | Cannot decrypt JWE (wrong key, corrupted ciphertext) |
| `SIGNATURE_INVALID` | JWS signature verification failed (tampered or wrong key) |
| `UNKNOWN_DOCUMENT_TYPE` | The `document_type` is not supported by receiver |
| `PAYLOAD_TOO_LARGE` | Message exceeds receiver's processing limits |
| `INTERNAL_ERROR` | Receiver encountered an internal processing error |

#### 7.3.5 J-MDN Delivery Protocol

**HTTP Request:**
```http
POST {receipt_webhook} HTTP/1.1
Host: {sender_host}
Content-Type: application/json
X-FideX-Original-Message-ID: {original_message_id}

{J-MDN JSON body}
```

**Expected Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "receipt_acknowledged": true
}
```

**Timing Requirements:**
- Receiver SHOULD send J-MDN within 5 minutes of receiving the original message
- If the receiver cannot process within 5 minutes, it SHOULD still send the J-MDN when processing completes
- There is no strict upper time limit (processing is asynchronous by design)

#### 7.3.6 J-MDN Delivery Retry

If the sender's `receipt_webhook` is unreachable, the receiver SHOULD retry J-MDN delivery:

- Attempt 1: Immediate
- Attempt 2: +1 minute
- Attempt 3: +5 minutes
- Attempt 4: +15 minutes
- Attempt 5: +1 hour

After 5 attempts, the receiver SHOULD log the failure and store the J-MDN for manual retrieval. The receiver MUST NOT discard an undelivered J-MDN.

### 7.4 Retry Semantics

Senders SHOULD retry failed transmissions with exponential backoff:
- Attempt 1: Immediate
- Attempt 2: +1 minute
- Attempt 3: +5 minutes
- Attempt 4: +15 minutes
- Attempt 5: +30 minutes
- Attempt 6: +1 hour

After 5-6 attempts, message SHOULD transition to FAILED state requiring manual intervention.

---

## 8. Error Handling

### 8.1 HTTP Status Codes

| Code | Meaning | Sender Action |
|------|---------|---------------|
| 202 | Accepted for processing | Wait for J-MDN |
| 400 | Invalid message structure | Do not retry (permanent) |
| 401 | Authentication failed | Do not retry (permanent) |
| 413 | Payload too large | Do not retry (permanent) |
| 429 | Rate limit exceeded | Retry with backoff |
| 500 | Server error | Retry with backoff |
| 503 | Service unavailable | Retry with backoff |

### 8.2 Error Response Format

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable description",
    "timestamp": "2026-02-20T18:00:00Z"
  }
}
```

### 8.3 Standard Error Codes

**Message Transmission:**
- `INVALID_ROUTING_HEADER`: Missing or malformed routing header
- `UNKNOWN_RECEIVER`: receiver_id not recognized
- `UNKNOWN_DOCUMENT_TYPE`: document_type not supported
- `PAYLOAD_TOO_LARGE`: Exceeds size limit

**Cryptographic:**
- `DECRYPTION_FAILED`: Cannot decrypt JWE
- `SIGNATURE_INVALID`: JWS signature verification failed
- `UNKNOWN_KEY_ID`: Key ID not found in JWKS

**Discovery:**
- `INVALID_TOKEN`: Security token invalid or expired
- `DUPLICATE_REGISTRATION`: Partner already registered
- `CONFIG_UNREACHABLE`: Cannot fetch AS5 configuration

---

## 9. Security Considerations

### 9.1 Threat Model

FideX addresses:
- **Man-in-the-Middle:** TLS 1.3 transport + JWE encryption
- **Message Tampering:** JWS signatures with hash verification
- **Replay Attacks:** Unique message IDs + timestamp validation
- **Repudiation:** Cryptographic signatures + signed receipts
- **Key Compromise:** Key rotation + JWKS distribution

### 9.2 Implementation Requirements

Implementations MUST:
- Validate TLS certificates against trusted CA store
- Use constant-time comparison for signature verification
- Maintain cache of recent message IDs to detect replays
- Reject messages with timestamps outside acceptable window (±15 minutes RECOMMENDED)
- Generate cryptographically secure random message IDs
- Never expose private keys in logs or error messages

Implementations SHOULD:
- Implement rate limiting on all endpoints
- Use separate keys for signing and encryption
- Rotate keys annually
- Monitor for unusual patterns (authentication failures, invalid signatures)

### 9.3 Key Management

Private keys:
- MUST be generated with cryptographically secure random number generator
- MUST be stored encrypted at rest
- MUST NOT be transmitted over network
- SHOULD be stored in hardware security module (HSM) for high-security deployments

Public keys:
- MUST be distributed via JWKS only
- SHOULD include `kid` that identifies purpose and date
- MAY be cached for up to 24 hours

### 9.4 Compliance

FideX is designed to support:
- Non-repudiation requirements (legally binding transactions)
- FDA 21 CFR Part 11 (electronic signatures)
- GDPR data processing agreements
- Industry EDI standards (GS1, UN/CEFACT)

Audit trail requirements:
- Message metadata: 7 years (RECOMMENDED)
- Cryptographic signatures: 7 years (RECOMMENDED)
- J-MDN receipts: 7 years (RECOMMENDED)

---

## 10. References

### 10.1 Normative References

**[RFC2119]** Bradner, S., "Key words for use in RFCs to Indicate Requirement Levels", BCP 14, RFC 2119, March 1997.

**[RFC5246]** Dierks, T. and E. Rescorla, "The Transport Layer Security (TLS) Protocol Version 1.2", RFC 5246, August 2008.

**[RFC7515]** Jones, M., Bradley, J., and N. Sakimura, "JSON Web Signature (JWS)", RFC 7515, May 2015.

**[RFC7516]** Jones, M. and J. Hildebrand, "JSON Web Encryption (JWE)", RFC 7516, May 2015.

**[RFC7517]** Jones, M., "JSON Web Key (JWK)", RFC 7517, May 2015.

**[RFC7518]** Jones, M., "JSON Web Algorithms (JWA)", RFC 7518, May 2015.

**[RFC8446]** Rescorla, E., "The Transport Layer Security (TLS) Protocol Version 1.3", RFC 8446, August 2018.

### 10.2 Informative References

**[RFC4130]** Moberg, D. and R. Drummond, "MIME-Based Secure Peer-to-Peer Business Data Interchange Using HTTP, Applicability Statement 2 (AS2)", RFC 4130, July 2005.

**[RFC7540]** Belshe, M., Peon, R., and M. Thomson, Ed., "Hypertext Transfer Protocol Version 2 (HTTP/2)", RFC 7540, May 2015.

**[RFC9114]** Bishop, M., Ed., "HTTP/3", RFC 9114, June 2022.

**[GS1]** GS1, "GS1 Web Vocabulary", https://www.gs1.org/voc/

**[OASIS-ebMS]** OASIS, "ebXML Messaging Services Version 3.0", October 2007.

---

## Appendix A: Complete Message Example

**HTTP Request:**
```http
POST /api/v1/receive HTTP/1.1
Host: partner.example.com
Content-Type: application/json

{
  "routing_header": {
    "fidex_version": "1.0",
    "message_id": "fdx-a1b2c3d4-e5f6-g7h8",
    "sender_id": "urn:gln:1234567890123",
    "receiver_id": "urn:gln:9876543210987",
    "document_type": "GS1_ORDER_JSON",
    "timestamp": "2026-02-20T18:30:00Z",
    "receipt_webhook": "https://sender.example.com/receipt"
  },
  "encrypted_payload": "eyJhbGciOiJSU0EtT0FFUCIsImVuYyI6..."
}
```

**HTTP Response:**
```http
HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "status": "accepted",
  "message_id": "fdx-a1b2c3d4-e5f6-g7h8",
  "timestamp": "2026-02-20T18:30:00Z"
}
```

**J-MDN Receipt:**
```http
POST /receipt HTTP/1.1
Host: sender.example.com
Content-Type: application/json

{
  "original_message_id": "fdx-a1b2c3d4-e5f6-g7h8",
  "status": "DELIVERED",
  "hash_verification": "sha256-9f86d081...",
  "timestamp": "2026-02-20T18:30:02Z",
  "error_log": null,
  "signature": "eyJhbGciOiJSUzI1NiIsImtpZCI6..."
}
```

---

## Appendix B: Glossary

**AS5** - Application Statement 5, the FideX protocol designation

**B2B** - Business-to-Business electronic commerce

**EDI** - Electronic Data Interchange

**GLN** - Global Location Number (GS1 standard identifier)

**J-MDN** - JSON Message Disposition Notification

**JOSE** - JSON Object Signing and Encryption

**JWE** - JSON Web Encryption (RFC 7516)

**JWK** - JSON Web Key (RFC 7517)

**JWKS** - JSON Web Key Set

**JWS** - JSON Web Signature (RFC 7515)

**Node** - A FideX-compliant server

**URN** - Uniform Resource Name

---

## Appendix C: Conformance Profiles

FideX defines three conformance profiles to enable progressive adoption. Implementations MUST declare which profile(s) they conform to.

### C.1 FideX Core (REQUIRED for all conforming implementations)

An implementation claiming **FideX Core** conformance MUST support:

| Requirement | Specification Reference |
|-------------|------------------------|
| HTTP/1.1 over TLS 1.3 | Section 2.1 |
| `Content-Type: application/json` | Section 2.4 |
| Routing header with ALL required fields (including `receipt_webhook`) | Section 3.2 |
| Sign-then-encrypt: `JWE(JWS(payload))` | Section 3.3 |
| RS256 signature algorithm | Section 4.1 |
| RSA-OAEP key encryption + A256GCM content encryption | Section 4.2 |
| RSA key size ≥ 2048 bits | Section 4.1 |
| JWKS endpoint at `/.well-known/jwks.json` | Section 5.1 |
| AS5 configuration endpoint | Section 6.2 |
| 4-phase discovery handshake | Section 6.3 |
| Message state machine (QUEUED → SENT → DELIVERED/FAILED) | Section 7.1 |
| J-MDN generation and delivery (positive and negative) | Section 7.3 |
| J-MDN signature (JWS with RS256) | Section 7.3.3 |
| Hash verification in J-MDN (`sha256:{hex}`) | Section 7.3.2 |
| Standard error codes and HTTP status codes | Section 8 |
| Replay detection via message_id cache | Section 9.2 |
| Timestamp validation (±15 minutes) | Section 9.2 |

### C.2 FideX Enhanced (RECOMMENDED for production deployments)

An implementation claiming **FideX Enhanced** conformance MUST satisfy FideX Core AND:

| Requirement | Description |
|-------------|-------------|
| HTTP/2 support | Multiplexed connections for high-throughput partners |
| Separate signing and encryption keys | Different `kid` for `use: "sig"` and `use: "enc"` |
| RSA key size ≥ 4096 bits | Stronger cryptographic keys |
| Key rotation support | Publish overlapping keys during rotation period |
| J-MDN delivery retry | Retry undelivered J-MDNs per Section 7.3.6 |
| Rate limiting | Per-partner rate limiting on all endpoints |
| Structured JSON logging | Security event logging with correlation IDs |
| Health endpoints | `/health` and `/ready` endpoints |

### C.3 FideX Edge (OPTIONAL for specialized deployments)

An implementation claiming **FideX Edge** conformance MUST satisfy FideX Enhanced AND:

| Requirement | Description |
|-------------|-------------|
| HTTP/3 over QUIC | Connection resilience for mobile/edge nodes |
| Mutual TLS (mTLS) | Client certificate authentication |
| HSM key storage | Private keys stored in Hardware Security Module |
| Batch receipt support | Single J-MDN acknowledging multiple messages (future v1.1) |

### C.4 Conformance Declaration

Implementations SHOULD declare conformance in their AS5 configuration:

```json
{
  "fidex_version": "1.0",
  "conformance_profile": "enhanced",
  "...": "..."
}
```

Valid values: `"core"`, `"enhanced"`, `"edge"`

---

## Appendix D: Interoperability Test Vectors

This appendix provides known-answer test vectors that allow implementers to verify their JOSE cryptographic operations produce correct output. These keys are for TESTING ONLY and MUST NOT be used in production.

### D.1 Test RSA Key Pair (2048-bit, Testing Only)

**⚠️ WARNING: This key pair is PUBLIC and MUST NOT be used for production traffic.**

**Test Private Key (PEM):**
```
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA2a2rwplBQLHgcAD3AFHS1FLo1KRJcE3sSSSqk+oeaBAXnCk7
pQUlFFjnMIYrijWO2UhqxJ6dVB1YzZ2MBaPC/k1XjDw8CbU79gg3K9gVq0GUS4Ar
QFPnHSQPIBPOpa8NJM0YXf1FBfHKcHa7kW0jxbGSXjCx0J3AmWP1V0QHqbVW8c/Z
gxjTFZHQkdFKLRsweQzS0JxLpLQGnRKFi5pr5V0PC8BDONH4MjK5pT1UEDf5NJRU
TqBuqBv7JjfBCm3Aq0rz2BisJQr3yyIPJz1KjhvtG5R8LUCNqW5cfAqpDkE3Fjns
Y6ZJCWPe7+bOz5W/qK+3h7Gfjcp96yzC1+HewIDAQABAoIBAF0k1r0UVqVWZK/x
g0JYwB0eFaRxqjLLcsZJNL6eHdBDVcf7gVdVH/CWKHl3mMnVMLYrgrjGv0v3Xk1p
jI2e/1dM6+xRnPOD9f0HH8Z5Z1A5eHbDqS7IQJKPB8IEVAEv+q8I0U+IJ7xORVj
y8r+o9nJaFvmM1a4YelFQlpIjErFVTFXfB1R13FwFN4fR24PsF27YBGMqB85HJNx
d8K8x3S4CTQPzl0MUj3z6LYNzKhJLqSq8MN5dA3fcqSZ8uft+LJDB4h1J0QK6Hrp
EzYRH6SiAF0qVjOOz1QIOhmFbkFbvccbxNj0N4c0y7G/FDOsjE7C1e7VQm9fcFre
JEd8a0ECgYEA74MZxbS5JBbGO/1HVHBj+hFkNjThMaXrjzr5MF0Uy3J5tB+VgVrN
XxB2BjvYT1GU5LRrq5A7I8kG3H+dKe+R10MDZM28KFbOYR4a1y5qlXe7Lsj7J28V
dZH+xyLqZnJNLiK3aY3DVfi3pd8z8lO+FT7Mmk1bWKy3IkzmPl+V0ECgYEA6Glqm
a6dFfZxL7Jr6pGZ7kYv3x3I0qNkELBT0j3g6SQdLqr0RXlxfBFdGmajb9dlh0C7x
H9V9M22KFpna4/OIhPD9DSj5pmf2W+oMQIfWq5KFXlq1J3MhcIqDE3cVMGJL/J2w
K5f6MXY2SHbO6EH6XzPl0g8Wa2b5e5V3d3c1YbsCgYBF1c6LMVBRcWv3j+n7nxBa
VKR7VINcL1LFBI1H5Vbha/kFdcwR8coxSDV1xJsyi+VE9d1DXHUC3R1jrSPHb5Fe
GPbwN3p5sXB5pBLUNHr3rZdyGdjU8fJ8jHdHJ8V4NpFV3CDp9F7k3Cob3LXhI3rl
NqK5mH7HPm0InMf3BnbPAQKBgD3y2asv+ORA4p4l6ESfNJB7BjfjFBIPJCn0X4oB
lYGjXAPWeN+BDPUJ5jKVA5bg0Bh+hUmL3XDBf7mS7VfaJkw6wlJQpO5MJNq+7hjh
Q7JARsDNkh+Pk6IFxPe/QiUHCOiLxlp2eRUqBWED7E7GQLHJ3VLEheSV+Lq2+YC0
sJkxAoGBALXfJKLZlNwvvBMDnmjlCsI5GCoFCiNddrYrO/GIXdV8/BcOBnL9K+TU
KVmJRJfZhSN0AfdQrfABNV0yNBBaeXVW2kXR28lMt15RpVqIK0PboH+hXfCxPqnV
K6LYwxBTcX/iJLPSiJy7Cmi7NhfQhGBFq2bVeJFnF0H8r9g3vb5v
-----END RSA PRIVATE KEY-----
```

**Test Public Key (PEM):**
```
-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2a2rwplBQLHgcAD3AFHS
1FLo1KRJcE3sSSSqk+oeaBAXnCk7pQUlFFjnMIYrijWO2UhqxJ6dVB1YzZ2MBaPC
/k1XjDw8CbU79gg3K9gVq0GUS4ArQFPnHSQPIBPOpa8NJM0YXf1FBfHKcHa7kW0j
xbGSXjCx0J3AmWP1V0QHqbVW8c/ZgxjTFZHQkdFKLRsweQzS0JxLpLQGnRKFi5pr
5V0PC8BDONH4MjK5pT1UEDf5NJRUTqBuqBv7JjfBCm3Aq0rz2BisJQr3yyIPJz1K
jhvtG5R8LUCNqW5cfAqpDkE3FjnsY6ZJCWPe7+bOz5W/qK+3h7Gfjcp96yzC1+He
wIDAQAB
-----END PUBLIC KEY-----
```

### D.2 Test Payload

**Raw Business Payload (UTF-8 bytes):**
```json
{"order_id":"PO-TEST-001","amount":100.00,"currency":"USD"}
```

**SHA-256 Hash of Payload:**
```
sha256:bf21a9e8fbc5a3846fb05b4fa0859e0917b2202f9a69e4c98b7b0f09cb281e71
```

### D.3 Expected Test Outputs

Given the test key pair and payload above:

**JWS Header:**
```json
{"alg":"RS256","kid":"test-sign-2026-01"}
```

**JWE Header:**
```json
{"alg":"RSA-OAEP","enc":"A256GCM","kid":"test-enc-2026-01"}
```

**Routing Header:**
```json
{
  "fidex_version": "1.0",
  "message_id": "fdx-00000000-0000-0000-0000-000000000001",
  "sender_id": "urn:gln:0000000000001",
  "receiver_id": "urn:gln:0000000000002",
  "document_type": "GS1_ORDER_JSON",
  "timestamp": "2026-01-01T00:00:00.000Z",
  "receipt_webhook": "https://test.sender.example.com/receipt"
}
```

**Expected J-MDN (on success):**
```json
{
  "original_message_id": "fdx-00000000-0000-0000-0000-000000000001",
  "status": "DELIVERED",
  "receiver_id": "urn:gln:0000000000002",
  "hash_verification": "sha256:bf21a9e8fbc5a3846fb05b4fa0859e0917b2202f9a69e4c98b7b0f09cb281e71",
  "timestamp": "2026-01-01T00:00:01.000Z",
  "error_log": null,
  "signature": "<JWS signed by receiver's private key>"
}
```

### D.4 Verification Procedure

To verify a FideX implementation:

1. **Sign Test:** Sign the test payload using RS256 with the test private key. Verify the resulting JWS using the test public key. The verified payload MUST match the original.

2. **Encrypt Test:** Encrypt the JWS from step 1 using RSA-OAEP/A256GCM with the test public key. Decrypt using the test private key. The decrypted content MUST match the JWS from step 1.

3. **Hash Test:** Compute `SHA-256` of the raw payload bytes. The result MUST equal `bf21a9e8fbc5a3846fb05b4fa0859e0917b2202f9a69e4c98b7b0f09cb281e71`.

4. **Round-Trip Test:** Construct a complete FideX envelope using the test routing header and encrypted payload. Parse the envelope, decrypt, verify signature, and extract payload. The result MUST match the original test payload.

5. **J-MDN Test:** Construct a J-MDN for the test message. Sign with the test private key. Verify the J-MDN signature using the test public key.

---

## Appendix E: JSON Schema Definitions

This appendix provides JSON Schema (draft-07) definitions for machine-validation of FideX structures.

### E.1 Routing Header Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://fidex-protocol.org/schemas/v1/routing-header.json",
  "title": "FideX Routing Header",
  "type": "object",
  "required": ["fidex_version", "message_id", "sender_id", "receiver_id", "document_type", "timestamp", "receipt_webhook"],
  "additionalProperties": true,
  "properties": {
    "fidex_version": {
      "type": "string",
      "pattern": "^\\d+\\.\\d+$",
      "description": "Protocol version (major.minor)"
    },
    "message_id": {
      "type": "string",
      "minLength": 1,
      "maxLength": 256,
      "description": "Globally unique message identifier"
    },
    "sender_id": {
      "type": "string",
      "pattern": "^urn:(gln|duns|lei|tin|custom):.+$",
      "description": "URN of sending organization"
    },
    "receiver_id": {
      "type": "string",
      "pattern": "^urn:(gln|duns|lei|tin|custom):.+$",
      "description": "URN of receiving organization"
    },
    "document_type": {
      "type": "string",
      "pattern": "^[A-Z0-9_]+$",
      "minLength": 1,
      "maxLength": 128,
      "description": "Business document type identifier"
    },
    "timestamp": {
      "type": "string",
      "format": "date-time",
      "description": "ISO 8601 UTC timestamp (YYYY-MM-DDTHH:mm:ss.SSSZ)"
    },
    "receipt_webhook": {
      "type": "string",
      "format": "uri",
      "pattern": "^https://",
      "description": "HTTPS URL for J-MDN delivery"
    },
    "payload_digest": {
      "type": "string",
      "pattern": "^sha256:[a-f0-9]{64}$",
      "description": "SHA-256 digest of encrypted_payload"
    }
  },
  "patternProperties": {
    "^x-": { "description": "Extension fields" }
  }
}
```

### E.2 FideX Envelope Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://fidex-protocol.org/schemas/v1/envelope.json",
  "title": "FideX Message Envelope",
  "type": "object",
  "required": ["routing_header", "encrypted_payload"],
  "additionalProperties": false,
  "properties": {
    "routing_header": { "$ref": "routing-header.json" },
    "encrypted_payload": {
      "type": "string",
      "minLength": 1,
      "description": "JWE compact serialization"
    }
  }
}
```

### E.3 J-MDN Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://fidex-protocol.org/schemas/v1/jmdn.json",
  "title": "FideX J-MDN (JSON Message Disposition Notification)",
  "type": "object",
  "required": ["original_message_id", "status", "receiver_id", "hash_verification", "timestamp", "error_log", "signature"],
  "additionalProperties": false,
  "properties": {
    "original_message_id": { "type": "string", "minLength": 1 },
    "status": { "type": "string", "enum": ["DELIVERED", "FAILED"] },
    "receiver_id": { "type": "string", "pattern": "^urn:(gln|duns|lei|tin|custom):.+$" },
    "hash_verification": { "type": "string", "pattern": "^sha256:[a-f0-9]{64}$" },
    "timestamp": { "type": "string", "format": "date-time" },
    "error_log": {
      "oneOf": [
        { "type": "null" },
        { "$ref": "#/definitions/error_log_object" }
      ]
    },
    "signature": { "type": "string", "minLength": 1, "description": "JWS compact serialization" }
  },
  "if": { "properties": { "status": { "const": "DELIVERED" } } },
  "then": { "properties": { "error_log": { "type": "null" } } },
  "else": { "properties": { "error_log": { "$ref": "#/definitions/error_log_object" } } },
  "definitions": {
    "error_log_object": {
      "type": "object",
      "required": ["error_code", "error_message"],
      "properties": {
        "error_code": {
          "type": "string",
          "enum": ["DECRYPTION_FAILED", "SIGNATURE_INVALID", "UNKNOWN_DOCUMENT_TYPE", "PAYLOAD_TOO_LARGE", "INTERNAL_ERROR"]
        },
        "error_message": { "type": "string" },
        "details": { "type": "string" }
      }
    }
  }
}
```

### E.4 Error Response Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://fidex-protocol.org/schemas/v1/error.json",
  "title": "FideX Error Response",
  "type": "object",
  "required": ["error"],
  "properties": {
    "error": {
      "type": "object",
      "required": ["code", "message", "timestamp"],
      "properties": {
        "code": { "type": "string" },
        "message": { "type": "string" },
        "timestamp": { "type": "string", "format": "date-time" }
      }
    }
  }
}
```

### E.5 AS5 Configuration Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://fidex-protocol.org/schemas/v1/as5-config.json",
  "title": "FideX AS5 Configuration",
  "type": "object",
  "required": ["fidex_version", "supported_versions", "node_id", "organization_name", "public_domain", "endpoints", "security"],
  "properties": {
    "fidex_version": { "type": "string", "pattern": "^\\d+\\.\\d+$" },
    "supported_versions": { "type": "array", "items": { "type": "string" }, "minItems": 1 },
    "conformance_profile": { "type": "string", "enum": ["core", "enhanced", "edge"] },
    "node_id": { "type": "string", "pattern": "^urn:" },
    "organization_name": { "type": "string", "minLength": 1 },
    "public_domain": { "type": "string", "format": "hostname" },
    "supported_document_types": { "type": "array", "items": { "type": "string", "pattern": "^[A-Z0-9_]+$" } },
    "endpoints": {
      "type": "object",
      "required": ["receive_message", "register", "jwks"],
      "properties": {
        "receive_message": { "type": "string", "format": "uri" },
        "receive_receipt": { "type": "string", "format": "uri" },
        "register": { "type": "string", "format": "uri" },
        "jwks": { "type": "string", "format": "uri" }
      }
    },
    "security": {
      "type": "object",
      "required": ["signature_algorithm", "encryption_algorithm", "content_encryption", "minimum_key_size"],
      "properties": {
        "signature_algorithm": { "type": "string" },
        "encryption_algorithm": { "type": "string" },
        "content_encryption": { "type": "string" },
        "minimum_key_size": { "type": "integer", "minimum": 2048 }
      }
    }
  }
}
```

---

## Document Status

**Version:** 1.0 Draft  
**Status:** Proposed Standard  
**Last Updated:** February 23, 2026  
**License:** Creative Commons Attribution 4.0 International (CC BY 4.0)

**Change Log:**
- 2026-02-23 (Phase 3): JSON Schema definitions (Appendix E) for routing header, envelope, J-MDN, error response, and AS5 config. Security guide restructured with 15-threat control matrix. Implementation guide expanded with comprehensive error handling patterns (Section 8)
- 2026-02-23 (Phase 2): Document type registry (Section 3.3), payload_digest field, version negotiation (Section 6.2.1), partner de-registration (Section 6.5), AS5 config expanded with supported_versions/conformance_profile/supported_document_types, quick start guide
- 2026-02-23 (Phase 1): Document hierarchy preamble, complete J-MDN spec (7 sub-sections), conformance profiles, interoperability test vectors, receipt_webhook made REQUIRED, timestamp format standardized
- 2026-02-20: Initial specification release

**Feedback:**
Submit issues or proposals to: [github.com/fidex-protocol/specification]

---

*End of FideX Protocol Specification*
