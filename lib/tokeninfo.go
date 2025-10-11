package lib

import (
	"fmt"
	"regexp"
	"time"

	"golang.org/x/crypto/sha3"
)

// TokenInfo represents complete token metadata for the blockchain
type TokenInfo struct {
	// Token ID - for custom tokens, this is the TX ID of the minting transaction
	TokenID string `json:"token_id"` // Transaction ID that created this token (or genesis hash)

	// Human-readable information
	Ticker string `json:"ticker"` // 3-32 chars, [A-Za-z0-9] only
	Desc   string `json:"desc"`   // 0-64 chars, [A-Za-z0-9] only (optional description)

	// Token economics
	MaxMint       uint64 `json:"max_mint"`       // Maximum base units (before decimals), max 21 million
	MaxDecimals   uint8  `json:"max_decimals"`   // Number of decimal places (0-8 for SHADOW decimals)
	TotalSupply   uint64 `json:"total_supply"`   // Total token supply in smallest unit (MaxMint * 10^MaxDecimals)
	LockedShadow  uint64 `json:"locked_shadow"`  // SHADOW satoshis locked (1:1 with TotalSupply for custom tokens)
	TotalMelted   uint64 `json:"total_melted"`   // Total tokens melted (for tracking when ticker can be reused)
	MintVersion   uint8  `json:"mint_version"`   // Version of minting logic (currently 0)

	// Creation metadata
	CreatorAddress Address `json:"creator_address"` // Address that created this token
	CreationTime   int64   `json:"creation_time"`   // Unix timestamp when created
}

// GenesisTokenInfo creates the base SHADOW token for the network
func GenesisTokenInfo() *TokenInfo {
	// Use a deterministic address for genesis (all zeros for system)
	var genesisAddr Address // Zero address for system-created tokens

	// Fixed genesis creation time for deterministic token ID
	genesisTime := int64(1704067200) // 2024-01-01 00:00:00 GMT

	maxMint := uint64(21_000_000)  // 21 million base units
	maxDecimals := uint8(8)        // 8 decimal places
	totalSupply := uint64(2_100_000_000_000_000) // 21M * 10^8

	tokenInfo := &TokenInfo{
		TokenID:        calculateGenesisTokenID(), // Deterministic genesis hash
		Ticker:         "SHADOW",
		Desc:           "Base token for Shadow Network",
		MaxMint:        maxMint,
		MaxDecimals:    maxDecimals,
		TotalSupply:    totalSupply,
		LockedShadow:   0, // Base token doesn't lock SHADOW
		TotalMelted:    0, // No tokens melted yet
		MintVersion:    0,
		CreatorAddress: genesisAddr,
		CreationTime:   genesisTime,
	}

	return tokenInfo
}

// calculateGenesisTokenID creates a deterministic token ID for genesis SHADOW token
func calculateGenesisTokenID() string {
	// Use deterministic hash based on genesis parameters
	// This ensures SHADOW token ID is stable across code changes
	genesisTime := int64(1704067200) // 2024-01-01 00:00:00 GMT
	maxMint := uint64(21_000_000)
	maxDecimals := uint8(8)
	ticker := "SHADOW"

	// Hash the genesis parameters
	hashInput := fmt.Sprintf("%s_%d_%d_%d", ticker, genesisTime, maxMint, maxDecimals)
	hash := make([]byte, 32)
	sha3.ShakeSum256(hash, []byte(hashInput))
	return fmt.Sprintf("%x", hash)
}

// IsFullyMelted returns true if all tokens have been melted
func (ti *TokenInfo) IsFullyMelted() bool {
	return ti.TotalMelted >= ti.TotalSupply
}

// Validate checks if the token info is valid per the spec
func (ti *TokenInfo) Validate() error {
	// Validate ticker (3-32 chars, [A-Za-z0-9] only)
	if len(ti.Ticker) < 3 || len(ti.Ticker) > 32 {
		return fmt.Errorf("ticker must be 3-32 characters, got %d", len(ti.Ticker))
	}

	tickerRegex := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	if !tickerRegex.MatchString(ti.Ticker) {
		return fmt.Errorf("ticker must contain only A-Z, a-z, 0-9")
	}

	// Validate desc (0-64 chars, [A-Za-z0-9] only)
	if len(ti.Desc) > 64 {
		return fmt.Errorf("desc must be 0-64 characters, got %d", len(ti.Desc))
	}

	if len(ti.Desc) > 0 {
		descRegex := regexp.MustCompile(`^[A-Za-z0-9]+$`)
		if !descRegex.MatchString(ti.Desc) {
			return fmt.Errorf("desc must contain only A-Z, a-z, 0-9")
		}
	}

	// Validate MAX_MINT (1 to 21 million base units)
	if ti.MaxMint == 0 || ti.MaxMint > 21_000_000 {
		return fmt.Errorf("max_mint must be 1 to 21,000,000, got %d", ti.MaxMint)
	}

	// Validate MAX_DECIMALS (0-8, cannot exceed SHADOW decimals)
	if ti.MaxDecimals > 8 {
		return fmt.Errorf("max_decimals cannot exceed 8 (SHADOW decimals), got %d", ti.MaxDecimals)
	}

	// Validate MINT_VERSION (currently must be 0)
	if ti.MintVersion != 0 {
		return fmt.Errorf("mint_version must be 0, got %d", ti.MintVersion)
	}

	// Validate TotalSupply matches MaxMint * 10^MaxDecimals
	expectedSupply := ti.MaxMint
	for i := uint8(0); i < ti.MaxDecimals; i++ {
		expectedSupply *= 10
	}
	if ti.TotalSupply != expectedSupply {
		return fmt.Errorf("total_supply (%d) doesn't match max_mint * 10^max_decimals (%d)",
			ti.TotalSupply, expectedSupply)
	}

	// For custom tokens, validate staking (LockedShadow must equal TotalSupply)
	if !ti.IsBaseToken() && ti.LockedShadow != ti.TotalSupply {
		return fmt.Errorf("locked_shadow (%d) must equal total_supply (%d) for custom tokens",
			ti.LockedShadow, ti.TotalSupply)
	}

	// Validate creation time
	if ti.CreationTime <= 0 {
		return fmt.Errorf("creation_time must be positive")
	}

	// Validate token ID is not empty
	if ti.TokenID == "" {
		return fmt.Errorf("token_id cannot be empty")
	}

	return nil
}

// SetTokenID sets the token ID (should be TX ID of minting transaction)
func (ti *TokenInfo) SetTokenID(txID string) {
	ti.TokenID = txID
}

// IsBaseToken returns true if this is the base SHADOW token
func (ti *TokenInfo) IsBaseToken() bool {
	genesis := GenesisTokenInfo()
	return ti.TokenID == genesis.TokenID
}

// FormatSupply formats the total supply with proper decimal places
func (ti *TokenInfo) FormatSupply() string {
	if ti.MaxDecimals == 0 {
		return fmt.Sprintf("%d %s", ti.TotalSupply, ti.Ticker)
	}

	divisor := uint64(1)
	for i := uint8(0); i < ti.MaxDecimals; i++ {
		divisor *= 10
	}

	whole := ti.TotalSupply / divisor
	fractional := ti.TotalSupply % divisor

	formatStr := fmt.Sprintf("%%d.%%0%dd %%s", ti.MaxDecimals)
	return fmt.Sprintf(formatStr, whole, fractional, ti.Ticker)
}

// CalculateStakingRequirement calculates required SHADOW staking for minting (1:1 with total supply)
func (ti *TokenInfo) CalculateStakingRequirement() uint64 {
	if ti.IsBaseToken() {
		return 0 // Base token doesn't require staking
	}

	// 1:1 staking: total_supply satoshis of SHADOW required
	return ti.TotalSupply
}

// CalculateMeltValue calculates SHADOW returned when melting tokens (proportional to locked amount)
func (ti *TokenInfo) CalculateMeltValue(tokenAmount uint64) uint64 {
	if ti.IsBaseToken() {
		return 0 // Cannot melt SHADOW
	}

	if ti.TotalSupply == 0 {
		return 0
	}

	// Return proportional SHADOW: (melted_amount / total_supply) * locked_shadow
	return (tokenAmount * ti.LockedShadow) / ti.TotalSupply
}

// CreateCustomToken creates a new custom token (token ID will be set when minting TX is created)
func CreateCustomToken(ticker, desc string, maxMint uint64, maxDecimals uint8, creatorAddress Address) (*TokenInfo, error) {
	// Calculate total supply
	totalSupply := maxMint
	for i := uint8(0); i < maxDecimals; i++ {
		totalSupply *= 10
	}

	tokenInfo := &TokenInfo{
		TokenID:        "", // Will be set to TX ID when minted
		Ticker:         ticker,
		Desc:           desc,
		MaxMint:        maxMint,
		MaxDecimals:    maxDecimals,
		TotalSupply:    totalSupply,
		LockedShadow:   totalSupply, // 1:1 staking
		TotalMelted:    0,
		MintVersion:    0,
		CreatorAddress: creatorAddress,
		CreationTime:   time.Now().Unix(),
	}

	// Validate the token info (except TokenID which will be set later)
	// Temporarily set a dummy TokenID for validation
	tokenInfo.TokenID = "pending"
	if err := tokenInfo.Validate(); err != nil {
		return nil, fmt.Errorf("invalid token info: %w", err)
	}
	tokenInfo.TokenID = "" // Clear it

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

	// Check ticker availability (must be unique unless previous token fully melted)
	if err := tr.CheckTickerAvailable(tokenInfo.Ticker); err != nil {
		return err
	}

	tr.Tokens[tokenInfo.TokenID] = tokenInfo
	return nil
}

// CheckTickerAvailable returns error if ticker is in use by an active token
func (tr *TokenRegistry) CheckTickerAvailable(ticker string) error {
	for _, token := range tr.Tokens {
		if token.Ticker == ticker && !token.IsFullyMelted() {
			return fmt.Errorf("ticker %s already in use by token %s", ticker, token.TokenID)
		}
	}
	return nil
}

// RecordMelt updates the total melted amount for a token
func (tr *TokenRegistry) RecordMelt(tokenID string, amount uint64) error {
	token, exists := tr.Tokens[tokenID]
	if !exists {
		return fmt.Errorf("token %s not found", tokenID)
	}

	token.TotalMelted += amount
	if token.TotalMelted > token.TotalSupply {
		return fmt.Errorf("total melted (%d) exceeds total supply (%d)", token.TotalMelted, token.TotalSupply)
	}

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
