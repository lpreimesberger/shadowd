package lib

import (
	"fmt"
	"log"
)

// Example demonstrates the basic usage of the post-quantum blockchain cryptography library
func ExampleBasicUsage() {
	// Generate key pairs for Alice and Bob
	alice, err := GenerateKeyPair()
	if err != nil {
		log.Fatal("Failed to generate Alice's key pair:", err)
	}

	bob, err := GenerateKeyPair()
	if err != nil {
		log.Fatal("Failed to generate Bob's key pair:", err)
	}

	fmt.Printf("Alice's address: %s\n", alice.Address().String()[:16]+"...")
	fmt.Printf("Bob's address: %s\n", bob.Address().String()[:16]+"...")

	// Alice creates a transaction to send 100 SHADOW to Bob
	tx := NewTxBuilder().
		From(alice.Address()).
		To(bob.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Data([]byte("Hello Bob!")).
		Build()

	// Alice signs the transaction
	err = tx.Sign(alice)
	if err != nil {
		log.Fatal("Failed to sign transaction:", err)
	}

	// Validate the transaction
	if tx.IsValid() {
		fmt.Println("✓ Transaction is valid!")
	} else {
		fmt.Println("✗ Transaction is invalid!")
	}

	// Get transaction ID
	txID, err := tx.ID()
	if err != nil {
		log.Fatal("Failed to get transaction ID:", err)
	}

	fmt.Printf("Transaction ID: %s...\n", txID[:16])
	fmt.Printf("Transaction: %s\n", tx.String())

	// Output:
	// Alice's address: ...
	// Bob's address: ...
	// ✓ Transaction is valid!
	// Transaction ID: ...
	// Transaction: Transaction{ID: ..., From: ..., To: ..., Amount: 100, Fee: 1}
}

// ExampleDeterministicKeys shows how to generate deterministic key pairs from seeds
func ExampleDeterministicKeys() {
	// Create a seed for deterministic key generation
	seed := [32]byte{}
	copy(seed[:], "my_deterministic_seed_for_wallet")

	// Generate the same key pair every time from this seed
	keyPair := GenerateKeyPairFromSeed(seed)
	address := keyPair.Address()

	fmt.Printf("Deterministic address: %s\n", address.String()[:16]+"...")

	// Generate again - should be identical
	keyPair2 := GenerateKeyPairFromSeed(seed)
	address2 := keyPair2.Address()

	if address == address2 {
		fmt.Println("✓ Deterministic key generation works!")
	} else {
		fmt.Println("✗ Deterministic key generation failed!")
	}

	// Output:
	// Deterministic address: ...
	// ✓ Deterministic key generation works!
}

// ExampleSignatureWithContext shows how to use context strings in signatures
func ExampleSignatureWithContext() {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		log.Fatal("Failed to generate key pair:", err)
	}

	message := []byte("Important blockchain message")
	context := []byte("shadowy_blockchain_v1")

	// Sign with context
	signature, err := keyPair.SignWithContext(message, context)
	if err != nil {
		log.Fatal("Failed to sign with context:", err)
	}

	// Verify with correct context
	if VerifySignatureWithContext(message, context, signature, keyPair.PublicKey) {
		fmt.Println("✓ Context signature verification succeeded!")
	} else {
		fmt.Println("✗ Context signature verification failed!")
	}

	// Try to verify with wrong context - should fail
	wrongContext := []byte("wrong_context")
	if !VerifySignatureWithContext(message, wrongContext, signature, keyPair.PublicKey) {
		fmt.Println("✓ Context signature correctly rejected wrong context!")
	} else {
		fmt.Println("✗ Context signature incorrectly accepted wrong context!")
	}

	// Output:
	// ✓ Context signature verification succeeded!
	// ✓ Context signature correctly rejected wrong context!
}

// ExampleAddressValidation demonstrates address validation from public keys
func ExampleAddressValidation() {
	// Generate a key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		log.Fatal("Failed to generate key pair:", err)
	}

	// Serialize and parse the public key
	publicKeyBytes, err := PublicKeyToBytes(keyPair.PublicKey)
	if err != nil {
		log.Fatal("Failed to serialize public key:", err)
	}

	reconstructedKey, err := PublicKeyFromBytes(publicKeyBytes)
	if err != nil {
		log.Fatal("Failed to parse public key:", err)
	}

	// Check address derivation
	originalAddress := keyPair.Address()
	reconstructedAddress := DeriveAddress(reconstructedKey)

	if originalAddress == reconstructedAddress {
		fmt.Println("✓ Address validation from public key works!")
	} else {
		fmt.Println("✗ Address validation failed!")
	}

	fmt.Printf("Address: %s\n", originalAddress.String()[:16]+"...")

	// Output:
	// ✓ Address validation from public key works!
	// Address: ...
}

// ExampleTransactionChain demonstrates a chain of transactions
func ExampleTransactionChain() {
	// Create three participants
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()
	charlie, _ := GenerateKeyPair()

	fmt.Println("=== Transaction Chain Example ===")

	// Transaction 1: Alice sends 100 to Bob
	tx1 := NewTxBuilder().
		From(alice.Address()).
		To(bob.Address()).
		Amount(100).
		Fee(1).
		Nonce(1).
		Build()
	tx1.Sign(alice)

	// Transaction 2: Bob sends 50 to Charlie
	tx2 := NewTxBuilder().
		From(bob.Address()).
		To(charlie.Address()).
		Amount(50).
		Fee(1).
		Nonce(1).
		Build()
	tx2.Sign(bob)

	// Transaction 3: Charlie sends 25 back to Alice
	tx3 := NewTxBuilder().
		From(charlie.Address()).
		To(alice.Address()).
		Amount(25).
		Fee(1).
		Nonce(1).
		Build()
	tx3.Sign(charlie)

	// Validate all transactions
	transactions := []*Transaction{tx1, tx2, tx3}
	for i, tx := range transactions {
		if tx.IsValid() {
			fmt.Printf("Transaction %d: ✓ Valid\n", i+1)
		} else {
			fmt.Printf("Transaction %d: ✗ Invalid\n", i+1)
		}
	}

	// Output:
	// === Transaction Chain Example ===
	// Transaction 1: ✓ Valid
	// Transaction 2: ✓ Valid
	// Transaction 3: ✓ Valid
}
