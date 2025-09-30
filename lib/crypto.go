package lib

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

// KeyPair represents an ML-DSA87 key pair
type KeyPair struct {
	PublicKey  *mldsa87.PublicKey
	PrivateKey *mldsa87.PrivateKey
}

// GenerateKeyPair creates a new ML-DSA87 key pair
func GenerateKeyPair() (*KeyPair, error) {
	pk, sk, err := mldsa87.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ML-DSA87 key generation failed: %w", err)
	}

	return &KeyPair{
		PublicKey:  pk,
		PrivateKey: sk,
	}, nil
}

// GenerateKeyPairFromSeed creates a deterministic ML-DSA87 key pair from a seed
func GenerateKeyPairFromSeed(seed [mldsa87.SeedSize]byte) *KeyPair {
	pk, sk := mldsa87.NewKeyFromSeed(&seed)

	return &KeyPair{
		PublicKey:  pk,
		PrivateKey: sk,
	}
}

// Sign creates an ML-DSA87 signature for the given message
func (kp *KeyPair) Sign(message []byte) ([]byte, error) {
	if len(message) == 0 {
		return nil, errors.New("cannot sign empty message")
	}

	signature := make([]byte, mldsa87.SignatureSize)
	mldsa87.SignTo(kp.PrivateKey, message, nil, false, signature)

	return signature, nil
}

// SignWithContext creates an ML-DSA87 signature with context for the given message
func (kp *KeyPair) SignWithContext(message, context []byte) ([]byte, error) {
	if len(message) == 0 {
		return nil, errors.New("cannot sign empty message")
	}
	if len(context) > 255 {
		return nil, errors.New("context too long (max 255 bytes)")
	}

	signature := make([]byte, mldsa87.SignatureSize)
	mldsa87.SignTo(kp.PrivateKey, message, context, false, signature)

	return signature, nil
}

// VerifySignature verifies an ML-DSA87 signature
func VerifySignature(message, signature []byte, publicKey *mldsa87.PublicKey) bool {
	if len(message) == 0 || len(signature) == 0 || publicKey == nil {
		return false
	}

	return mldsa87.Verify(publicKey, message, nil, signature)
}

// VerifySignatureWithContext verifies an ML-DSA87 signature with context
func VerifySignatureWithContext(message, context, signature []byte, publicKey *mldsa87.PublicKey) bool {
	if len(message) == 0 || len(signature) == 0 || publicKey == nil {
		return false
	}
	if len(context) > 255 {
		return false
	}

	return mldsa87.Verify(publicKey, message, context, signature)
}

// Address returns the address derived from this key pair's public key
func (kp *KeyPair) Address() Address {
	return DeriveAddress(kp.PublicKey)
}

// PublicKeyFromBytes creates a public key from bytes
func PublicKeyFromBytes(bytes []byte) (*mldsa87.PublicKey, error) {
	if len(bytes) != mldsa87.PublicKeySize {
		return nil, fmt.Errorf("ML-DSA87 public key must be %d bytes, got %d",
			mldsa87.PublicKeySize, len(bytes))
	}

	var pk mldsa87.PublicKey
	if err := pk.UnmarshalBinary(bytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal public key: %w", err)
	}

	return &pk, nil
}

// PublicKeyToBytes converts a public key to bytes
func PublicKeyToBytes(pk *mldsa87.PublicKey) ([]byte, error) {
	return pk.MarshalBinary()
}

// ParseSignature converts a hex string to a signature
func ParseSignature(hexStr string) ([]byte, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}

	if len(bytes) == 0 || len(bytes) > mldsa87.SignatureSize {
		return nil, fmt.Errorf("ML-DSA87 signature must be 1-%d bytes, got %d",
			mldsa87.SignatureSize, len(bytes))
	}

	return bytes, nil
}

// GetCurrentTimestamp returns the current Unix timestamp
func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}
