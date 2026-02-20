# Product Context: FideX AS5 Node

## Why This Project Exists

### The Problem
Modern businesses need to exchange sensitive documents (purchase orders, invoices, shipment notifications) with trading partners securely and reliably. Traditional solutions like AS2, AS4, or proprietary EDI networks are:
- **Expensive**: High licensing costs and vendor lock-in
- **Complex**: Difficult to configure and maintain
- **Inflexible**: Hard to integrate with modern cloud-native applications
- **Opaque**: Limited visibility into message status and partner health

### The Solution
FideX AS5 Node provides an **open-source, lightweight, and modern** B2B messaging solution that:
- Uses industry-standard JSON Web Encryption (JWE) and signatures (JWS)
- Implements automated partner discovery (no manual certificate exchange)
- Provides real-time visibility through a web dashboard
- Integrates easily with existing ERP systems via REST API or file system
- Runs as a single binary with minimal dependencies

## Problems It Solves

### 1. Partner Onboarding Friction
**Traditional**: Manual exchange of certificates, IP addresses, and endpoint URLs via email
**FideX AS5**: Automated 4-step handshake using discovery URLs and single-use security tokens

### 2. Message Visibility Gap
**Traditional**: Check logs, database queries, or vendor portal for message status
**FideX AS5**: Real-time dashboard showing message flow, partner status, and system health

### 3. Integration Complexity
**Traditional**: Proprietary APIs, complex adapters, or file format conversions
**FideX AS5**: Clean REST API and hot-folder monitoring for drag-and-drop integration

### 4. Security Concerns
**Traditional**: Shared secrets, manual certificate rotation, unclear key management
**FideX AS5**: Public key infrastructure (PKI) with JWKS, automated key refresh, and secure storage

### 5. Operational Overhead
**Traditional**: Multiple moving parts (message broker, database, application server)
**FideX AS5**: Single Go binary, embedded SQLite, no external dependencies

## How It Should Work

### User Experience: ERP Integration
```
1. ERP system sends POST to /api/v1/transmit with business document
2. Node encrypts with partner's public key and signs with own private key
3. Node delivers encrypted message to partner's endpoint
4. Node receives async receipt (J-MDN) confirming delivery
5. Node calls ERP webhook with delivery confirmation
```

### User Experience: Partner Discovery
```
1. Admin enters partner's discovery URL in dashboard
2. Node fetches partner's AS5 configuration and public keys
3. Node registers with partner using single-use security token
4. Partner appears in dashboard as "Connected"
5. Messages can now be sent to partner immediately
```

### User Experience: Legacy Integration
```
1. User drops XML/JSON file into fidex/outbox/ folder
2. File watcher detects new file
3. Node parses file, determines destination partner
4. Node processes message using standard flow
5. Original file moved to fidex/archive/ with timestamp
```

### User Experience: Dashboard Monitoring
```
1. Admin logs into web dashboard (session-based auth)
2. Dashboard shows real-time metrics via WebSocket
3. Admin sees: messages sent/received, partner status, recent activity
4. Admin can trigger manual partner discovery
5. Admin can manage users, rotate keys, and update settings
```

## User Experience Goals

### For System Administrators
- **Simple Deployment**: Single binary, minimal configuration
- **Easy Monitoring**: Clear dashboard with actionable insights
- **Quick Troubleshooting**: Detailed logs and error messages
- **Low Maintenance**: Automatic retry, self-healing capabilities

### For Developers (ERP Integration)
- **Clear API**: RESTful endpoints with OpenAPI documentation
- **Predictable Behavior**: Consistent error handling and status codes
- **Easy Testing**: Test endpoints and sample payloads provided
- **Flexible Integration**: Support both API and file-based workflows

### For Business Users
- **Transparent Operations**: Know which messages are in flight
- **Partner Management**: Easy to add/remove trading partners
- **Audit Trail**: Complete history of message exchanges
- **Reliability**: Confidence that messages won't be lost

## Key User Journeys

### Journey 1: First-Time Setup
1. Download and run fidex-node binary
2. Note the auto-generated API key (displayed in console)
3. Open dashboard at http://localhost:8080
4. Login with default credentials (admin/fidex-node)
5. Change password and review settings
6. Share public key and discovery URL with first trading partner

### Journey 2: Add Trading Partner
1. Obtain partner's discovery URL (e.g., https://partner.com/.well-known/as5-configuration)
2. Generate security token in dashboard (valid for 30 minutes)
3. Send security token to partner via secure channel
4. Enter partner's discovery URL in dashboard
5. Click "Discover Partner" button
6. System completes 4-step handshake automatically
7. Partner appears as "Connected" in partner list

### Journey 3: Send Business Document
**Option A: REST API**
```bash
curl -X POST http://localhost:8080/api/v1/transmit \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "destination_partner_id": "urn:gln:partner:123",
    "document_type": "GS1_ORDER_JSON",
    "receipt_webhook": "https://erp.mycompany.com/receipt",
    "payload": {...}
  }'
```

**Option B: File Drop**
1. Create JSON file with message payload
2. Save as `order-12345.json` in `fidex/outbox/`
3. File is automatically detected and processed
4. Original file moved to `fidex/archive/order-12345-20260220-103045.json`

### Journey 4: Monitor Message Status
1. Open dashboard and view "Recent Messages" table
2. See message with status: QUEUED → SENT → DELIVERED
3. Click message to view details (timestamps, partner, document type)
4. If failed, see error message and retry schedule

## Success Metrics
- **Time to First Message**: <30 minutes from download to sending first message
- **Partner Onboarding Time**: <5 minutes with automated discovery
- **Message Processing Time**: <2 seconds from API call to queue insertion
- **Uptime**: 99.9% availability with automatic recovery
- **User Satisfaction**: Clear understanding of message status at all times
