# Project Charter: FideX AS5 Node

## Project Overview
FideX AS5 Node is a production-ready implementation of an edge node for secure B2B message exchange. It enables organizations to send and receive encrypted business documents with trading partners using the FideX AS5 protocol.

## Core Purpose
To provide a secure, reliable, and standards-compliant messaging infrastructure for business-to-business (B2B) communications, particularly focused on supply chain and EDI (Electronic Data Interchange) scenarios.

## Key Objectives

### Primary Goals
1. **Secure Message Exchange**: End-to-end encryption using JWE and digital signatures using JWS
2. **Partner Discovery**: Automated 4-step handshake for secure partner onboarding
3. **Reliability**: Message queue with automatic retry and exponential backoff
4. **Integration**: Support for legacy ERP systems via hot-folder and REST API
5. **Monitoring**: Real-time dashboard for message tracking and partner management

### Technical Goals
1. **Clean Architecture**: Implement hexagonal architecture with SOLID principles
2. **Testability**: Comprehensive unit and integration test coverage
3. **Maintainability**: Clear separation of concerns, no global state
4. **Performance**: Efficient message processing with concurrent operations
5. **Security**: PKI-based authentication, session management, IP allowlisting

## Project Scope

### In Scope
- FideX AS5 protocol implementation (v1.0)
- Partner discovery and registration
- Message encryption/decryption (JWE)
- Digital signatures (JWS)
- Receipt management (J-MDN)
- SQLite-based persistence
- Web dashboard UI
- Internal REST API for ERP integration
- Public REST API for B2B communication
- File system watcher for legacy integration
- Message queue with retry logic

### Out of Scope
- Multi-node clustering
- Real-time replication
- Advanced workflow orchestration
- Document transformation/mapping
- Built-in EDI parsing (delegates to ERP)

## Success Criteria

### Technical
- Zero global mutable state (interfaces and DI only)
- >80% test coverage for critical paths
- All HTTP handlers testable in isolation
- Clean dependency graph (no circular dependencies)
- Graceful shutdown of all services

### Functional
- Successfully complete partner discovery handshake
- Send and receive encrypted messages
- Process async receipts correctly
- Automatic retry on transient failures
- Dashboard displays real-time metrics

## Current Status
**Phase**: Active Development → Refactoring Phase 1
**Version**: Pre-release (moving toward v1.0)
**Focus**: Code quality improvements, eliminating technical debt

## Stakeholders
- **Development Team**: Implementation and testing
- **Security Team**: Cryptographic validation and security review
- **Operations Team**: Deployment and monitoring
- **Business Users**: ERP integration and partner management

## Timeline
- **Phase 1 Refactoring**: Current sprint (foundation improvements)
- **Phase 2 Refactoring**: Next sprint (infrastructure patterns)
- **Phase 3 Refactoring**: Following sprint (polish and cleanup)
- **v1.0 Release**: After Phase 3 completion + security audit
