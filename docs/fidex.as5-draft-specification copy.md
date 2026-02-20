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

FideX is fundamentally a transport-agnostic application layer protocol, but it mandates the use of secure web protocols for all data in transit. This section defines the required and optional transport mechanisms, connection management, and security requirements.

### 1.1 Supported Transport Protocols

#### 1.1.1 HTTP/1.1 over TLS 1.3 (Primary Transport)

**Status:** REQUIRED

HTTP/1.1 over TLS 1.3 is the baseline transport protocol that all FideX nodes MUST support.

**Specifications:**
- Protocol: HTTP/1.1 as defined in RFC 7230-7235
- Transport Security: TLS 1.3 (RFC 8446) or TLS 1.2 (RFC 5246) with restrictions
- Default Port: 443 (HTTPS)
- Connection: Keep-Alive recommended for efficiency

**TLS 1.3 Requirements:**
- Cipher Suites (in order of preference):
  1. `TLS_AES_256_GCM_SHA384`
  2. `TLS_CHACHA20_POLY1305_SHA256`
  3. `TLS_AES_128_GCM_SHA256`
- Certificate Validation: MUST validate full certificate chain
- Server Name Indication (SNI): REQUIRED
- Session Resumption: RECOMMENDED (TLS 1.3 0-RTT acceptable with replay protection)

**TLS 1.2 Fallback (Deprecated but Supported):**
If TLS 1.3 is not available, TLS 1.2 MAY be used with the following constraints:
- MUST use ECDHE key exchange (Perfect Forward Secrecy)
- Allowed Cipher Suites:
  - `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384`
  - `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`
  - `TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256`
- MUST NOT use: RC4, 3DES, MD5, SHA1-based MACs, NULL ciphers
- Minimum RSA key size: 2048 bits (4096 recommended)
- Minimum ECDHE curve: P-256 (P-384 or P-521 recommended)

**HTTP Request Format:**
```http
POST /api/v1/receive HTTP/1.1
Host: fidex.partner.com
Content-Type: application/json
Content-Length: 2048
User-Agent: FideX-Node/1.0
Connection: keep-alive

{
  "routing_header": {...},
  "encrypted_payload": "eyJhbGc..."
}
```

#### 1.1.2 HTTP/2 over TLS 1.3 (Multiplexed Transport)

**Status:** RECOMMENDED

HTTP/2 provides significant performance benefits for high-volume message exchange through multiplexing, header compression, and stream prioritization.

**Specifications:**
- Protocol: HTTP/2 as defined in RFC 7540
- Transport Security: TLS 1.3 (TLS 1.2 with ALPN acceptable)
- ALPN Identifier: `h2`
- Default Port: 443 (HTTPS)

**Benefits:**
- **Multiplexing:** Send multiple messages over single TCP connection
- **Header Compression:** HPACK reduces overhead for repeated headers
- **Server Push:** Not used in FideX (reserved for future extensions)
- **Stream Prioritization:** Future use for priority message routing

**Connection Parameters:**
- `SETTINGS_MAX_CONCURRENT_STREAMS`: Recommended minimum 100
- `SETTINGS_INITIAL_WINDOW_SIZE`: 65535 bytes (default)
- `SETTINGS_MAX_FRAME_SIZE`: 16384 bytes (default)
- `SETTINGS_MAX_HEADER_LIST_SIZE`: 16384 bytes minimum

**Usage Scenarios:**
- Batch transmission of multiple messages to same partner
- High-frequency trading partner connections (>100 messages/hour)
- Reduced latency for request/response cycles

**Example with HTTP/2:**
```
Client establishes single TLS 1.3 connection
├─ Stream 1: POST /api/v1/receive (Message A)
├─ Stream 3: POST /api/v1/receive (Message B)
├─ Stream 5: POST /api/v1/receive (Message C)
└─ All streams multiplex over single TCP connection
```

#### 1.1.3 HTTP/3 over QUIC (Resilient Transport)

**Status:** OPTIONAL (Recommended for Mobile/Edge Nodes)

HTTP/3 with QUIC provides connection resilience across network changes, making it ideal for mobile warehouses, field operations, or unreliable networks.

**Specifications:**
- Protocol: HTTP/3 as defined in RFC 9114
- Transport: QUIC as defined in RFC 9000
- ALPN Identifier: `h3`
- Default Port: 443 (UDP)

**Benefits:**
- **Connection Migration:** Maintain connection across IP address changes
- **0-RTT Resumption:** Faster reconnection with replay protection
- **Improved Loss Recovery:** Per-stream loss recovery (no head-of-line blocking)
- **Built-in TLS 1.3:** Encrypted by design

**Connection Migration Example:**
```
Mobile warehouse node:
1. Establishes QUIC connection over WiFi (IP: 192.168.1.100)
2. Moves to cellular network (IP: 10.20.30.40)
3. QUIC connection persists using connection ID
4. No message transmission interruption
```

**Discovery and Upgrade:**
Nodes advertise HTTP/3 support via `Alt-Svc` header:
```http
HTTP/1.1 202 Accepted
Alt-Svc: h3=":443"; ma=86400
```

Clients MAY upgrade to HTTP/3 on subsequent connections.

### 1.2 Transport Security Requirements

#### 1.2.1 Certificate Management

**Server Certificates:**
- MUST be issued by trusted Certificate Authority (CA)
- MUST include Subject Alternative Name (SAN) matching public_domain
- Certificate chain MUST be complete and valid
- Recommended certificate lifetime: 90 days (automated renewal)
- Support for wildcard certificates: Acceptable (*.example.com)

**Certificate Validation:**
Clients MUST perform:
1. Certificate chain validation against OS/system trust store
2. Hostname verification (RFC 6125)
3. Certificate expiration check
4. Certificate Revocation Status (OCSP or CRL check)

**Self-Signed Certificates:**
Self-signed certificates are NOT RECOMMENDED for production but MAY be used for:
- Development and testing environments
- Internal networks with explicit trust configuration
- MUST NOT be used on public internet without proper security review

#### 1.2.2 Mutual TLS (mTLS)

**Status:** OPTIONAL

Mutual TLS provides additional transport-layer authentication where both client and server present certificates.

**When to Use mTLS:**
- High-security environments requiring defense-in-depth
- Government or regulated industry deployments
- Internal networks with existing PKI infrastructure
- Complementary to (not replacement for) JOSE signatures

**Configuration:**
```yaml
tls_config:
  client_auth: require
  client_ca_file: /path/to/partner-ca-bundle.pem
  verify_client_cert: true
```

**Certificate Requirements:**
- Client certificates MUST contain partner identifier in CN or SAN
- Client certificates MUST be issued by mutually agreed CA
- Certificate rotation MUST be coordinated between partners

**Important:** Even with mTLS, payload signatures (JWS) remain REQUIRED for non-repudiation.

### 1.3 Connection Management

#### 1.3.1 Connection Pooling

Implementations SHOULD maintain persistent connections to frequently contacted partners.

**Recommended Parameters:**
- Maximum idle connections per host: 10
- Maximum connections per host: 100
- Idle connection timeout: 90 seconds
- Connection lifetime: 30 minutes (rotate for security)
- TCP keep-alive: Enabled (60 second interval)

**Connection Pool Example (Go):**
```go
transport := &http.Transport{
    MaxIdleConnsPerHost: 10,
    MaxConnsPerHost:     100,
    IdleConnTimeout:     90 * time.Second,
    TLSHandshakeTimeout: 10 * time.Second,
    ResponseHeaderTimeout: 30 * time.Second,
}
```

#### 1.3.2 Timeouts

**Connection Timeout:** 10 seconds
- Time to establish TCP connection

**TLS Handshake Timeout:** 10 seconds
- Time to complete TLS negotiation

**Request Timeout:** 30 seconds
- Total time for HTTP request/response cycle
- Includes transmission and basic validation
- Does NOT include decryption/processing time

**Keep-Alive Timeout:** 60 seconds
- How long to wait for next request on persistent connection

**Idle Connection Timeout:** 90 seconds
- How long to keep unused connection in pool

#### 1.3.3 Retry Behavior at Transport Layer

**Transient Network Errors:**
Implementations SHOULD retry on transient network errors:
- Connection refused (server not reachable)
- Connection timeout
- Connection reset
- DNS resolution failure (temporary)

**Non-Retryable Errors:**
- TLS certificate validation failure
- TLS handshake failure (cipher mismatch)
- HTTP 4xx errors (except 429 Too Many Requests)
- HTTP 401/403 (authentication/authorization failure)

**Retry Strategy:**
See Section 7.2 for application-level retry logic with exponential backoff.

### 1.4 Request/Response Patterns

#### 1.4.1 Synchronous Message Transmission

**Request Pattern:**
```http
POST /api/v1/receive HTTP/1.1
Host: partner.example.com
Content-Type: application/json
Authorization: Bearer <optional-api-key>
X-FideX-Request-ID: req-uuid-12345
Content-Length: 4096

{
  "routing_header": {...},
  "encrypted_payload": "..."
}
```

**Success Response:**
```http
HTTP/1.1 202 Accepted
Content-Type: application/json
X-FideX-Message-ID: fdx-uuid-67890

{
  "status": "accepted",
  "message_id": "fdx-uuid-67890",
  "timestamp": "2026-02-20T18:00:00Z"
}
```

The HTTP 202 response indicates structural acceptance only. Business-level acknowledgment comes via J-MDN (Section 4.2).

#### 1.4.2 Asynchronous Receipt Delivery

**J-MDN Delivery Pattern:**
```http
POST /erp/fidex/receipt HTTP/1.1
Host: sender.example.com
Content-Type: application/json
X-FideX-Original-Message-ID: fdx-uuid-12345

{
  "original_message_id": "fdx-uuid-12345",
  "status": "DELIVERED",
  "timestamp": "2026-02-20T18:00:05Z",
  "signature": "eyJhbGc..."
}
```

**Receipt Acknowledgment:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "receipt_acknowledged": true
}
```

### 1.5 Transport-Level Security Headers

#### 1.5.1 Required Headers

**Content-Type:** `application/json`
- All FideX messages use JSON encoding

**User-Agent:** `FideX-Node/<version>`
- Identifies node implementation and version
- Example: `FideX-Node/1.0.0 (Go/1.24; linux/amd64)`

#### 1.5.2 Recommended Headers

**Strict-Transport-Security (HSTS):**
```http
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
```

**X-Content-Type-Options:**
```http
X-Content-Type-Options: nosniff
```

**X-Frame-Options:**
```http
X-Frame-Options: DENY
```

**Content-Security-Policy:**
```http
Content-Security-Policy: default-src 'none'
```

#### 1.5.3 Correlation Headers

**X-FideX-Request-ID:** Unique identifier for request tracing
**X-FideX-Message-ID:** Message identifier from routing_header
**X-FideX-Correlation-ID:** End-to-end correlation across systems

### 1.6 Bandwidth and Performance Considerations

#### 1.6.1 Message Size Limits

**Recommended Maximum:** 10 MB per message
- Typical EDI documents: 10-500 KB
- Large documents (catalogs, images): Up to 10 MB
- Exceeding 10 MB: Consider chunking or alternative transfer

**Size Validation:**
Receiving nodes SHOULD validate `Content-Length` before accepting payload:
```http
HTTP/1.1 413 Payload Too Large
Content-Type: application/json

{
  "error": {
    "code": "PAYLOAD_TOO_LARGE",
    "message": "Message exceeds maximum size of 10MB",
    "max_size_bytes": 10485760
  }
}
```

#### 1.6.2 Compression

**Content Encoding:** Compression at HTTP level is OPTIONAL
```http
Accept-Encoding: gzip, deflate
```

Response:
```http
Content-Encoding: gzip
```

**Note:** The JWE payload itself provides some compression through compact serialization. Additional HTTP-level compression provides marginal benefit.

#### 1.6.3 Performance Targets

**Throughput:**
- Single node: 1000 messages/hour minimum
- With HTTP/2: 5000+ messages/hour
- Network bandwidth: ~10 Mbps for typical workloads

**Latency:**
- P50 (median): <500ms end-to-end
- P95: <2 seconds end-to-end
- P99: <5 seconds end-to-end

### 1.7 Transport Protocol Selection Algorithm

Implementations MAY use the following algorithm to select optimal transport:

```
IF partner_supports_http3 AND mobile_or_unstable_network THEN
    USE HTTP/3 over QUIC
ELSE IF message_batch_size > 10 OR message_frequency > 100/hour THEN
    USE HTTP/2
ELSE
    USE HTTP/1.1
END IF
```

Nodes advertise supported protocols in AS5 configuration (Section 6.2).

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

## 6. Partner Discovery Mechanism

FideX introduces a modern, automated partner onboarding workflow that eliminates manual certificate exchange via email. This 4-step handshake ensures mutual authentication and key exchange.

### 6.1 Discovery Endpoints

Every FideX node must expose standardized discovery endpoints:

* **Configuration Endpoint:** `/.well-known/as5-configuration`
* **JWKS Endpoint:** `/.well-known/jwks.json`
* **Registration Endpoint:** `/api/v1/register`

### 6.2 AS5 Configuration Document

The AS5 Configuration document describes node capabilities and endpoints:

```json
{
  "fidex_version": "1.0",
  "node_id": "urn:gln:0614141000005",
  "organization_name": "ACME Supply Corp",
  "public_domain": "fidex.acmesupply.com",
  "capabilities": {
    "supported_document_types": ["GS1_ORDER_JSON", "GS1_INVOICE_JSON", "X12_850"],
    "max_message_size_mb": 10,
    "supports_compression": true
  },
  "endpoints": {
    "receive_message": "https://fidex.acmesupply.com/api/v1/receive",
    "register": "https://fidex.acmesupply.com/api/v1/register",
    "jwks": "https://fidex.acmesupply.com/.well-known/jwks.json"
  },
  "security": {
    "signature_algorithm": "RS256",
    "encryption_algorithm": "RSA-OAEP",
    "content_encryption": "A256GCM",
    "minimum_key_size": 2048
  }
}
```

### 6.3 Four-Step Discovery Handshake

**Step 1: Initiator Fetches Configuration**
- Initiator retrieves AS5 configuration from `/.well-known/as5-configuration`
- Initiator retrieves public keys from `/.well-known/jwks.json`

**Step 2: Initiator Requests Registration**
- Initiator generates a single-use security token (valid 30 minutes)
- Initiator shares token with Responder via secure out-of-band channel
- Initiator sends registration request with token to Responder's `/api/v1/register`

**Step 3: Responder Validates and Registers**
- Responder validates security token
- Responder fetches Initiator's AS5 configuration and JWKS
- Responder stores Initiator's details and public keys
- Responder returns success confirmation

**Step 4: Mutual Recognition Complete**
- Both parties now have each other's public keys
- Message exchange can begin immediately

### 6.4 Security Token Specification

```json
{
  "token": "a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6",
  "issuer_node_id": "urn:gln:0614141000005",
  "issued_at": "2026-02-20T18:00:00Z",
  "expires_at": "2026-02-20T18:30:00Z",
  "discovery_url": "https://fidex.acmesupply.com/.well-known/as5-configuration"
}
```

Tokens must be:
- Cryptographically random (UUID v4 or equivalent)
- Single-use (invalidated after successful registration)
- Time-limited (recommended: 30 minutes)
- Transmitted out-of-band (email, phone, secure messaging)

## 7. Message Lifecycle and State Management

### 7.1 Message States

FideX messages progress through defined states:

| State | Description | Transitions To |
|-------|-------------|----------------|
| `QUEUED` | Message created, awaiting transmission | `SENT`, `FAILED` |
| `SENT` | Message transmitted to partner, awaiting receipt | `DELIVERED`, `FAILED` |
| `DELIVERED` | J-MDN receipt received and verified | (terminal) |
| `FAILED` | Max retries exceeded or permanent error | (terminal) |

### 7.2 Retry Logic

Failed transmissions must follow exponential backoff:

| Attempt | Delay | Total Elapsed |
|---------|-------|---------------|
| 1 (initial) | 0s | 0s |
| 2 | 1m | 1m |
| 3 | 5m | 6m |
| 4 | 15m | 21m |
| 5 | 30m | 51m |
| 6 | 1h | 1h 51m |

After 5 retry attempts, the message transitions to `FAILED` state and requires manual intervention.

### 7.3 Message Timeout

- **Network Timeout:** 30 seconds per HTTP request
- **Receipt Timeout:** 5 minutes to receive J-MDN after successful HTTP 202
- **Processing Timeout:** No strict limit (asynchronous by design)

## 8. Error Handling and Fault Tolerance

### 8.1 HTTP Status Code Semantics

FideX defines specific meanings for HTTP status codes:

| Code | Meaning | Retry? |
|------|---------|--------|
| `202 Accepted` | Structurally valid, queued for processing | No (success) |
| `400 Bad Request` | Invalid routing header or malformed JWE | No (permanent) |
| `401 Unauthorized` | Unknown partner or invalid signature | No (permanent) |
| `413 Payload Too Large` | Message exceeds size limit | No (permanent) |
| `429 Too Many Requests` | Rate limit exceeded | Yes (backoff) |
| `500 Internal Server Error` | Temporary server issue | Yes (backoff) |
| `503 Service Unavailable` | Node overloaded or maintenance | Yes (backoff) |

### 8.2 Error Response Format

```json
{
  "error": {
    "code": "INVALID_ROUTING_HEADER",
    "message": "Missing required field: receiver_id",
    "timestamp": "2026-02-20T18:00:00Z",
    "request_id": "req-uuid-1234-5678"
  }
}
```

### 8.3 J-MDN Error States

When a receiving node cannot process a message, it must send a negative J-MDN:

```json
{
  "original_message_id": "fdx-uuid-1234-5678",
  "status": "FAILED",
  "hash_verification": "sha256-hash-of-original-payload",
  "timestamp": "2026-02-20T18:00:02Z",
  "error_log": {
    "error_code": "DECRYPTION_FAILED",
    "error_message": "Unable to decrypt payload with provided key",
    "details": "Key ID mismatch or corrupted ciphertext"
  }
}
```

Standard error codes:
- `DECRYPTION_FAILED`: Cannot decrypt JWE
- `SIGNATURE_INVALID`: JWS signature verification failed
- `UNKNOWN_DOCUMENT_TYPE`: Document type not supported
- `PAYLOAD_TOO_LARGE`: Exceeds processing limits
- `INTERNAL_ERROR`: Receiving system error

## 9. Security Considerations

### 9.1 Threat Model

FideX addresses the following threats:

**Man-in-the-Middle (MITM)**
- Mitigation: TLS 1.3 for transport, JWE for payload encryption

**Message Tampering**
- Mitigation: JWS signatures with RS256, hash verification in J-MDN

**Replay Attacks**
- Mitigation: Unique `message_id`, timestamp validation, idempotency checks

**Key Compromise**
- Mitigation: Key rotation via JWKS, short-lived registration tokens

**Repudiation**
- Mitigation: Cryptographic signatures with private keys, signed J-MDNs

### 9.2 Key Management Best Practices

**Key Generation**
- Use RSA 2048-bit minimum (4096-bit recommended)
- Generate keys in secure environment (HSM or secure enclave preferred)
- Never transmit private keys over network

**Key Storage**
- Private keys must be encrypted at rest
- Use OS keychain or dedicated secrets manager
- Restrict file system permissions (600 or equivalent)

**Key Rotation**
- Rotate keys annually or after suspected compromise
- Update JWKS endpoint atomically
- Maintain backward compatibility period (30 days recommended)

**Key Revocation**
- Remove compromised keys from JWKS immediately
- Notify partners via out-of-band channel
- Re-establish trust via discovery handshake

### 9.3 TLS Configuration

**Minimum Requirements**
- TLS 1.3 (TLS 1.2 acceptable with ECDHE cipher suites)
- Perfect Forward Secrecy (PFS) required
- Certificate validation with proper CA chain

**Cipher Suites (TLS 1.3)**
- `TLS_AES_256_GCM_SHA384` (recommended)
- `TLS_CHACHA20_POLY1305_SHA256`
- `TLS_AES_128_GCM_SHA256`

**mTLS (Optional)**
- Client certificates can augment JOSE signatures
- Useful for additional transport-layer authentication
- Not required if JOSE signatures properly verified

### 9.4 Rate Limiting

Nodes should implement rate limiting to prevent abuse:

- **Per Partner:** 1000 messages/hour under normal conditions
- **Burst Allowance:** 100 messages in 1 minute window
- **Discovery Endpoints:** 10 requests/hour (prevents enumeration)
- **Failed Authentication:** Exponential backoff after 3 failures

## 10. Compliance and Legal Considerations

### 10.1 Non-Repudiation Requirements

For legally binding transactions, implementations must:

1. **Preserve Original Signatures**: Store JWS signatures with messages
2. **Maintain Audit Trail**: Log all message state transitions with timestamps
3. **Archive J-MDNs**: Retain signed receipts for regulatory period
4. **Timestamp Authority**: Use RFC 3161 TSA for legal-grade timestamps (optional)

### 10.2 Data Retention

**Minimum Retention (Audit Trail)**
- Message metadata: 7 years
- J-MDN receipts: 7 years
- Cryptographic signatures: 7 years

**Optional Retention (Business Payload)**
- Raw business documents: Per organizational policy
- Encrypted payload storage: May be deleted after ERP ingestion

### 10.3 Privacy and GDPR

**Personal Data Handling**
- FideX routing headers may contain identifiers (GLNs, org names)
- Implement data subject access requests (DSAR) mechanisms
- Support right to erasure (except legally required audit trails)
- Document data processing agreements with trading partners

### 10.4 Industry Compliance

**EDI Regulations**
- Maintains AS2 RFC 4130 principles (adapted for modern web)
- Compatible with FDA 21 CFR Part 11 (electronic signatures)
- Meets HIPAA security requirements (with proper implementation)

**Supply Chain Standards**
- Designed for GS1 standards integration
- Compatible with EPCIS event exchange
- Supports UN/CEFACT trade facilitation frameworks

## 11. Implementation Guidelines

### 11.1 Node Architecture Recommendations

**Hexagonal Architecture**
- Separate business logic from transport/storage adapters
- Use dependency injection for testability
- Define clear port/adapter boundaries

**Persistence**
- Store messages, partners, and keys in durable storage
- Use ACID transactions for state changes
- Implement write-ahead logging for crash recovery

**Concurrency**
- Support parallel message processing
- Use message queue for retry management
- Implement graceful shutdown with connection draining

### 11.2 Performance Optimization

**Message Batching**
- Use HTTP/2 multiplexing for multiple concurrent messages
- Consider bulk receipt acknowledgment for high-volume scenarios

**Caching**
- Cache partner public keys (refresh on JWKS updates)
- Cache AS5 configurations (TTL: 1 hour)
- Use CDN for static discovery documents

**Connection Pooling**
- Reuse HTTP connections to same partners
- Configure appropriate timeouts and keep-alive

### 11.3 Monitoring and Observability

**Metrics to Track**
- Messages sent/received per minute
- Message processing latency (p50, p95, p99)
- Retry rate and failure rate
- Partner availability
- Queue depth

**Logging Requirements**
- Structured logging (JSON format recommended)
- Include correlation IDs for request tracing
- Log security events (authentication failures, key rotation)
- Never log sensitive payloads or private keys

**Health Checks**
- `/health` endpoint for liveness probe
- `/ready` endpoint for readiness probe
- Check database connectivity, queue health, partner reachability

### 11.4 Testing Strategies

**Unit Testing**
- Test JOSE encryption/decryption independently
- Mock partner repositories and HTTP clients
- Verify routing header validation logic

**Integration Testing**
- Test complete message flow end-to-end
- Simulate partner discovery handshake
- Test retry logic with controlled failures

**Security Testing**
- Verify signature validation (reject tampered messages)
- Test key rotation scenarios
- Attempt replay attacks
- Validate TLS configuration

**Interoperability Testing**
- Exchange messages with reference implementation
- Test with different JOSE library implementations
- Verify cross-platform compatibility

## 12. Versioning and Extensibility

### 12.1 Protocol Versioning

**Version Field**
- All routing headers include `fidex_version` field
- Current version: `"1.0"`
- Future versions maintain backward compatibility for 2 years

**Version Negotiation**
- Nodes advertise supported versions in AS5 configuration
- Initiator selects highest mutually supported version
- Version mismatch results in HTTP 400 with clear error message

### 12.2 Extension Mechanism

**Custom Headers**
- Implementations may add custom fields prefixed with `x-`
- Example: `"x-priority": "HIGH"` for priority routing
- Standard fields take precedence over extensions

**Document Type Registry**
- Standard types: `GS1_ORDER_JSON`, `GS1_INVOICE_JSON`, `X12_850`, `EDIFACT_ORDERS`
- Custom types use reverse domain notation: `com.acme.CUSTOM_ORDER_V2`

### 12.3 Deprecation Policy

**Breaking Changes**
- Require major version increment (2.0, 3.0)
- Minimum 1-year advance notice
- Simultaneous support for old and new versions during transition

**Non-Breaking Changes**
- Can be introduced in minor versions (1.1, 1.2)
- Examples: New optional fields, additional document types
- Backward compatibility required

## 13. Reference Implementation

### 13.1 FideX AS5 Node (Golang)

An open-source reference implementation is available:

**Repository:** [github.com/fidex-protocol/fidex-node]
**Language:** Go 1.24+
**License:** Apache 2.0

**Features:**
- Complete AS5 protocol implementation
- Automated partner discovery
- Web-based dashboard
- REST API for ERP integration
- Hot-folder monitoring for legacy systems
- SQLite persistence (PostgreSQL compatible)

### 13.2 Client Libraries

**fidex-js** (Node.js/TypeScript)
```javascript
import { FideXClient } from '@fidex/client';

const client = new FideXClient({
  nodeUrl: 'https://fidex.mycompany.com',
  apiKey: 'your-api-key'
});

await client.sendMessage({
  destinationPartnerId: 'urn:gln:partner:123',
  documentType: 'GS1_ORDER_JSON',
  payload: orderData
});
```

**fidex-python** (Python 3.10+)
```python
from fidex import FideXClient

client = FideXClient(
    node_url='https://fidex.mycompany.com',
    api_key='your-api-key'
)

client.send_message(
    destination_partner_id='urn:gln:partner:123',
    document_type='GS1_ORDER_JSON',
    payload=order_data
)
```

## 14. Comparison with Legacy Standards

### 14.1 AS2 vs. FideX

| Feature | AS2 (RFC 4130) | FideX |
|---------|----------------|-------|
| Transport | HTTP/1.1 + S/MIME | HTTP/2+ + JOSE |
| Encryption | S/MIME (CMS) | JWE (modern crypto) |
| Signatures | S/MIME | JWS (RS256) |
| Key Exchange | Manual (email) | Automated (discovery) |
| Receipt | MDN (email-style) | J-MDN (JSON webhook) |
| Payload Format | MIME multipart | JSON envelope |
| Developer Experience | Complex (email heritage) | Simple (REST API) |

### 14.2 AS4 vs. FideX

| Feature | AS4 (OASIS) | FideX |
|---------|-------------|-------|
| Architecture | SOAP/XML messaging | RESTful JSON |
| Message Format | ebMS XML | JSON routing + JOSE |
| Reliability | WS-ReliableMessaging | HTTP + queue + retry |
| Security | WS-Security | JOSE + TLS 1.3 |
| Complexity | High (SOAP stack) | Low (HTTP + JSON) |
| Tooling | Limited modern support | Native web dev tools |

### 14.3 Migration Path

**From AS2:**
1. Deploy FideX node alongside AS2 gateway
2. Onboard new partners using FideX discovery
3. Migrate existing partners incrementally
4. Maintain AS2 for legacy partners during transition

**From AS4:**
1. Map ebMS message structure to FideX routing header
2. Wrap existing XML payloads in JWE envelope
3. Replace WS-Security with JOSE signatures
4. Update endpoints to REST architecture

## 15. Future Roadmap

### 15.1 Planned Enhancements (v1.1)

- **Message Compression:** Gzip compression for large payloads
- **Batch Receipts:** Single J-MDN for multiple messages
- **Priority Queuing:** Express lane for time-sensitive documents
- **Webhook Retries:** Automatic retry for receipt delivery failures

### 15.2 Under Consideration (v2.0)

- **GraphQL API:** Alternative to REST for complex queries
- **WebSocket Streaming:** Real-time message push (vs. pull)
- **Multi-Tenant Support:** Single node serving multiple organizations
- **Blockchain Integration:** Immutable audit trail via distributed ledger

### 15.3 Research Topics

- **Quantum-Resistant Crypto:** Post-quantum signature algorithms
- **Zero-Knowledge Proofs:** Privacy-preserving business logic
- **Decentralized Identity:** DID-based partner authentication

## 16. Appendices

### Appendix A: Complete Message Example

**Outbound API Request:**
```json
POST /api/v1/transmit HTTP/1.1
Host: fidex-local.mycompany.com
Authorization: Bearer api-key-12345
Content-Type: application/json

{
  "destination_partner_id": "urn:gln:0614141000012",
  "document_type": "GS1_ORDER_JSON",
  "receipt_webhook": "https://erp.mycompany.com/fidex/receipt",
  "payload": {
    "order_id": "PO-2026-00123",
    "order_date": "2026-02-20",
    "buyer": {
      "gln": "0614141000005",
      "name": "ACME Supply Corp"
    },
    "seller": {
      "gln": "0614141000012",
      "name": "Widget Manufacturing Inc"
    },
    "line_items": [
      {
        "gtin": "00012345678905",
        "description": "Widget Model X",
        "quantity": 100,
        "unit_price": 24.99,
        "currency": "USD"
      }
    ],
    "total_amount": 2499.00
  }
}
```

**Transmitted FideX Message:**
```json
{
  "routing_header": {
    "fidex_version": "1.0",
    "message_id": "fdx-a1b2c3d4-e5f6-g7h8",
    "sender_id": "urn:gln:0614141000005",
    "receiver_id": "urn:gln:0614141000012",
    "document_type": "GS1_ORDER_JSON",
    "timestamp": "2026-02-20T18:30:00Z",
    "receipt_webhook": "https://erp.mycompany.com/fidex/receipt"
  },
  "encrypted_payload": "eyJhbGciOiJSU0EtT0FFUCIsImVuYyI6IkEyNTZHQ00ifQ..."
}
```

**Received J-MDN:**
```json
POST /fidex/receipt HTTP/1.1
Host: erp.mycompany.com
Content-Type: application/json

{
  "original_message_id": "fdx-a1b2c3d4-e5f6-g7h8",
  "status": "DELIVERED",
  "hash_verification": "sha256-9f86d081884c7d659a2feaa0c55ad015...",
  "timestamp": "2026-02-20T18:30:02Z",
  "error_log": null,
  "signature": "eyJhbGciOiJSUzI1NiIsImtpZCI6InJlY2VpdmVyLWtleS0xIn0..."
}
```

### Appendix B: JWKS Example

```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "kid": "node-2026-02-primary",
      "alg": "RS256",
      "n": "xGOr-H7A-PWX...",
      "e": "AQAB"
    },
    {
      "kty": "RSA",
      "use": "enc",
      "kid": "node-2026-02-primary",
      "alg": "RSA-OAEP",
      "n": "xGOr-H7A-PWX...",
      "e": "AQAB"
    }
  ]
}
```

### Appendix C: Glossary

**AS5**: Application Statement 5 - The FideX protocol designation
**B2B**: Business-to-Business electronic commerce
**EDI**: Electronic Data Interchange
**GLN**: Global Location Number (GS1 standard)
**GTIN**: Global Trade Item Number
**J-MDN**: JSON Message Disposition Notification
**JOSE**: JSON Object Signing and Encryption
**JWE**: JSON Web Encryption (RFC 7516)
**JWK**: JSON Web Key (RFC 7517)
**JWKS**: JSON Web Key Set
**JWS**: JSON Web Signature (RFC 7515)
**mTLS**: Mutual TLS (client certificate authentication)
**PKI**: Public Key Infrastructure
**TLS**: Transport Layer Security

### Appendix D: References

**IETF RFCs:**
- RFC 7515: JSON Web Signature (JWS)
- RFC 7516: JSON Web Encryption (JWE)
- RFC 7517: JSON Web Key (JWK)
- RFC 7518: JSON Web Algorithms (JWA)
- RFC 8446: The Transport Layer Security (TLS) Protocol Version 1.3
- RFC 4130: AS2 Protocol (legacy reference)

**Standards Bodies:**
- GS1: Global standards for business communication
- OASIS: AS4 and ebXML messaging
- UN/CEFACT: Trade facilitation and electronic business

### Appendix E: Contact and Governance

**Protocol Steward:** FideX Protocol Foundation (proposed)
**Mailing List:** [protocol@fidex.org]
**Issue Tracker:** [github.com/fidex-protocol/specification/issues]
**Community Forum:** [discuss.fidex.org]

**Contributing:**
- Propose changes via RFC process
- Reference implementation PRs welcome
- Join working group meetings (monthly)

---

## Document Status

**Version:** 1.0 (Draft)
**Status:** Request for Comments
**Last Updated:** February 20, 2026
**Authors:** FideX Protocol Working Group
**License:** Creative Commons Attribution 4.0 International (CC BY 4.0)

**Change Log:**
- 2026-02-20: Initial draft release (v1.0)
- Sections 1-5: Core protocol specification
- Sections 6-11: Implementation guidelines
- Sections 12-16: Extensions, compliance, and appendices

**Next Steps:**
1. Community review period (60 days)
2. Reference implementation validation
3. Interoperability testing with partners
4. Security audit by third party
5. Proposed standard publication (v1.0 final)
