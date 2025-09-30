package lib

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// GenesisDoc defines the initial state of a Shadowy blockchain
// Compatible with Tendermint genesis format
type GenesisDoc struct {
	GenesisTime     time.Time          `json:"genesis_time"`
	ChainID         string             `json:"chain_id"`
	InitialHeight   int64              `json:"initial_height"`
	ConsensusParams *ConsensusParams   `json:"consensus_params,omitempty"`
	Validators      []GenesisValidator `json:"validators,omitempty"`
	AppHash         []byte             `json:"app_hash"`
	AppState        json.RawMessage    `json:"app_state,omitempty"`
}

// ConsensusParams defines consensus parameters for the blockchain
type ConsensusParams struct {
	Block     BlockParams     `json:"block"`
	Evidence  EvidenceParams  `json:"evidence"`
	Validator ValidatorParams `json:"validator"`
	Version   VersionParams   `json:"version,omitempty"`
}

// BlockParams defines block-related consensus parameters
type BlockParams struct {
	MaxBytes   int64 `json:"max_bytes"`
	MaxGas     int64 `json:"max_gas"`
	TimeIotaMs int64 `json:"time_iota_ms"`
}

// EvidenceParams defines evidence-related consensus parameters
type EvidenceParams struct {
	MaxAgeNumBlocks int64         `json:"max_age_num_blocks"`
	MaxAgeDuration  time.Duration `json:"max_age_duration"`
	MaxBytes        int64         `json:"max_bytes"`
}

// ValidatorParams defines validator-related consensus parameters
type ValidatorParams struct {
	PubKeyTypes []string `json:"pub_key_types"`
}

// VersionParams defines version-related consensus parameters
type VersionParams struct {
	AppVersion uint64 `json:"app_version"`
}

// GenesisValidator represents a genesis validator
type GenesisValidator struct {
	Address []byte `json:"address"`
	PubKey  PubKey `json:"pub_key"`
	Power   int64  `json:"power"`
	Name    string `json:"name"`
}

// PubKey represents a public key for validators
type PubKey struct {
	Type  string `json:"type"`
	Value []byte `json:"value"`
}

// ShadowAppState defines the application-specific state for Shadowy blockchain
type ShadowAppState struct {
	// Genesis token information
	GenesisToken *TokenInfo `json:"genesis_token"`

	// Initial UTXO set (coinbase outputs to genesis validators)
	InitialUTXOs []*UTXO `json:"initial_utxos"`

	// Token registry state
	TokenRegistry map[string]*TokenInfo `json:"token_registry"`

	// Network parameters
	NetworkParams NetworkParams `json:"network_params"`
}

// NetworkParams defines network-specific parameters
type NetworkParams struct {
	// Minimum transaction fees
	MinTxFee        uint64 `json:"min_tx_fee"`
	MinTokenMintFee uint64 `json:"min_token_mint_fee"`

	// Block reward parameters
	BlockReward        uint64 `json:"block_reward"`
	BlockRewardHalving uint64 `json:"block_reward_halving_interval"`

	// Token economics
	MinTokenStaking uint64 `json:"min_token_staking"`
	MaxTokenSupply  uint64 `json:"max_token_supply"`

	// Network identifiers
	NetworkID  string `json:"network_id"`
	MagicBytes []byte `json:"magic_bytes"`
}

// DefaultConsensusParams returns default consensus parameters optimized for post-quantum blockchain
func DefaultConsensusParams() *ConsensusParams {
	return &ConsensusParams{
		Block: BlockParams{
			MaxBytes:   1048576,  // 1MB blocks
			MaxGas:     10000000, // 10M gas limit
			TimeIotaMs: 1000,     // 1 second block time minimum
		},
		Evidence: EvidenceParams{
			MaxAgeNumBlocks: 100000,         // ~27 hours at 1s blocks
			MaxAgeDuration:  48 * time.Hour, // 48 hours
			MaxBytes:        1048576,        // 1MB evidence
		},
		Validator: ValidatorParams{
			PubKeyTypes: []string{"ml-dsa87"}, // Post-quantum signature scheme
		},
		Version: VersionParams{
			AppVersion: 1,
		},
	}
}

// DefaultNetworkParams returns default network parameters
func DefaultNetworkParams() NetworkParams {
	return NetworkParams{
		MinTxFee:           1000,             // 0.00001000 SHADOW
		MinTokenMintFee:    10000,            // 0.00010000 SHADOW
		BlockReward:        5000000000,       // 50 SHADOW per block
		BlockRewardHalving: 210000,           // Halving every ~210k blocks
		MinTokenStaking:    100000000,        // 1 SHADOW minimum staking
		MaxTokenSupply:     2100000000000000, // 21M SHADOW max supply
		NetworkID:          "shadowy-testnet-1",
		MagicBytes:         []byte{0x53, 0x48, 0x41, 0x44}, // "SHAD"
	}
}

// NewTestnetGenesis creates a new testnet genesis with embedded configuration
func NewTestnetGenesis() (*GenesisDoc, error) {
	// Generate genesis validator key pair
	genesisValidator, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate genesis validator key: %w", err)
	}

	// Create genesis token
	genesisToken := GenesisTokenInfo()

	// Create initial coinbase UTXO for genesis validator (pre-mine)
	genesisUTXO := &UTXO{
		TxID:        "genesis_coinbase_0000000000000000000000000000000000000000000000000000000000000000",
		OutputIndex: 0,
		Output:      CreateShadowOutput(genesisValidator.Address(), 1000000000000), // 10,000 SHADOW pre-mine
		BlockHeight: 0,
		IsSpent:     false,
	}

	// Create token registry
	tokenRegistry := map[string]*TokenInfo{
		genesisToken.TokenID: genesisToken,
	}

	// Create application state
	appState := &ShadowAppState{
		GenesisToken:  genesisToken,
		InitialUTXOs:  []*UTXO{genesisUTXO},
		TokenRegistry: tokenRegistry,
		NetworkParams: DefaultNetworkParams(),
	}

	appStateBytes, err := json.Marshal(appState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal app state: %w", err)
	}

	// Create genesis validator
	validatorPubKeyBytes, err := PublicKeyToBytes(genesisValidator.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize validator public key: %w", err)
	}

	validatorAddress := genesisValidator.Address()
	validator := GenesisValidator{
		Address: validatorAddress[:],
		PubKey: PubKey{
			Type:  "ml-dsa87",
			Value: validatorPubKeyBytes,
		},
		Power: 10, // Voting power
		Name:  "genesis-validator",
	}

	// Create genesis document
	genesis := &GenesisDoc{
		GenesisTime:     time.Unix(1704067200, 0), // Same as genesis token (Jan 1, 2024)
		ChainID:         "shadowy-testnet-1",
		InitialHeight:   1,
		ConsensusParams: DefaultConsensusParams(),
		Validators:      []GenesisValidator{validator},
		AppHash:         nil, // Will be calculated by Tendermint
		AppState:        appStateBytes,
	}

	return genesis, nil
}

// ToJSON serializes the genesis document to JSON
func (gd *GenesisDoc) ToJSON() ([]byte, error) {
	return json.MarshalIndent(gd, "", "  ")
}

// SaveToFile saves the genesis document to a file
func (gd *GenesisDoc) SaveToFile(filename string) error {
	jsonBytes, err := gd.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal genesis to JSON: %w", err)
	}

	return os.WriteFile(filename, jsonBytes, 0644)
}

// ValidateGenesis validates the genesis document
func (gd *GenesisDoc) ValidateGenesis() error {
	if gd.ChainID == "" {
		return fmt.Errorf("genesis chain_id cannot be empty")
	}

	if gd.GenesisTime.IsZero() {
		return fmt.Errorf("genesis time cannot be zero")
	}

	if len(gd.Validators) == 0 {
		return fmt.Errorf("genesis must have at least one validator")
	}

	// Validate application state
	if len(gd.AppState) == 0 {
		return fmt.Errorf("genesis app_state cannot be empty")
	}

	var appState ShadowAppState
	if err := json.Unmarshal(gd.AppState, &appState); err != nil {
		return fmt.Errorf("invalid app_state format: %w", err)
	}

	// Validate genesis token
	if appState.GenesisToken == nil {
		return fmt.Errorf("genesis app_state must include genesis token")
	}

	if err := appState.GenesisToken.Validate(); err != nil {
		return fmt.Errorf("invalid genesis token: %w", err)
	}

	// Validate initial UTXOs are consistent with genesis validator
	for i, utxo := range appState.InitialUTXOs {
		if utxo.BlockHeight != 0 {
			return fmt.Errorf("initial UTXO %d must have block height 0 (genesis)", i)
		}

		// Check that UTXO recipient matches a genesis validator
		validRecipient := false
		for _, validator := range gd.Validators {
			validatorAddr := Address{}
			copy(validatorAddr[:], validator.Address)
			if utxo.Output.Address == validatorAddr {
				validRecipient = true
				break
			}
		}

		if !validRecipient {
			return fmt.Errorf("initial UTXO %d recipient must be a genesis validator", i)
		}
	}

	return nil
}

// IsGenesisBlock returns true if this is block height 0 (special genesis validation)
func IsGenesisBlock(blockHeight uint64) bool {
	return blockHeight == 0
}

// ValidateGenesisTransaction validates transactions in the genesis block
func ValidateGenesisTransaction(tx *Transaction, blockHeight uint64) error {
	// Genesis block can only contain coinbase transactions
	if blockHeight == 0 && tx.TxType != TxTypeCoinbase {
		return fmt.Errorf("genesis block can only contain coinbase transactions, got %s", tx.TxType.String())
	}

	// Special case: genesis coinbase transactions don't need signatures
	if blockHeight == 0 && tx.TxType == TxTypeCoinbase {
		return validateCoinbaseTransaction(tx)
	}

	// For all other cases, use normal transaction validation
	return ValidateTransaction(tx)
}

// EmbeddedTestnetGenesisJSON contains the static testnet genesis for single-binary distribution
const EmbeddedTestnetGenesisJSON = `{
  "genesis_time": "2024-01-01T00:00:00.000Z",
  "chain_id": "shadowy-testnet-1",
  "initial_height": "1",
  "consensus_params": {
    "block": {
      "max_bytes": 1048576,
      "max_gas": 10000000,
      "time_iota_ms": 1000
    },
    "evidence": {
      "max_age_num_blocks": 100000,
      "max_age_duration": 172800000000000,
      "max_bytes": 1048576
    },
    "validator": {
      "pub_key_types": [
        "ml-dsa87"
      ]
    },
    "version": {
      "app_version": 1
    }
  },
  "validators": [
    {
      "address": "Gu8r8ZLTldVwd+SpyrSKoTAN+catiP5b23DwUvAbYIw=",
      "pub_key": {
        "type": "ml-dsa87",
        "value": "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"
      },
      "power": 10,
      "name": "genesis-validator"
    }
  ],
  "app_hash": null,
  "app_state": {
    "genesis_token": {
      "name": "Shadow",
      "ticker": "SHADOW",
      "total_supply": 2100000000000000,
      "decimals": 8,
      "melt_value_per_token": 0,
      "creator_address": [
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
      ],
      "creation_time": 1704067200,
      "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626"
    },
    "initial_utxos": [
      {
        "tx_id": "genesis_coinbase_0000000000000000000000000000000000000000000000000000000000000000",
        "output_index": 0,
        "output": {
          "amount": 1000000000000,
          "address": [26,239,43,241,146,211,149,213,112,119,228,169,202,180,138,161,48,13,249,198,173,136,254,91,219,112,240,82,240,27,96,140],
          "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
          "token_type": "native",
          "script_pub_key": "dhrvK/GS05XVcHfkqcq0iqEwDfnGrYj+W9tw8FLwG2CM"
        },
        "block_height": 0,
        "is_spent": false
      }
    ],
    "token_registry": {
      "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626": {
        "name": "Shadow",
        "ticker": "SHADOW",
        "total_supply": 2100000000000000,
        "decimals": 8,
        "melt_value_per_token": 0,
        "creator_address": [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        "creation_time": 1704067200,
        "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626"
      }
    },
    "network_params": {
      "min_tx_fee": 1000,
      "min_token_mint_fee": 10000,
      "block_reward": 5000000000,
      "block_reward_halving_interval": 210000,
      "min_token_staking": 100000000,
      "max_token_supply": 2100000000000000,
      "network_id": "shadowy-testnet-1",
      "magic_bytes": "U0hBRA=="
    }
  }
}`

// GetEmbeddedTestnetGenesis returns the embedded testnet genesis as a JSON string
func GetEmbeddedTestnetGenesis() string {
	return EmbeddedTestnetGenesisJSON
}

// ParseEmbeddedGenesis parses the embedded genesis into a GenesisDoc
func ParseEmbeddedGenesis() (*GenesisDoc, error) {
	var genesis GenesisDoc
	if err := json.Unmarshal([]byte(EmbeddedTestnetGenesisJSON), &genesis); err != nil {
		return nil, fmt.Errorf("failed to parse embedded genesis: %w", err)
	}
	return &genesis, nil
}
