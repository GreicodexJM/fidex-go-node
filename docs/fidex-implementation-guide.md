# FideX Implementation Guide

**Version:** 1.1  
**Date:** February 23, 2026  
**Audience:** Developers implementing FideX protocol clients or nodes

---

> **Document Status: INFORMATIVE**
>
> This document provides implementation examples and patterns for FideX integrators.
> It is NOT the authoritative specification. See `fidex-protocol-specification.md` for normative requirements.
>
> **Document Hierarchy:**
> - `fidex-protocol-specification.md` — **NORMATIVE** authoritative specification
> - `openapi.yaml` — **NORMATIVE** machine-readable contract
> - `fidex-security-guide.md` — INFORMATIVE security operations guide
> - **This document** — INFORMATIVE implementation examples
> - `fidex-quickstart.md` — INFORMATIVE 5-minute quick start

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Getting Started](#2-getting-started)
3. [Message Encryption and Signing](#3-message-encryption-and-signing)
4. [Message Decryption and Verification](#4-message-decryption-and-verification)
5. [JWKS Management](#5-jwks-management)
6. [Partner Discovery Implementation](#6-partner-discovery-implementation)
7. [HTTP Client Configuration](#7-http-client-configuration)
8. [Error Handling Patterns](#8-error-handling-patterns)
9. [Testing and Debugging](#9-testing-and-debugging)
10. [Production Deployment](#10-production-deployment)

---

## 1. Introduction

This guide provides practical implementation examples for the FideX Protocol in five popular programming languages:
- **JavaScript/Node.js**
- **Go**
- **PHP**
- **Python**
- **Java**

See [FideX Protocol Specification](./fidex-protocol-specification.md) for protocol details.

---

## 2. Getting Started

### 2.1 Prerequisites

Before implementing FideX, ensure you have:
- RSA key pair (2048-bit minimum, 4096-bit recommended)
- TLS certificate for your domain (from trusted CA)
- Understanding of JOSE (JWS/JWE) concepts

### 2.2 Required Libraries

**JavaScript/Node.js**
```bash
npm install jose node-fetch
```
- `jose` - JOSE implementation (https://github.com/panva/jose)

**Go**
```bash
go get github.com/go-jose/go-jose/v4
```
- `go-jose/v4` - Complete JOSE implementation

**PHP**
```bash
composer require web-token/jwt-framework
```
- `web-token/jwt-framework` - JWT Framework for PHP

**Python**
```bash
pip install python-jose cryptography requests
```
- `python-jose` - JOSE implementation
- `cryptography` - Cryptographic primitives

**Java**
```xml
<dependency>
    <groupId>com.nimbusds</groupId>
    <artifactId>nimbus-jose-jwt</artifactId>
    <version>9.37</version>
</dependency>
```
- `nimbus-jose-jwt` - Comprehensive JOSE library

### 2.3 Key Generation

**Using OpenSSL (All Platforms)**
```bash
# Generate 4096-bit RSA private key
openssl genrsa -out private_key.pem 4096

# Extract public key
openssl rsa -in private_key.pem -pubout -out public_key.pem

# View key details
openssl rsa -in private_key.pem -noout -text
```

**Programmatic Generation**

**JavaScript/Node.js**
```javascript
import { generateKeyPair, exportJWK, exportPKCS8, exportSPKI } from 'jose';
import { writeFileSync } from 'fs';

async function generateKeys() {
  const { privateKey, publicKey } = await generateKeyPair('RS256', {
    modulusLength: 4096
  });
  
  // Export as PEM
  const privateKeyPEM = await exportPKCS8(privateKey);
  const publicKeyPEM = await exportSPKI(publicKey);
  
  writeFileSync('private_key.pem', privateKeyPEM);
  writeFileSync('public_key.pem', publicKeyPEM);
  
  console.log('Keys generated successfully');
}

generateKeys();
```

**Go**
```go
package main

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "os"
)

func generateKeys() error {
    // Generate 4096-bit RSA key
    privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
    if err != nil {
        return err
    }
    
    // Save private key
    privateFile, err := os.Create("private_key.pem")
    if err != nil {
        return err
    }
    defer privateFile.Close()
    
    privateBytes := x509.MarshalPKCS1PrivateKey(privateKey)
    pem.Encode(privateFile, &pem.Block{
        Type:  "RSA PRIVATE KEY",
        Bytes: privateBytes,
    })
    
    // Save public key
    publicFile, err := os.Create("public_key.pem")
    if err != nil {
        return err
    }
    defer publicFile.Close()
    
    publicBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
    if err != nil {
        return err
    }
    
    pem.Encode(publicFile, &pem.Block{
        Type:  "PUBLIC KEY",
        Bytes: publicBytes,
    })
    
    return nil
}
```

**Python**
```python
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.backends import default_backend

def generate_keys():
    # Generate 4096-bit RSA key
    private_key = rsa.generate_private_key(
        public_exponent=65537,
        key_size=4096,
        backend=default_backend()
    )
    
    # Save private key
    with open('private_key.pem', 'wb') as f:
        f.write(private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption()
        ))
    
    # Save public key
    public_key = private_key.public_key()
    with open('public_key.pem', 'wb') as f:
        f.write(public_key.public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo
        ))
    
    print('Keys generated successfully')

if __name__ == '__main__':
    generate_keys()
```

---

## 3. Message Encryption and Signing

FideX uses Sign-then-Encrypt: `JWE(JWS(payload))`

### 3.1 Complete Implementation Examples

**JavaScript/Node.js**
```javascript
import { SignJWT, EncryptJWT } from 'jose';
import { readFileSync } from 'fs';
import * as crypto from 'crypto';

async function encryptMessage(payload, senderPrivateKeyPEM, receiverPublicKeyPEM, senderKID, receiverKID) {
  // Load keys
  const senderPrivateKey = crypto.createPrivateKey(senderPrivateKeyPEM);
  const receiverPublicKey = crypto.createPublicKey(receiverPublicKeyPEM);
  
  // Step 1: Sign payload with sender's private key (JWS)
  const jws = await new SignJWT(payload)
    .setProtectedHeader({ alg: 'RS256', kid: senderKID })
    .sign(senderPrivateKey);
  
  // Step 2: Encrypt JWS with receiver's public key (JWE)
  const jwe = await new EncryptJWT({ jws })
    .setProtectedHeader({ 
      alg: 'RSA-OAEP', 
      enc: 'A256GCM',
      kid: receiverKID 
    })
    .encrypt(receiverPublicKey);
  
  return jwe;
}

// Usage
const payload = { order_id: 'PO-123', amount: 1000 };
const senderPrivKey = readFileSync('sender_private_key.pem');
const receiverPubKey = readFileSync('receiver_public_key.pem');

const encrypted = await encryptMessage(
  payload,
  senderPrivKey,
  receiverPubKey,
  'sender-sign-2026-02',
  'receiver-enc-2026-02'
);

console.log('Encrypted payload:', encrypted);
```

**Go**
```go
package main

import (
    "crypto/rsa"
    "crypto/x509"
    "encoding/json"
    "encoding/pem"
    "os"
    
    "github.com/go-jose/go-jose/v4"
)

func encryptMessage(
    payload interface{},
    senderPrivateKey *rsa.PrivateKey,
    receiverPublicKey *rsa.PublicKey,
    senderKID, receiverKID string,
) (string, error) {
    // Step 1: Sign with sender's private key (JWS)
    signingKey := jose.SigningKey{
        Algorithm: jose.RS256,
        Key:       senderPrivateKey,
    }
    
    signer, err := jose.NewSigner(
        signingKey,
        &jose.SignerOptions{
            ExtraHeaders: map[jose.HeaderKey]interface{}{
                "kid": senderKID,
            },
        },
    )
    if err != nil {
        return "", err
    }
    
    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return "", err
    }
    
    jws, err := signer.Sign(payloadBytes)
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
            Key:       receiverPublicKey,
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

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
    keyBytes, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    block, _ := pem.Decode(keyBytes)
    return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
    keyBytes, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    block, _ := pem.Decode(keyBytes)
    pub, err := x509.ParsePKIXPublicKey(block.Bytes)
    if err != nil {
        return nil, err
    }
    
    return pub.(*rsa.PublicKey), nil
}

// Usage
func main() {
    payload := map[string]interface{}{
        "order_id": "PO-123",
        "amount":   1000,
    }
    
    senderPrivKey, _ := loadPrivateKey("sender_private_key.pem")
    receiverPubKey, _ := loadPublicKey("receiver_public_key.pem")
    
    encrypted, err := encryptMessage(
        payload,
        senderPrivKey,
        receiverPubKey,
        "sender-sign-2026-02",
        "receiver-enc-2026-02",
    )
    
    if err != nil {
        panic(err)
    }
    
    println("Encrypted payload:", encrypted)
}
```

**PHP**
```php
<?php
use Jose\Component\Core\AlgorithmManager;
use Jose\Component\Core\JWK;
use Jose\Component\Encryption\Algorithm\KeyEncryption\RSAOAEP;
use Jose\Component\Encryption\Algorithm\ContentEncryption\A256GCM;
use Jose\Component\Encryption\Compression\CompressionMethodManager;
use Jose\Component\Encryption\Compression\Deflate;
use Jose\Component\Encryption\JWEBuilder;
use Jose\Component\Encryption\Serializer\CompactSerializer as JWESerializer;
use Jose\Component\Signature\Algorithm\RS256;
use Jose\Component\Signature\JWSBuilder;
use Jose\Component\Signature\Serializer\CompactSerializer as JWSSerializer;

function encryptMessage(
    array $payload,
    string $senderPrivateKeyPath,
    string $receiverPublicKeyPath,
    string $senderKID,
    string $receiverKID
): string {
    // Load keys
    $senderPrivateKey = JWK::createFromKeyFile($senderPrivateKeyPath);
    $receiverPublicKey = JWK::createFromKeyFile($receiverPublicKeyPath);
    
    // Step 1: Sign with sender's private key (JWS)
    $algorithmManager = new AlgorithmManager([new RS256()]);
    $jwsBuilder = new JWSBuilder($algorithmManager);
    
    $jws = $jwsBuilder
        ->create()
        ->withPayload(json_encode($payload))
        ->addSignature($senderPrivateKey, [
            'alg' => 'RS256',
            'kid' => $senderKID
        ])
        ->build();
    
    $jwsSerializer = new JWSSerializer();
    $jwsCompact = $jwsSerializer->serialize($jws, 0);
    
    // Step 2: Encrypt JWS with receiver's public key (JWE)
    $keyEncryptionAlgorithmManager = new AlgorithmManager([new RSAOAEP()]);
    $contentEncryptionAlgorithmManager = new AlgorithmManager([new A256GCM()]);
    $compressionMethodManager = new CompressionMethodManager([new Deflate()]);
    
    $jweBuilder = new JWEBuilder(
        $keyEncryptionAlgorithmManager,
        $contentEncryptionAlgorithmManager,
        $compressionMethodManager
    );
    
    $jwe = $jweBuilder
        ->create()
        ->withPayload($jwsCompact)
        ->withSharedProtectedHeader([
            'alg' => 'RSA-OAEP',
            'enc' => 'A256GCM',
            'kid' => $receiverKID
        ])
        ->addRecipient($receiverPublicKey)
        ->build();
    
    $jweSerializer = new JWESerializer();
    return $jweSerializer->serialize($jwe, 0);
}

// Usage
$payload = ['order_id' => 'PO-123', 'amount' => 1000];

$encrypted = encryptMessage(
    $payload,
    'sender_private_key.pem',
    'receiver_public_key.pem',
    'sender-sign-2026-02',
    'receiver-enc-2026-02'
);

echo "Encrypted payload: " . $encrypted . PHP_EOL;
```

**Python**
```python
from jose import jws, jwe
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.backends import default_backend
import json

def load_private_key(path):
    with open(path, 'rb') as f:
        return serialization.load_pem_private_key(
            f.read(),
            password=None,
            backend=default_backend()
        )

def load_public_key(path):
    with open(path, 'rb') as f:
        return serialization.load_pem_public_key(
            f.read(),
            backend=default_backend()
        )

def encrypt_message(payload, sender_private_key, receiver_public_key, 
                   sender_kid, receiver_kid):
    # Step 1: Sign with sender's private key (JWS)
    payload_json = json.dumps(payload)
    
    jws_token = jws.sign(
        payload_json,
        sender_private_key,
        algorithm='RS256',
        headers={'kid': sender_kid}
    )
    
    # Step 2: Encrypt JWS with receiver's public key (JWE)
    jwe_token = jwe.encrypt(
        jws_token,
        receiver_public_key,
        algorithm='RSA-OAEP',
        encryption='A256GCM',
        kid=receiver_kid
    )
    
    return jwe_token

# Usage
payload = {'order_id': 'PO-123', 'amount': 1000}

sender_private_key = load_private_key('sender_private_key.pem')
receiver_public_key = load_public_key('receiver_public_key.pem')

encrypted = encrypt_message(
    payload,
    sender_private_key,
    receiver_public_key,
    'sender-sign-2026-02',
    'receiver-enc-2026-02'
)

print(f"Encrypted payload: {encrypted}")
```

**Java**
```java
import com.nimbusds.jose.*;
import com.nimbusds.jose.crypto.*;
import com.nimbusds.jose.jwk.*;
import com.nimbusds.jwt.*;
import java.security.interfaces.*;
import java.nio.file.*;
import java.util.Map;

public class FideXEncryption {
    
    public static String encryptMessage(
        Map<String, Object> payload,
        RSAPrivateKey senderPrivateKey,
        RSAPublicKey receiverPublicKey,
        String senderKID,
        String receiverKID
    ) throws Exception {
        
        // Step 1: Sign with sender's private key (JWS)
        JWSHeader jwsHeader = new JWSHeader.Builder(JWSAlgorithm.RS256)
            .keyID(senderKID)
            .build();
        
        JWTClaimsSet claimsSet = new JWTClaimsSet.Builder()
            .claim("data", payload)
            .build();
        
        SignedJWT signedJWT = new SignedJWT(jwsHeader, claimsSet);
        
        JWSSigner signer = new RSASSASigner(senderPrivateKey);
        signedJWT.sign(signer);
        
        String jwsCompact = signedJWT.serialize();
        
        // Step 2: Encrypt JWS with receiver's public key (JWE)
        JWEHeader jweHeader = new JWEHeader.Builder(
            JWEAlgorithm.RSA_OAEP, 
            EncryptionMethod.A256GCM
        )
        .keyID(receiverKID)
        .build();
        
        JWEObject jweObject = new JWEObject(jweHeader, new Payload(jwsCompact));
        
        JWEEncrypter encrypter = new RSAEncrypter(receiverPublicKey);
        jweObject.encrypt(encrypter);
        
        return jweObject.serialize();
    }
    
    // Usage
    public static void main(String[] args) throws Exception {
        Map<String, Object> payload = Map.of(
            "order_id", "PO-123",
            "amount", 1000
        );
        
        // Load keys (implementation depends on key format)
        RSAPrivateKey senderPrivateKey = loadPrivateKey("sender_private_key.pem");
        RSAPublicKey receiverPublicKey = loadPublicKey("receiver_public_key.pem");
        
        String encrypted = encryptMessage(
            payload,
            senderPrivateKey,
            receiverPublicKey,
            "sender-sign-2026-02",
            "receiver-enc-2026-02"
        );
        
        System.out.println("Encrypted payload: " + encrypted);
    }
    
    // Helper methods for key loading
    private static RSAPrivateKey loadPrivateKey(String path) throws Exception {
        byte[] keyBytes = Files.readAllBytes(Paths.get(path));
        String keyPEM = new String(keyBytes)
            .replace("-----BEGIN RSA PRIVATE KEY-----", "")
            .replace("-----END RSA PRIVATE KEY-----", "")
            .replaceAll("\\s", "");
        
        byte[] decoded = java.util.Base64.getDecoder().decode(keyPEM);
        
        java.security.KeyFactory keyFactory = 
            java.security.KeyFactory.getInstance("RSA");
        java.security.spec.PKCS8EncodedKeySpec keySpec = 
            new java.security.spec.PKCS8EncodedKeySpec(decoded);
        
        return (RSAPrivateKey) keyFactory.generatePrivate(keySpec);
    }
    
    private static RSAPublicKey loadPublicKey(String path) throws Exception {
        byte[] keyBytes = Files.readAllBytes(Paths.get(path));
        String keyPEM = new String(keyBytes)
            .replace("-----BEGIN PUBLIC KEY-----", "")
            .replace("-----END PUBLIC KEY-----", "")
            .replaceAll("\\s", "");
        
        byte[] decoded = java.util.Base64.getDecoder().decode(keyPEM);
        
        java.security.KeyFactory keyFactory = 
            java.security.KeyFactory.getInstance("RSA");
        java.security.spec.X509EncodedKeySpec keySpec = 
            new java.security.spec.X509EncodedKeySpec(decoded);
        
        return (RSAPublicKey) keyFactory.generatePublic(keySpec);
    }
}
```

### 3.2 Creating Complete FideX Message

After encryption, combine with routing header:

```javascript
// JavaScript example
const fidexMessage = {
  routing_header: {
    fidex_version: "1.0",
    message_id: "fdx-" + crypto.randomUUID(),
    sender_id: "urn:gln:1234567890123",
    receiver_id: "urn:gln:9876543210987",
    document_type: "GS1_ORDER_JSON",
    timestamp: new Date().toISOString(),
    receipt_webhook: "https://sender.example.com/receipt"
  },
  encrypted_payload: encrypted // JWE from above
};

// Send via HTTP POST
await fetch('https://partner.example.com/api/v1/receive', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(fidexMessage)
});
```

---

## 4. Message Decryption and Verification

Receiving nodes must decrypt JWE and verify JWS signature.

### 4.1 Complete Implementation Examples

**JavaScript/Node.js**
```javascript
import { jwtDecrypt, jwtVerify } from 'jose';
import * as crypto from 'crypto';

async function decryptAndVerify(jweToken, receiverPrivateKeyPEM, senderPublicKeyPEM) {
  const receiverPrivateKey = crypto.createPrivateKey(receiverPrivateKeyPEM);
  const senderPublicKey = crypto.createPublicKey(senderPublicKeyPEM);
  
  // Step 1: Decrypt JWE with receiver's private key
  const { payload: jwePayload } = await jwtDecrypt(jweToken, receiverPrivateKey);
  
  const jwsToken = jwePayload.jws;
  
  // Step 2: Verify JWS signature with sender's public key
  const { payload } = await jwtVerify(jwsToken, senderPublicKey);
  
  return payload;
}

// Usage
const receivedMessage = {
  routing_header: { /* ... */ },
  encrypted_payload: "eyJhbGc..."
};

const receiverPrivKey = readFileSync('receiver_private_key.pem');
const senderPubKey = readFileSync('sender_public_key.pem');

const decrypted = await decryptAndVerify(
  receivedMessage.encrypted_payload,
  receiverPrivKey,
  senderPubKey
);

console.log('Decrypted payload:', decrypted);
```

**Go**
```go
func decryptAndVerify(
    jweCompact string,
    receiverPrivateKey *rsa.PrivateKey,
    senderPublicKey *rsa.PublicKey,
) ([]byte, error) {
    // Step 1: Decrypt JWE
    jwe, err := jose.ParseEncrypted(jweCompact)
    if err != nil {
        return nil, fmt.Errorf("failed to parse JWE: %w", err)
    }
    
    jwsBytes, err := jwe.Decrypt(receiverPrivateKey)
    if err != nil {
        return nil, fmt.Errorf("decryption failed: %w", err)
    }
    
    // Step 2: Verify JWS signature
    jws, err := jose.ParseSigned(string(jwsBytes))
    if err != nil {
        return nil, fmt.Errorf("failed to parse JWS: %w", err)
    }
    
    payload, err := jws.Verify(senderPublicKey)
    if err != nil {
        return nil, fmt.Errorf("signature verification failed: %w", err)
    }
    
    return payload, nil
}

// Usage
receiverPrivKey, _ := loadPrivateKey("receiver_private_key.pem")
senderPubKey, _ := loadPublicKey("sender_public_key.pem")

decrypted, err := decryptAndVerify(
    receivedMessage.EncryptedPayload,
    receiverPrivKey,
    senderPubKey,
)

if err != nil {
    log.Fatalf("Failed to decrypt: %v", err)
}

var payload map[string]interface{}
json.Unmarshal(decrypted, &payload)
fmt.Printf("Decrypted payload: %+v\n", payload)
```

**PHP**
```php
<?php
use Jose\Component\Encryption\JWEDecrypter;
use Jose\Component\Encryption\Serializer\CompactSerializer as JWESerializer;
use Jose\Component\Signature\JWSVerifier;
use Jose\Component\Signature\Serializer\CompactSerializer as JWSSerializer;

function decryptAndVerify(
    string $jweCompact,
    JWK $receiverPrivateKey,
    JWK $senderPublicKey
): array {
    // Step 1: Decrypt JWE
    $jweSerializer = new JWESerializer();
    $jwe = $jweSerializer->unserialize($jweCompact);
    
    $keyEncryptionAlgorithmManager = new AlgorithmManager([new RSAOAEP()]);
    $contentEncryptionAlgorithmManager = new AlgorithmManager([new A256GCM()]);
    $compressionMethodManager = new CompressionMethodManager([new Deflate()]);
    
    $jweDecrypter = new JWEDecrypter(
        $keyEncryptionAlgorithmManager,
        $contentEncryptionAlgorithmManager,
        $compressionMethodManager
    );
    
    $success = $jweDecrypter->decryptUsingKey($jwe, $receiverPrivateKey, 0);
    
    if (!$success) {
        throw new Exception('Decryption failed');
    }
    
    $jwsCompact = $jwe->getPayload();
    
    // Step 2: Verify JWS signature
    $jwsSerializer = new JWSSerializer();
    $jws = $jwsSerializer->unserialize($jwsCompact);
    
    $algorithmManager = new AlgorithmManager([new RS256()]);
    $jwsVerifier = new JWSVerifier($algorithmManager);
    
    $isValid = $jwsVerifier->verifyWithKey($jws, $senderPublicKey, 0);
    
    if (!$isValid) {
        throw new Exception('Signature verification failed');
    }
    
    return json_decode($jws->getPayload(), true);
}

// Usage
$receiverPrivKey = JWK::createFromKeyFile('receiver_private_key.pem');
$senderPubKey = JWK::createFromKeyFile('sender_public_key.pem');

$decrypted = decryptAndVerify(
    $receivedMessage['encrypted_payload'],
    $receiverPrivKey,
    $senderPubKey
);

print_r($decrypted);
```

**Python**
```python
from jose import jwe, jws
import json

def decrypt_and_verify(jwe_token, receiver_private_key, sender_public_key):
    # Step 1: Decrypt JWE
    jws_token = jwe.decrypt(jwe_token, receiver_private_key)
    
    # Step 2: Verify JWS signature
    payload = jws.verify(
        jws_token,
        sender_public_key,
        algorithms=['RS256']
    )
    
    return json.loads(payload)

# Usage
receiver_private_key = load_private_key('receiver_private_key.pem')
sender_public_key = load_public_key('sender_public_key.pem')

decrypted = decrypt_and_verify(
    received_message['encrypted_payload'],
    receiver_private_key,
    sender_public_key
)

print(f"Decrypted payload: {decrypted}")
```

**Java**
```java
public class FideXDecryption {
    
    public static Map<String, Object> decryptAndVerify(
        String jweCompact,
        RSAPrivateKey receiverPrivateKey,
        RSAPublicKey senderPublicKey
    ) throws Exception {
        
        // Step 1: Decrypt JWE
        JWEObject jweObject = JWEObject.parse(jweCompact);
        
        JWEDecrypter decrypter = new RSADecrypter(receiverPrivateKey);
        jweObject.decrypt(decrypter);
        
        String jwsCompact = jweObject.getPayload().toString();
        
        // Step 2: Verify JWS signature
        SignedJWT signedJWT = SignedJWT.parse(jwsCompact);
        
        JWSVerifier verifier = new RSASSAVerifier(senderPublicKey);
        
        if (!signedJWT.verify(verifier)) {
            throw new Exception("Signature verification failed");
        }
        
        JWTClaimsSet claims = signedJWT.getJWTClaimsSet();
        return (Map<String, Object>) claims.getClaim("data");
    }
    
    // Usage
    public static void main(String[] args) throws Exception {
        RSAPrivateKey receiverPrivKey = loadPrivateKey("receiver_private_key.pem");
        RSAPublicKey senderPubKey = loadPublicKey("sender_public_key.pem");
        
        Map<String, Object> decrypted = decryptAndVerify(
            receivedMessage.get("encrypted_payload"),
            receiverPrivKey,
            senderPubKey
        );
        
        System.out.println("Decrypted payload: " + decrypted);
    }
}
```

### 4.2 Error Handling

Always handle decryption/verification failures gracefully:

```javascript
// JavaScript example with proper error handling
async function processIncomingMessage(fidexMessage) {
  try {
    // Validate routing header
    if (!fidexMessage.routing_header || !fidexMessage.encrypted_payload) {
      return {
        status: 400,
        error: { code: 'INVALID_ROUTING_HEADER', message: 'Missing required fields' }
      };
    }
    
    // Decrypt and verify
    const payload = await decryptAndVerify(
      fidexMessage.encrypted_payload,
      receiverPrivateKey,
      senderPublicKey
    );
    
    // Send positive J-MDN
    await sendJMDN({
      original_message_id: fidexMessage.routing_header.message_id,
      status: 'DELIVERED',
      hash_verification: computeHash(payload),
      timestamp: new Date().toISOString()
    }, fidexMessage.routing_header.receipt_webhook);
    
    return { status: 202, payload };
    
  } catch (error) {
    // Send negative J-MDN
    await sendJMDN({
      original_message_id: fidexMessage.routing_header.message_id,
      status: 'FAILED',
      error_log: {
        error_code: error.message.includes('decrypt') ? 'DECRYPTION_FAILED' : 'SIGNATURE_INVALID',
        error_message: error.message
      },
      timestamp: new Date().toISOString()
    }, fidexMessage.routing_header.receipt_webhook);
    
    return { status: 401, error: { code: 'CRYPTO_ERROR', message: error.message } };
  }
}
```

---

## 5. JWKS Management

### 5.1 Publishing JWKS Endpoint

Expose public keys at `/.well-known/jwks.json`:

**JavaScript/Express**
```javascript
app.get('/.well-known/jwks.json', async (req, res) => {
  const publicKey = await loadPublicKey('public_key.pem');
  const jwk = await exportJWK(publicKey);
  
  res.json({
    keys: [{
      kty: "RSA",
      use: "sig",
      kid: "node-2026-02-primary",
      alg: "RS256",
      n: jwk.n,
      e: jwk.e
    }]
  });
});
```

**Go**
```go
func jwksHandler(w http.ResponseWriter, r *http.Request) {
    publicKey, _ := loadPublicKey("public_key.pem")
    
    jwk := jose.JSONWebKey{
        Key:       publicKey,
        KeyID:     "node-2026-02-primary",
        Algorithm: string(jose.RS256),
        Use:       "sig",
    }
    
    jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}
    
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Cache-Control", "public, max-age=3600")
    json.NewEncoder(w).Encode(jwks)
}
```

### 5.2 Fetching Partner JWKS

**JavaScript**
```javascript
async function fetchPartnerJWKS(publicDomain) {
  const url = `https://${publicDomain}/.well-known/jwks.json`;
  const response = await fetch(url, { timeout: 10000 });
  
  if (!response.ok) {
    throw new Error(`Failed to fetch JWKS: ${response.status}`);
  }
  
  return await response.json();
}
```

## 6. Partner Discovery Implementation

### 6.1 Initiating Discovery

**JavaScript**
```javascript
async function discoverPartner(as5ConfigUrl, securityToken) {
  // Step 1: Fetch AS5 configuration
  const configUrl = securityToken ? 
    `${as5ConfigUrl}?token=${securityToken}` : as5ConfigUrl;
  
  const config = await fetch(configUrl).then(r => r.json());
  
  // Step 2: Fetch partner's JWKS
  const jwks = await fetch(config.endpoints.jwks).then(r => r.json());
  
  // Step 3: Build signed registration request
  const registrationPayload = {
    fidex_version: "1.0",
    initiator_node_id: "urn:gln:1234567890123",
    initiator_as5_config_url: "https://my-node.example.com/.well-known/as5-configuration",
    security_token: securityToken,
    timestamp: new Date().toISOString()
  };
  
  const signedRequest = await signJWT(registrationPayload, myPrivateKey, "my-kid");
  
  // Step 4: Send registration
  const registerResponse = await fetch(config.endpoints.register, {
    method: 'POST',
    headers: { 'Content-Type': 'application/jose' },
    body: signedRequest
  });
  
  if (!registerResponse.ok) {
    throw new Error('Registration failed');
  }
  
  // Step 5: Store partner details
  await storePartner({
    node_id: config.node_id,
    organization_name: config.organization_name,
    receive_endpoint: config.endpoints.receive_message,
    public_keys: jwks.keys
  });
  
  return config;
}
```

### 6.2 Handling Registration Requests

**Go**
```go
func registerHandler(w http.ResponseWriter, r *http.Request) {
    // Read JWS
    body, _ := io.ReadAll(r.Body)
    
    // Parse and verify signature
    jws, _ := jose.ParseSigned(string(body))
    
    // Fetch initiator's JWKS and verify
    var regPayload RegistrationPayload
    json.Unmarshal(payload, &regPayload)
    
    // Validate token
    if !validateToken(regPayload.SecurityToken) {
        http.Error(w, "Invalid token", 401)
        return
    }
    
    // Fetch initiator's config and JWKS
    initiatorConfig := fetchAS5Config(regPayload.InitiatorAS5ConfigURL)
    initiatorJWKS := fetchJWKS(initiatorConfig.Endpoints.JWKS)
    
    // Store partner
    storePartner(initiatorConfig, initiatorJWKS)
    
    // Return success
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "registered",
        "responder_node_id": nodeConfig.NodeID,
        "timestamp": time.Now().Format(time.RFC3339),
    })
}
```

## 7. HTTP Client Configuration

### 7.1 TLS Configuration

**JavaScript/Node.js**
```javascript
import https from 'https';

const httpsAgent = new https.Agent({
  minVersion: 'TLSv1.3',
  maxVersion: 'TLSv1.3',
  rejectUnauthorized: true, // Validate certificates
  keepAlive: true,
  keepAliveMsecs: 60000,
  timeout: 30000
});

// Use with fetch
await fetch(url, { agent: httpsAgent });
```

**Go**
```go
client := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS13,
            CipherSuites: []uint16{
                tls.TLS_AES_256_GCM_SHA384,
                tls.TLS_CHACHA20_POLY1305_SHA256,
            },
        },
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

### 7.2 Retry Logic

**Python**
```python
import requests
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry

def create_fidex_session():
    session = requests.Session()
    
    retry_strategy = Retry(
        total=5,
        backoff_factor=1,
        status_forcelist=[429, 500, 502, 503, 504],
        allowed_methods=["POST"]
    )
    
    adapter = HTTPAdapter(max_retries=retry_strategy)
    session.mount("https://", adapter)
    
    return session

session = create_fidex_session()
response = session.post(url, json=message, timeout=30)
```

## 8. Error Handling Patterns

Proper error handling is critical for FideX implementations. Errors occur at multiple layers and each requires a specific response pattern.

### 8.1 Error Classification

FideX errors fall into three categories with different response strategies:

| Category | HTTP Response | J-MDN Sent? | Retry? | Examples |
|----------|--------------|-------------|--------|----------|
| **Structural** | 4xx immediately | No | Never | Missing routing_header, malformed JSON |
| **Cryptographic** | 202 Accepted | Yes (FAILED) | Never | Wrong key, invalid signature, corrupted JWE |
| **Transient** | 5xx / timeout | No | Yes (backoff) | Server overload, database down, network timeout |

### 8.2 Receiver Error Handling Decision Tree

```
Message received
  │
  ├─ Can parse JSON? ──No──→ HTTP 400 + INVALID_ROUTING_HEADER (stop)
  │
  ├─ Has routing_header + encrypted_payload? ──No──→ HTTP 400 (stop)
  │
  ├─ Is sender_id a known partner? ──No──→ HTTP 401 + UNKNOWN_RECEIVER (stop)
  │
  ├─ Is message_id a duplicate? ──Yes──→ HTTP 202 (idempotent, no re-process)
  │
  ├─ Is timestamp within ±15 min? ──No──→ HTTP 400 + stale timestamp (stop)
  │
  ├─ HTTP 202 Accepted (queued for async processing)
  │
  ├─ Can decrypt JWE? ──No──→ J-MDN FAILED: DECRYPTION_FAILED
  │
  ├─ Can verify JWS? ──No──→ J-MDN FAILED: SIGNATURE_INVALID
  │
  ├─ Is document_type supported? ──No──→ J-MDN FAILED: UNKNOWN_DOCUMENT_TYPE
  │
  └─ Process OK ──→ J-MDN DELIVERED + hash_verification
```

### 8.3 Implementation: Go Error Handler

```go
// FideX error types with categorization
type FideXError struct {
    Code       string // Machine-readable error code
    Message    string // Human-readable description
    HTTPStatus int    // HTTP status code to return
    Retryable  bool   // Whether sender should retry
    SendJMDN   bool   // Whether to send J-MDN for this error
}

var (
    ErrInvalidRoutingHeader = &FideXError{"INVALID_ROUTING_HEADER", "Missing or malformed routing header", 400, false, false}
    ErrUnknownReceiver      = &FideXError{"UNKNOWN_RECEIVER", "Receiver ID not recognized", 401, false, false}
    ErrUnknownSender        = &FideXError{"UNKNOWN_SENDER", "Sender not a registered partner", 401, false, false}
    ErrDuplicateMessage     = &FideXError{"DUPLICATE_MESSAGE", "Message already processed", 202, false, false}
    ErrStaleTimestamp       = &FideXError{"STALE_TIMESTAMP", "Timestamp outside ±15 minute window", 400, false, false}
    ErrPayloadTooLarge      = &FideXError{"PAYLOAD_TOO_LARGE", "Message exceeds size limit", 413, false, false}
    ErrRateLimited          = &FideXError{"RATE_LIMITED", "Rate limit exceeded", 429, true, false}
    ErrDecryptionFailed     = &FideXError{"DECRYPTION_FAILED", "Cannot decrypt payload", 0, false, true}
    ErrSignatureInvalid     = &FideXError{"SIGNATURE_INVALID", "Signature verification failed", 0, false, true}
    ErrUnknownDocType       = &FideXError{"UNKNOWN_DOCUMENT_TYPE", "Document type not supported", 0, false, true}
    ErrInternalError        = &FideXError{"INTERNAL_ERROR", "Internal processing error", 500, true, true}
)

func handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Phase 1: Structural validation (synchronous, pre-202)
    var envelope FideXEnvelope
    if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
        respondError(w, ErrInvalidRoutingHeader)
        return
    }

    if err := validateRoutingHeader(envelope.RoutingHeader); err != nil {
        respondError(w, ErrInvalidRoutingHeader)
        return
    }

    if !isKnownPartner(ctx, envelope.RoutingHeader.SenderID) {
        respondError(w, ErrUnknownSender)
        return
    }

    if isDuplicate(ctx, envelope.RoutingHeader.MessageID) {
        respondAccepted(w, envelope.RoutingHeader.MessageID) // Idempotent
        return
    }

    if !isTimestampValid(envelope.RoutingHeader.Timestamp, 15*time.Minute) {
        respondError(w, ErrStaleTimestamp)
        return
    }

    // Phase 2: Accept and queue (return 202 immediately)
    respondAccepted(w, envelope.RoutingHeader.MessageID)

    // Phase 3: Async processing (in background goroutine)
    go processMessageAsync(ctx, envelope)
}

func processMessageAsync(ctx context.Context, envelope FideXEnvelope) {
    var jmdn JMDN
    jmdn.OriginalMessageID = envelope.RoutingHeader.MessageID
    jmdn.ReceiverID = envelope.RoutingHeader.ReceiverID
    jmdn.Timestamp = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

    // Attempt decrypt
    payload, err := decryptJWE(envelope.EncryptedPayload, myPrivateKey)
    if err != nil {
        jmdn.Status = "FAILED"
        jmdn.HashVerification = "sha256:" + strings.Repeat("0", 64)
        jmdn.ErrorLog = &ErrorLog{Code: "DECRYPTION_FAILED", Message: err.Error()}
        sendJMDNWithRetry(ctx, envelope.RoutingHeader.ReceiptWebhook, jmdn)
        return
    }

    // Attempt verify signature
    businessPayload, err := verifyJWS(payload, partnerPublicKey)
    if err != nil {
        jmdn.Status = "FAILED"
        jmdn.HashVerification = "sha256:" + strings.Repeat("0", 64)
        jmdn.ErrorLog = &ErrorLog{Code: "SIGNATURE_INVALID", Message: err.Error()}
        sendJMDNWithRetry(ctx, envelope.RoutingHeader.ReceiptWebhook, jmdn)
        return
    }

    // Success
    jmdn.Status = "DELIVERED"
    jmdn.HashVerification = computeSHA256(businessPayload)
    jmdn.ErrorLog = nil
    sendJMDNWithRetry(ctx, envelope.RoutingHeader.ReceiptWebhook, jmdn)

    // Deliver to local ERP
    deliverToERP(ctx, businessPayload)
}
```

### 8.4 Implementation: JavaScript Error Handler

```javascript
// Typed FideX errors
class FideXError extends Error {
  constructor(code, message, httpStatus, retryable, sendJMDN) {
    super(message);
    this.code = code;
    this.httpStatus = httpStatus;
    this.retryable = retryable;
    this.sendJMDN = sendJMDN;
  }
}

const ERRORS = {
  INVALID_ROUTING_HEADER: new FideXError('INVALID_ROUTING_HEADER', 'Missing or malformed routing header', 400, false, false),
  UNKNOWN_SENDER:         new FideXError('UNKNOWN_SENDER', 'Sender not registered', 401, false, false),
  STALE_TIMESTAMP:        new FideXError('STALE_TIMESTAMP', 'Timestamp outside window', 400, false, false),
  DECRYPTION_FAILED:      new FideXError('DECRYPTION_FAILED', 'Cannot decrypt payload', null, false, true),
  SIGNATURE_INVALID:      new FideXError('SIGNATURE_INVALID', 'Signature verification failed', null, false, true),
};

async function receiveHandler(req, res) {
  const envelope = req.body;

  // Phase 1: Structural validation
  try {
    validateRoutingHeader(envelope.routing_header);
    await validatePartner(envelope.routing_header.sender_id);
    validateTimestamp(envelope.routing_header.timestamp);
    checkReplayAttack(envelope.routing_header.message_id);
  } catch (err) {
    if (err instanceof FideXError) {
      return res.status(err.httpStatus).json({
        error: { code: err.code, message: err.message, timestamp: new Date().toISOString() }
      });
    }
    return res.status(500).json({ error: { code: 'INTERNAL_ERROR', message: 'Unexpected error' } });
  }

  // Phase 2: Accept
  res.status(202).json({
    status: 'accepted',
    message_id: envelope.routing_header.message_id,
    timestamp: new Date().toISOString()
  });

  // Phase 3: Async processing
  processAsync(envelope).catch(err => {
    console.error('Async processing failed:', err);
  });
}
```

### 8.5 J-MDN Delivery with Retry

```go
// Retry schedule per spec §7.3.6
var jmdnRetryDelays = []time.Duration{
    0,              // Attempt 1: immediate
    1 * time.Minute,  // Attempt 2
    5 * time.Minute,  // Attempt 3
    15 * time.Minute, // Attempt 4
    1 * time.Hour,    // Attempt 5
}

func sendJMDNWithRetry(ctx context.Context, webhookURL string, jmdn JMDN) {
    // Sign J-MDN before sending
    jmdn.Signature = signJMDN(jmdn, myPrivateKey)

    for attempt, delay := range jmdnRetryDelays {
        if delay > 0 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                log.Printf("J-MDN delivery cancelled for %s", jmdn.OriginalMessageID)
                return
            }
        }

        err := postJMDN(ctx, webhookURL, jmdn)
        if err == nil {
            log.Printf("J-MDN delivered for %s (attempt %d)", jmdn.OriginalMessageID, attempt+1)
            return
        }

        log.Printf("J-MDN delivery failed for %s (attempt %d/%d): %v",
            jmdn.OriginalMessageID, attempt+1, len(jmdnRetryDelays), err)
    }

    // All attempts exhausted — store for manual retrieval, NEVER discard
    storeUndeliveredJMDN(jmdn)
    log.Printf("J-MDN stored for manual retrieval: %s", jmdn.OriginalMessageID)
}
```

### 8.6 Sender-Side Error Handling

```go
// Message transmission with retry per spec §7.4
var sendRetryDelays = []time.Duration{
    0, 1 * time.Minute, 5 * time.Minute,
    15 * time.Minute, 30 * time.Minute, 1 * time.Hour,
}

func transmitMessage(ctx context.Context, msg FideXEnvelope, partnerEndpoint string) error {
    for attempt, delay := range sendRetryDelays {
        if delay > 0 {
            time.Sleep(delay)
        }

        statusCode, err := postMessage(ctx, partnerEndpoint, msg)
        if err != nil {
            log.Printf("Transmission error (attempt %d): %v", attempt+1, err)
            continue // Network error → retry
        }

        switch {
        case statusCode == 202:
            updateMessageState(msg.RoutingHeader.MessageID, "SENT")
            return nil // Success — wait for J-MDN

        case statusCode == 429 || statusCode >= 500:
            log.Printf("Retryable HTTP %d (attempt %d)", statusCode, attempt+1)
            continue // Transient error → retry

        case statusCode >= 400 && statusCode < 500:
            updateMessageState(msg.RoutingHeader.MessageID, "FAILED")
            return fmt.Errorf("permanent rejection: HTTP %d", statusCode) // Do NOT retry
        }
    }

    updateMessageState(msg.RoutingHeader.MessageID, "FAILED")
    return fmt.Errorf("max retries exceeded for message %s", msg.RoutingHeader.MessageID)
}
```

### 8.7 Security: Never Leak Sensitive Data in Errors

```go
// WRONG — leaks internal state
respondError(w, fmt.Sprintf("Key %s not found in HSM slot 3", kid))

// CORRECT — generic message, internal details logged only
log.Printf("Key lookup failed: kid=%s, hsm_slot=3, err=%v", kid, err)
respondError(w, &FideXError{Code: "UNKNOWN_KEY_ID", Message: "Key ID not found in JWKS"})
```

**Rules:**
- NEVER include private key material, file paths, or stack traces in HTTP responses
- NEVER include partner identifiers from other partners
- Log detailed diagnostics server-side only
- J-MDN `error_log.details` MUST NOT contain sensitive data (per spec §7.3.4)

---

## 9. Testing and Debugging

### 9.1 Test Message Exchange

**JavaScript Test**
```javascript
import { describe, it, expect } from 'vitest';

describe('FideX Message Exchange', () => {
  it('should encrypt and decrypt message', async () => {
    const payload = { test: 'data' };
    
    // Encrypt
    const encrypted = await encryptMessage(
      payload,
      senderPrivateKey,
      receiverPublicKey,
      'sender-kid',
      'receiver-kid'
    );
    
    expect(encrypted).toBeTruthy();
    
    // Decrypt
    const decrypted = await decryptAndVerify(
      encrypted,
      receiverPrivateKey,
      senderPublicKey
    );
    
    expect(decrypted).toEqual(payload);
  });
  
  it('should reject tampered messages', async () => {
    const encrypted = await encryptMessage(payload, ...);
    const tampered = encrypted + 'x'; // Corrupt
    
    await expect(
      decryptAndVerify(tampered, ...)
    ).rejects.toThrow();
  });
});
```

### 9.2 Debugging Tips

**Enable Verbose Logging:**
```javascript
// JavaScript
import debug from 'debug';
const log = debug('fidex:crypto');

log('Encrypting message:', payload);
log('Using sender key:', senderKID);
```

**Validate JWKS:**
```bash
# Test JWKS endpoint
curl -v https://your-node.example.com/.well-known/jwks.json

# Verify JWS signature online
# https://jwt.io (paste JWS token)
```

**Common Issues:**
- **"Signature verification failed"**: Wrong public key or kid mismatch
- **"Decryption failed"**: Wrong private key or corrupted JWE
- **"Unknown key ID"**: JWKS not cached or key rotation issue
- **TLS errors**: Certificate validation or cipher suite mismatch

## 10. Production Deployment

### 10.1 Security Checklist

- [ ] Private keys stored encrypted (AWS Secrets Manager, HashiCorp Vault)
- [ ] TLS 1.3 enabled with strong cipher suites
- [ ] Certificate from trusted CA (Let's Encrypt, DigiCert)
- [ ] Rate limiting on all endpoints (1000 msg/hour per partner)
- [ ] JWKS endpoint cached with 1-hour TTL
- [ ] Message ID replay detection enabled
- [ ] Timestamp validation (±15 minutes)
- [ ] Monitoring and alerting configured
- [ ] Backup key pairs generated
- [ ] Key rotation schedule defined (annual)

### 10.2 Performance Optimization

**Connection Pooling:**
```javascript
// Reuse connections per partner
const partnerClients = new Map();

function getClientForPartner(partnerId) {
  if (!partnerClients.has(partnerId)) {
    partnerClients.set(partnerId, createHttpsClient());
  }
  return partnerClients.get(partnerId);
}
```

**JWKS Caching:**
```go
type JWKSCache struct {
    cache map[string]*CachedJWKS
    mu    sync.RWMutex
}

type CachedJWKS struct {
    JWKS      *jose.JSONWebKeySet
    ExpiresAt time.Time
}

func (c *JWKSCache) Get(partnerID string) (*jose.JSONWebKeySet, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    cached, ok := c.cache[partnerID]
    if !ok || time.Now().After(cached.ExpiresAt) {
        return nil, false
    }
    
    return cached.JWKS, true
}
```

### 10.3 Monitoring Metrics

Track these metrics:
- Messages sent/received per minute
- Encryption/decryption latency (p50, p95, p99)
- Failed signature verifications
- Partner availability
- Queue depth
- JWKS cache hit ratio

**Prometheus Example:**
```go
var (
    messagesProcessed = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "fidex_messages_processed_total",
            Help: "Total messages processed",
        },
        []string{"direction", "partner_id", "status"},
    )
    
    processingDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "fidex_processing_duration_seconds",
            Help: "Message processing duration",
        },
        []string{"operation"},
    )
)
```

### 10.4 Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o fidex-node ./cmd/fidex-node

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/fidex-node .
COPY --from=builder /app/keys ./keys

EXPOSE 8080 8443
CMD ["./fidex-node"]
```

**docker-compose.yml:**
```yaml
version: '3.8'
services:
  fidex-node:
    image: fidex-node:latest
    ports:
      - "8080:8080"
      - "8443:8443"
    environment:
      - FIDEX_NODE_ID=urn:gln:1234567890123
      - FIDEX_PUBLIC_DOMAIN=node.example.com
    volumes:
      - ./keys:/root/keys:ro
      - ./data:/root/data
    restart: unless-stopped
```

### 10.5 Health Checks

**Endpoint:**
```javascript
app.get('/health', (req, res) => {
  const health = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    checks: {
      database: checkDatabase(),
      jwks: checkJWKS(),
      privateKey: checkPrivateKey()
    }
  };
  
  const allHealthy = Object.values(health.checks).every(c => c.status === 'ok');
  res.status(allHealthy ? 200 : 503).json(health);
});
```

---

## Appendix: Quick Reference

### Message Flow Summary
1. **Sender**: Payload → Sign (JWS) → Encrypt (JWE) → POST to partner
2. **Receiver**: Receive → Decrypt (JWE) → Verify (JWS) → Process → Send J-MDN
3. **Sender**: Receive J-MDN → Verify signature → Update status

### Algorithm Quick Reference
- **Signature**: RS256 (RSA + SHA-256)
- **Key Encryption**: RSA-OAEP
- **Content Encryption**: A256GCM (AES-256-GCM)
- **Key Size**: 2048-bit minimum, 4096-bit recommended

### Standard Error Codes
- `INVALID_ROUTING_HEADER`: Missing/malformed header
- `DECRYPTION_FAILED`: Cannot decrypt JWE
- `SIGNATURE_INVALID`: JWS verification failed
- `UNKNOWN_KEY_ID`: Key not in JWKS
- `INVALID_TOKEN`: Discovery token invalid
- `PAYLOAD_TOO_LARGE`: Exceeds size limit

### Useful Links
- Protocol Spec: `./fidex-protocol-specification.md`
- JOSE RFCs: RFC 7515 (JWS), RFC 7516 (JWE), RFC 7517 (JWK)
- Test JWT: https://jwt.io
- TLS Config: https://ssl-config.mozilla.org

---

*End of FideX Implementation Guide*
