# Shadowy Blockchain Cryptography Library

A clean, post-quantum cryptography library for blockchain applications using ML-DSA87 signatures.

## Features

- **Post-Quantum Security**: Uses ML-DSA87 (NIST standardized signature scheme)
- **Address Derivation**: Derives compact 32-byte addresses from public key hashes
- **Transaction Validation**: Validates transactions without pre-storing public keys
- **Context Signatures**: Supports domain separation with context strings
- **Deterministic Keys**: Generate reproducible key pairs from seeds

## Architecture

The library implements a clean address model:

1. **Address = BLAKE2b-256(PublicKey)**
2. **Transactions include the full public key** for validation
3. **Validation**: Verify address matches public key hash, then verify ML-DSA87 signature

This approach:
- Keeps addresses compact (32 bytes vs ~2592 bytes for ML-DSA87 public keys)
- Allows validation without pre-storing public keys
- Maintains security (address collision = finding hash preimage)
- Follows standard blockchain patterns

## Quick Start

```go
import "shadowy/lib"

// Generate key pairs
alice, _ := lib.GenerateKeyPair()
bob, _ := lib.GenerateKeyPair()

// Create and sign transaction
tx := lib.NewTxBuilder().
    From(alice.Address()).
    To(bob.Address()).
    Amount(100).
    Fee(1).
    Nonce(1).
    Build()

tx.Sign(alice)

// Validate transaction
if tx.IsValid() {
    fmt.Println("✓ Transaction is valid!")
}
```

## Key Components

### Cryptographic Types

- `Address` - 32-byte blockchain address (derived from public key)
- `KeyPair` - ML-DSA87 public/private key pair
- `Transaction` - Blockchain transaction with embedded public key

### Core Functions

- `GenerateKeyPair()` - Create new random key pair
- `GenerateKeyPairFromSeed()` - Create deterministic key pair
- `DeriveAddress()` - Derive address from public key
- `VerifySignature()` - Verify ML-DSA87 signature

### Transaction Builder

```go
tx := NewTxBuilder().
    From(senderAddress).
    To(recipientAddress).
    Amount(1000).
    Fee(10).
    Nonce(42).
    Data([]byte("optional data")).
    Build()
```

## Dependencies

- [Cloudflare CIRCL](https://github.com/cloudflare/circl) - ML-DSA87 implementation
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) - BLAKE2b hashing

## Testing

Run the comprehensive test suite:

```bash
cd lib
go test -v
```

## Security Notes

- This library uses experimental post-quantum cryptography
- ML-DSA87 provides NIST security level 5
- Always use proper randomness for key generation in production
- Validate all transactions before processing