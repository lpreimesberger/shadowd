# Cryptography in Shadowy

## Overview

Shadowy uses post-quantum cryptography to ensure security against both classical and quantum computer attacks.

## ML-DSA87 Signatures

### What is ML-DSA87?
- **Full Name**: Module-Lattice-Based Digital Signature Algorithm, parameter set 87
- **Standard**: FIPS 204 (NIST Post-Quantum Cryptography Standard)
- **Security**: ~128-bit classical security, quantum-resistant
- **Library**: Cloudflare CIRCL (cryptographically secure implementation)

### Key Properties
- **Public Key Size**: ~1,952 bytes
- **Private Key Size**: ~4,032 bytes
- **Signature Size**: ~3,309 bytes
- **Security Level**: NIST Level 3 (equivalent to AES-192)

### Why ML-DSA87?
1. **Quantum Resistance**: Secure against Shor's algorithm
2. **NIST Standardized**: Official post-quantum standard (2024)
3. **Performance**: Reasonable key/signature sizes for blockchain use
4. **Maturity**: Well-studied lattice-based cryptography

## Address Derivation

### Function
```go
func DeriveAddress(publicKey *mldsa87.PublicKey) Address {
    pkBytes, _ := publicKey.MarshalBinary()
    hash := blake2b.Sum256(pkBytes)
    return Address(hash)
}
```

### Process
1. **Serialize**: Convert ML-DSA87 public key to bytes
2. **Hash**: Apply BLAKE2b-256 hash function
3. **Result**: 32-byte address (64 hex characters)

### Example
```
Public Key: [1952 bytes of ML-DSA87 public key data]
           ↓ MarshalBinary()
Bytes:     [1952 bytes]
           ↓ BLAKE2b-256
Address:   a8b033b8fde716ee88528ff8e17ad7b764fb06e26e7caf92f5d5bb775be13918
```

## Why BLAKE2b-256?

### Advantages
- **Speed**: 2-3x faster than SHA-256
- **Security**: Cryptographically secure (based on ChaCha stream cipher)
- **Post-Quantum**: No known quantum attacks
- **Adoption**: Used in Zcash, IPFS, and other modern systems

### Properties
- **Input**: Any length data
- **Output**: 32 bytes (256 bits)
- **Collision Resistance**: 2^128 operations
- **Preimage Resistance**: 2^256 operations

## Key Generation

### Process
```go
func GenerateKeyPair() (*KeyPair, error) {
    publicKey, privateKey, err := mldsa87.GenerateKey(nil)
    if err != nil {
        return nil, err
    }

    address := DeriveAddress(publicKey)

    return &KeyPair{
        PublicKey:  publicKey,
        PrivateKey: privateKey,
        Address:    address,
    }, nil
}
```

### Security
- **Entropy Source**: Cryptographically secure random number generator
- **Key Space**: ~2^256 possible key pairs
- **Uniqueness**: Address collisions astronomically unlikely

## Transaction Signing

### Signing Process
```go
func (kp *KeyPair) Sign(message []byte) ([]byte, error) {
    return mldsa87.Sign(kp.PrivateKey, message, nil)
}
```

### Verification Process
```go
func VerifySignature(message, signature []byte, publicKey *mldsa87.PublicKey) bool {
    return mldsa87.Verify(publicKey, message, signature, nil)
}
```

### Transaction Hashing
```go
func (tx *Transaction) Hash() ([]byte, error) {
    // Create unsigned copy (exclude signature fields)
    unsignedTx := &Transaction{
        TxType: tx.TxType,
        // ... other fields except signature
    }

    bytes, err := json.Marshal(unsignedTx)
    if err != nil {
        return nil, err
    }

    hash := blake2b.Sum256(bytes)
    return hash[:], nil
}
```

## Security Model

### Address = Hash(PublicKey) Benefits
1. **No Pre-storage**: Don't need to store public keys in blockchain state
2. **Verification**: Can validate signatures by deriving address from embedded public key
3. **Compact**: Addresses are fixed 32-byte size regardless of public key size
4. **Privacy**: Public key only revealed when spending (not when receiving)

### Attack Resistance
- **Quantum Attacks**: ML-DSA87 resists Shor's algorithm
- **Hash Attacks**: BLAKE2b-256 provides strong collision resistance
- **Address Collision**: 2^256 address space makes collisions infeasible
- **Signature Forgery**: Lattice problems are quantum-hard

## Comparison with Classical Systems

| Property | Bitcoin (ECDSA) | Ethereum (ECDSA) | Shadowy (ML-DSA87) |
|----------|----------------|------------------|-------------------|
| **Quantum Safe** | ❌ No | ❌ No | ✅ Yes |
| **Public Key** | 33 bytes | 64 bytes | 1,952 bytes |
| **Signature** | ~72 bytes | ~65 bytes | 3,309 bytes |
| **Address** | 20 bytes | 20 bytes | 32 bytes |
| **Hash Function** | SHA-256 | Keccak-256 | BLAKE2b-256 |

### Trade-offs
- **Size**: Larger keys/signatures for quantum resistance
- **Performance**: Slightly slower signing/verification
- **Future-proof**: Secure against quantum computers
- **Standardized**: NIST-approved algorithm

## Implementation Notes

### Library Choice
- **Cloudflare CIRCL**: Production-ready, well-tested implementation
- **FIPS 204 Compliant**: Follows official NIST standard
- **Active Development**: Regular security updates and optimizations

### Error Handling
```go
// Always check for errors in cryptographic operations
publicKey, privateKey, err := mldsa87.GenerateKey(nil)
if err != nil {
    return nil, fmt.Errorf("failed to generate key pair: %w", err)
}

signature, err := mldsa87.Sign(privateKey, message, nil)
if err != nil {
    return nil, fmt.Errorf("failed to sign message: %w", err)
}
```

### Best Practices
1. **Use secure random sources** for key generation
2. **Always verify signatures** before processing transactions
3. **Handle errors gracefully** in cryptographic operations
4. **Keep private keys secure** and never log them
5. **Use constant-time operations** where possible

## References

- [FIPS 204: Module-Lattice-Based Digital Signature Standard](https://csrc.nist.gov/pubs/fips/204/final)
- [Cloudflare CIRCL Library](https://github.com/cloudflare/circl)
- [BLAKE2 Specification](https://tools.ietf.org/html/rfc7693)
- [Post-Quantum Cryptography FAQ](https://csrc.nist.gov/projects/post-quantum-cryptography/faqs)