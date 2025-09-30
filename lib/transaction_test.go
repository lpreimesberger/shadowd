package lib

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTxBuilder(t *testing.T) {
	// Create key pairs for testing
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	senderAddr := sender.Address()
	recipientAddr := recipient.Address()

	// Build transaction
	tx := NewTxBuilder().
		From(senderAddr).
		To(recipientAddr).
		Amount(1000).
		Fee(10).
		Nonce(42).
		Data([]byte("test data")).
		Build()

	// Verify fields
	if tx.From != senderAddr {
		t.Fatal("From address mismatch")
	}

	if tx.To != recipientAddr {
		t.Fatal("To address mismatch")
	}

	if tx.Amount != 1000 {
		t.Fatal("Amount mismatch")
	}

	if tx.Fee != 10 {
		t.Fatal("Fee mismatch")
	}

	if tx.Nonce != 42 {
		t.Fatal("Nonce mismatch")
	}

	if string(tx.Data) != "test data" {
		t.Fatal("Data mismatch")
	}

	if tx.Timestamp == 0 {
		t.Fatal("Timestamp not set")
	}
}

func TestTransactionSigning(t *testing.T) {
	// Create key pairs
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	// Create transaction
	tx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(500).
		Fee(5).
		Nonce(1).
		Build()

	// Sign transaction
	err = tx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Check signature fields are set
	if len(tx.PublicKey) == 0 {
		t.Fatal("Public key not set after signing")
	}

	if len(tx.Signature) == 0 {
		t.Fatal("Signature not set after signing")
	}

	// Validate transaction
	if !tx.IsValid() {
		t.Fatal("Signed transaction should be valid")
	}
}

func TestTransactionValidation(t *testing.T) {
	// Create key pairs
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	// Valid transaction
	validTx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Build()

	err = validTx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign valid transaction: %v", err)
	}

	err = ValidateTransaction(validTx)
	if err != nil {
		t.Fatalf("Valid transaction failed validation: %v", err)
	}

	// Test nil transaction
	err = ValidateTransaction(nil)
	if err == nil {
		t.Fatal("Should fail to validate nil transaction")
	}

	// Test transaction with no amount and no data
	noAmountTx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(0).
		Fee(1).
		Nonce(1).
		Build()

	err = noAmountTx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	err = ValidateTransaction(noAmountTx)
	if err == nil {
		t.Fatal("Should fail to validate transaction with no amount and no data")
	}

	// Test transaction with no fee
	noFeeTx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(100).
		Fee(0).
		Nonce(1).
		Build()

	err = ValidateTransaction(noFeeTx)
	if err == nil {
		t.Fatal("Should fail to validate transaction with no fee")
	}

	// Test unsigned transaction
	unsignedTx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Build()

	err = ValidateTransaction(unsignedTx)
	if err == nil {
		t.Fatal("Should fail to validate unsigned transaction")
	}
}

func TestTransactionWithWrongSender(t *testing.T) {
	// Create key pairs
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	wrongSender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate wrong sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	// Create transaction with wrong sender address
	tx := NewTxBuilder().
		From(wrongSender.Address()). // Wrong address
		To(recipient.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Build()

	// Try to sign with different key pair
	err = tx.Sign(sender)
	if err == nil {
		t.Fatal("Should fail to sign transaction with mismatched sender address")
	}
}

func TestTransactionHash(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	tx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Timestamp(1234567890).
		Build()

	// Hash before signing
	hash1, err := tx.Hash()
	if err != nil {
		t.Fatalf("Failed to compute transaction hash: %v", err)
	}

	if len(hash1) != 32 {
		t.Fatalf("Hash should be 32 bytes, got %d", len(hash1))
	}

	// Sign transaction
	err = tx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Hash after signing should be the same (excludes signature)
	hash2, err := tx.Hash()
	if err != nil {
		t.Fatalf("Failed to compute transaction hash after signing: %v", err)
	}

	if len(hash1) != len(hash2) {
		t.Fatal("Hash length changed after signing")
	}

	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Fatal("Transaction hash changed after signing")
		}
	}

	// Different transactions should have different hashes
	tx2 := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(200). // Different amount
		Fee(1).
		Nonce(1).
		Timestamp(1234567890).
		Build()

	hash3, err := tx2.Hash()
	if err != nil {
		t.Fatalf("Failed to compute second transaction hash: %v", err)
	}

	for i := range hash1 {
		if hash1[i] == hash3[i] {
			continue
		}
		// Found a difference, this is expected
		return
	}
	t.Fatal("Different transactions produced identical hashes")
}

func TestTransactionID(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	tx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Build()

	// Should fail to get ID before signing (no signature)
	_, err = tx.ID()
	if err == nil {
		t.Fatal("Should fail to get ID of unsigned transaction")
	}

	// Sign transaction
	err = tx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Should succeed after signing
	id, err := tx.ID()
	if err != nil {
		t.Fatalf("Failed to get transaction ID: %v", err)
	}

	if len(id) == 0 {
		t.Fatal("Transaction ID is empty")
	}

	// ID should be deterministic
	id2, err := tx.ID()
	if err != nil {
		t.Fatalf("Failed to get transaction ID second time: %v", err)
	}

	if id != id2 {
		t.Fatal("Transaction ID is not deterministic")
	}
}

func TestTransactionSerialization(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	originalTx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(500).
		Fee(10).
		Nonce(42).
		Data([]byte("serialization test")).
		Timestamp(1234567890).
		Build()

	err = originalTx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(originalTx)
	if err != nil {
		t.Fatalf("Failed to marshal transaction: %v", err)
	}

	// Deserialize from JSON
	var deserializedTx Transaction
	err = json.Unmarshal(jsonBytes, &deserializedTx)
	if err != nil {
		t.Fatalf("Failed to unmarshal transaction: %v", err)
	}

	// Verify fields match
	if originalTx.From != deserializedTx.From {
		t.Fatal("From address mismatch after serialization")
	}

	if originalTx.To != deserializedTx.To {
		t.Fatal("To address mismatch after serialization")
	}

	if originalTx.Amount != deserializedTx.Amount {
		t.Fatal("Amount mismatch after serialization")
	}

	if originalTx.Fee != deserializedTx.Fee {
		t.Fatal("Fee mismatch after serialization")
	}

	if originalTx.Nonce != deserializedTx.Nonce {
		t.Fatal("Nonce mismatch after serialization")
	}

	if originalTx.Timestamp != deserializedTx.Timestamp {
		t.Fatal("Timestamp mismatch after serialization")
	}

	if string(originalTx.Data) != string(deserializedTx.Data) {
		t.Fatal("Data mismatch after serialization")
	}

	// Verify signature fields
	if len(originalTx.PublicKey) != len(deserializedTx.PublicKey) {
		t.Fatal("Public key length mismatch after serialization")
	}

	if len(originalTx.Signature) != len(deserializedTx.Signature) {
		t.Fatal("Signature length mismatch after serialization")
	}

	// Verify deserialized transaction is still valid
	if !deserializedTx.IsValid() {
		t.Fatal("Deserialized transaction is not valid")
	}

	// IDs should match
	originalID, _ := originalTx.ID()
	deserializedID, _ := deserializedTx.ID()

	if originalID != deserializedID {
		t.Fatal("Transaction IDs differ after serialization")
	}
}

func TestTransactionWithDataOnly(t *testing.T) {
	sender, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate sender key pair: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate recipient key pair: %v", err)
	}

	// Transaction with data but no amount (like a smart contract call)
	tx := NewTxBuilder().
		From(sender.Address()).
		To(recipient.Address()).
		Amount(0).
		Fee(1).
		Nonce(1).
		Data([]byte("smart_contract_call")).
		Build()

	err = tx.Sign(sender)
	if err != nil {
		t.Fatalf("Failed to sign data-only transaction: %v", err)
	}

	// Should be valid (has data even though amount is 0)
	if !tx.IsValid() {
		t.Fatal("Data-only transaction should be valid")
	}
}
