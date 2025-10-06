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
      "max_bytes": "1048576",
      "max_gas": "10000000",
      "time_iota_ms": "1000"
    },
    "evidence": {
      "max_age_num_blocks": "100000",
      "max_age_duration": "172800000000000",
      "max_bytes": "1048576"
    },
    "validator": {
      "pub_key_types": [
        "ed25519"
      ]
    },
    "version": {
      "app_version": "1"
    }
  },
  "validators": [
    {
      "address": "3EAAAF83A6B08647528D2283FD64AEE7E6C49AEA",
      "pub_key": {
        "type": "tendermint/PubKeyEd25519",
        "value": "X0N24gaoq8S7keROPPxsLqLkZukP+NbeHDtUILIqqes="
      },
      "power": "10",
      "name": "genesis-validator"
    }
  ],
  "app_hash": "",
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
