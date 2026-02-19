Here is the draft of the **Official FideX Protocol Specification**. It is structured similarly to an IETF RFC (Request for Comments) to establish immediate credibility, but modernized for web developers.

This document serves as the foundational rulebook for the "AS5" standard.

---

# FideX Protocol Specification (Draft v1.0)

**Title:** Fast Integration for Digital Enterprises eXchange (FideX)
**Status:** Draft / Proposed Standard
**Target Application:** B2B Supply Chain & EDI Interoperability

## Abstract

This specification defines the **FideX Protocol**, a modernized, payload-agnostic transport layer for Business-to-Business (B2B) Electronic Data Interchange (EDI). FideX replaces legacy AS2 and AS4 standards by leveraging RESTful web services, the JSON Object Signing and Encryption (JOSE) framework, and asynchronous state management. The goal is to provide enterprise-grade Non-Repudiation of Origin and Data Integrity using tools native to modern web developers.

---

## 1. Transport Layer

FideX is fundamentally a transport-agnostic application layer, but it mandates the use of secure web protocols for data in transit.

* **Primary Transport:** HTTP/1.1 over TLS 1.3 (HTTPS).
* **Multiplexed Transport:** HTTP/2 is highly recommended for batch document transmission to reduce TCP connection overhead.
* **Resilient Transport:** HTTP/3 (QUIC) is supported and recommended for edge nodes (e.g., mobile warehouses) to maintain connection state across dynamic IPs.
* **Authentication:** Mutual TLS (mTLS) is *optional* but supported. Primary authentication relies on the cryptographic payload signatures.

## 2. The Cryptographic Envelope (JOSE)

FideX does not use S/MIME or PGP. All payloads must be secured using the IETF standard JOSE framework (RFC 7515, RFC 7516).

### 2.1 The Routing Header

Every FideX transmission must include a cleartext JSON routing header so gateways can route the message without needing to decrypt the payload.

```json
{
  "fidex_version": "1.0",
  "message_id": "fdx-uuid-1234-5678",
  "sender_id": "urn:gln:0614141000005",
  "receiver_id": "urn:gln:0614141000012",
  "document_type": "GS1_ORDER_JSON",
  "timestamp": "2026-02-19T15:00:00Z",
  "receipt_webhook": "https://api.sender.com/fidex/receipt"
}

```

### 2.2 Signing and Encryption (JWS/JWE)

1. **Hashing & Signing:** The raw business document (e.g., the GS1 JSON Order) is hashed and signed using the Sender's Private RSA Key. This creates a **JSON Web Signature (JWS)**, guaranteeing Non-Repudiation.
2. **Encryption:** The resulting JWS is then encrypted using the Receiver's Public RSA Key. This creates a **JSON Web Encryption (JWE)** token, guaranteeing Data Privacy.
3. **Transmission:** The final HTTP POST body consists only of the Routing Header and the JWE string.

## 3. Key Management

* **Key Sets:** Public keys are distributed via JSON Web Key Sets (JWKS) (RFC 7517).
* **Discovery:** A FideX node must expose a public `/.well-known/jwks.json` endpoint to allow automated key rotation and discovery by Trading Partners.

## 4. Asynchronous State Management (J-MDN)

FideX strictly separates the *network receipt* from the *business receipt*.

### 4.1 The Synchronous Network Response

When a FideX Node receives an HTTP POST, it must perform basic structural validation (Are the headers present? Is it a valid JWE string?).

* If structurally valid, the Node returns **`HTTP 202 Accepted`**. This *does not* mean the document was successfully decrypted or ingested into the ERP.

### 4.2 The Asynchronous Business Receipt (J-MDN)

To replace the legacy AS2 MDN (Message Disposition Notification), FideX introduces the **J-MDN**. Once the receiving Node successfully decrypts the payload and verifies the signature, it must send a cryptographic receipt back to the Sender's `receipt_webhook`.

**J-MDN Payload Structure:**

```json
{
  "original_message_id": "fdx-uuid-1234-5678",
  "status": "DELIVERED",
  "hash_verification": "sha256-hash-of-original-payload",
  "timestamp": "2026-02-19T15:00:02Z",
  "error_log": null
}

```

*Note: The J-MDN itself must be signed (JWS) by the Receiver so the Sender has legal proof of delivery.*

## 5. Payload Agnosticism

The FideX Protocol is strictly a secure envelope. It does not dictate the contents of the business document.

* **Recommended:** Modern implementations should use the **GS1 Semantic Data Model (JSON binding)**.
* **Supported:** Legacy XML, EDIFACT, X12, CSV, or proprietary ERP JSON (like Odoo native dictionaries) can be safely wrapped and transmitted inside the FideX envelope.