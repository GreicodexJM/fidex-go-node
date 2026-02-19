# Discovery Process Implementation and Tests

This directory contains the implementation and comprehensive tests for the FideX AS5 partner discovery and registration process.

## Overview

The discovery process enables two FideX nodes to establish a secure partnership through a 4-step handshake process:

1. **AS5 Configuration Retrieval** - Fetch partner's AS5 configuration document
2. **JWKS Retrieval** - Fetch and cache partner's public keys
3. **Webhook Registration** - Register with partner using security token
4. **Partner Profile Creation** - Store partner profile and validate connection

## Files

### Core Implementation

- `as5_config.go` - AS5 configuration document generation and retrieval
- `tokens.go` - Single-use security token management
- `service.go` - Discovery service orchestrating the handshake process

### Supporting Files

- `discovery_test.go` - Comprehensive test suite covering all scenarios
- `README.md` - This file

## Discovery Process Flow

```
┌─────────────┐                                    ┌─────────────┐
│   Node A    │                                    │   Node B    │
│  (Initiator)│                                    │ (Responder) │
└──────┬──────┘                                    └──────┬──────┘
       │                                                  │
       │ 1. GET /.well-known/as5-configuration            │
       ├─────────────────────────────────────────────────>│
       │                                                  │
       │         AS5 Configuration (JSON)                 │
       │<─────────────────────────────────────────────────┤
       │                                                  │
       │ 2. GET /.well-known/jwks.json                    │
       ├─────────────────────────────────────────────────>│
       │                                                  │
       │              JWKS (JSON)                         │
       │<─────────────────────────────────────────────────┤
       │                                                  │
       │ 3. POST /as5/onboarding/webhook                  │
       │    {node_config + security_token}                │
       ├─────────────────────────────────────────────────>│
       │                                                  │
       │         200 OK {success: true}                   │
       │<─────────────────────────────────────────────────┤
       │                                                  │
       │ 4. Save Partner Profile to Database              │
       │    (Optional: Send test message)                 │
       │                                                  │
```

## Components

### AS5 Configuration

The AS5 configuration document contains all endpoints and capabilities:

```json
{
  "issuer": "urn:gln:node-id",
  "organization_name": "Organization Name",
  "jwks_uri": "https://node.example.com/.well-known/jwks.json",
  "message_endpoint": "https://node.example.com/api/v1/inbound",
  "mdn_receipt_endpoint": "https://node.example.com/api/v1/receipt",
  "algorithms_supported": ["RS256", "RSA-OAEP", "A256GCM"],
  "webhook_registration_endpoint": "https://node.example.com/as5/onboarding/webhook"
}
```

### Security Tokens

- Single-use tokens for webhook registration
- Configurable expiration time (default: 30 minutes)
- Automatically cleaned up after use or expiration
- Thread-safe token storage

### Partner Profile

Partner profiles stored in the database include:

- Partner ID (unique identifier)
- Organization name
- JWKS URL and cached public keys
- Message and receipt endpoints
- Last key refresh timestamp
- Creation and update timestamps

## Test Coverage

The test suite includes 11 comprehensive tests:

### Configuration Tests
- ✅ `TestGenerateAS5Config` - Configuration generation
- ✅ `TestFetchAS5Config` - Remote configuration retrieval
- ✅ `TestFetchAS5ConfigInvalidURL` - Error handling for invalid URLs
- ✅ `TestFetchAS5ConfigInvalidJSON` - Error handling for malformed responses

### Token Management Tests
- ✅ `TestTokenStore` - Token generation and validation
- ✅ `TestTokenStoreExpiration` - Token expiration handling

### Discovery Process Tests
- ✅ `TestCompleteDiscoveryHandshake` - Full 4-step handshake process
- ✅ `TestWebhookRegistrationHandler` - Webhook registration endpoint
- ✅ `TestWebhookRegistrationInvalidToken` - Invalid token rejection

### Database Tests
- ✅ `TestPartnerDatabaseOperations` - CRUD operations
- ✅ `TestPartnerDatabaseConstraints` - Data validation and constraints
- ✅ `TestDiscoveryWithConnectionValidation` - Connection validation with test message

## Running Tests

```bash
# Run all discovery tests
go test -v ./internal/discovery/

# Run with timeout
go test -v ./internal/discovery/ -timeout 30s

# Run specific test
go test -v ./internal/discovery/ -run TestCompleteDiscoveryHandshake

# Run with coverage
go test -v ./internal/discovery/ -cover
```

## Test Results

All tests pass successfully:

```
PASS: TestGenerateAS5Config (0.00s)
PASS: TestFetchAS5Config (0.00s)
PASS: TestFetchAS5ConfigInvalidURL (5.01s)
PASS: TestFetchAS5ConfigInvalidJSON (0.00s)
PASS: TestTokenStore (0.00s)
PASS: TestTokenStoreExpiration (2.01s)
PASS: TestCompleteDiscoveryHandshake (0.41s)
PASS: TestWebhookRegistrationHandler (0.24s)
PASS: TestWebhookRegistrationInvalidToken (0.07s)
PASS: TestPartnerDatabaseOperations (0.12s)
PASS: TestPartnerDatabaseConstraints (0.25s)
PASS: TestDiscoveryWithConnectionValidation (0.27s)

Total: 8.380s
```

## Usage Example

```go
// Create token store
tokenStore := discovery.NewTokenStore()

// Create discovery service
nodeConfig := discovery.NodeConfig{
    NodeID:           "urn:gln:my-company:node-123",
    OrganizationName: "My Company",
    BaseURL:          "https://node.mycompany.com",
}
discoveryService := discovery.NewDiscoveryService(nodeConfig, tokenStore)

// Initiate handshake with remote partner
partner, err := discoveryService.InitiatePartnerHandshake(
    "https://partner.example.com/.well-known/as5-configuration",
)
if err != nil {
    log.Fatalf("Discovery failed: %v", err)
}

log.Printf("Successfully partnered with: %s", partner.Name)
```

## Security Considerations

1. **Token Security**
   - Tokens are single-use only
   - Tokens expire after configured duration
   - Tokens are securely generated using crypto/rand

2. **HTTPS Required**
   - All discovery endpoints should use HTTPS in production
   - Certificate validation is enforced

3. **Input Validation**
   - All configuration fields are validated
   - Partner data is sanitized before storage
   - JWKS format is validated before caching

## Error Handling

The implementation handles various error scenarios:

- Network failures during configuration/JWKS retrieval
- Invalid or malformed JSON responses
- Expired or invalid security tokens
- Missing required configuration fields
- Database constraint violations
- Duplicate partner registrations

## Future Enhancements

- QR code generation for easy partner onboarding
- Automatic key rotation and refresh
- Partner status monitoring
- Connection health checks
- Retry logic for failed handshakes
- Event logging and audit trail
