package lib

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/crypto/sha3"
)

// TokenInfo represents complete token metadata for the blockchain
type TokenInfo struct {
	// Human-readable information
	Name   string `json:"name"`   // ASCII only, max 64 characters
	Ticker string `json:"ticker"` // ASCII only, max 16 characters

	// Token economics
	TotalSupply       uint64 `json:"total_supply"`         // Total token supply (in smallest unit)
	Decimals          uint8  `json:"decimals"`             // Number of decimal places
	MeltValuePerToken uint64 `json:"melt_value_per_token"` // Base currency returned per token when melted

	// Creation metadata
	CreatorAddress Address `json:"creator_address"` // Address that created this token
	CreationTime   int64   `json:"creation_time"`   // GMT creation timestamp for uniqueness

	// Computed identifier (not serialized in hash calculation)
	TokenID string `json:"token_id,omitempty"` // SHAKE256 hash of serialized data
}

// GenesisTokenInfo creates the base SHADOW token for the network
func GenesisTokenInfo() *TokenInfo {
	// Use a deterministic address for genesis (all zeros for system)
	var genesisAddr Address // Zero address for system-created tokens

	// Fixed genesis creation time for deterministic token ID
	genesisTime := int64(1704067200) // 2024-01-01 00:00:00 GMT

	tokenInfo := &TokenInfo{
		Name:              "Shadow",
		Ticker:            "SHADOW",
		TotalSupply:       2100000000000000, // 21 million SHADOW with 8 decimals
		Decimals:          8,
		MeltValuePerToken: 0, // Base currency has no melt value
		CreatorAddress:    genesisAddr,
		CreationTime:      genesisTime,
	}

	// Calculate token ID
	tokenID, err := tokenInfo.CalculateTokenID()
	if err != nil {
		panic(fmt.Sprintf("Failed to calculate genesis token ID: %v", err))
	}

	tokenInfo.TokenID = tokenID
	return tokenInfo
}

// CalculateTokenID computes the SHAKE256 hash of the serialized token data
func (ti *TokenInfo) CalculateTokenID() (string, error) {
	// Create a copy without the TokenID field for hashing
	hashData := struct {
		Name              string  `json:"name"`
		Ticker            string  `json:"ticker"`
		TotalSupply       uint64  `json:"total_supply"`
		Decimals          uint8   `json:"decimals"`
		MeltValuePerToken uint64  `json:"melt_value_per_token"`
		CreatorAddress    Address `json:"creator_address"`
		CreationTime      int64   `json:"creation_time"`
	}{
		Name:              ti.Name,
		Ticker:            ti.Ticker,
		TotalSupply:       ti.TotalSupply,
		Decimals:          ti.Decimals,
		MeltValuePerToken: ti.MeltValuePerToken,
		CreatorAddress:    ti.CreatorAddress,
		CreationTime:      ti.CreationTime,
	}

	// Serialize to JSON for consistent hashing
	jsonData, err := json.Marshal(hashData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token info: %w", err)
	}

	// Use SHAKE256 with 32-byte output (same as SHA256 length)
	hash := make([]byte, 32)
	sha3.ShakeSum256(hash, jsonData)

	return fmt.Sprintf("%x", hash), nil
}

// Validate checks if the token info is valid
func (ti *TokenInfo) Validate() error {
	// Validate name
	if len(ti.Name) == 0 || len(ti.Name) > 64 {
		return fmt.Errorf("token name must be 1-64 characters, got %d", len(ti.Name))
	}

	if !isASCII(ti.Name) {
		return fmt.Errorf("token name must be ASCII only")
	}

	// Validate ticker
	if len(ti.Ticker) == 0 || len(ti.Ticker) > 16 {
		return fmt.Errorf("token ticker must be 1-16 characters, got %d", len(ti.Ticker))
	}

	if !isASCII(ti.Ticker) {
		return fmt.Errorf("token ticker must be ASCII only")
	}

	// Validate ticker format (alphanumeric and underscores only)
	tickerRegex := regexp.MustCompile(`^[A-Z0-9_]+$`)
	if !tickerRegex.MatchString(ti.Ticker) {
		return fmt.Errorf("token ticker must contain only uppercase letters, numbers, and underscores")
	}

	// Validate economics
	if ti.TotalSupply == 0 {
		return fmt.Errorf("token total supply must be greater than zero")
	}

	if ti.Decimals > 18 {
		return fmt.Errorf("token decimals cannot exceed 18")
	}

	// Validate creation time
	if ti.CreationTime <= 0 {
		return fmt.Errorf("token creation time must be positive")
	}

	// Validate that calculated token ID matches stored ID (if present)
	if ti.TokenID != "" {
		calculatedID, err := ti.CalculateTokenID()
		if err != nil {
			return fmt.Errorf("failed to calculate token ID for validation: %w", err)
		}

		if ti.TokenID != calculatedID {
			return fmt.Errorf("token ID mismatch: stored %s, calculated %s", ti.TokenID, calculatedID)
		}
	}

	return nil
}

// SetTokenID calculates and sets the token ID
func (ti *TokenInfo) SetTokenID() error {
	tokenID, err := ti.CalculateTokenID()
	if err != nil {
		return err
	}

	ti.TokenID = tokenID
	return nil
}

// IsBaseToken returns true if this is the base SHADOW token
func (ti *TokenInfo) IsBaseToken() bool {
	genesis := GenesisTokenInfo()
	return ti.TokenID == genesis.TokenID
}

// FormatSupply formats the total supply with proper decimal places
func (ti *TokenInfo) FormatSupply() string {
	if ti.Decimals == 0 {
		return fmt.Sprintf("%d %s", ti.TotalSupply, ti.Ticker)
	}

	divisor := uint64(1)
	for i := uint8(0); i < ti.Decimals; i++ {
		divisor *= 10
	}

	whole := ti.TotalSupply / divisor
	fractional := ti.TotalSupply % divisor

	formatStr := fmt.Sprintf("%%d.%%0%dd %%s", ti.Decimals)
	return fmt.Sprintf(formatStr, whole, fractional, ti.Ticker)
}

// CalculateStakingRequirement calculates required base token staking for minting
func (ti *TokenInfo) CalculateStakingRequirement(mintAmount uint64) uint64 {
	if ti.IsBaseToken() {
		return 0 // Base token doesn't require staking
	}

	// Staking requirement is proportional to melt value
	// If melt value per token is 0, require 1 satoshi per token unit to prevent dust
	if ti.MeltValuePerToken == 0 {
		return mintAmount
	}

	return mintAmount * ti.MeltValuePerToken
}

// CalculateMeltValue calculates base currency returned when melting tokens
func (ti *TokenInfo) CalculateMeltValue(tokenAmount uint64) uint64 {
	if ti.IsBaseToken() {
		return tokenAmount // Base token melts to itself
	}

	return tokenAmount * ti.MeltValuePerToken
}

// CreateCustomToken creates a new custom token
func CreateCustomToken(name, ticker string, totalSupply uint64, decimals uint8,
	meltValuePerToken uint64, creatorAddress Address) (*TokenInfo, error) {

	tokenInfo := &TokenInfo{
		Name:              name,
		Ticker:            ticker,
		TotalSupply:       totalSupply,
		Decimals:          decimals,
		MeltValuePerToken: meltValuePerToken,
		CreatorAddress:    creatorAddress,
		CreationTime:      time.Now().Unix(),
	}

	// Validate the token info
	if err := tokenInfo.Validate(); err != nil {
		return nil, fmt.Errorf("invalid token info: %w", err)
	}

	// Calculate and set token ID
	if err := tokenInfo.SetTokenID(); err != nil {
		return nil, fmt.Errorf("failed to set token ID: %w", err)
	}

	return tokenInfo, nil
}

// TokenRegistry represents a collection of token information
type TokenRegistry struct {
	Tokens map[string]*TokenInfo `json:"tokens"` // TokenID -> TokenInfo
}

// NewTokenRegistry creates a new token registry with genesis token
func NewTokenRegistry() *TokenRegistry {
	genesis := GenesisTokenInfo()

	registry := &TokenRegistry{
		Tokens: make(map[string]*TokenInfo),
	}

	registry.Tokens[genesis.TokenID] = genesis
	return registry
}

// RegisterToken adds a new token to the registry
func (tr *TokenRegistry) RegisterToken(tokenInfo *TokenInfo) error {
	if err := tokenInfo.Validate(); err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	if _, exists := tr.Tokens[tokenInfo.TokenID]; exists {
		return fmt.Errorf("token %s already registered", tokenInfo.TokenID)
	}

	tr.Tokens[tokenInfo.TokenID] = tokenInfo
	return nil
}

// GetToken retrieves token info by ID
func (tr *TokenRegistry) GetToken(tokenID string) (*TokenInfo, bool) {
	token, exists := tr.Tokens[tokenID]
	return token, exists
}

// GetGenesisTokenID returns the genesis SHADOW token ID
func (tr *TokenRegistry) GetGenesisTokenID() string {
	genesis := GenesisTokenInfo()
	return genesis.TokenID
}

// ValidateTokenID checks if a token ID exists in the registry
func (tr *TokenRegistry) ValidateTokenID(tokenID string) bool {
	_, exists := tr.Tokens[tokenID]
	return exists
}

// GetTokenByTicker finds a token by its ticker symbol
func (tr *TokenRegistry) GetTokenByTicker(ticker string) (*TokenInfo, bool) {
	for _, token := range tr.Tokens {
		if token.Ticker == ticker {
			return token, true
		}
	}
	return nil, false
}

// ListTokens returns all registered tokens
func (tr *TokenRegistry) ListTokens() []*TokenInfo {
	var tokens []*TokenInfo
	for _, token := range tr.Tokens {
		tokens = append(tokens, token)
	}
	return tokens
}

// GetTokenCount returns the number of registered tokens
func (tr *TokenRegistry) GetTokenCount() int {
	return len(tr.Tokens)
}

// Helper function to check if string is ASCII
func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// Global token registry instance
var globalTokenRegistry *TokenRegistry

// InitializeTokenRegistry initializes the global token registry
func InitializeTokenRegistry() {
	globalTokenRegistry = NewTokenRegistry()
}

// GetGlobalTokenRegistry returns the global token registry
func GetGlobalTokenRegistry() *TokenRegistry {
	if globalTokenRegistry == nil {
		InitializeTokenRegistry()
	}
	return globalTokenRegistry
}

// GetGenesisToken returns the genesis SHADOW token info
func GetGenesisToken() *TokenInfo {
	return GenesisTokenInfo()
}

// IsValidTokenID checks if a token ID is valid (exists in global registry)
func IsValidTokenID(tokenID string) bool {
	registry := GetGlobalTokenRegistry()
	return registry.ValidateTokenID(tokenID)
}
