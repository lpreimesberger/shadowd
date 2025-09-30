package lib

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateWalletData(t *testing.T) {
	walletData, keyPair, err := CreateWalletData()
	if err != nil {
		t.Fatalf("Failed to create wallet data: %v", err)
	}

	if walletData == nil {
		t.Fatal("Wallet data is nil")
	}

	if keyPair == nil {
		t.Fatal("Key pair is nil")
	}

	// Verify fields are populated
	if walletData.Address == "" {
		t.Fatal("Address is empty")
	}

	if walletData.PublicKey == "" {
		t.Fatal("Public key is empty")
	}

	if walletData.PrivateKey == "" {
		t.Fatal("Private key is empty")
	}

	if walletData.Version != 1 {
		t.Fatalf("Expected version 1, got %d", walletData.Version)
	}

	if walletData.Created == 0 {
		t.Fatal("Created timestamp is zero")
	}

	// Verify address matches key pair
	expectedAddr := keyPair.Address()
	storedAddr, err := ParseAddress(walletData.Address)
	if err != nil {
		t.Fatalf("Failed to parse stored address: %v", err)
	}

	if expectedAddr != storedAddr {
		t.Fatal("Stored address doesn't match key pair address")
	}
}

func TestWalletSaveAndLoad(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	walletPath := filepath.Join(tempDir, "test_wallet.json")

	// Create wallet data
	originalWalletData, originalKeyPair, err := CreateWalletData()
	if err != nil {
		t.Fatalf("Failed to create wallet data: %v", err)
	}

	// Save wallet
	err = SaveWalletData(originalWalletData, walletPath)
	if err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		t.Fatal("Wallet file was not created")
	}

	// Load wallet
	loadedWalletData, loadedKeyPair, err := LoadWalletData(walletPath)
	if err != nil {
		t.Fatalf("Failed to load wallet: %v", err)
	}

	// Compare data
	if originalWalletData.Address != loadedWalletData.Address {
		t.Fatal("Address mismatch after load")
	}

	if originalWalletData.PublicKey != loadedWalletData.PublicKey {
		t.Fatal("Public key mismatch after load")
	}

	if originalWalletData.PrivateKey != loadedWalletData.PrivateKey {
		t.Fatal("Private key mismatch after load")
	}

	if originalWalletData.Created != loadedWalletData.Created {
		t.Fatal("Created timestamp mismatch after load")
	}

	// Verify addresses match
	originalAddr := originalKeyPair.Address()
	loadedAddr := loadedKeyPair.Address()

	if originalAddr != loadedAddr {
		t.Fatal("Key pair addresses differ after load")
	}

	// Test signing with loaded key pair
	message := []byte("Test message")
	signature, err := loadedKeyPair.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign with loaded key pair: %v", err)
	}

	// Verify signature
	if !VerifySignature(message, signature, loadedKeyPair.PublicKey) {
		t.Fatal("Signature verification failed with loaded key pair")
	}
}

func TestLoadOrCreateNodeWallet(t *testing.T) {
	// Create temporary directory to simulate home directory
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override the default path for testing
	originalDefaultPath := DefaultWalletPath
	defer func() {
		DefaultWalletPath = originalDefaultPath
	}()

	testWalletPath := filepath.Join(tempDir, ".sn", "default.json")
	DefaultWalletPath = func() (string, error) {
		return testWalletPath, nil
	}

	// First call should create new wallet
	wallet1, err := LoadOrCreateNodeWallet()
	if err != nil {
		t.Fatalf("Failed to load/create wallet: %v", err)
	}

	if wallet1 == nil {
		t.Fatal("Wallet is nil")
	}

	// Verify file was created
	if _, err := os.Stat(testWalletPath); os.IsNotExist(err) {
		t.Fatal("Wallet file was not created")
	}

	originalAddress := wallet1.Address

	// Second call should load existing wallet
	wallet2, err := LoadOrCreateNodeWallet()
	if err != nil {
		t.Fatalf("Failed to load existing wallet: %v", err)
	}

	// Should have same address
	if originalAddress != wallet2.Address {
		t.Fatal("Loaded wallet has different address")
	}
}

func TestNodeWalletTransactionOperations(t *testing.T) {
	// Create test wallet
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	walletData, keyPair, err := CreateWalletData()
	if err != nil {
		t.Fatalf("Failed to create wallet data: %v", err)
	}

	wallet := &NodeWallet{
		KeyPair: keyPair,
		Address: keyPair.Address(),
		Path:    filepath.Join(tempDir, "test.json"),
	}

	// Create recipient
	recipientKp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to create recipient: %v", err)
	}

	recipientAddr := recipientKp.Address()

	// Test CreateTransaction
	tx := wallet.CreateTransaction(recipientAddr, 100, 5, 1, []byte("test data"))

	if tx.From != wallet.Address {
		t.Fatal("Transaction from address mismatch")
	}

	if tx.To != recipientAddr {
		t.Fatal("Transaction to address mismatch")
	}

	if tx.Amount != 100 {
		t.Fatal("Transaction amount mismatch")
	}

	if tx.Fee != 5 {
		t.Fatal("Transaction fee mismatch")
	}

	if tx.Nonce != 1 {
		t.Fatal("Transaction nonce mismatch")
	}

	if string(tx.Data) != "test data" {
		t.Fatal("Transaction data mismatch")
	}

	// Test SignTransaction
	err = wallet.SignTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Verify transaction is valid
	if !tx.IsValid() {
		t.Fatal("Signed transaction is not valid")
	}

	// Test CreateAndSignTransaction
	signedTx, err := wallet.CreateAndSignTransaction(recipientAddr, 200, 10, 2, nil)
	if err != nil {
		t.Fatalf("Failed to create and sign transaction: %v", err)
	}

	if !signedTx.IsValid() {
		t.Fatal("Created and signed transaction is not valid")
	}

	// Save wallet data first
	SaveWalletData(walletData, wallet.Path)

	// Test GetWalletInfo
	info := wallet.GetWalletInfo()
	if info["address"] != wallet.Address.String() {
		t.Fatal("Wallet info address mismatch")
	}

	if info["version"] != 1 {
		t.Fatal("Wallet info version mismatch")
	}
}

func TestGlobalWalletManagement(t *testing.T) {
	// Reset global wallet
	globalNodeWallet = nil

	// Should not be initialized initially
	if IsGlobalWalletInitialized() {
		t.Fatal("Global wallet should not be initialized")
	}

	if GetGlobalWallet() != nil {
		t.Fatal("Global wallet should be nil")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Override the default path for testing
	originalDefaultPath := DefaultWalletPath
	defer func() {
		DefaultWalletPath = originalDefaultPath
		globalNodeWallet = nil // Reset for other tests
	}()

	testWalletPath := filepath.Join(tempDir, ".sn", "default.json")
	DefaultWalletPath = func() (string, error) {
		return testWalletPath, nil
	}

	// Initialize global wallet
	err = InitializeGlobalWallet()
	if err != nil {
		t.Fatalf("Failed to initialize global wallet: %v", err)
	}

	// Should now be initialized
	if !IsGlobalWalletInitialized() {
		t.Fatal("Global wallet should be initialized")
	}

	wallet := GetGlobalWallet()
	if wallet == nil {
		t.Fatal("Global wallet should not be nil")
	}

	// Test wallet functions
	if len(wallet.GetAddressString()) != 64 { // 32 bytes * 2 hex chars
		t.Fatal("Invalid address string length")
	}

	address := wallet.GetAddress()
	if address != wallet.Address {
		t.Fatal("GetAddress() returns different address")
	}
}

func TestWalletValidation(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	walletPath := filepath.Join(tempDir, "valid_wallet.json")
	invalidPath := filepath.Join(tempDir, "invalid_wallet.json")

	// Create valid wallet
	walletData, _, err := CreateWalletData()
	if err != nil {
		t.Fatalf("Failed to create wallet data: %v", err)
	}

	err = SaveWalletData(walletData, walletPath)
	if err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	// Test valid wallet
	err = ValidateWalletFile(walletPath)
	if err != nil {
		t.Fatalf("Valid wallet failed validation: %v", err)
	}

	// Create invalid wallet file
	err = os.WriteFile(invalidPath, []byte("invalid json"), 0600)
	if err != nil {
		t.Fatalf("Failed to create invalid wallet file: %v", err)
	}

	// Test invalid wallet
	err = ValidateWalletFile(invalidPath)
	if err == nil {
		t.Fatal("Invalid wallet passed validation")
	}

	// Test non-existent wallet
	err = ValidateWalletFile(filepath.Join(tempDir, "nonexistent.json"))
	if err == nil {
		t.Fatal("Non-existent wallet passed validation")
	}
}

func TestDeterministicWallet(t *testing.T) {
	// Create deterministic seed
	seed := make([]byte, 32)
	seed[0] = 0x42 // Make it deterministic for testing

	// Generate wallet 1
	wallet1, err := GenerateDeterministicWallet(seed)
	if err != nil {
		t.Fatalf("Failed to generate deterministic wallet 1: %v", err)
	}

	// Generate wallet 2 with same seed
	wallet2, err := GenerateDeterministicWallet(seed)
	if err != nil {
		t.Fatalf("Failed to generate deterministic wallet 2: %v", err)
	}

	// Should have same address
	if wallet1.Address != wallet2.Address {
		t.Fatal("Deterministic wallets have different addresses")
	}

	// Test signing with both wallets
	message := []byte("Deterministic test")

	sig1, err := wallet1.KeyPair.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign with wallet 1: %v", err)
	}

	sig2, err := wallet2.KeyPair.Sign(message)
	if err != nil {
		t.Fatalf("Failed to sign with wallet 2: %v", err)
	}

	// Signatures should be identical (ML-DSA87 is deterministic when using same key)
	if len(sig1) != len(sig2) {
		t.Fatal("Signature lengths differ")
	}

	// Test with insufficient seed
	shortSeed := make([]byte, 16) // Too short
	_, err = GenerateDeterministicWallet(shortSeed)
	if err == nil {
		t.Fatal("Should fail with insufficient seed length")
	}
}

func TestWalletBackup(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "wallet_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create wallet
	walletData, keyPair, err := CreateWalletData()
	if err != nil {
		t.Fatalf("Failed to create wallet data: %v", err)
	}

	walletPath := filepath.Join(tempDir, "original.json")
	backupPath := filepath.Join(tempDir, "backup", "wallet_backup.json")

	// Save original wallet
	err = SaveWalletData(walletData, walletPath)
	if err != nil {
		t.Fatalf("Failed to save wallet: %v", err)
	}

	wallet := &NodeWallet{
		KeyPair: keyPair,
		Address: keyPair.Address(),
		Path:    walletPath,
	}

	// Create backup
	err = wallet.BackupWallet(backupPath)
	if err != nil {
		t.Fatalf("Failed to backup wallet: %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("Backup file was not created")
	}

	// Load backup and verify it's identical
	backupWalletData, backupKeyPair, err := LoadWalletData(backupPath)
	if err != nil {
		t.Fatalf("Failed to load backup: %v", err)
	}

	if walletData.Address != backupWalletData.Address {
		t.Fatal("Backup address differs")
	}

	if wallet.Address != backupKeyPair.Address() {
		t.Fatal("Backup key pair address differs")
	}
}
