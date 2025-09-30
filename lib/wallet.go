package lib

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

// WalletData represents the JSON structure saved to disk
type WalletData struct {
	Address    string `json:"address"`
	PublicKey  string `json:"public_key"`  // Base64 encoded
	PrivateKey string `json:"private_key"` // Base64 encoded
	Created    int64  `json:"created"`     // Unix timestamp
	Version    int    `json:"version"`     // Wallet format version
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

// CreateWalletData creates a new wallet data structure
func CreateWalletData() (*WalletData, *KeyPair, error) {
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
		Address:    keyPair.Address().String(),
		PublicKey:  base64.StdEncoding.EncodeToString(publicKeyBytes),
		PrivateKey: base64.StdEncoding.EncodeToString(privateKeyBytes),
		Created:    GetCurrentTimestamp(),
		Version:    1,
	}

	return walletData, keyPair, nil
}

// LoadWalletData loads wallet data from a JSON file
func LoadWalletData(path string) (*WalletData, *KeyPair, error) {
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
	if walletData.Version != 1 {
		return nil, nil, fmt.Errorf("unsupported wallet version: %d", walletData.Version)
	}

	// Decode keys from base64
	publicKeyBytes, err := base64.StdEncoding.DecodeString(walletData.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode public key: %w", err)
	}

	privateKeyBytes, err := base64.StdEncoding.DecodeString(walletData.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode private key: %w", err)
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
func LoadOrCreateNodeWallet() (*NodeWallet, error) {
	walletPath, err := DefaultWalletPath()
	if err != nil {
		return nil, fmt.Errorf("failed to determine wallet path: %w", err)
	}

	var walletData *WalletData
	var keyPair *KeyPair

	// Try to load existing wallet
	if _, err := os.Stat(walletPath); err == nil {
		// Wallet exists, load it
		walletData, keyPair, err = LoadWalletData(walletPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load existing wallet: %w", err)
		}
		fmt.Printf("🔑 Loaded existing wallet: %s\n", walletData.Address[:16]+"...")
	} else if os.IsNotExist(err) {
		// Wallet doesn't exist, create it
		walletData, keyPair, err = CreateWalletData()
		if err != nil {
			return nil, fmt.Errorf("failed to create new wallet: %w", err)
		}

		// Save the new wallet
		if err := SaveWalletData(walletData, walletPath); err != nil {
			return nil, fmt.Errorf("failed to save new wallet: %w", err)
		}

		fmt.Printf("🆕 Created new wallet: %s\n", walletData.Address[:16]+"...")
		fmt.Printf("📁 Wallet saved to: %s\n", walletPath)
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
func InitializeGlobalWallet() error {
	wallet, err := LoadOrCreateNodeWallet()
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
func (nw *NodeWallet) CreateCoinbaseTransaction(blockHeight uint64, reward uint64) *Transaction {
	return CreateCoinbaseTransaction(nw.Address, blockHeight, reward)
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
func (nw *NodeWallet) CreateAndSignCoinbaseTransaction(blockHeight uint64, reward uint64) (*Transaction, error) {
	tx := nw.CreateCoinbaseTransaction(blockHeight, reward)

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
	// Load wallet data for creation time
	walletData, _, err := LoadWalletData(nw.Path)
	created := int64(0)
	if err == nil {
		created = walletData.Created
	}

	return map[string]interface{}{
		"address":       nw.Address.String(),
		"address_short": nw.Address.String()[:16] + "...",
		"path":          nw.Path,
		"created":       created,
		"version":       1,
	}
}

// ValidateWalletFile validates the integrity of a wallet file
func ValidateWalletFile(path string) error {
	_, _, err := LoadWalletData(path)
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
