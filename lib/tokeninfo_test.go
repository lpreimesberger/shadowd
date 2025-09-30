package lib

import (
	"testing"
	"time"
)

func TestGenesisTokenInfo(t *testing.T) {
	genesis := GenesisTokenInfo()

	// Test basic properties
	if genesis.Name != "Shadow" {
		t.Errorf("Expected name 'Shadow', got %s", genesis.Name)
	}

	if genesis.Ticker != "SHADOW" {
		t.Errorf("Expected ticker 'SHADOW', got %s", genesis.Ticker)
	}

	if genesis.TotalSupply != 2100000000000000 {
		t.Errorf("Expected total supply 2100000000000000, got %d", genesis.TotalSupply)
	}

	if genesis.Decimals != 8 {
		t.Errorf("Expected decimals 8, got %d", genesis.Decimals)
	}

	if genesis.MeltValuePerToken != 0 {
		t.Errorf("Expected melt value 0 for base token, got %d", genesis.MeltValuePerToken)
	}

	if genesis.CreationTime != 1704067200 {
		t.Errorf("Expected fixed creation time, got %d", genesis.CreationTime)
	}

	// Test token ID is calculated
	if genesis.TokenID == "" {
		t.Error("Genesis token ID should not be empty")
	}

	if len(genesis.TokenID) != 64 {
		t.Errorf("Token ID should be 64 hex characters, got %d", len(genesis.TokenID))
	}

	// Test that genesis token is base token
	if !genesis.IsBaseToken() {
		t.Error("Genesis token should be identified as base token")
	}

	// Test validation
	if err := genesis.Validate(); err != nil {
		t.Errorf("Genesis token should be valid: %v", err)
	}
}

func TestTokenIDCalculation(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	tokenInfo := &TokenInfo{
		Name:              "Test Token",
		Ticker:            "TEST",
		TotalSupply:       1000000,
		Decimals:          4,
		MeltValuePerToken: 100,
		CreatorAddress:    kp.Address(),
		CreationTime:      time.Now().Unix(),
	}

	// Calculate token ID
	tokenID1, err := tokenInfo.CalculateTokenID()
	if err != nil {
		t.Fatalf("Failed to calculate token ID: %v", err)
	}

	// Should be deterministic
	tokenID2, err := tokenInfo.CalculateTokenID()
	if err != nil {
		t.Fatalf("Failed to calculate token ID second time: %v", err)
	}

	if tokenID1 != tokenID2 {
		t.Error("Token ID calculation should be deterministic")
	}

	// Should be 64 hex characters (32 bytes * 2)
	if len(tokenID1) != 64 {
		t.Errorf("Token ID should be 64 hex characters, got %d", len(tokenID1))
	}

	// Different tokens should have different IDs
	differentToken := &TokenInfo{
		Name:              "Different Token",
		Ticker:            "DIFF",
		TotalSupply:       2000000,
		Decimals:          4,
		MeltValuePerToken: 100,
		CreatorAddress:    kp.Address(),
		CreationTime:      tokenInfo.CreationTime,
	}

	differentID, err := differentToken.CalculateTokenID()
	if err != nil {
		t.Fatalf("Failed to calculate different token ID: %v", err)
	}

	if tokenID1 == differentID {
		t.Error("Different tokens should have different token IDs")
	}
}

func TestTokenInfoValidation(t *testing.T) {
	kp, _ := GenerateKeyPair()

	tests := []struct {
		name      string
		tokenInfo *TokenInfo
		shouldErr bool
		errMsg    string
	}{
		{
			name: "valid token",
			tokenInfo: &TokenInfo{
				Name:              "Valid Token",
				Ticker:            "VALID",
				TotalSupply:       1000000,
				Decimals:          8,
				MeltValuePerToken: 100,
				CreatorAddress:    kp.Address(),
				CreationTime:      time.Now().Unix(),
			},
			shouldErr: false,
		},
		{
			name: "empty name",
			tokenInfo: &TokenInfo{
				Name:           "",
				Ticker:         "EMPTY",
				TotalSupply:    1000000,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token name must be 1-64 characters",
		},
		{
			name: "name too long",
			tokenInfo: &TokenInfo{
				Name:           "This is a very long token name that exceeds the maximum allowed length of 64 characters",
				Ticker:         "LONG",
				TotalSupply:    1000000,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token name must be 1-64 characters",
		},
		{
			name: "empty ticker",
			tokenInfo: &TokenInfo{
				Name:           "Empty Ticker",
				Ticker:         "",
				TotalSupply:    1000000,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token ticker must be 1-16 characters",
		},
		{
			name: "invalid ticker format",
			tokenInfo: &TokenInfo{
				Name:           "Invalid Ticker",
				Ticker:         "invalid-ticker",
				TotalSupply:    1000000,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token ticker must contain only uppercase letters",
		},
		{
			name: "zero total supply",
			tokenInfo: &TokenInfo{
				Name:           "Zero Supply",
				Ticker:         "ZERO",
				TotalSupply:    0,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token total supply must be greater than zero",
		},
		{
			name: "too many decimals",
			tokenInfo: &TokenInfo{
				Name:           "Too Many Decimals",
				Ticker:         "DECIMAL",
				TotalSupply:    1000000,
				Decimals:       19,
				CreatorAddress: kp.Address(),
				CreationTime:   time.Now().Unix(),
			},
			shouldErr: true,
			errMsg:    "token decimals cannot exceed 18",
		},
		{
			name: "invalid creation time",
			tokenInfo: &TokenInfo{
				Name:           "Invalid Time",
				Ticker:         "TIME",
				TotalSupply:    1000000,
				Decimals:       8,
				CreatorAddress: kp.Address(),
				CreationTime:   0,
			},
			shouldErr: true,
			errMsg:    "token creation time must be positive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.tokenInfo.Validate()

			if test.shouldErr {
				if err == nil {
					t.Errorf("Expected validation error but got none")
				} else if test.errMsg != "" && err.Error()[:len(test.errMsg)] != test.errMsg {
					t.Errorf("Expected error message to start with %q, got %q", test.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error but got: %v", err)
				}
			}
		})
	}
}

func TestCreateCustomToken(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	tokenInfo, err := CreateCustomToken(
		"Custom Token",
		"CUSTOM",
		1000000000, // 10 tokens with 8 decimals
		8,
		500, // 0.000005 SHADOW melt value per token
		kp.Address(),
	)

	if err != nil {
		t.Fatalf("Failed to create custom token: %v", err)
	}

	if tokenInfo.TokenID == "" {
		t.Error("Custom token should have calculated token ID")
	}

	if tokenInfo.IsBaseToken() {
		t.Error("Custom token should not be identified as base token")
	}

	// Test staking requirement calculation
	stakingReq := tokenInfo.CalculateStakingRequirement(100000000) // 1 token
	expectedStaking := uint64(100000000 * 500)                     // 1 token * melt value
	if stakingReq != expectedStaking {
		t.Errorf("Expected staking requirement %d, got %d", expectedStaking, stakingReq)
	}

	// Test melt value calculation
	meltValue := tokenInfo.CalculateMeltValue(100000000) // 1 token
	expectedMelt := uint64(100000000 * 500)              // 1 token * melt value
	if meltValue != expectedMelt {
		t.Errorf("Expected melt value %d, got %d", expectedMelt, meltValue)
	}
}

func TestFormatSupply(t *testing.T) {
	tests := []struct {
		name        string
		totalSupply uint64
		decimals    uint8
		ticker      string
		expected    string
	}{
		{
			name:        "no decimals",
			totalSupply: 1000,
			decimals:    0,
			ticker:      "NODEC",
			expected:    "1000 NODEC",
		},
		{
			name:        "with decimals",
			totalSupply: 100000000, // 1.0 with 8 decimals
			decimals:    8,
			ticker:      "DECIMAL",
			expected:    "1.00000000 DECIMAL",
		},
		{
			name:        "fractional",
			totalSupply: 150000000, // 1.5 with 8 decimals
			decimals:    8,
			ticker:      "FRAC",
			expected:    "1.50000000 FRAC",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tokenInfo := &TokenInfo{
				TotalSupply: test.totalSupply,
				Decimals:    test.decimals,
				Ticker:      test.ticker,
			}

			result := tokenInfo.FormatSupply()
			if result != test.expected {
				t.Errorf("Expected formatted supply %s, got %s", test.expected, result)
			}
		})
	}
}

func TestTokenRegistry(t *testing.T) {
	registry := NewTokenRegistry()

	// Should start with genesis token
	if registry.GetTokenCount() != 1 {
		t.Errorf("Expected 1 token in new registry, got %d", registry.GetTokenCount())
	}

	genesisTokenID := registry.GetGenesisTokenID()
	if genesisTokenID == "" {
		t.Error("Genesis token ID should not be empty")
	}

	// Should be able to find genesis token
	genesis, found := registry.GetToken(genesisTokenID)
	if !found {
		t.Error("Should find genesis token")
	}

	if !genesis.IsBaseToken() {
		t.Error("Found token should be base token")
	}

	// Test finding by ticker
	shadowToken, found := registry.GetTokenByTicker("SHADOW")
	if !found {
		t.Error("Should find SHADOW token by ticker")
	}

	if shadowToken.TokenID != genesisTokenID {
		t.Error("SHADOW token should be genesis token")
	}

	// Create and register custom token
	kp, _ := GenerateKeyPair()
	customToken, err := CreateCustomToken("Custom", "CUSTOM", 1000, 2, 100, kp.Address())
	if err != nil {
		t.Fatalf("Failed to create custom token: %v", err)
	}

	err = registry.RegisterToken(customToken)
	if err != nil {
		t.Fatalf("Failed to register custom token: %v", err)
	}

	if registry.GetTokenCount() != 2 {
		t.Errorf("Expected 2 tokens after registration, got %d", registry.GetTokenCount())
	}

	// Should find custom token
	found = registry.ValidateTokenID(customToken.TokenID)
	if !found {
		t.Error("Should validate custom token ID")
	}

	// Test duplicate registration
	err = registry.RegisterToken(customToken)
	if err == nil {
		t.Error("Should fail to register duplicate token")
	}

	// Test global registry functions
	globalRegistry := GetGlobalTokenRegistry()
	if globalRegistry == nil {
		t.Error("Global registry should not be nil")
	}

	genesisFromGlobal := GetGenesisToken()
	if genesisFromGlobal.TokenID != genesisTokenID {
		t.Error("Global genesis token should match registry genesis")
	}
}

func TestASCIIValidation(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"Valid ASCII", true},
		{"Numbers123", true},
		{"Symbols!@#$%", true},
		{"Ümläüts", false}, // Non-ASCII
		{"中文", false},      // Non-ASCII
		{"Émojis😀", false}, // Non-ASCII
		{"", true},         // Empty string is ASCII
	}

	for _, test := range tests {
		result := isASCII(test.input)
		if result != test.expected {
			t.Errorf("isASCII(%q) = %t, expected %t", test.input, result, test.expected)
		}
	}
}
