# FideX Annotated Specification (Draft v1.0)

**Title:** Fast Integration for Digital Enterprises eXchange (FideX)
**Status:** Draft / Proposed Standard
**Target Application:** B2B Supply Chain & EDI Interoperability

---

> **Document Status: INFORMATIVE**
>
> This is the **annotated companion document** to the FideX Protocol Specification.
> It provides detailed rationale, architecture explanations, code examples, and
> implementation guidance that supplement the normative specification.
>
> **This document is NOT the authoritative specification.** In case of conflict,
> the normative specification takes precedence:
>
> - `fidex-protocol-specification.md` — **NORMATIVE** authoritative specification
> - **This document** (`fidex-annotated-specification.md`) — INFORMATIVE companion
> - `fidex-security-guide.md` — INFORMATIVE operational security best practices
> - `fidex-implementation-guide.md` — INFORMATIVE multi-language implementation examples
> - `openapi.yaml` — **NORMATIVE** OpenAPI 3.0 machine-readable contract
>
> Developers should read the normative specification first, then consult this
> document for deeper understanding and practical examples.

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

FideX does not use S/MIME or PGP. All payloads MUST be secured using the IETF standard JOSE (JSON Object Signing and Encryption) framework as defined in RFC 7515 (JWS) and RFC 7516 (JWE). This section defines the cryptographic requirements, message structure, and security properties.

### 2.1 The Routing Header

Every FideX transmission MUST include a cleartext JSON routing header. This design allows intermediary gateways, load balancers, and routing infrastructure to process messages without requiring access to private decryption keys.

#### 2.1.1 Routing Header Structure

**Complete Specification:**

```json
{
  "fidex_version": "1.0",
  "message_id": "fdx-a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6",
  "sender_id": "urn:gln:0614141000005",
  "receiver_id": "urn:gln:0614141000012",
  "document_type": "GS1_ORDER_JSON",
  "timestamp": "2026-02-20T18:30:00Z",
  "receipt_webhook": "https://api.sender.com/fidex/receipt"
}
```

#### 2.1.2 Routing Header Field Definitions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fidex_version` | string | YES | Protocol version. Current: `"1.0"` |
| `message_id` | string | YES | Globally unique message identifier (UUID v4 format) |
| `sender_id` | string | YES | URN identifying sender organization (e.g., GLN, DUNS) |
| `receiver_id` | string | YES | URN identifying receiver organization |
| `document_type` | string | YES | Business document type identifier |
| `timestamp` | string | YES | ISO 8601 timestamp (UTC) when message was created |
| `receipt_webhook` | string | NO | HTTPS URL where J-MDN should be delivered |

**Field Constraints:**

**fidex_version:**
- Format: Semantic versioning `"major.minor"`
- Example: `"1.0"`, `"1.1"`, `"2.0"`
- Nodes MUST reject messages with unsupported versions

**message_id:**
- Format: UUID v4 (RFC 4122)
- Example: `"fdx-a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6"`
- Prefix `"fdx-"` is RECOMMENDED but not required
- MUST be globally unique (no collisions across all messages ever)
- Used for idempotency and duplicate detection

**sender_id / receiver_id:**
- Format: URN with organizational identifier
- Standard prefixes:
  - `urn:gln:` - GS1 Global Location Number
  - `urn:duns:` - Dun & Bradstreet D-U-N-S Number
  - `urn:lei:` - Legal Entity Identifier
  - `urn:rif:` - Fiscal Identification Reference
  - `urn:custom:` - Custom identifier scheme
- Example: `"urn:gln:0614141000005"`
- MUST match node_id in partner's AS5 configuration

**document_type:**
- Format: Uppercase alphanumeric with underscores
- Standard types:
  - `GS1_ORDER_JSON` - GS1 purchase order
  - `GS1_INVOICE_JSON` - GS1 invoice
  - `GS1_DESADV_JSON` - GS1 despatch advice
  - `X12_850` - ANSI X12 850 Purchase Order
  - `EDIFACT_ORDERS` - UN/EDIFACT ORDERS message
- Custom types: Reverse domain notation `COM_ACME_CUSTOM_DOC_V2`

**timestamp:**
- Format: ISO 8601 with UTC timezone
- Example: `"2026-02-20T18:30:00Z"`
- MUST include timezone designator `Z` (UTC)
- Millisecond precision is OPTIONAL: `"2026-02-20T18:30:00.123Z"`
- Used for message ordering, replay detection, and audit trails

**receipt_webhook:**
- Format: HTTPS URL (HTTP not allowed)
- Example: `"https://erp.sender.com/api/v1/fidex/receipt"`
- MUST be accessible from receiver's network
- Receiver will POST J-MDN to this endpoint

#### 2.1.3 Optional Extension Fields

Implementations MAY include additional fields prefixed with `x-`:

```json
{
  "fidex_version": "1.0",
  "message_id": "fdx-uuid-12345",
  "sender_id": "urn:gln:0614141000005",
  "receiver_id": "urn:gln:0614141000012",
  "document_type": "GS1_ORDER_JSON",
  "timestamp": "2026-02-20T18:30:00Z",
  "receipt_webhook": "https://api.sender.com/fidex/receipt",
  "x-priority": "HIGH",
  "x-correlation-id": "po-2026-00123",
  "x-environment": "production"
}
```

Extension fields are ignored by nodes that don't recognize them.

#### 2.1.4 Routing Header Validation

Receiving nodes MUST validate the routing header before attempting decryption:

1. **Structure Validation**: JSON parseable, all required fields present
2. **Format Validation**: Each field matches expected format/constraints
3. **Semantic Validation**: 
   - `receiver_id` matches node's own identifier
   - `sender_id` corresponds to known partner
   - `document_type` is supported
   - `timestamp` is within acceptable time window (±15 minutes recommended)

**Validation Failure Response:**
```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": {
    "code": "INVALID_ROUTING_HEADER",
    "message": "Missing required field: receipt_webhook",
    "field": "receipt_webhook"
  }
}
```

### 2.2 Signing and Encryption (JWS/JWE)

FideX uses a **nested JWT structure**: Sign-then-Encrypt (JWS nested inside JWE). This ensures both authenticity (signature) and confidentiality (encryption).

#### 2.2.1 Cryptographic Processing Flow

```
┌─────────────────────────────────────────────────────┐
│  1. Raw Business Payload (JSON)                     │
│     {"order_id": "PO-123", "amount": 1000, ...}    │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│  2. Sign with Sender's Private Key (JWS)           │
│     Algorithm: RS256                                │
│     Result: eyJhbGc...signature-protected-payload   │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│  3. Encrypt with Receiver's Public Key (JWE)       │
│     Key Algorithm: RSA-OAEP                         │
│     Content Encryption: A256GCM                     │
│     Result: eyJhbGc...encrypted-jws                 │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│  4. Combine with Routing Header                     │
│     {                                               │
│       "routing_header": {...},                      │
│       "encrypted_payload": "eyJhbGc..."            │
│     }                                               │
└─────────────────────────────────────────────────────┘
```

**Decryption Flow (Receiver):**
```
1. Receive HTTP POST
2. Validate routing header
3. Extract encrypted_payload
4. Decrypt JWE using Receiver's Private Key → JWS
5. Verify JWS signature using Sender's Public Key → Payload
6. Process business payload
7. Send J-MDN receipt
```

#### 2.2.2 JSON Web Signature (JWS) Specification

**Algorithm:** RS256 (RSA Signature with SHA-256)

**JWS Header:**
```json
{
  "alg": "RS256",
  "typ": "JWT",
  "kid": "sender-key-2026-02-primary"
}
```

**Header Parameters:**

| Parameter | Value | Description |
|-----------|-------|-------------|
| `alg` | `RS256` | REQUIRED. RSA signature with SHA-256 |
| `typ` | `JWT` | OPTIONAL. Media type of JWS |
| `kid` | string | REQUIRED. Key ID matching JWKS entry |

**JWS Compact Serialization:**
```
eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6InNlbmRlci1rZXktMjAyNi0wMi1wcmltYXJ5In0.
eyJvcmRlcl9pZCI6IlBPLTEyMyIsImFtb3VudCI6MTAwMH0.
dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk...
```

Structure: `base64url(header).base64url(payload).base64url(signature)`

**Signing Process:**

1. Serialize payload as UTF-8 JSON
2. Create JWS header with `alg: RS256` and sender's `kid`
3. Compute signing input: `base64url(header) + '.' + base64url(payload)`
4. Sign using RSASSA-PKCS1-v1_5 with SHA-256
5. Construct JWS: `signing_input + '.' + base64url(signature)`

**Security Properties:**
- **Non-Repudiation**: Only sender with private key could create valid signature
- **Integrity**: Any tampering invalidates signature
- **Authentication**: Signature proves sender identity

**Key Requirements:**
- Minimum key size: 2048 bits (4096 bits RECOMMENDED)
- Key format: RSA private key in PEM or JWK format
- Key rotation: Annual or after suspected compromise

#### 2.2.3 JSON Web Encryption (JWE) Specification

**Algorithms:**
- **Key Encryption:** RSA-OAEP (RSA Optimal Asymmetric Encryption Padding)
- **Content Encryption:** A256GCM (AES-256 in Galois/Counter Mode)

**JWE Header:**
```json
{
  "alg": "RSA-OAEP",
  "enc": "A256GCM",
  "kid": "receiver-key-2026-02-primary"
}
```

**Header Parameters:**

| Parameter | Value | Description |
|-----------|-------|-------------|
| `alg` | `RSA-OAEP` | REQUIRED. Key encryption algorithm |
| `enc` | `A256GCM` | REQUIRED. Content encryption algorithm |
| `kid` | string | REQUIRED. Receiver's key ID from JWKS |

**JWE Compact Serialization:**
```
eyJhbGciOiJSU0EtT0FFUCIsImVuYyI6IkEyNTZHQ00iLCJraWQiOiJyZWNlaXZlci1rZXktMjAyNi0wMi1wcmltYXJ5In0.
OKOawDo13gRp2ojaHV7LFpZcgV7T6DVZKTyKOMTYUmKoTCVJRgckCL9kiMT03JGeipsEdY3mx_etLbbWSrFr05kLzcSr4qKAq7YN7e9jwQRb23nfa6c9d-StnImGyFDbSv04uVuxIp5Zms1gNxKKK2Da14B8S4rzVRltdYwam_lDp5XnZAYpQdb76FdIKLaVmqgfwX7XWRxv2322i-vDxRfqNzo_tETKzpVLzfiwQyeyPGLBIO56YJ7eObdv0je81860ppamavo35UgoRdbYaBcoh9QcfylQr66oc6vFWXRcZ_ZT2LawVCWTIy3brGPi6UklfCpIMfIjf7iGdXKHzg.
48V1_ALb6US04U3b.
5eym8TW_c8SuK0ltJ3rpYIzOeDQz7TALvtu6UG9oMo4vpzs9tX_EFShS8iB7j6jiSyfIgSSF9X4RJTk3JM8qA3KLkW3xeRgPa-3PJFjVOJ1eajAVDMjOHBiZFKe4TF7y3Y9hAjRzSqn6.
XFBoMYUZodetZdvTiFvSkQ
```

Structure: `base64url(header).base64url(encrypted_key).base64url(iv).base64url(ciphertext).base64url(auth_tag)`

**Encryption Process:**

1. Generate random 256-bit Content Encryption Key (CEK)
2. Generate random 96-bit Initialization Vector (IV)
3. Encrypt JWS using AES-256-GCM with CEK and IV
4. Encrypt CEK using receiver's RSA public key with OAEP
5. Construct JWE: header + encrypted_CEK + IV + ciphertext + auth_tag

**Decryption Process:**

1. Parse JWE into five parts
2. Decrypt encrypted_CEK using receiver's RSA private key → CEK
3. Decrypt ciphertext using AES-256-GCM with CEK and IV → JWS
4. Verify authentication tag
5. Continue to JWS verification

**Security Properties:**
- **Confidentiality**: Only receiver with private key can decrypt
- **Authenticated Encryption**: GCM provides both encryption and integrity
- **Forward Secrecy**: Each message uses unique CEK and IV

**Key Requirements:**
- Minimum RSA key size: 2048 bits (4096 bits RECOMMENDED)
- CEK: 256 bits, randomly generated per message
- IV: 96 bits, randomly generated per message, MUST NOT repeat

#### 2.2.4 Complete Message Structure

**Final Transmission Format:**

```json
{
  "routing_header": {
    "fidex_version": "1.0",
    "message_id": "fdx-a1b2c3d4-e5f6-g7h8-i9j0-k1l2m3n4o5p6",
    "sender_id": "urn:gln:0614141000005",
    "receiver_id": "urn:gln:0614141000012",
    "document_type": "GS1_ORDER_JSON",
    "timestamp": "2026-02-20T18:30:00Z",
    "receipt_webhook": "https://api.sender.com/fidex/receipt"
  },
  "encrypted_payload": "eyJhbGciOiJSU0EtT0FFUCIsImVuYyI6IkEyNTZHQ00ifQ.OKOawDo13gRp2ojaHV7LFpZcgV7T6DVZKTyKOMTYUmKoTCVJRgckCL9kiMT03JGeipsEdY3mx_etLbbWSrFr05kLzcSr4qKAq7YN7e9jwQRb23nfa6c9d-StnImGyFDbSv04uVuxIp5Zms1gNxKKK2Da14B8S4rzVRltdYwam_lDp5XnZAYpQdb76FdIKLaVmqgfwX7XWRxv2322i-vDxRfqNzo_tETKzpVLzfiwQyeyPGLBIO56YJ7eObdv0je81860ppamavo35UgoRdbYaBcoh9QcfylQr66oc6vFWXRcZ_ZT2LawVCWTIy3brGPi6UklfCpIMfIjf7iGdXKHzg.48V1_ALb6US04U3b.5eym8TW_c8SuK0ltJ3rpYIzOeDQz7TALvtu6UG9oMo4vpzs9tX_EFShS8iB7j6jiSyfIgSSF9X4RJTk3JM8qA3KLkW3xeRgPa-3PJFjVOJ1eajAVDMjOHBiZFKe4TF7y3Y9hAjRzSqn6.XFBoMYUZodetZdvTiFvSkQ"
}
```

### 2.3 Cryptographic Algorithm Requirements

#### 2.3.1 Mandatory Algorithms (MUST Support)

| Operation | Algorithm | Standard | Key Size |
|-----------|-----------|----------|----------|
| Signature | RS256 | RFC 7518 | 2048-bit RSA |
| Key Encryption | RSA-OAEP | RFC 7518 | 2048-bit RSA |
| Content Encryption | A256GCM | RFC 7518 | 256-bit AES |
| Hash Function | SHA-256 | FIPS 180-4 | - |

#### 2.3.2 Optional Algorithms (MAY Support)

| Operation | Algorithm | Notes |
|-----------|-----------|-------|
| Signature | RS384, RS512 | Stronger SHA variants |
| Signature | PS256, PS384, PS512 | RSASSA-PSS |
| Key Encryption | RSA-OAEP-256 | OAEP with SHA-256 |
| Content Encryption | A128GCM, A192GCM | Weaker, not recommended |

#### 2.3.3 Prohibited Algorithms (MUST NOT Use)

- `none` - No signature/encryption
- `HS256` - Symmetric signatures (no non-repudiation)
- Any algorithm with known vulnerabilities
- RSA keys smaller than 2048 bits

### 2.4 Key Identifier (kid) Management

#### 2.4.1 Key ID Format

**Recommended Format:** `{node-id}-{purpose}-{date}-{sequence}`

**Examples:**
- `sender-sign-2026-02-primary`
- `receiver-enc-2026-02-backup`
- `node-a-2026-Q1-001`

**Requirements:**
- MUST be unique within organization's JWKS
- SHOULD be descriptive (purpose, date)
- MUST match `kid` in JWKS endpoint

#### 2.4.2 Key ID Resolution

When processing a message:

1. Extract `kid` from JWS/JWE header
2. Lookup corresponding key in JWKS
3. If not found, refresh JWKS from partner (cache may be stale)
4. If still not found, reject message with error

**Error Response:**
```json
{
  "error": {
    "code": "UNKNOWN_KEY_ID",
    "message": "Key ID 'sender-sign-2026-02-primary' not found in partner JWKS",
    "kid": "sender-sign-2026-02-primary"
  }
}
```

### 2.5 Security Considerations for JOSE

#### 2.5.1 Timing Attack Prevention

Implementations MUST use constant-time comparison for:
- Signature verification
- Authentication tag validation
- Key ID matching

Avoid early-exit on mismatch that could leak timing information.

#### 2.5.2 Replay Attack Prevention

Nodes SHOULD maintain a cache of recent `message_id` values:

```python
# Pseudocode
def check_replay(message_id):
    if message_id in recent_messages_cache:
        reject("Duplicate message_id detected")
    
    recent_messages_cache.add(message_id, ttl=24_hours)
```

Cache TTL: 24 hours minimum (longer if storage permits)

#### 2.5.3 Timestamp Validation

Nodes SHOULD reject messages with timestamps outside acceptable window:

```python
# Pseudocode
current_time = now()
message_time = parse_iso8601(routing_header['timestamp'])
time_diff = abs(current_time - message_time)

if time_diff > 15_minutes:
    reject("Message timestamp outside acceptable window")
```

Acceptable window: ±15 minutes (accounts for clock skew)

#### 2.5.4 Padding Oracle Prevention

When using RSA-OAEP:
- Use proper OAEP implementation with secure padding
- Do not expose padding validation errors to sender
- Always return generic "decryption failed" error

### 2.6 Implementation Guidance

#### 2.6.1 Recommended Libraries

**Go:**
- `go-jose/go-jose/v4` - Complete JOSE implementation

**JavaScript/Node.js:**
- `jose` - Modern JOSE library by panva

**Python:**
- `python-jose` - JOSE implementation for Python
- `authlib` - Authentication library with JOSE support

**Java:**
- `nimbus-jose-jwt` - Comprehensive JOSE library

#### 2.6.2 Code Example (Go)

**Encryption (Sender):**
```go
import "github.com/go-jose/go-jose/v4"

func encryptMessage(payload []byte, senderPrivKey *rsa.PrivateKey, 
                    receiverPubKey *rsa.PublicKey, senderKID, receiverKID string) (string, error) {
    
    // Step 1: Sign with sender's private key (JWS)
    signingKey := jose.SigningKey{
        Algorithm: jose.RS256,
        Key:       senderPrivKey,
    }
    
    signer, err := jose.NewSigner(signingKey, &jose.SignerOptions{
        ExtraHeaders: map[jose.HeaderKey]interface{}{
            "kid": senderKID,
        },
    })
    if err != nil {
        return "", err
    }
    
    jws, err := signer.Sign(payload)
    if err != nil {
        return "", err
    }
    
    jwsCompact, err := jws.CompactSerialize()
    if err != nil {
        return "", err
    }
    
    // Step 2: Encrypt JWS with receiver's public key (JWE)
    encrypter, err := jose.NewEncrypter(
        jose.A256GCM,
        jose.Recipient{
            Algorithm: jose.RSA_OAEP,
            Key:       receiverPubKey,
            KeyID:     receiverKID,
        },
        nil,
    )
    if err != nil {
        return "", err
    }
    
    jwe, err := encrypter.Encrypt([]byte(jwsCompact))
    if err != nil {
        return "", err
    }
    
    return jwe.CompactSerialize()
}
```

**Decryption (Receiver):**
```go
func decryptMessage(jweCompact string, receiverPrivKey *rsa.PrivateKey, 
                    senderPubKey *rsa.PublicKey) ([]byte, error) {
    
    // Step 1: Parse and decrypt JWE
    jwe, err := jose.ParseEncrypted(jweCompact)
    if err != nil {
        return nil, err
    }
    
    jwsBytes, err := jwe.Decrypt(receiverPrivKey)
    if err != nil {
        return nil, errors.New("decryption failed")
    }
    
    // Step 2: Parse and verify JWS signature
    jws, err := jose.ParseSigned(string(jwsBytes))
    if err != nil {
        return nil, err
    }
    
    payload, err := jws.Verify(senderPubKey)
    if err != nil {
        return nil, errors.New("signature verification failed")
    }
    
    return payload, nil
}
```

### 2.7 Error Scenarios

#### 2.7.1 Decryption Failures

**Cause:** Wrong receiver private key, corrupted ciphertext, or tampered message

**Response:**
```http
HTTP/1.1 400 Bad Request

{
  "error": {
    "code": "DECRYPTION_FAILED",
    "message": "Unable to decrypt message payload"
  }
}
```

**J-MDN (Negative Acknowledgment):**
```json
{
  "original_message_id": "fdx-uuid-12345",
  "status": "FAILED",
  "error_log": {
    "error_code": "DECRYPTION_FAILED",
    "error_message": "Unable to decrypt payload with provided key"
  }
}
```

#### 2.7.2 Signature Verification Failures

**Cause:** Wrong sender public key, tampered payload, or invalid signature

**Response:**
```http
HTTP/1.1 401 Unauthorized

{
  "error": {
    "code": "SIGNATURE_INVALID",
    "message": "Message signature verification failed"
  }
}
```

**Action:** Reject message, do not send J-MDN (authentication failure)

#### 2.7.3 Unknown Key ID

**Cause:** Key rotation, stale JWKS cache, or wrong `kid`

**Response:**
```http
HTTP/1.1 401 Unauthorized

{
  "error": {
    "code": "UNKNOWN_KEY_ID",
    "message": "Key ID not found in partner JWKS",
    "kid": "sender-sign-2026-02-primary"
  }
}
```

**Recommended Action:** Refresh JWKS from partner's `/.well-known/jwks.json` and retry

## 3. Key Management

Public key cryptography is fundamental to FideX security. This section defines how nodes publish, discover, rotate, and manage RSA key pairs used for message signing and encryption.

### 3.1 JSON Web Key Set (JWKS) Specification

All FideX nodes MUST expose a public JWKS endpoint publishing their public keys. This enables automated key discovery and eliminates manual certificate exchange.

#### 3.1.1 JWKS Endpoint Requirements

**Endpoint URL:** `https://{public_domain}/.well-known/jwks.json`

**Example:** `https://fidex.acmesupply.com/.well-known/jwks.json`

**Requirements:**
- MUST be publicly accessible (no authentication required)
- MUST use HTTPS (HTTP not allowed)
- MUST return `Content-Type: application/json`
- SHOULD include appropriate cache headers
- SHOULD support HTTP/2 for performance
- MUST be available 24/7 (high availability)

**HTTP Response Headers:**
```http
HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: public, max-age=3600
Access-Control-Allow-Origin: *
```

**Cache Recommendations:**
- `max-age=3600` (1 hour) - Balance between freshness and load
- Clients SHOULD respect cache headers
- Clients MUST support cache invalidation on key rotation

#### 3.1.2 JWKS Document Structure

**Complete Example:**

```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "kid": "acme-sign-2026-02-primary",
      "alg": "RS256",
      "n": "xGOr-H7A-PWXsz7d8kN6vLKHY...",
      "e": "AQAB"
    },
    {
      "kty": "RSA",
      "use": "enc",
      "kid": "acme-enc-2026-02-primary",
      "alg": "RSA-OAEP",
      "n": "yHPs-I8B-QYXtz8e9lO7wMLIZ...",
      "e": "AQAB"
    },
    {
      "kty": "RSA",
      "use": "sig",
      "kid": "acme-sign-2026-01-backup",
      "alg": "RS256",
      "n": "zIQt-J9C-RZYuz9f0mP8xNMJa...",
      "e": "AQAB"
    }
  ]
}
```

#### 3.1.3 JWK Field Definitions

**Required Fields:**

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `kty` | string | Key Type | `"RSA"` |
| `use` | string | Public Key Use | `"sig"` or `"enc"` |
| `kid` | string | Key ID (unique identifier) | `"acme-sign-2026-02-primary"` |
| `alg` | string | Algorithm | `"RS256"` or `"RSA-OAEP"` |
| `n` | string | RSA Modulus (base64url encoded) | `"xGOr-H7A..."` |
| `e` | string | RSA Exponent (base64url encoded) | `"AQAB"` (65537) |

**Field Specifications:**

**kty (Key Type):**
- MUST be `"RSA"` for FideX v1.0
- Future versions may support `"EC"` (Elliptic Curve) or `"OKP"` (Ed25519)

**use (Public Key Use):**
- `"sig"` - Used for signature verification (JWS)
- `"enc"` - Used for encryption (JWE)
- A single physical key MAY be listed twice with different `use` values
- Best practice: Use separate keys for signing and encryption

**kid (Key ID):**
- MUST be unique within the JWKS
- SHOULD be descriptive and include purpose, date, and sequence
- Recommended format: `{org}-{purpose}-{date}-{sequence}`
- Examples: `"acme-sign-2026-02-primary"`, `"widget-enc-2026-Q1-001"`

**alg (Algorithm):**
- For signing keys: `"RS256"` (REQUIRED), `"RS384"`, `"RS512"`, `"PS256"`, `"PS384"`, `"PS512"` (OPTIONAL)
- For encryption keys: `"RSA-OAEP"` (REQUIRED), `"RSA-OAEP-256"` (OPTIONAL)

**n (RSA Modulus):**
- Base64url-encoded big-endian representation of RSA modulus
- Minimum length: 2048 bits (4096 bits RECOMMENDED)
- No padding characters

**e (RSA Exponent):**
- Base64url-encoded big-endian representation of RSA public exponent
- Typically `"AQAB"` (decimal 65537)

#### 3.1.4 Optional JWK Fields

**x5c (X.509 Certificate Chain):**
```json
{
  "kty": "RSA",
  "use": "sig",
  "kid": "acme-sign-2026-02-primary",
  "alg": "RS256",
  "n": "xGOr-H7A-PWX...",
  "e": "AQAB",
  "x5c": [
    "MIIC+DCCAeCgAwIBAgIJBIGjYW6hFpn2MA0GCSqGSIb3DQEBBQUAMCMxITAfBgNV...",
    "MIIC+DCCAeCgAwIBAgIJBIGjYW6hFpn2MA0GCSqGSIb3DQEBBQUAMCMxITAfBgNV..."
  ]
}
```

- Array of base64-encoded X.509 certificates
- First certificate MUST contain public key
- Chain MUST be in order (cert → intermediate CA → root CA)
- OPTIONAL for FideX (JOSE signatures sufficient)

**x5t (X.509 Certificate SHA-1 Thumbprint):**
- Base64url-encoded SHA-1 thumbprint of first certificate in `x5c`
- Used for quick certificate lookup
- OPTIONAL

**x5t#S256 (X.509 Certificate SHA-256 Thumbprint):**
- Base64url-encoded SHA-256 thumbprint
- Preferred over `x5t` (SHA-1 deprecated)
- OPTIONAL

### 3.2 Key Lifecycle Management

#### 3.2.1 Key Generation

**Initial Key Generation:**

Nodes MUST generate RSA key pairs before first operation:

```bash
# OpenSSL example
openssl genrsa -out private_key.pem 4096
openssl rsa -in private_key.pem -pubout -out public_key.pem

# Extract public key components for JWKS
openssl rsa -in private_key.pem -noout -modulus
openssl rsa -in private_key.pem -noout -text
```

**Key Generation Requirements:**
- Algorithm: RSA
- Minimum key size: 2048 bits
- Recommended key size: 4096 bits
- Exponent: 65537 (standard)
- Randomness: Cryptographically secure random number generator (CSRNG)
- Environment: Generate in secure, air-gapped environment if possible

**Key Pair Recommendations:**
- Generate separate key pairs for signing and encryption
- Generate backup key pair during initial setup
- Store private keys encrypted at rest
- Never transmit private keys over network

#### 3.2.2 Key Rotation

**Rotation Schedule:**
- Routine rotation: Annually
- Emergency rotation: Immediately upon suspected compromise
- Recommended rotation window: 30 days (overlap period)

**Key Rotation Process:**

**Phase 1: Generate New Key (Day 0)**
```
1. Generate new RSA key pair
2. Assign new kid (e.g., "acme-sign-2026-03-primary")
3. Store private key securely
4. DO NOT publish yet
```

**Phase 2: Publish Both Keys (Day 0 - Day 30)**
```
1. Add new public key to JWKS (alongside old key)
2. Update AS5 configuration if needed
3. Continue signing with OLD key
4. Accept messages signed with EITHER key
5. Notify partners of upcoming key change
```

Example JWKS during transition:
```json
{
  "keys": [
    {
      "kid": "acme-sign-2026-03-primary",
      "use": "sig",
      "alg": "RS256",
      "...": "new key"
    },
    {
      "kid": "acme-sign-2026-02-primary",
      "use": "sig",
      "alg": "RS256",
      "...": "old key (deprecated)"
    }
  ]
}
```

**Phase 3: Switch to New Key (Day 30)**
```
1. Begin signing all NEW messages with NEW key
2. Continue accepting messages signed with EITHER key
3. Keep both keys in JWKS for another 30 days
```

**Phase 4: Remove Old Key (Day 60)**
```
1. Remove old key from JWKS
2. Reject messages signed with old key
3. Archive old private key (for audit/dispute resolution)
4. Retention period: 7 years (compliance requirement)
```

**Automation Recommendations:**
```yaml
# Automated key rotation configuration
key_rotation:
  schedule: "0 0 1 * *"  # First day of month
  advance_notice_days: 30
  overlap_period_days: 60
  notify_partners: true
  auto_publish: true
  auto_switch: false  # Require manual approval
```

#### 3.2.3 Key Revocation

**Immediate Revocation Scenarios:**
- Private key compromise (stolen, exposed, leaked)
- Employee departure with key access
- Security breach or unauthorized access
- Key generation weakness discovered

**Emergency Revocation Process:**

**Step 1: Immediate Actions (Within 1 Hour)**
```
1. Remove compromised key from JWKS immediately
2. Generate and publish new key pair
3. Update all internal systems to use new key
4. Send emergency notifications to all partners
```

**Step 2: Partner Notification (Within 24 Hours)**
```
Subject: URGENT: FideX Key Revocation - Action Required

Dear Partner,

We have revoked our signing key due to a security event.

Revoked Key ID: acme-sign-2026-02-primary
Revoked At: 2026-02-20T14:30:00Z
Reason: Precautionary measure

New Key ID: acme-sign-2026-02-emergency
Effective Immediately: 2026-02-20T15:00:00Z

Action Required:
1. Refresh our JWKS: https://fidex.acmesupply.com/.well-known/jwks.json
2. Reject messages signed with revoked key
3. Accept messages signed with new key

Contact: security@acmesupply.com
```

**Step 3: Forensics and Review (Within 1 Week)**
```
1. Investigate root cause
2. Review access logs and message audit trails
3. Determine if any messages were forged
4. Implement preventive controls
5. Document incident and lessons learned
```

**Revocation Without Compromise:**
For planned key sunset (algorithm upgrade, organizational change):
- Follow standard rotation process (30-60 day overlap)
- No emergency procedures needed

### 3.3 Key Discovery and Caching

#### 3.3.1 JWKS Discovery Process

**Initial Partner Onboarding:**
```
1. Obtain partner's public_domain from AS5 configuration
2. Construct JWKS URL: https://{public_domain}/.well-known/jwks.json
3. Fetch JWKS over HTTPS
4. Validate JSON structure
5. Extract and store public keys
6. Cache JWKS locally
```

**JWKS Fetch Example (Go):**
```go
func fetchJWKS(publicDomain string) (*jose.JSONWebKeySet, error) {
    url := fmt.Sprintf("https://%s/.well-known/jwks.json", publicDomain)
    
    client := &http.Client{
        Timeout: 10 * time.Second,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                MinVersion: tls.VersionTLS13,
            },
        },
    }
    
    resp, err := client.Get(url)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
    }
    
    var jwks jose.JSONWebKeySet
    if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
        return nil, fmt.Errorf("failed to parse JWKS: %w", err)
    }
    
    return &jwks, nil
}
```

#### 3.3.2 JWKS Caching Strategy

**Cache Parameters:**
- **Initial TTL:** 1 hour (from `Cache-Control: max-age`)
- **Minimum TTL:** 5 minutes (even if server says shorter)
- **Maximum TTL:** 24 hours (even if server says longer)
- **Refresh on Miss:** If `kid` not found, immediately refresh JWKS

**Cache Refresh Triggers:**
1. **TTL Expiration:** Automatic background refresh
2. **Unknown kid:** Immediate foreground refresh
3. **Partner Notification:** Manual cache invalidation
4. **Periodic Health Check:** Every 6 hours regardless of TTL

**Cache Implementation (Pseudocode):**
```python
class JWKSCache:
    def __init__(self):
        self.cache = {}  # partner_id -> (jwks, expiry)
        self.lock = threading.Lock()
    
    def get_key(self, partner_id, kid):
        # Check cache
        if partner_id in self.cache:
            jwks, expiry = self.cache[partner_id]
            if time.now() < expiry:
                key = jwks.find_key(kid)
                if key:
                    return key
        
        # Cache miss or expired - refresh
        with self.lock:
            # Double-check after acquiring lock
            if partner_id in self.cache:
                jwks, expiry = self.cache[partner_id]
                if time.now() < expiry:
                    key = jwks.find_key(kid)
                    if key:
                        return key
            
            # Fetch fresh JWKS
            jwks = self.fetch_jwks(partner_id)
            expiry = time.now() + 1_hour
            self.cache[partner_id] = (jwks, expiry)
            
            return jwks.find_key(kid)
```

#### 3.3.3 JWKS Validation

**Validation Checklist:**

Upon fetching JWKS, nodes MUST validate:

1. **Transport Security:**
   - Connection used HTTPS
   - Valid TLS certificate
   - Certificate matches public_domain

2. **Content Type:**
   - HTTP `Content-Type: application/json`
   - Valid JSON structure

3. **JWKS Structure:**
   - Top-level `keys` array present
   - At least one key in array
   - Each key has required fields (`kty`, `use`, `kid`, `alg`, `n`, `e`)

4. **Key Constraints:**
   - `kty` is `"RSA"`
   - `use` is `"sig"` or `"enc"`
   - `kid` is unique within set
   - `n` (modulus) is at least 2048 bits
   - `e` (exponent) is reasonable (typically 65537)

5. **Security Checks:**
   - No duplicate `kid` values
   - Key sizes meet minimum requirements
   - Algorithms are permitted

**Validation Failure Handling:**
```json
{
  "error": {
    "code": "INVALID_JWKS",
    "message": "Partner JWKS validation failed",
    "details": "RSA modulus too small: 1024 bits (minimum 2048)",
    "partner_id": "urn:gln:0614141000012"
  }
}
```

### 3.4 Multiple Key Support

#### 3.4.1 Key Purposes

Nodes SHOULD maintain separate keys for different purposes:

**Signing Keys (`use: "sig"`):**
- Used to sign outbound messages (JWS)
- Private key kept on sending node
- Public key published in JWKS

**Encryption Keys (`use: "enc"`):**
- Used to decrypt inbound messages (JWE)
- Private key kept on receiving node
- Public key published in JWKS

**Separation Benefits:**
- Key compromise limits blast radius
- Different rotation schedules possible
- Compliance with regulatory requirements
- Hardware Security Module (HSM) optimization

#### 3.4.2 Primary and Backup Keys

**Recommended Key Structure:**

```json
{
  "keys": [
    {
      "kid": "acme-sign-2026-02-primary",
      "use": "sig",
      "alg": "RS256",
      "...": "Active signing key"
    },
    {
      "kid": "acme-sign-2026-01-backup",
      "use": "sig",
      "alg": "RS256",
      "...": "Backup signing key (for emergency)"
    },
    {
      "kid": "acme-enc-2026-02-primary",
      "use": "enc",
      "alg": "RSA-OAEP",
      "...": "Active encryption key"
    },
    {
      "kid": "acme-enc-2026-01-backup",
      "use": "enc",
      "alg": "RSA-OAEP",
      "...": "Backup encryption key"
    }
  ]
}
```

**Key Selection Algorithm (Sender):**
```
FOR signing:
    1. Use PRIMARY signing key (most recent kid)
    2. Include kid in JWS header
    
FOR encryption:
    1. Fetch partner's JWKS
    2. Find all keys with use="enc"
    3. Prefer key with most recent kid
    4. Include kid in JWE header
```

**Key Selection Algorithm (Receiver):**
```
FOR decryption:
    1. Extract kid from JWE header
    2. Lookup private key for this kid
    3. If not found, reject message
    4. Decrypt using matched private key
    
FOR signature verification:
    1. Decrypt JWE → get JWS
    2. Extract kid from JWS header
    3. Fetch sender's JWKS
    4. Find public key for this kid
    5. Verify signature
```

### 3.5 Key Storage and Security

#### 3.5.1 Private Key Storage

**File System Storage:**
```bash
# Recommended permissions (Unix/Linux)
chmod 600 private_key.pem
chown fidex-service:fidex-service private_key.pem

# Store in protected directory
/opt/fidex/keys/
├── private_key.pem          (600, encrypted at rest)
├── public_key.pem           (644, can be public)
└── backup_private_key.pem   (600, encrypted at rest)
```

**Environment Variables (Not Recommended for Production):**
```bash
# For development/testing only
export FIDEX_PRIVATE_KEY="$(cat private_key.pem)"
```

**Secrets Management (Recommended for Production):**

**HashiCorp Vault:**
```bash
vault kv put secret/fidex/keys/sign-primary \
    private_key=@private_key.pem \
    kid="acme-sign-2026-02-primary"
```

**AWS Secrets Manager:**
```bash
aws secretsmanager create-secret \
    --name fidex/keys/sign-primary \
    --secret-binary file://private_key.pem
```

**Kubernetes Secrets:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: fidex-signing-key
type: Opaque
data:
  private_key: <base64-encoded-pem>
```

#### 3.5.2 Hardware Security Module (HSM)

For high-security environments, store private keys in HSM:

**Benefits:**
- Private keys never leave HSM
- Tamper-resistant hardware
- FIPS 140-2 Level 2/3 compliance
- Cryptographic operations performed in HSM

**Integration Example (PKCS#11):**
```go
import "github.com/miekg/pkcs11"

func signWithHSM(data []byte, slotID uint, keyLabel string) ([]byte, error) {
    ctx := pkcs11.New("/usr/lib/libpkcs11.so")
    ctx.Initialize()
    defer ctx.Finalize()
    
    session, err := ctx.OpenSession(slotID, pkcs11.CKF_SERIAL_SESSION)
    if err != nil {
        return nil, err
    }
    defer ctx.CloseSession(session)
    
    // Find private key by label
    template := []*pkcs11.Attribute{
        pkcs11.NewAttribute(pkcs11.CKA_LABEL, keyLabel),
        pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
    }
    
    ctx.FindObjectsInit(session, template)
    objs, _, err := ctx.FindObjects(session, 1)
    ctx.FindObjectsFinal(session)
    
    if len(objs) == 0 {
        return nil, errors.New("private key not found in HSM")
    }
    
    // Sign data using HSM
    mechanism := []*pkcs11.Mechanism{
        pkcs11.NewMechanism(pkcs11.CKM_RSA_PKCS, nil),
    }
    
    ctx.SignInit(session, mechanism, objs[0])
    signature, err := ctx.Sign(session, data)
    
    return signature, err
}
```

### 3.6 Key Management Best Practices

#### 3.6.1 Operational Best Practices

1. **Principle of Least Privilege**
   - Limit access to private keys
   - Use service accounts with minimal permissions
   - Audit all key access

2. **Key Separation**
   - Separate signing and encryption keys
   - Different keys for different environments (dev/staging/prod)
   - Consider separate keys per partner for high-security scenarios

3. **Backup and Recovery**
   - Encrypted backups of private keys
   - Secure offline storage
   - Documented recovery procedures
   - Regular recovery drills

4. **Monitoring and Alerting**
   - Log all key usage
   - Alert on unusual key access patterns
   - Monitor JWKS endpoint availability
   - Track key rotation schedules

5. **Documentation**
   - Maintain key inventory (kid, purpose, creation date, rotation date)
   - Document key generation procedures
   - Document emergency revocation procedures
   - Keep audit trail of all key operations

#### 3.6.2 Security Checklist

- [ ] Private keys generated with CSRNG
- [ ] Key size ≥ 2048 bits (4096 recommended)
- [ ] Private keys encrypted at rest
- [ ] Private keys stored in secrets manager or HSM
- [ ] File permissions properly configured (600)
- [ ] Separate keys for signing and encryption
- [ ] Backup keys generated and secured
- [ ] Key rotation schedule defined (annual minimum)
- [ ] Emergency revocation procedures documented
- [ ] Access to private keys logged and monitored
- [ ] JWKS endpoint secured with HTTPS
- [ ] JWKS endpoint has high availability
- [ ] JWKS caching implemented
- [ ] Key compromise incident response plan exists

#### 3.6.3 Common Pitfalls to Avoid

**❌ DO NOT:**
- Store private keys in version control (Git)
- Share private keys via email or chat
- Use same key pair across multiple environments
- Generate keys with insufficient entropy
- Use key sizes < 2048 bits
- Expose private keys in logs or error messages
- Skip key rotation schedule
- Forget to remove revoked keys from JWKS

**✅ DO:**
- Use secrets management systems
- Generate separate key pairs for each environment
- Rotate keys annually
- Monitor JWKS endpoint
- Cache JWKS appropriately
- Validate fetched JWKS before use
- Document all key operations
- Test key rotation procedures regularly

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

FideX introduces a modern, automated partner onboarding workflow that eliminates manual certificate exchange via email. This cryptographically-signed handshake ensures mutual authentication and secure key exchange between trading partners.

### 6.1 Discovery Endpoints

Every FideX node MUST expose the following endpoints:

* **AS5 Configuration Endpoint:** Any HTTPS URL (e.g., `/as5/config`, `/partner/discover`, or `/.well-known/as5-configuration`)
* **JWKS Endpoint:** `/.well-known/jwks.json` (MUST be well-known location)
* **Registration Endpoint:** `/api/v1/register` (MUST accept signed registration requests)

**Note:** The AS5 configuration endpoint is NOT required to be at a well-known location. The URL can be arbitrary and is typically shared via secure channel or QR code.

### 6.2 AS5 Configuration Document

The AS5 Configuration document describes node capabilities, endpoints, and identity:

**Complete Example:**

```json
{
  "fidex_version": "1.0",
  "node_id": "urn:gln:0614141000005",
  "organization_name": "ACME Supply Corp",
  "public_domain": "fidex.acmesupply.com",
  "capabilities": {
    "supported_document_types": ["GS1_ORDER_JSON", "GS1_INVOICE_JSON", "X12_850"],
    "max_message_size_mb": 10,
    "supports_compression": true,
    "supported_protocols": ["http/1.1", "http/2", "http/3"]
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

**Field Descriptions:**

| Field | Required | Description |
|-------|----------|-------------|
| `fidex_version` | YES | Protocol version (e.g., "1.0") |
| `node_id` | YES | Unique URN identifier for this node |
| `organization_name` | YES | Human-readable organization name |
| `public_domain` | YES | Public-facing domain name |
| `capabilities` | NO | Node capabilities and supported features |
| `endpoints` | YES | Required service endpoints |
| `security` | YES | Cryptographic algorithms and requirements |

### 6.3 Discovery URL with Optional Security Token

The AS5 configuration URL MAY include an optional single-use security token as a query parameter:

**URL Format:**
```
https://fidex.acmesupply.com/as5/config?token={security_token}
```

**Examples:**

**Public Discovery (No Token Required):**
```
https://fidex.acmesupply.com/.well-known/as5-configuration
```

**Tokenized Discovery (Token Required):**
```
https://fidex.partner.com/discover/a1b2c3d4-e5f6?token=xyz789abc
```

**Security Token Properties:**
- OPTIONAL (responder decides if required)
- Cryptographically random (UUID v4 or equivalent)
- Single-use (invalidated after first successful fetch)
- Time-limited (recommended: 30 minutes)
- Included in URL query string

**QR Code Sharing:**

Discovery URLs are ideal for QR code sharing:

```
┌─────────────────────────┐
│  ████ ▄▄▄▄▄ █▀█ ▄▄▄▄▄ ██│
│  █ ▄▄ █   █ ██▄ █   █ ██│
│  █ ▀▀ █▄▄▄█ ███ █▄▄▄█ ██│
│  ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄│
│  ▀▄▀ █▀▀█ ▀█▀ ▄▀ ▀▄ █▀▄▀│
│  █▀▄▀█▄▄▀▀▄▀█▄ ▀██▀ ▄█▄█│
│  Encoded URL:              │
│  https://fidex.acme.com/   │
│  discover?token=xyz        │
└─────────────────────────┘
```

User scans QR code → Browser opens URL → Initiator fetches configuration

### 6.4 Partner Discovery Handshake (Revised)

The discovery process consists of a cryptographically-signed mutual exchange:

#### Phase 1: Initiator Fetches Responder Configuration

**Step 1: Obtain Discovery URL**

Initiator obtains responder's AS5 configuration URL via:
- QR code scan
- Email or chat message
- Partner portal
- Direct URL entry

**Step 2: Fetch AS5 Configuration**

```http
GET /as5/config?token=a1b2c3d4-e5f6-g7h8 HTTP/1.1
Host: fidex.partner.com
User-Agent: FideX-Node/1.0
```

**Response:**
```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "fidex_version": "1.0",
  "node_id": "urn:gln:0614141000012",
  "organization_name": "Widget Manufacturing Inc",
  "endpoints": {
    "receive_message": "https://fidex.partner.com/api/v1/receive",
    "register": "https://fidex.partner.com/api/v1/register",
    "jwks": "https://fidex.partner.com/.well-known/jwks.json"
  },
  ...
}
```

**Step 3: Fetch Responder's JWKS**

Initiator fetches public keys from well-known JWKS endpoint:

```http
GET /.well-known/jwks.json HTTP/1.1
Host: fidex.partner.com
```

Initiator now has:
- Responder's `node_id`, `organization_name`, and `public_domain`
- Responder's registration endpoint URL
- Responder's public keys for encryption

#### Phase 2: Initiator Sends Signed Registration Request

**Step 4: Build Registration Request**

Initiator creates a signed registration request:

```json
{
  "fidex_version": "1.0",
  "initiator_node_id": "urn:gln:0614141000005",
  "initiator_as5_config_url": "https://fidex.acmesupply.com/.well-known/as5-configuration",
  "security_token": "a1b2c3d4-e5f6-g7h8",
  "timestamp": "2026-02-20T19:00:00Z"
}
```

**Step 5: Sign Registration Request**

Initiator signs the request with its own private key (JWS):

```
{
  "alg": "RS256",
  "kid": "acme-sign-2026-02-primary"
}
.
{
  "fidex_version": "1.0",
  "initiator_node_id": "urn:gln:0614141000005",
  "initiator_as5_config_url": "https://fidex.acmesupply.com/.well-known/as5-configuration",
  "security_token": "a1b2c3d4-e5f6-g7h8",
  "timestamp": "2026-02-20T19:00:00Z"
}
.
[signature]
```

**Step 6: Send Signed Registration Request**

```http
POST /api/v1/register HTTP/1.1
Host: fidex.partner.com
Content-Type: application/jose
User-Agent: FideX-Node/1.0

eyJhbGciOiJSUzI1NiIsImtpZCI6ImFjbWUtc2lnbi0yMDI2LTAyLXByaW1hcnkifQ.eyJmaWRleF92ZXJzaW9uIjoiMS4wIiwiaW5pdGlhdG9yX25vZGVfaWQiOiJ1cm46Z2xuOjA2MTQxNDEwMDAwMDUiLCJpbml0aWF0b3JfYXM1X2NvbmZpZ191cmwiOiJodHRwczovL2ZpZGV4LmFjbWVzdXBwbHkuY29tLy53ZWxsLWtub3duL2FzNS1jb25maWd1cmF0aW9uIiwic2VjdXJpdHlfdG9rZW4iOiJhMWIyYzNkNC1lNWY2LWc3aDgiLCJ0aW1lc3RhbXAiOiIyMDI2LTAyLTIwVDE5OjAwOjAwWiJ9.dBjftJeZ4CVP...
```

#### Phase 3: Responder Validates and Registers

**Step 7: Validate Security Token**

Responder validates the security token (if provided):
- Token matches expected value
- Token not previously used
- Token not expired

**Step 8: Fetch Initiator's AS5 Configuration**

Responder fetches initiator's configuration from URL in registration request:

```http
GET /.well-known/as5-configuration HTTP/1.1
Host: fidex.acmesupply.com
```

**Step 9: Fetch Initiator's JWKS**

Responder fetches initiator's public keys:

```http
GET /.well-known/jwks.json HTTP/1.1
Host: fidex.acmesupply.com
```

**Step 10: Validate Registration Signature**

Responder verifies JWS signature using initiator's public key:
1. Parse JWS from request body
2. Extract `kid` from JWS header
3. Lookup public key in initiator's JWKS
4. Verify signature using RS256
5. Extract and validate payload

**Signature Validation Pseudocode:**
```python
def validate_registration(jws_compact, initiator_jwks):
    # Parse JWS
    header, payload, signature = parse_jws(jws_compact)
    
    # Find signing key
    kid = header['kid']
    public_key = initiator_jwks.find_key(kid, use='sig')
    
    if not public_key:
        reject("Unknown key ID")
    
    # Verify signature
    if not verify_rs256(header, payload, signature, public_key):
        reject("Invalid signature")
    
    # Validate payload
    registration = decode_json(payload)
    
    if registration['timestamp'] > now() + 15_minutes:
        reject("Timestamp in future")
    
    if registration['timestamp'] < now() - 15_minutes:
        reject("Timestamp too old")
    
    return registration
```

**Step 11: Store Initiator Details**

Responder stores initiator information in database:
```sql
INSERT INTO partners (
    node_id,
    organization_name,
    public_domain,
    as5_config_url,
    jwks_url,
    receive_endpoint,
    public_keys_json,
    status,
    registered_at
) VALUES (
    'urn:gln:0614141000005',
    'ACME Supply Corp',
    'fidex.acmesupply.com',
    'https://fidex.acmesupply.com/.well-known/as5-configuration',
    'https://fidex.acmesupply.com/.well-known/jwks.json',
    'https://fidex.acmesupply.com/api/v1/receive',
    '{"keys": [...]}',
    'ACTIVE',
    '2026-02-20 19:00:05'
);
```

**Step 12: Return Success Response**

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "status": "registered",
  "responder_node_id": "urn:gln:0614141000012",
  "responder_as5_config_url": "https://fidex.partner.com/.well-known/as5-configuration",
  "message": "Successfully registered ACME Supply Corp",
  "timestamp": "2026-02-20T19:00:05Z"
}
```

#### Phase 4: Initiator Stores Responder Details

**Step 13: Process Success Response**

Initiator receives confirmation and stores responder information:

```sql
INSERT INTO partners (
    node_id,
    organization_name,
    public_domain,
    as5_config_url,
    jwks_url,
    receive_endpoint,
    public_keys_json,
    status,
    registered_at
) VALUES (
    'urn:gln:0614141000012',
    'Widget Manufacturing Inc',
    'fidex.partner.com',
    'https://fidex.partner.com/.well-known/as5-configuration',
    'https://fidex.partner.com/.well-known/jwks.json',
    'https://fidex.partner.com/api/v1/receive',
    '{"keys": [...]}',
    'ACTIVE',
    '2026-02-20 19:00:05'
);
```

**Step 14: Mutual Recognition Complete**

Both parties now have:
- ✅ Each other's node identifiers and organization names
- ✅ Each other's public keys for encryption and signature verification
- ✅ Each other's message receiving endpoints
- ✅ Cryptographic proof of identity (signed registration)

**Test messages can begin immediately.**

### 6.5 Registration Request Specification

**Registration Request Payload:**

```json
{
  "fidex_version": "1.0",
  "initiator_node_id": "urn:gln:0614141000005",
  "initiator_as5_config_url": "https://fidex.acmesupply.com/.well-known/as5-configuration",
  "security_token": "a1b2c3d4-e5f6-g7h8",
  "timestamp": "2026-02-20T19:00:00Z",
  "contact_email": "integration@acmesupply.com"
}
```

**Field Descriptions:**

| Field | Required | Description |
|-------|----------|-------------|
| `fidex_version` | YES | Protocol version being used |
| `initiator_node_id` | YES | Initiator's unique node identifier |
| `initiator_as5_config_url` | YES | URL where responder can fetch initiator's AS5 config |
| `security_token` | NO | Single-use token (if required by responder) |
| `timestamp` | YES | ISO 8601 timestamp of registration request |
| `contact_email` | NO | Technical contact for troubleshooting |

**Security Requirements:**
- Request MUST be signed with initiator's private key (JWS)
- Signature MUST use RS256 algorithm
- Timestamp MUST be within ±15 minutes of current time
- Security token (if provided) MUST match responder's expectation

### 6.6 Discovery Error Scenarios

#### 6.6.1 Invalid or Expired Token

**Request:**
```http
GET /as5/config?token=expired-token HTTP/1.1
Host: fidex.partner.com
```

**Response:**
```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": {
    "code": "INVALID_TOKEN",
    "message": "Security token is invalid or expired",
    "timestamp": "2026-02-20T19:00:00Z"
  }
}
```

#### 6.6.2 Invalid Signature

**Response:**
```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": {
    "code": "SIGNATURE_INVALID",
    "message": "Registration request signature verification failed",
    "timestamp": "2026-02-20T19:00:00Z"
  }
}
```

#### 6.6.3 Duplicate Registration

**Response:**
```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "error": {
    "code": "ALREADY_REGISTERED",
    "message": "Partner urn:gln:0614141000005 is already registered",
    "existing_registration_date": "2026-02-15T10:30:00Z"
  }
}
```

#### 6.6.4 Unreachable AS5 Configuration

**Response:**
```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "error": {
    "code": "CONFIG_UNREACHABLE",
    "message": "Unable to fetch initiator AS5 configuration",
    "initiator_url": "https://fidex.acmesupply.com/.well-known/as5-configuration",
    "details": "Connection timeout after 10 seconds"
  }
}
```

### 6.7 Discovery Best Practices

**For Responders (Receiving Registration):**
1. **Validate Token First:** Check security token before fetching remote resources
2. **Rate Limit:** Limit registration attempts (e.g., 10/hour per IP)
3. **Verify Signature:** Always verify JWS signature before trusting payload
4. **Fetch with Timeout:** Use 10-second timeout when fetching initiator's config
5. **Cache JWKS:** Cache initiator's JWKS after successful registration
6. **Log Registrations:** Maintain audit log of all registration attempts
7. **Notify Admins:** Alert administrators of new partner registrations

**For Initiators (Requesting Registration):**
1. **Secure Token Transmission:** Never log or expose security tokens
2. **Verify HTTPS:** Ensure responder uses valid TLS certificate
3. **Handle Errors:** Implement retry with exponential backoff for network errors
4. **Store Credentials:** Securely store responder's details after success
5. **Test Connection:** Send test message immediately after registration
6. **Monitor Status:** Track registration status in dashboard

**Security Considerations:**
- Security tokens prevent unauthorized discovery/enumeration
- Signed registration prevents man-in-the-middle attacks
- Timestamp validation prevents replay attacks
- Mutual JWKS fetching ensures both parties have authentic public keys
- QR codes should be displayed only to authorized personnel

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
