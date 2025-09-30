package lib

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	if kp == nil {
		t.Fatal("Generated key pair is nil")
	}

	if kp.PublicKey == nil {
		t.Fatal("Public key is nil")
	}

	if kp.PrivateKey == nil {
		t.Fatal("Private key is nil")
	}
}

func TestGenerateKeyPairFromSeed(t *testing.T) {
	// Create deterministic seed
	var seed [mldsa87.SeedSize]byte
	copy(seed[:], "test_seed_for_deterministic_key_generation_")

	kp1 := GenerateKeyPairFromSeed(seed)
	kp2 := GenerateKeyPairFromSeed(seed)

	// Should generate identical key pairs
	pk1Bytes, err1 := PublicKeyToBytes(kp1.PublicKey)
	pk2Bytes, err2 := PublicKeyToBytes(kp2.PublicKey)

	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to serialize public keys: %v, %v", err1, err2)
	}

	if len(pk1Bytes) != len(pk2Bytes) {
		t.Fatal("Public key sizes differ")
	}

	for i := range pk1Bytes {
		if pk1Bytes[i] != pk2Bytes[i] {
			t.Fatal("Deterministic key generation failed - keys differ")
		}
	}
}

func TestDeriveAddressConsistency(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr1 := DeriveAddress(kp.PublicKey)
	addr2 := kp.Address()

	// Should be the same
	if addr1 != addr2 {
		t.Fatal("Address derivation methods produce different results")
	}

	// Address should be 32 bytes
	if len(addr1) != 32 {
		t.Fatalf("Address should be 32 bytes, got %d", len(addr1))
	}

	// Should be deterministic
	addr3 := DeriveAddress(kp.PublicKey)
	if addr1 != addr3 {
		t.Fatal("Address derivation is not deterministic")
	}
}

func TestSignAndVerify(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	message := []byte("Hello, post-quantum blockchain!")

	// Sign message
	signature, err := kp.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	if len(signature) == 0 {
		t.Fatal("Signature is empty")
	}

	// Verify signature
	if !VerifySignature(message, signature, kp.PublicKey) {
		t.Fatal("Signature verification failed")
	}

	// Verify with wrong message should fail
	wrongMessage := []byte("Wrong message")
	if VerifySignature(wrongMessage, signature, kp.PublicKey) {
		t.Fatal("Signature verification should fail with wrong message")
	}

	// Verify with wrong public key should fail
	wrongKp, _ := GenerateKeyPair()
	if VerifySignature(message, signature, wrongKp.PublicKey) {
		t.Fatal("Signature verification should fail with wrong public key")
	}
}

func TestSignWithContext(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	message := []byte("Context test message")
	context := []byte("blockchain_v1")

	// Sign with context
	signature, err := kp.SignWithContext(message, context)
	if err != nil {
		t.Fatalf("Failed to sign with context: %v", err)
	}

	// Verify with context
	if !VerifySignatureWithContext(message, context, signature, kp.PublicKey) {
		t.Fatal("Context signature verification failed")
	}

	// Verify with wrong context should fail
	wrongContext := []byte("wrong_context")
	if VerifySignatureWithContext(message, wrongContext, signature, kp.PublicKey) {
		t.Fatal("Context signature verification should fail with wrong context")
	}

	// Verify without context should fail
	if VerifySignature(message, signature, kp.PublicKey) {
		t.Fatal("Context signature should not verify without context")
	}
}

func TestSignWithLongContext(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	message := []byte("Test message")
	longContext := make([]byte, 256) // Too long, max is 255
	_, err = rand.Read(longContext)
	if err != nil {
		t.Fatalf("Failed to generate random context: %v", err)
	}

	// Should fail with context too long
	_, err = kp.SignWithContext(message, longContext)
	if err == nil {
		t.Fatal("Should fail when context is too long")
	}
}

func TestEmptyMessageSigning(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Should fail to sign empty message
	_, err = kp.Sign([]byte{})
	if err == nil {
		t.Fatal("Should fail to sign empty message")
	}

	_, err = kp.Sign(nil)
	if err == nil {
		t.Fatal("Should fail to sign nil message")
	}
}

func TestPublicKeySerializationRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Serialize public key
	pkBytes, err := PublicKeyToBytes(kp.PublicKey)
	if err != nil {
		t.Fatalf("Failed to serialize public key: %v", err)
	}

	// Deserialize public key
	pkReconstructed, err := PublicKeyFromBytes(pkBytes)
	if err != nil {
		t.Fatalf("Failed to deserialize public key: %v", err)
	}

	// Should be able to verify signature with reconstructed key
	message := []byte("Test message for serialization")
	signature, err := kp.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	if !VerifySignature(message, signature, pkReconstructed) {
		t.Fatal("Signature verification failed with reconstructed public key")
	}

	// Addresses should match
	originalAddr := DeriveAddress(kp.PublicKey)
	reconstructedAddr := DeriveAddress(pkReconstructed)

	if originalAddr != reconstructedAddr {
		t.Fatal("Addresses differ after public key serialization round trip")
	}
}

func TestAddressParsing(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := kp.Address()
	hexStr := addr.String()

	// Parse back
	parsedAddr, _, err := ParseAddress(hexStr)
	if err != nil {
		t.Fatalf("Failed to parse address: %v", err)
	}

	if addr != parsedAddr {
		t.Fatal("Address parsing round trip failed")
	}

	// Test invalid hex
	_, _, err = ParseAddress("invalid_hex")
	if err == nil {
		t.Fatal("Should fail to parse invalid hex")
	}

	// Test wrong length
	_, _, err = ParseAddress("1234")
	if err == nil {
		t.Fatal("Should fail to parse wrong length address")
	}
}

func TestSignatureParsing(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	message := []byte("Test signature parsing")
	signature, err := kp.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	hexStr := fmt.Sprintf("%x", signature)

	// Parse back
	parsedSig, err := ParseSignature(hexStr)
	if err != nil {
		t.Fatalf("Failed to parse signature: %v", err)
	}

	if len(signature) != len(parsedSig) {
		t.Fatal("Signature lengths differ after parsing")
	}

	for i := range signature {
		if signature[i] != parsedSig[i] {
			t.Fatal("Signature parsing round trip failed")
		}
	}

	// Verify parsed signature works
	if !VerifySignature(message, parsedSig, kp.PublicKey) {
		t.Fatal("Parsed signature verification failed")
	}

	// Test invalid hex
	_, err = ParseSignature("invalid_hex")
	if err == nil {
		t.Fatal("Should fail to parse invalid hex signature")
	}
}

func TestNilInputValidation(t *testing.T) {
	// Test verify with nil inputs
	if VerifySignature(nil, []byte{1}, nil) {
		t.Fatal("Should fail to verify with nil inputs")
	}

	if VerifySignature([]byte{1}, nil, nil) {
		t.Fatal("Should fail to verify with nil signature")
	}

	if VerifySignatureWithContext(nil, nil, nil, nil) {
		t.Fatal("Should fail to verify context signature with nil inputs")
	}

	// Test with oversized context
	if VerifySignatureWithContext([]byte{1}, make([]byte, 256), []byte{1}, nil) {
		t.Fatal("Should fail to verify with oversized context")
	}
}
