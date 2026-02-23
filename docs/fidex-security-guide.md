# FideX Security Best Practices Guide

**Version:** 1.1  
**Date:** February 23, 2026  
**Audience:** Security Engineers, DevOps, System Administrators

---

> **Document Status: INFORMATIVE**
>
> This document provides operational security best practices for FideX deployments.
> It is NOT the authoritative specification. See `fidex-protocol-specification.md` for normative requirements.
>
> **Document Hierarchy:**
> - `fidex-protocol-specification.md` — **NORMATIVE** authoritative specification
> - `openapi.yaml` — **NORMATIVE** machine-readable contract
> - **This document** — INFORMATIVE security operations guide
> - `fidex-implementation-guide.md` — INFORMATIVE implementation examples
> - `fidex-quickstart.md` — INFORMATIVE 5-minute quick start

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Threat Model and Controls Matrix](#threat-model-and-controls-matrix)
3. [Key Management](#2-key-management)
4. [TLS/HTTPS Configuration](#3-tlshttps-configuration)
5. [Authentication and Authorization](#4-authentication-and-authorization)
6. [Network Security](#5-network-security)
7. [Logging and Monitoring](#6-logging-and-monitoring)
8. [Incident Response](#7-incident-response)
9. [Compliance and Auditing](#8-compliance-and-auditing)
10. [Security Checklist](#9-security-checklist)

---

## 1. Introduction

This guide provides security best practices for deploying and operating FideX nodes in production environments. It covers key management, network security, monitoring, and compliance requirements.

### 1.1 Security Principles

FideX security is built on:
- **Defense in Depth**: Multiple layers of security controls
- **Least Privilege**: Minimal permissions for all components
- **Zero Trust**: Never trust, always verify
- **Separation of Duties**: Different keys for different purposes

### 1.2 Threat Model and Controls Matrix

The following matrix maps each threat to specific FideX controls and where they are implemented:

| # | Threat | Attack Vector | FideX Control | Spec Reference | Layer |
|---|--------|---------------|---------------|----------------|-------|
| T1 | **Man-in-the-Middle** | Network interception | TLS 1.3 transport encryption | §2.1 | Transport |
| T2 | **Eavesdropping** | Passive network sniffing | JWE payload encryption (RSA-OAEP + A256GCM) | §4.2 | Application |
| T3 | **Message Tampering** | Modify message in transit | JWS signature (RS256) + hash verification in J-MDN | §4.1, §7.3.2 | Application |
| T4 | **Replay Attack** | Resend captured message | Unique `message_id` cache + timestamp validation (±15 min) | §9.2 | Application |
| T5 | **Sender Impersonation** | Forge sender identity | JWS signature verified against sender's JWKS public key | §4.1, §5.1 | Application |
| T6 | **Receiver Impersonation** | Fake receiving node | JWE encrypted with receiver's JWKS public key + TLS certificate validation | §4.2, §3.3 | Transport+App |
| T7 | **Repudiation (Sender)** | Deny sending message | JWS signature = non-repudiation of origin (private key possession proof) | §4.1 | Application |
| T8 | **Repudiation (Receiver)** | Deny receiving message | Signed J-MDN = non-repudiation of receipt (receiver's JWS signature) | §7.3.3 | Application |
| T9 | **Key Compromise** | Stolen private key | Key rotation via JWKS, emergency revocation, HSM storage | §5.3, §2.6 | Infrastructure |
| T10 | **Partner Enumeration** | Discover trading partners | Single-use security tokens on discovery URLs, rate limiting | §6.2, §4.3 | Application |
| T11 | **Denial of Service** | Flood endpoints | Rate limiting per partner, connection limits, DDoS protection | §5.2, §4.3 | Infrastructure |
| T12 | **Payload Size Attack** | Oversized messages | 10 MB max message size, Content-Length validation | §8.1 | Application |
| T13 | **Timing Attack** | Side-channel on crypto ops | Constant-time signature verification | §9.2 | Application |
| T14 | **Padding Oracle** | JWE decryption probing | RSA-OAEP (secure padding), generic error responses | §4.2, §9.2 | Application |
| T15 | **Algorithm Downgrade** | Force weak crypto | Prohibited algorithm list, `none` algorithm banned | §4.3 | Application |

**Defense Layers:**

```
┌──────────────────────────────────────────────────────┐
│  Layer 4: Infrastructure                              │
│  DDoS protection, firewall, network segmentation     │
├──────────────────────────────────────────────────────┤
│  Layer 3: Transport                                   │
│  TLS 1.3, certificate validation, HSTS               │
├──────────────────────────────────────────────────────┤
│  Layer 2: Application (FideX Protocol)                │
│  JWS signatures, JWE encryption, J-MDN receipts,     │
│  replay detection, timestamp validation               │
├──────────────────────────────────────────────────────┤
│  Layer 1: Key Management                              │
│  HSM storage, key rotation, JWKS distribution,        │
│  emergency revocation                                 │
└──────────────────────────────────────────────────────┘
```

---

## 2. Key Management

### 2.1 Key Generation

**Requirements:**
- Minimum 2048-bit RSA keys (4096-bit RECOMMENDED)
- Use cryptographically secure random number generator
- Generate keys in secure, air-gapped environment for high-security deployments
- Never reuse keys across different nodes or purposes

**Generation Tools:**
```bash
# Production-grade key generation
openssl genrsa -out private_key.pem 4096

# Verify key strength
openssl rsa -in private_key.pem -text -noout | grep "Private-Key"
```

### 2.2 Key Storage

**Private Key Storage Options:**

**1. Filesystem (Development Only)**
```bash
# Store with restricted permissions
chmod 400 private_key.pem
chown fidex-service:fidex-service private_key.pem
```

**2. Hardware Security Module (HSM) - RECOMMENDED**
- FIPS 140-2 Level 2 or higher
- Supports PKCS#11 interface
- Examples: AWS CloudHSM, Azure Key Vault, Thales Luna HSM

**3. Cloud Key Management Services**
- AWS Secrets Manager
- Azure Key Vault
- Google Cloud KMS
- HashiCorp Vault

**Implementation Example (AWS Secrets Manager):**
```javascript
// Node.js
import { SecretsManagerClient, GetSecretValueCommand } from '@aws-sdk/client-secrets-manager';

async function loadPrivateKey() {
  const client = new SecretsManagerClient({ region: 'us-east-1' });
  const response = await client.send(
    new GetSecretValueCommand({ SecretId: 'fidex/private-key' })
  );
  return response.SecretString;
}
```

**Go Example (HashiCorp Vault):**
```go
import (
    vault "github.com/hashicorp/vault/api"
)

func loadPrivateKey() (string, error) {
    client, _ := vault.NewClient(vault.DefaultConfig())
    secret, err := client.Logical().Read("secret/data/fidex/private-key")
    if err != nil {
        return "", err
    }
    return secret.Data["value"].(string), nil
}
```

### 2.3 Key Rotation

**Rotation Schedule:**
- Production keys: **Annual rotation** (minimum)
- High-security environments: **Quarterly rotation**
- Compromised keys: **Immediate rotation**

**Rotation Process:**

**Phase 1: Generate New Key Pair (Day 0)**
```bash
openssl genrsa -out private_key_2027.pem 4096
openssl rsa -in private_key_2027.pem -pubout -out public_key_2027.pem
```

**Phase 2: Publish Both Keys (Day 1-30)**
```json
{
  "keys": [
    {
      "kid": "node-2026-primary",
      "use": "sig",
      "alg": "RS256",
      "...": "old key"
    },
    {
      "kid": "node-2027-primary",
      "use": "sig",
      "alg": "RS256",
      "...": "new key"
    }
  ]
}
```

**Phase 3: Switch to New Key (Day 31)**
- Begin signing with new key
- Continue accepting messages encrypted with either key

**Phase 4: Deprecate Old Key (Day 60)**
- Remove old key from JWKS
- Notify partners via email

**Automated Rotation Script:**
```bash
#!/bin/bash
# fidex-key-rotation.sh

NEW_KEY="private_key_$(date +%Y).pem"
OLD_JWKS="/var/fidex/jwks.json"

# Generate new key
openssl genrsa -out "$NEW_KEY" 4096

# Add to JWKS (implementation specific)
./add-key-to-jwks.sh "$NEW_KEY"

# Schedule old key removal in 60 days
echo "./remove-old-key.sh" | at now + 60 days

echo "Key rotation initiated. Old key will be removed in 60 days."
```

### 2.4 Key Backup and Recovery

**Backup Strategy:**
```bash
# Encrypted backup
openssl enc -aes-256-cbc -salt \
  -in private_key.pem \
  -out private_key.pem.enc \
  -pass file:backup_password.txt

# Store in multiple locations
aws s3 cp private_key.pem.enc s3://fidex-backups/keys/
```

**Recovery Procedures:**
1. Retrieve encrypted backup from secure storage
2. Decrypt in secure environment
3. Validate key integrity
4. Restore to key management system
5. Update JWKS if key ID changed

### 2.5 HSM Integration

**PKCS#11 Configuration:**
```go
// Go with SoftHSM example
import (
    "github.com/miekg/pkcs11"
)

func initHSM() (*pkcs11.Ctx, error) {
    p := pkcs11.New("/usr/lib/softhsm/libsofthsm2.so")
    
    if err := p.Initialize(); err != nil {
        return nil, err
    }
    
    slots, err := p.GetSlotList(true)
    if err != nil {
        return nil, err
    }
    
    session, err := p.OpenSession(slots[0], pkcs11.CKF_SERIAL_SESSION|pkcs11.CKF_RW_SESSION)
    if err != nil {
        return nil, err
    }
    
    // Login with PIN
    err = p.Login(session, pkcs11.CKU_USER, "1234")
    return p, err
}
```

### 2.6 Key Compromise Response

**If Private Key is Compromised:**

1. **Immediate Actions (Within 1 hour):**
   - Revoke compromised key
   - Generate new key pair
   - Update JWKS with new key
   - Disable message processing temporarily

2. **Notification (Within 4 hours):**
   - Notify all trading partners
   - Provide new public key
   - Document incident

3. **Investigation (Within 24 hours):**
   - Determine scope of compromise
   - Review audit logs
   - Identify affected messages

4. **Remediation (Within 1 week):**
   - Implement additional controls
   - Update security procedures
   - Conduct post-incident review

**Notification Template:**
```
Subject: URGENT: FideX Key Rotation - Security Incident

Dear Partner,

We are notifying you of a security incident requiring immediate key rotation.

OLD Key ID: node-2026-primary (COMPROMISED - DO NOT USE)
NEW Key ID: node-2027-emergency

Action Required:
1. Update your systems with our new public key
2. Download from: https://node.example.com/.well-known/jwks.json
3. Acknowledge receipt of this notification

Timeline:
- Incident detected: 2026-02-20 14:00 UTC
- New key published: 2026-02-20 15:00 UTC
- Old key disabled: 2026-02-20 16:00 UTC

Contact: security@example.com | +1-555-0100 (24/7)
```

---

## 3. TLS/HTTPS Configuration

### 3.1 TLS Version Requirements

**Mandatory:**
- TLS 1.3 (RFC 8446) - PRIMARY
- TLS 1.2 (RFC 5246) - FALLBACK ONLY with PFS

**Prohibited:**
- TLS 1.1 and earlier (deprecated, vulnerable)
- SSL 2.0/3.0 (insecure)

### 3.2 Cipher Suite Configuration

**Recommended TLS 1.3 Cipher Suites:**
```nginx
# Nginx configuration
ssl_protocols TLSv1.3 TLSv1.2;
ssl_ciphers 'TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256:TLS_AES_128_GCM_SHA256:ECDHE-RSA-AES256-GCM-SHA384';
ssl_prefer_server_ciphers on;
ssl_ecdh_curve secp384r1;
```

**Apache Configuration:**
```apache
SSLProtocol -all +TLSv1.3 +TLSv1.2
SSLCipherSuite TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256
SSLHonorCipherOrder on
SSLSessionTickets off
```

**Go Application:**
```go
import "crypto/tls"

tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CipherSuites: []uint16{
        tls.TLS_AES_256_GCM_SHA384,
        tls.TLS_CHACHA20_POLY1305_SHA256,
    },
    PreferServerCipherSuites: true,
}
```

### 3.3 Certificate Management

**Certificate Requirements:**
- Domain validated (DV) minimum, Organization Validated (OV) recommended
- From trusted Certificate Authority (Let's Encrypt, DigiCert, etc.)
- Validity period: 90 days (automated renewal recommended)
- Include complete certificate chain

**Let's Encrypt with Certbot:**
```bash
# Install certbot
apt-get install certbot python3-certbot-nginx

# Obtain certificate
certbot --nginx -d fidex.example.com

# Auto-renewal (cron)
0 0,12 * * * certbot renew --quiet
```

**Certificate Pinning (Optional, High Security):**
```go
// Pin expected certificate public key
expectedSPKI := "sha256/abcd1234..."

tlsConfig := &tls.Config{
    VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
        for _, chain := range verifiedChains {
            spki := sha256.Sum256(chain[0].RawSubjectPublicKeyInfo)
            if base64.StdEncoding.EncodeToString(spki[:]) == expectedSPKI {
                return nil
            }
        }
        return errors.New("certificate pin mismatch")
    },
}
```

### 3.4 HSTS Configuration

**Enable HTTP Strict Transport Security:**
```nginx
# Nginx
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
```

### 3.5 Testing TLS Configuration

```bash
# Test with SSL Labs
# Visit: https://www.ssllabs.com/ssltest/

# Test with testssl.sh
git clone https://github.com/drwetter/testssl.sh
cd testssl.sh
./testssl.sh https://fidex.example.com

# Test cipher suites
nmap --script ssl-enum-ciphers -p 443 fidex.example.com
```

---

## 4. Authentication and Authorization

### 4.1 Message-Level Authentication

**Cryptographic Signatures:**
- All messages MUST be signed with sender's private key
- Signature verification MUST occur before processing
- Failed verification MUST result in immediate rejection

**Implementation:**
```javascript
async function authenticateMessage(fidexMessage, senderPublicKey) {
  try {
    const payload = await decryptAndVerify(
      fidexMessage.encrypted_payload,
      receiverPrivateKey,
      senderPublicKey
    );
    
    // Check sender_id matches certificate
    if (payload.sender_id !== fidexMessage.routing_header.sender_id) {
      throw new Error('Sender ID mismatch');
    }
    
    return payload;
  } catch (error) {
    logSecurityEvent('AUTH_FAILURE', {
      sender: fidexMessage.routing_header.sender_id,
      error: error.message
    });
    throw error;
  }
}
```

### 4.2 Partner Whitelisting

**Maintain Partner Registry:**
```sql
CREATE TABLE partners (
    partner_id VARCHAR(255) PRIMARY KEY,
    organization_name VARCHAR(255),
    public_key_jwks TEXT,
    receive_endpoint VARCHAR(512),
    status ENUM('active', 'suspended', 'revoked'),
    onboarded_at TIMESTAMP,
    last_message_at TIMESTAMP
);

CREATE INDEX idx_partner_status ON partners(status);
```

**Authorization Check:**
```go
func isAuthorizedPartner(partnerID string) (bool, error) {
    var status string
    err := db.QueryRow(
        "SELECT status FROM partners WHERE partner_id = ? AND status = 'active'",
        partnerID,
    ).Scan(&status)
    
    return err == nil && status == "active", err
}
```

### 4.3 Rate Limiting

**Per-Partner Rate Limits:**
```javascript
// Redis-based rate limiting
import Redis from 'ioredis';
const redis = new Redis();

async function checkRateLimit(partnerID) {
  const key = `ratelimit:${partnerID}:${Date.now() / 3600000 | 0}`;
  const count = await redis.incr(key);
  await redis.expire(key, 3600);
  
  const limit = 1000; // messages per hour
  if (count > limit) {
    throw new Error('Rate limit exceeded');
  }
}
```

**Nginx Rate Limiting:**
```nginx
limit_req_zone $binary_remote_addr zone=fidex:10m rate=10r/s;

location /api/v1/receive {
    limit_req zone=fidex burst=20 nodelay;
    proxy_pass http://fidex_backend;
}
```

### 4.4 API Key Authentication (Dashboard/Admin)

**For web UI and admin APIs:**
```javascript
// Generate API key
function generateAPIKey() {
  return crypto.randomBytes(32).toString('hex');
}

// Validate API key
async function validateAPIKey(req, res, next) {
  const apiKey = req.headers['x-api-key'];
  
  if (!apiKey) {
    return res.status(401).json({ error: 'Missing API key' });
  }
  
  const valid = await db.query(
    'SELECT user_id FROM api_keys WHERE key_hash = ? AND expires_at > NOW()',
    [sha256(apiKey)]
  );
  
  if (!valid) {
    return res.status(401).json({ error: 'Invalid API key' });
  }
  
  next();
}
```

---

## 5. Network Security

### 5.1 Firewall Configuration

**iptables Rules:**
```bash
#!/bin/bash
# fidex-firewall.sh

# Flush existing rules
iptables -F

# Default policies
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT ACCEPT

# Allow loopback
iptables -A INPUT -i lo -j ACCEPT

# Allow established connections
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Allow HTTPS (443)
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Allow HTTP (80) for Let's Encrypt
iptables -A INPUT -p tcp --dport 80 -j ACCEPT

# Rate limit new connections
iptables -A INPUT -p tcp --dport 443 -m state --state NEW -m recent --set
iptables -A INPUT -p tcp --dport 443 -m state --state NEW -m recent --update --seconds 60 --hitcount 20 -j DROP

# Log dropped packets
iptables -A INPUT -j LOG --log-prefix "IPTables-Dropped: "

# Save rules
iptables-save > /etc/iptables/rules.v4
```

### 5.2 DDoS Protection

**Cloudflare Configuration:**
```yaml
# cloudflare-waf-rules.yaml
rules:
  - description: "Rate limit API endpoints"
    expression: '(http.request.uri.path eq "/api/v1/receive")'
    action: "challenge"
    ratelimit:
      characteristics: ["ip.src"]
      period: 60
      requests_per_period: 100
      
  - description: "Block suspicious user agents"
    expression: '(http.user_agent contains "curl" or http.user_agent contains "python")'
    action: "block"
```

**Application-Level Protection:**
```go
// Implement connection limits
server := &http.Server{
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      10 * time.Second,
    MaxHeaderBytes:    1 << 20, // 1 MB
    IdleTimeout:       30 * time.Second,
    ReadHeaderTimeout: 5 * time.Second,
}
```

### 5.3 Network Segmentation

**Architecture:**
```
Internet
    |
    v
[Load Balancer] (Public Subnet)
    |
    v
[FideX Nodes] (Private Subnet - 10.0.1.0/24)
    |
    v
[Database] (Data Subnet - 10.0.2.0/24)
```

**AWS Security Groups:**
```terraform
resource "aws_security_group" "fidex_nodes" {
  name = "fidex-nodes"
  vpc_id = aws_vpc.main.id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    security_groups = [aws_security_group.load_balancer.id]
  }

  egress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
```

### 5.4 VPN for Partner Connections (Optional)

**For high-security B2B connections:**
```bash
# WireGuard VPN configuration
[Interface]
PrivateKey = <server-private-key>
Address = 10.10.10.1/24
ListenPort = 51820

[Peer]
# Partner A
PublicKey = <partner-a-public-key>
AllowedIPs = 10.10.10.2/32

[Peer]
# Partner B
PublicKey = <partner-b-public-key>
AllowedIPs = 10.10.10.3/32
```

---

## 6. Logging and Monitoring

### 6.1 Security Event Logging

**What to Log:**
- All authentication attempts (success and failure)
- Message encryption/decryption operations
- Key access and usage
- Partner registration events
- Rate limit violations
- System errors and exceptions
- Configuration changes

**Log Format (JSON):**
```json
{
  "timestamp": "2026-02-20T18:00:00Z",
  "event_type": "AUTH_FAILURE",
  "severity": "WARNING",
  "partner_id": "urn:gln:1234567890123",
  "ip_address": "203.0.113.42",
  "message_id": "fdx-abc123",
  "error": "Signature verification failed",
  "user_agent": "FideXClient/1.0"
}
```

**Implementation:**
```go
import (
    "github.com/sirupsen/logrus"
)

func logSecurityEvent(eventType string, details map[string]interface{}) {
    log := logrus.WithFields(logrus.Fields{
        "event_type": eventType,
        "timestamp": time.Now().UTC(),
    })
    
    for k, v := range details {
        log = log.WithField(k, v)
    }
    
    log.Warn("Security event")
}
```

### 6.2 Audit Trail

**Database Schema:**
```sql
CREATE TABLE audit_log (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_type VARCHAR(50),
    actor VARCHAR(255),
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    action VARCHAR(50),
    details JSON,
    ip_address VARCHAR(45),
    INDEX idx_timestamp (timestamp),
    INDEX idx_actor (actor),
    INDEX idx_event_type (event_type)
);

-- Retention: 7 years for compliance
ALTER TABLE audit_log ADD CONSTRAINT check_retention 
CHECK (timestamp > DATE_SUB(NOW(), INTERVAL 7 YEAR));
```

### 6.3 Metrics and Alerting

**Prometheus Metrics:**
```go
var (
    authFailures = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "fidex_auth_failures_total",
            Help: "Total authentication failures",
        },
        []string{"partner_id", "reason"},
    )
    
    suspiciousActivity = promauto.NewCounter(
        prometheus.CounterOpts{
            Name: "fidex_suspicious_activity_total",
            Help: "Suspicious activity detected",
        },
    )
)
```

**Alert Rules (Prometheus):**
```yaml
groups:
  - name: fidex_security
    rules:
      - alert: HighAuthFailureRate
        expr: rate(fidex_auth_failures_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High authentication failure rate"
          
      - alert: KeyCompromiseIndicator
        expr: fidex_suspicious_activity_total > 0
        labels:
          severity: critical
        annotations:
          summary: "Possible key compromise detected"
          
      - alert: UnusualTrafficPattern
        expr: rate(fidex_messages_processed_total[1h]) > 2 * rate(fidex_messages_processed_total[24h] offset 1h)
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Unusual traffic pattern detected"
```

### 6.4 SIEM Integration

**Splunk Forwarder:**
```conf
# inputs.conf
[monitor:///var/log/fidex/*.log]
disabled = false
index = fidex_security
sourcetype = fidex:json
```

**ELK Stack:**
```yaml
# filebeat.yml
filebeat.inputs:
  - type: log
    enabled: true
    paths:
      - /var/log/fidex/*.log
    json.keys_under_root: true
    
output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "fidex-security-%{+yyyy.MM.dd}"
```

---

## 7. Incident Response

### 7.1 Incident Response Plan

**Severity Levels:**

**P1 - Critical (Response < 1 hour)**
- Private key compromise
- System-wide service outage
- Data breach

**P2 - High (Response < 4 hours)**
- Multiple authentication failures
- DDoS attack
- Vulnerability disclosure

**P3 - Medium (Response < 24 hours)**
- Single partner authentication issues
- Performance degradation

**P4 - Low (Response < 1 week)**
- Configuration issues
- Minor bugs

### 7.2 Incident Response Procedures

**Phase 1: Detection and Analysis**
```bash
# Automated detection script
#!/bin/bash

# Check for auth failures
AUTH_FAILURES=$(grep "AUTH_FAILURE" /var/log/fidex/security.log | wc -l)
if [ $AUTH_FAILURES -gt 100 ]; then
    /usr/local/bin/alert-oncall.sh "High auth failure rate: $AUTH_FAILURES"
fi

# Check for signature failures
SIG_FAILURES=$(grep "SIGNATURE_INVALID" /var/log/fidex/security.log | wc -l)
if [ $SIG_FAILURES -gt 50 ]; then
    /usr/local/bin/alert-oncall.sh "High signature failure rate: $SIG_FAILURES"
fi
```

**Phase 2: Containment**
```bash
# Emergency shutdown script
#!/bin/bash
# emergency-shutdown.sh

echo "EMERGENCY SHUTDOWN INITIATED"
date >> /var/log/fidex/emergency.log

# Stop processing new messages
systemctl stop fidex-node

# Block all traffic (except management)
iptables -I INPUT 1 -p tcp --dport 443 -j DROP

# Notify stakeholders
./notify-incident.sh "EMERGENCY_SHUTDOWN"
```

**Phase 3: Eradication**
- Identify root cause
- Remove malicious code/access
- Patch vulnerabilities
- Rotate compromised keys

**Phase 4: Recovery**
```bash
# Recovery checklist
#!/bin/bash

echo "Starting recovery process..."

# 1. Verify system integrity
/usr/local/bin/verify-checksums.sh

# 2. Restore from clean backup if needed
# tar -xzf /backups/fidex-clean.tar.gz

# 3. Generate new keys if compromised
# ./generate-emergency-keys.sh

# 4. Update all partners
# ./notify-partners-key-change.sh

# 5. Restart services
systemctl start fidex-node

# 6. Monitor closely
tail -f /var/log/fidex/*.log
```

**Phase 5: Post-Incident Review**
- Document timeline
- Root cause analysis
- Update procedures
- Implement preventive measures

### 7.3 Communication Templates

**Internal Notification:**
```
TO: security-team@example.com, devops@example.com
SUBJECT: [P1] SECURITY INCIDENT - Key Compromise Suspected

INCIDENT ID: INC-2026-0220-001
SEVERITY: P1 - Critical
DETECTED: 2026-02-20 14:35 UTC
STATUS: Containment Phase

SUMMARY:
Unusual authentication pattern detected suggesting possible key compromise.

IMPACT:
- Message processing temporarily halted
- 3 partners affected
- No confirmed data breach

ACTIONS TAKEN:
- System isolated
- Logs preserved
- Investigation ongoing

NEXT STEPS:
- Complete forensic analysis (ETA: 2 hours)
- Generate new key pair if confirmed
- Partner notification if required

INCIDENT COMMANDER: Jane Doe (jane@example.com, +1-555-0100)
```

---

## 8. Compliance and Auditing

### 8.1 Regulatory Compliance

**FDA 21 CFR Part 11 (Electronic Records/Signatures):**
- ✅ Non-repudiation via cryptographic signatures
- ✅ Audit trails for all transactions
- ✅ System validation and documentation
- ✅ Access controls and authentication

**GDPR (Data Protection):**
```javascript
// Data retention policy
async function enforceDataRetention() {
  // Delete messages older than required retention
  await db.execute(
    'DELETE FROM messages WHERE created_at < DATE_SUB(NOW(), INTERVAL 7 YEAR)'
  );
  
  // Anonymize audit logs
  await db.execute(
    'UPDATE audit_log SET ip_address = "0.0.0.0" WHERE timestamp < DATE_SUB(NOW(), INTERVAL 3 YEAR)'
  );
}
```

**PCI DSS (if handling payment data):**
- Encrypt all payment data in messages
- Maintain firewall configuration
- Regular vulnerability scanning
- Access control measures

### 8.2 Security Audits

**Annual Security Assessment Checklist:**

- [ ] Penetration testing (external)
- [ ] Vulnerability assessment
- [ ] Code security review
- [ ] Configuration audit
- [ ] Access control review
- [ ] Encryption verification
- [ ] Backup/recovery testing
- [ ] Incident response drill
- [ ] Compliance gap analysis
- [ ] Third-party risk assessment

**Audit Evidence Collection:**
```bash
#!/bin/bash
# collect-audit-evidence.sh

AUDIT_DIR="/tmp/fidex-audit-$(date +%Y%m%d)"
mkdir -p $AUDIT_DIR

# System configuration
cp /etc/fidex/*.conf $AUDIT_DIR/

# Key inventory
./list-active-keys.sh > $AUDIT_DIR/key-inventory.txt

# Access logs (last 90 days)
find /var/log/fidex -name "*.log" -mtime -90 -exec cp {} $AUDIT_DIR/ \;

# Partner registry
mysqldump fidex_db partners > $AUDIT_DIR/partners.sql

# Certificate status
openssl x509 -in /etc/ssl/certs/fidex.crt -text > $AUDIT_DIR/certificate.txt

# Create encrypted archive
tar -czf fidex-audit-$(date +%Y%m%d).tar.gz $AUDIT_DIR
openssl enc -aes-256-cbc -salt -in fidex-audit-$(date +%Y%m%d).tar.gz -out fidex-audit-$(date +%Y%m%d).tar.gz.enc
```

### 8.3 Continuous Compliance Monitoring

**Automated Compliance Checks:**
```python
# compliance-checker.py
import sys

def check_tls_version():
    # Verify TLS 1.3 is enabled
    result = subprocess.run(['openssl', 's_client', '-connect', 'localhost:443', '-tls1_3'], 
                          capture_output=True)
    return 'Protocol  : TLSv1.3' in result.stdout.decode()

def check_key_strength():
    # Verify key is at least 2048 bits
    result = subprocess.run(['openssl', 'rsa', '-in', 'private_key.pem', '-text', '-noout'],
                          capture_output=True)
    return 'Private-Key: (4096 bit)' in result.stdout.decode()

def check_log_retention():
    # Verify logs exist for required period
    oldest_log = subprocess.run(['find', '/var/log/fidex', '-name', '*.log', '-type', 'f', '-printf', '%T+\n'],
                               capture_output=True)
    # Parse and verify >= 7 years
    return True  # Implementation specific

checks = [
    ("TLS 1.3 Enabled", check_tls_version()),
    ("Key Strength >= 2048", check_key_strength()),
    ("Log Retention >= 7 years", check_log_retention()),
]

failed = [name for name, result in checks if not result]
if failed:
    print(f"COMPLIANCE FAILURES: {', '.join(failed)}")
    sys.exit(1)
else:
    print("All compliance checks passed")
```

---

## 9. Security Checklist

### 9.1 Pre-Production Checklist

**Infrastructure:**
- [ ] TLS 1.3 enabled and tested
- [ ] Valid SSL certificate installed
- [ ] Firewall rules configured and tested
- [ ] DDoS protection enabled
- [ ] Network segmentation implemented
- [ ] VPN configured (if required)

**Key Management:**
- [ ] 4096-bit RSA keys generated
- [ ] Private keys stored in HSM or encrypted storage
- [ ] Public keys published via JWKS
- [ ] Key backup procedures documented
- [ ] Key rotation schedule defined
- [ ] Emergency key rotation procedure tested

**Application Security:**
- [ ] All dependencies updated
- [ ] Vulnerability scan completed (no high/critical)
- [ ] Rate limiting configured
- [ ] Partner whitelist populated
- [ ] Replay attack protection enabled
- [ ] Timestamp validation configured (±15 min)
- [ ] Input validation implemented
- [ ] Error messages sanitized (no sensitive data)

**Logging and Monitoring:**
- [ ] Security event logging enabled
- [ ] Audit trail configured
- [ ] Log retention policy implemented (7 years)
- [ ] SIEM integration completed
- [ ] Alerting rules configured
- [ ] On-call rotation established
- [ ] Dashboard created for security metrics

**Operational:**
- [ ] Incident response plan documented
- [ ] Backup and recovery procedures tested
- [ ] Disaster recovery plan validated
- [ ] Security training completed
- [ ] Documentation up to date
- [ ] Runbooks created for common scenarios

### 9.2 Monthly Security Review

- [ ] Review authentication failure logs
- [ ] Audit partner access patterns
- [ ] Verify certificate expiration dates
- [ ] Check for security updates
- [ ] Review and update firewall rules
- [ ] Test backup restoration
- [ ] Verify log integrity
- [ ] Review access control lists

### 9.3 Quarterly Security Tasks

- [ ] Key rotation (if scheduled)
- [ ] Vulnerability assessment
- [ ] Penetration testing
- [ ] Compliance audit
- [ ] Incident response drill
- [ ] Security training refresh
- [ ] Third-party risk assessment
- [ ] Update security documentation

### 9.4 Annual Security Tasks

- [ ] Full security audit
- [ ] Disaster recovery test
- [ ] Review and update security policies
- [ ] Compliance certification renewal
- [ ] Architecture security review
- [ ] Cryptographic algorithm review
- [ ] Update threat model
- [ ] Executive security briefing

---

## Appendix A: Security Tools

**Recommended Tools:**

**Vulnerability Scanning:**
- OWASP ZAP
- Nessus
- Qualys

**Penetration Testing:**
- Metasploit
- Burp Suite
- Kali Linux

**Log Analysis:**
- ELK Stack (Elasticsearch, Logstash, Kibana)
- Splunk
- Graylog

**Network Security:**
- Wireshark
- nmap
- tcpdump

**Container Security:**
- Trivy
- Clair
- Anchore

---

## Appendix B: Security Contacts

**Emergency Contacts:**
- Security Team: security@example.com
- On-Call: +1-555-0100 (24/7)
- Incident Commander: Jane Doe

**Vendor Contacts:**
- HSM Support: hsm-support@vendor.com
- Certificate Authority: support@certauth.com
- Cloud Provider Security: aws-security@amazon.com

**Regulatory Bodies:**
- FDA (if applicable): info@fda.gov
- Data Protection Authority: dpa@example.gov

---

## Appendix C: References

- NIST Cybersecurity Framework: https://www.nist.gov/cyberframework
- OWASP Top 10: https://owasp.org/www-project-top-ten/
- CIS Controls: https://www.cisecurity.org/controls/
- ISO 27001: https://www.iso.org/isoiec-27001-information-security.html
- PCI DSS: https://www.pcisecuritystandards.org/
- GDPR: https://gdpr.eu/

---

*End of FideX Security Best Practices Guide*
