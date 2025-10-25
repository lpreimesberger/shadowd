package lib

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
	"golang.org/x/crypto/pbkdf2"
)

// WalletData represents the JSON structure saved to disk
type WalletData struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`  // Base64 encoded
	PrivateKey string `json:"private_key"` // Base64 encoded (encrypted if Encrypted=true)
	Created    int64  `json:"created"`     // Unix timestamp
	Version    int    `json:"version"`     // Wallet format version (1=plaintext, 2=encrypted)

	// Encryption fields (version 2 only)
	Encrypted bool   `json:"encrypted,omitempty"` // True if private key is encrypted
	Salt      string `json:"salt,omitempty"`      // Base64 encoded salt for PBKDF2 (32 bytes)
	Nonce     string `json:"nonce,omitempty"`     // Base64 encoded GCM nonce (12 bytes)
}

// NodeWallet represents the active wallet for a blockchain node
type NodeWallet struct {
	KeyPair *KeyPair
	Address Address
	Path    string // File path where wallet is stored
}

// Global node wallet instance
var globalNodeWallet *NodeWallet

// DefaultWalletPath returns the default wallet path ~/.sn/default.json
func DefaultWalletPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	walletDir := filepath.Join(homeDir, ".sn")
	walletPath := filepath.Join(walletDir, "default.json")

	return walletPath, nil
}

// deriveEncryptionKey derives an AES-256 key from a passphrase using PBKDF2
// Uses 600,000 iterations (OWASP recommendation as of 2023)
func deriveEncryptionKey(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, 600000, 32, sha256.New)
}

// encryptPrivateKey encrypts a private key using AES-256-GCM
// Returns: (ciphertext, salt, nonce, error)
func encryptPrivateKey(privateKeyBytes []byte, passphrase string) ([]byte, []byte, []byte, error) {
	// Generate random salt (32 bytes)
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from passphrase
	key := deriveEncryptionKey(passphrase, salt)

	// Create AES-256 cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the private key
	ciphertext := gcm.Seal(nil, nonce, privateKeyBytes, nil)

	return ciphertext, salt, nonce, nil
}

// decryptPrivateKey decrypts a private key using AES-256-GCM
func decryptPrivateKey(ciphertext []byte, passphrase string, salt []byte, nonce []byte) ([]byte, error) {
	// Derive encryption key from passphrase
	key := deriveEncryptionKey(passphrase, salt)

	// Create AES-256 cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the private key
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	return plaintext, nil
}

// CreateWalletData creates a new wallet data structure
// If passphrase is non-empty, encrypts the private key (version 2)
// If passphrase is empty, stores plaintext (version 1, backward compatible)
func CreateWalletData(passphrase string) (*WalletData, *KeyPair, error) {
	// Generate new key pair
	keyPair, err := GenerateKeyPair()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Serialize keys to base64
	publicKeyBytes, err := PublicKeyToBytes(keyPair.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize public key: %w", err)
	}

	privateKeyBytes, err := keyPair.PrivateKey.MarshalBinary()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to serialize private key: %w", err)
	}

	walletData := &WalletData{
		Address:   keyPair.Address().String(),
		PublicKey: base64.StdEncoding.EncodeToString(publicKeyBytes),
		Created:   GetCurrentTimestamp(),
	}

	// Encrypt private key if passphrase provided
	if passphrase != "" {
		ciphertext, salt, nonce, err := encryptPrivateKey(privateKeyBytes, passphrase)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to encrypt private key: %w", err)
		}

		walletData.PrivateKey = base64.StdEncoding.EncodeToString(ciphertext)
		walletData.Salt = base64.StdEncoding.EncodeToString(salt)
		walletData.Nonce = base64.StdEncoding.EncodeToString(nonce)
		walletData.Encrypted = true
		walletData.Version = 2
	} else {
		// Plaintext wallet (v1)
		walletData.PrivateKey = base64.StdEncoding.EncodeToString(privateKeyBytes)
		walletData.Version = 1
	}

	return walletData, keyPair, nil
}

// LoadWalletData loads wallet data from a JSON file
// For encrypted wallets (v2), passphrase must be provided
// For plaintext wallets (v1), passphrase is ignored
func LoadWalletData(path string, passphrase string) (*WalletData, *KeyPair, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read wallet file: %w", err)
	}

	// Parse JSON
	var walletData WalletData
	if err := json.Unmarshal(data, &walletData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse wallet JSON: %w", err)
	}

	// Validate version
	if walletData.Version != 1 && walletData.Version != 2 {
		return nil, nil, fmt.Errorf("unsupported wallet version: %d", walletData.Version)
	}

	// Decode public key from base64
	publicKeyBytes, err := base64.StdEncoding.DecodeString(walletData.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	// Decode private key (encrypted or plaintext)
	var privateKeyBytes []byte

	if walletData.Version == 2 && walletData.Encrypted {
		// Encrypted wallet - need passphrase
		if passphrase == "" {
			return nil, nil, fmt.Errorf("wallet is encrypted but no passphrase provided")
		}

		// Decode ciphertext, salt, and nonce
		ciphertext, err := base64.StdEncoding.DecodeString(walletData.PrivateKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode encrypted private key: %w", err)
		}

		salt, err := base64.StdEncoding.DecodeString(walletData.Salt)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode salt: %w", err)
		}

		nonce, err := base64.StdEncoding.DecodeString(walletData.Nonce)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode nonce: %w", err)
		}

		// Decrypt private key
		privateKeyBytes, err = decryptPrivateKey(ciphertext, passphrase, salt, nonce)
		if err != nil {
			return nil, nil, err // Error already has context
		}
	} else {
		// Plaintext wallet (v1) or unencrypted v2
		privateKeyBytes, err = base64.StdEncoding.DecodeString(walletData.PrivateKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode private key: %w", err)
		}
	}

	// Reconstruct key pair
	publicKey, err := PublicKeyFromBytes(publicKeyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reconstruct public key: %w", err)
	}

	var privateKey mldsa87.PrivateKey
	if err := privateKey.UnmarshalBinary(privateKeyBytes); err != nil {
		return nil, nil, fmt.Errorf("failed to reconstruct private key: %w", err)
	}

	keyPair := &KeyPair{
		PublicKey:  publicKey,
		PrivateKey: &privateKey,
	}

	// Verify address matches
	expectedAddress := keyPair.Address()
	storedAddress, _, err := ParseAddress(walletData.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid stored address: %w", err)
	}

	if expectedAddress != storedAddress {
		return nil, nil, fmt.Errorf("wallet corruption: address mismatch")
	}

	return &walletData, keyPair, nil
}

// SaveWalletData saves wallet data to a JSON file
func SaveWalletData(walletData *WalletData, path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create wallet directory: %w", err)
	}

	// Marshal to JSON with nice formatting
	jsonData, err := json.MarshalIndent(walletData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal wallet data: %w", err)
	}

	// Write to temporary file first, then rename (atomic write)
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write wallet file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to finalize wallet file: %w", err)
	}

	return nil
}

// LoadOrCreateNodeWallet loads wallet from ~/.sn/default.json or creates it if missing
// passphrase is used for encrypted wallets (v2). Empty string = plaintext wallet (v1)
func LoadOrCreateNodeWallet(passphrase string) (*NodeWallet, error) {
	walletPath, err := DefaultWalletPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine wallet path: %w", err)
	}

	var walletData *WalletData
	var keyPair *KeyPair

	// Try to load existing wallet
	if _, err := os.Stat(walletPath); err == nil {
		// Wallet exists, load it
		walletData, keyPair, err = LoadWalletData(walletPath, passphrase)
		if err != nil {
			return nil, fmt.Errorf("failed to load existing wallet: %w", err)
		}
		if walletData.Encrypted {
			fmt.Printf("🔑 Loaded existing encrypted wallet: %s\n", walletData.Address[:16]+"...")
		} else {
			fmt.Printf("🔑 Loaded existing wallet: %s\n", walletData.Address[:16]+"...")
		}
	} else if os.IsNotExist(err) {
		// Wallet doesn't exist, create it
		walletData, keyPair, err = CreateWalletData(passphrase)
		if err != nil {
			return nil, fmt.Errorf("failed to create new wallet: %w", err)
		}

		// Save the new wallet
		if err := SaveWalletData(walletData, walletPath); err != nil {
			return nil, fmt.Errorf("failed to save new wallet: %w", err)
		}

		if walletData.Encrypted {
			fmt.Printf("🆕 Created new encrypted wallet: %s\n", walletData.Address[:16]+"...")
			fmt.Printf("📁 Wallet saved to: %s\n", walletPath)
			fmt.Printf("⚠️  IMPORTANT: Store your passphrase securely! It cannot be recovered.\n")
		} else {
			fmt.Printf("🆕 Created new wallet: %s\n", walletData.Address[:16]+"...")
			fmt.Printf("📁 Wallet saved to: %s\n", walletPath)
		}
	} else {
		return nil, fmt.Errorf("failed to check wallet file: %w", err)
	}

	nodeWallet := &NodeWallet{
		KeyPair: keyPair,
		Address: keyPair.Address(),
		Path:    walletPath,
	}

	return nodeWallet, nil
}

// InitializeGlobalWallet initializes the global node wallet
// passphrase is used for encrypted wallets (v2). Empty string = plaintext wallet (v1)
func InitializeGlobalWallet(passphrase string) error {
	wallet, err := LoadOrCreateNodeWallet(passphrase)
	if err != nil {
		return fmt.Errorf("failed to initialize global wallet: %w", err)
	}

	globalNodeWallet = wallet
	return nil
}

// GetGlobalWallet returns the global node wallet (must call InitializeGlobalWallet first)
func GetGlobalWallet() *NodeWallet {
	return globalNodeWallet
}

// IsGlobalWalletInitialized returns true if the global wallet is initialized
func IsGlobalWalletInitialized() bool {
	return globalNodeWallet != nil
}

// GetNodeAddress returns the node's address
func (nw *NodeWallet) GetAddress() Address {
	return nw.Address
}

// GetAddressString returns the node's address as a hex string
func (nw *NodeWallet) GetAddressString() string {
	return nw.Address.String()
}

// GetPrivateKeyBytes returns the private key as bytes for mining
func (nw *NodeWallet) GetPrivateKeyBytes() []byte {
	if nw.KeyPair == nil || nw.KeyPair.PrivateKey == nil {
		return nil
	}
	// Convert mldsa87.PrivateKey to bytes using MarshalBinary
	privateKeyBytes, err := nw.KeyPair.PrivateKey.MarshalBinary()
	if err != nil {
		return nil
	}
	return privateKeyBytes
}

// SignTransaction signs a transaction with the node's key pair
func (nw *NodeWallet) SignTransaction(tx *Transaction) error {
	return tx.Sign(nw.KeyPair)
}

// CreateTransaction creates a new transaction from this node wallet (legacy - simplified UTXO)
func (nw *NodeWallet) CreateTransaction(to Address, amount, fee, nonce uint64, data []byte) *Transaction {
	builder := NewTxBuilder(TxTypeSend)

	// Add output for recipient
	builder.AddOutput(to, amount, "SHADOW")

	if data != nil {
		builder.SetData(data)
	}

	// Build transaction
	tx := builder.Build()

	// Set legacy fields for backward compatibility
	tx.From = &nw.Address
	tx.To = &to
	tx.Amount = &amount
	tx.Fee = &fee
	tx.Nonce = &nonce

	return tx
}

// CreateAndSignTransaction creates and signs a transaction in one step (legacy)
func (nw *NodeWallet) CreateAndSignTransaction(to Address, amount, fee, nonce uint64, data []byte) (*Transaction, error) {
	tx := nw.CreateTransaction(to, amount, fee, nonce, data)

	if err := nw.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// --- New UTXO-based transaction methods ---

// CreateCoinbaseTransaction creates a coinbase transaction for mining rewards
func (nw *NodeWallet) CreateCoinbaseTransaction(blockHeight uint64, reward uint64, blockTimestamp int64) *Transaction {
	return CreateCoinbaseTransaction(nw.Address, blockHeight, reward, blockTimestamp)
}

// CreateSimpleSendTransaction creates a simple send transaction (requires UTXOs)
func (nw *NodeWallet) CreateSimpleSendTransaction(utxos []*UTXO, toAddress Address, amount uint64) (*Transaction, error) {
	return CreateSimpleSendTransaction(utxos, toAddress, amount, nw.Address)
}

// CreateMintTokenTransaction creates a token minting transaction
func (nw *NodeWallet) CreateMintTokenTransaction(tokenID, tokenType string, amount uint64, recipientAddress Address, metadata []byte) *Transaction {
	return CreateMintTokenTransaction(tokenID, tokenType, amount, recipientAddress, metadata)
}

// CreateMeltTransaction creates a token melting transaction
func (nw *NodeWallet) CreateMeltTransaction(utxos []*UTXO, reason string) *Transaction {
	return CreateMeltTransaction(utxos, reason)
}

// CreateAndSignCoinbaseTransaction creates and signs a coinbase transaction
func (nw *NodeWallet) CreateAndSignCoinbaseTransaction(blockHeight uint64, reward uint64, blockTimestamp int64) (*Transaction, error) {
	tx := nw.CreateCoinbaseTransaction(blockHeight, reward, blockTimestamp)

	if err := nw.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign coinbase transaction: %w", err)
	}

	return tx, nil
}

// CreateAndSignSendTransaction creates and signs a send transaction with UTXOs
func (nw *NodeWallet) CreateAndSignSendTransaction(utxos []*UTXO, toAddress Address, amount uint64) (*Transaction, error) {
	tx, err := nw.CreateSimpleSendTransaction(utxos, toAddress, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to create send transaction: %w", err)
	}

	if err := nw.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign send transaction: %w", err)
	}

	return tx, nil
}

// CreateAndSignMintTokenTransaction creates and signs a token minting transaction
func (nw *NodeWallet) CreateAndSignMintTokenTransaction(tokenID, tokenType string, amount uint64, recipientAddress Address, metadata []byte) (*Transaction, error) {
	tx := nw.CreateMintTokenTransaction(tokenID, tokenType, amount, recipientAddress, metadata)

	if err := nw.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign mint token transaction: %w", err)
	}

	return tx, nil
}

// BackupWallet creates a backup of the wallet file
func (nw *NodeWallet) BackupWallet(backupPath string) error {
	// Read original wallet
	data, err := os.ReadFile(nw.Path)
	if err != nil {
		return fmt.Errorf("failed to read wallet file: %w", err)
	}

	// Ensure backup directory exists
	dir := filepath.Dir(backupPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Write backup
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	return nil
}

// GetWalletInfo returns human-readable wallet information
func (nw *NodeWallet) GetWalletInfo() map[string]interface{} {
	// Read wallet JSON directly for metadata (no decryption needed)
	data, err := os.ReadFile(nw.Path)
	if err != nil {
		return map[string]interface{}{
			"address":       nw.Address.String(),
			"address_short": nw.Address.String()[:16] + "...",
			"path":          nw.Path,
			"created":       int64(0),
			"version":       1,
			"encrypted":     false,
		}
	}

	var walletData WalletData
	json.Unmarshal(data, &walletData) // Ignore error, use defaults

	return map[string]interface{}{
		"address":       nw.Address.String(),
		"address_short": nw.Address.String()[:16] + "...",
		"path":          nw.Path,
		"created":       walletData.Created,
		"version":       walletData.Version,
		"encrypted":     walletData.Encrypted,
	}
}

// ValidateWalletFile validates the integrity of a wallet file
// For encrypted wallets, passphrase must be provided
func ValidateWalletFile(path string, passphrase string) error {
	_, _, err := LoadWalletData(path, passphrase)
	return err
}

// GenerateDeterministicWallet creates a wallet from a seed phrase/bytes
func GenerateDeterministicWallet(seed []byte) (*NodeWallet, error) {
	if len(seed) < 32 {
		return nil, fmt.Errorf("seed must be at least 32 bytes")
	}

	// Use first 32 bytes as seed
	var seedArray [mldsa87.SeedSize]byte
	copy(seedArray[:], seed[:32])

	keyPair := GenerateKeyPairFromSeed(seedArray)

	nodeWallet := &NodeWallet{
		KeyPair: keyPair,
		Address: keyPair.Address(),
		Path:    "", // No file path for deterministic wallet
	}

	return nodeWallet, nil
}
