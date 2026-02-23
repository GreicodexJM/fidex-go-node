# FideX Protocol Specification (AS5)

**Version:** 1.0 (Draft)  
**Status:** Proposed Standard  
**Date:** February 20, 2026  
**Authors:** FideX Protocol Working Group

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
| `fidex_version` | string | YES | Protocol version (e.g., "1.0") |
| `message_id` | string | YES | Globally unique identifier (UUID v4) |
| `sender_id` | string | YES | URN of sending organization |
| `receiver_id` | string | YES | URN of receiving organization |
| `document_type` | string | YES | Business document type identifier |
| `timestamp` | string | YES | ISO 8601 UTC timestamp |
| `receipt_webhook` | string | NO | HTTPS URL for J-MDN delivery |

**Identifier Format (sender_id / receiver_id):**
- `urn:gln:{gln}` - GS1 Global Location Number
- `urn:duns:{duns}` - D-U-N-S Number
- `urn:lei:{lei}` - Legal Entity Identifier
- `urn:tin:{tin}` - Tax Identification Number
- `urn:custom:{identifier}` - Custom scheme

**Extension Fields:**
Implementations MAY include additional fields prefixed with `x-`. Standard processors MUST ignore unknown extension fields.

### 3.3 Encrypted Payload

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
  "node_id": "urn:gln:1234567890123",
  "organization_name": "Example Corp",
  "public_domain": "fidex.example.com",
  "endpoints": {
    "receive_message": "https://fidex.example.com/api/v1/receive",
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

After successfully decrypting and processing a message, the receiver MUST send a J-MDN to the sender's `receipt_webhook`.

**J-MDN Structure:**
```json
{
  "original_message_id": "fdx-...",
  "status": "DELIVERED",
  "hash_verification": "sha256-...",
  "timestamp": "2026-02-20T18:00:02Z",
  "error_log": null
}
```

**J-MDN Requirements:**
- MUST be signed with receiver's private key (JWS)
- MUST include hash of original business payload
- SHOULD be sent within 5 minutes of receipt
- Sender MUST acknowledge J-MDN with HTTP 200

**Negative Acknowledgment:**
If processing fails, status MUST be "FAILED" and `error_log` MUST contain:
```json
{
  "error_code": "DECRYPTION_FAILED",
  "error_message": "Unable to decrypt payload",
  "details": "..."
}
```

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

## Document Status

**Version:** 1.0 Draft  
**Status:** Proposed Standard  
**Last Updated:** February 20, 2026  
**License:** Creative Commons Attribution 4.0 International (CC BY 4.0)

**Change Log:**
- 2026-02-20: Initial specification release

**Feedback:**
Submit issues or proposals to: [github.com/fidex-protocol/specification]

---

*End of FideX Protocol Specification*
