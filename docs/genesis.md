# Genesis Block and Tendermint Integration

## Overview

Shadowy uses Tendermint for Byzantine Fault Tolerant (BFT) consensus with a custom genesis block that initializes the post-quantum blockchain state.

## Genesis Block Structure

### Tendermint Compatibility
The genesis block follows Tendermint's `GenesisDoc` format:

```go
type GenesisDoc struct {
    GenesisTime     time.Time         `json:"genesis_time"`
    ChainID         string            `json:"chain_id"`
    InitialHeight   int64             `json:"initial_height"`
    ConsensusParams *ConsensusParams  `json:"consensus_params,omitempty"`
    Validators      []GenesisValidator `json:"validators,omitempty"`
    AppHash         []byte            `json:"app_hash"`
    AppState        json.RawMessage   `json:"app_state,omitempty"`
}
```

### Post-Quantum Extensions
Custom consensus parameters for ML-DSA87 support:

```go
type ValidatorParams struct {
    PubKeyTypes []string `json:"pub_key_types"`
}

// Configuration
PubKeyTypes: []string{"ml-dsa87"}
```

## Application State

### ShadowAppState Structure
The `AppState` contains Shadowy-specific initialization:

```go
type ShadowAppState struct {
    GenesisToken  *TokenInfo            `json:"genesis_token"`
    InitialUTXOs  []*UTXO              `json:"initial_utxos"`
    TokenRegistry map[string]*TokenInfo `json:"token_registry"`
    NetworkParams NetworkParams        `json:"network_params"`
}
```

### Genesis Token (SHADOW)
```json
{
  "name": "Shadow",
  "ticker": "SHADOW",
  "total_supply": 2100000000000000,
  "decimals": 8,
  "melt_value_per_token": 0,
  "creator_address": [0,0,0,...],
  "creation_time": 1704067200,
  "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626"
}
```

## Network Parameters

### Economic Parameters
```go
type NetworkParams struct {
    MinTxFee         uint64 `json:"min_tx_fee"`          // 0.00001000 SHADOW
    MinTokenMintFee  uint64 `json:"min_token_mint_fee"`  // 0.00010000 SHADOW
    BlockReward      uint64 `json:"block_reward"`        // 50.00000000 SHADOW
    BlockRewardHalving uint64 `json:"block_reward_halving_interval"` // 210,000 blocks
    MinTokenStaking  uint64 `json:"min_token_staking"`   // 1.00000000 SHADOW
    MaxTokenSupply   uint64 `json:"max_token_supply"`    // 21M SHADOW
    NetworkID        string `json:"network_id"`          // "shadowy-testnet-1"
    MagicBytes       []byte `json:"magic_bytes"`         // [0x53, 0x48, 0x41, 0x44]
}
```

### Consensus Parameters
```go
type ConsensusParams struct {
    Block: BlockParams{
        MaxBytes:   1048576,  // 1MB blocks
        MaxGas:     10000000, // 10M gas limit
        TimeIotaMs: 1000,     // 1 second minimum block time
    },
    Evidence: EvidenceParams{
        MaxAgeNumBlocks: 100000,           // ~27 hours at 1s blocks
        MaxAgeDuration:  48 * time.Hour,   // 48 hours
        MaxBytes:        1048576,          // 1MB evidence
    },
    Validator: ValidatorParams{
        PubKeyTypes: []string{"ml-dsa87"}, // Post-quantum validators
    },
}
```

## Genesis Validator

### Key Generation
```go
func NewTestnetGenesis() (*GenesisDoc, error) {
    // Generate genesis validator key pair
    genesisValidator, err := GenerateKeyPair()
    if err != nil {
        return nil, fmt.Errorf("failed to generate genesis validator key: %w", err)
    }

    // Create validator entry
    validator := GenesisValidator{
        Address: genesisValidator.Address()[:],
        PubKey: PubKey{
            Type:  "ml-dsa87",
            Value: validatorPubKeyBytes,
        },
        Power: 10,
        Name:  "genesis-validator",
    }
}
```

### Pre-mine Allocation
Genesis validator receives initial UTXO:

```go
genesisUTXO := &UTXO{
    TxID:        "genesis_coinbase_0000000000000000000000000000000000000000000000000000000000000000",
    OutputIndex: 0,
    Output:      CreateShadowOutput(genesisValidator.Address(), 1000000000000), // 10,000 SHADOW
    BlockHeight: 0,
    IsSpent:     false,
}
```

## Genesis Creation

### Embedded Testnet Genesis
```go
// Get embedded testnet genesis as JSON string
genesis := lib.GetEmbeddedTestnetGenesis()

// Or create new genesis programmatically
genesisDoc, err := lib.NewTestnetGenesis()
if err != nil {
    log.Fatal("Failed to create genesis:", err)
}
```

### JSON Output Example
```json
{
  "genesis_time": "2023-12-31T18:00:00-06:00",
  "chain_id": "shadowy-testnet-1",
  "initial_height": 1,
  "consensus_params": {
    "validator": {
      "pub_key_types": ["ml-dsa87"]
    }
  },
  "validators": [{
    "address": "Gu8r8ZLTldVwd+SpyrSKoTAN+catiP5b23DwUvAbYIw=",
    "pub_key": {
      "type": "ml-dsa87",
      "value": "lhnVrxQrCmlU2RuiiNVdQrYA..."
    },
    "power": 10,
    "name": "genesis-validator"
  }],
  "app_state": {
    "genesis_token": {...},
    "initial_utxos": [...],
    "token_registry": {...},
    "network_params": {...}
  }
}
```

## Validation Rules

### Genesis Validation
```go
func (gd *GenesisDoc) ValidateGenesis() error {
    // Standard Tendermint validation
    if gd.ChainID == "" {
        return fmt.Errorf("genesis chain_id cannot be empty")
    }

    // Shadowy-specific validation
    var appState ShadowAppState
    if err := json.Unmarshal(gd.AppState, &appState); err != nil {
        return fmt.Errorf("invalid app_state format: %w", err)
    }

    // Validate initial UTXOs match genesis validators
    for i, utxo := range appState.InitialUTXOs {
        if utxo.BlockHeight != 0 {
            return fmt.Errorf("initial UTXO %d must have block height 0", i)
        }

        // Check UTXO recipient is genesis validator
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
            return fmt.Errorf("initial UTXO %d recipient must be genesis validator", i)
        }
    }

    return nil
}
```

### Genesis Block Transaction Validation
Special rules for block height 0:

```go
func ValidateGenesisTransaction(tx *Transaction, blockHeight uint64) error {
    // Genesis block can only contain coinbase transactions
    if blockHeight == 0 && tx.TxType != TxTypeCoinbase {
        return fmt.Errorf("genesis block can only contain coinbase transactions")
    }

    // Genesis coinbase transactions don't need signatures
    if blockHeight == 0 && tx.TxType == TxTypeCoinbase {
        return validateCoinbaseTransaction(tx)
    }

    return ValidateTransaction(tx)
}
```

## Token ID Generation

### SHAKE256 Deterministic IDs
Genesis token uses deterministic ID generation:

```go
func GenesisTokenInfo() *TokenInfo {
    return &TokenInfo{
        Name:         "Shadow",
        Ticker:       "SHADOW",
        TotalSupply:  2100000000000000,
        Decimals:     8,
        MeltValuePerToken: 0,
        CreatorAddress:    Address{}, // Zero address for genesis
        CreationTime:     1704067200, // Fixed timestamp (Jan 1, 2024)
        TokenID:          calculateTokenID(...), // SHAKE256 hash
    }
}
```

### Deterministic Properties
- Same genesis always produces same token ID
- Token ID: `ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626`
- Enables reproducible testnet deployments

## Tendermint Integration

### ABCI Application Interface
Future integration points:

```go
// ABCI methods for Tendermint integration
type ShadowApp struct {
    genesis *GenesisDoc
}

func (app *ShadowApp) InitChain(req abci.RequestInitChain) abci.ResponseInitChain {
    // Initialize blockchain state from genesis
    return abci.ResponseInitChain{}
}

func (app *ShadowApp) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
    // Process block header, create coinbase transaction
    return abci.ResponseBeginBlock{}
}

func (app *ShadowApp) DeliverTx(req abci.RequestDeliverTx) abci.ResponseDeliverTx {
    // Validate and apply transaction
    return abci.ResponseDeliverTx{}
}
```

### Validator Set Management
```go
// Post-quantum validator updates
func (app *ShadowApp) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
    var validatorUpdates []abci.ValidatorUpdate

    // Add new ML-DSA87 validators
    for _, newValidator := range pendingValidators {
        validatorUpdates = append(validatorUpdates, abci.ValidatorUpdate{
            PubKey: abci.PubKey{
                Type: "ml-dsa87",
                Data: newValidator.PublicKey,
            },
            Power: newValidator.VotingPower,
        })
    }

    return abci.ResponseEndBlock{
        ValidatorUpdates: validatorUpdates,
    }
}
```

## Genesis File Management

### Save Genesis to File
```go
genesis, err := lib.NewTestnetGenesis()
if err != nil {
    log.Fatal("Failed to create genesis:", err)
}

err = genesis.SaveToFile("genesis.json")
if err != nil {
    log.Fatal("Failed to save genesis:", err)
}
```

### Load Genesis from File
```go
// Future implementation for loading external genesis
func LoadGenesisFromFile(filename string) (*GenesisDoc, error) {
    data, err := os.ReadFile(filename)
    if err != nil {
        return nil, err
    }

    var genesis GenesisDoc
    err = json.Unmarshal(data, &genesis)
    if err != nil {
        return nil, err
    }

    return &genesis, nil
}
```

## Chain Initialization

### Testnet Deployment
```bash
# Generate genesis file
./shadowy --generate-genesis > genesis.json

# Initialize Tendermint with genesis
tendermint init --home ~/.tendermint
cp genesis.json ~/.tendermint/config/

# Start Tendermint node
tendermint node --home ~/.tendermint
```

### Development Network
```go
// Quick development setup
func StartDevNetwork() {
    genesis := lib.GetEmbeddedTestnetGenesis()

    // Write to Tendermint config directory
    err := os.WriteFile("~/.tendermint/config/genesis.json", []byte(genesis), 0644)
    if err != nil {
        log.Fatal("Failed to write genesis:", err)
    }

    // Start ABCI application
    app := NewShadowApp(genesis)
    server := abci.NewSocketServer(":26658", app)
    server.Start()
}
```

## Security Considerations

### Genesis Validator Security
- Genesis validator controls initial network
- Should use secure key generation
- Consider multi-signature genesis for production

### Network Bootstrap
- Genesis provides initial trusted state
- All nodes must use identical genesis
- Hash verification prevents tampering

### Post-Quantum Considerations
- ML-DSA87 keys provide quantum resistance
- Large key sizes require careful handling
- Future algorithm updates may need genesis migration

## Troubleshooting

### Common Issues
1. **Invalid Genesis Hash**: Ensure all nodes use identical genesis file
2. **Validator Key Mismatch**: Verify ML-DSA87 key format
3. **UTXO Validation Failure**: Check initial UTXO recipients match validators
4. **Network ID Mismatch**: Confirm `chain_id` consistency across nodes

### Debugging
```bash
# Validate genesis file
./shadowy --validate-genesis genesis.json

# Show genesis hash
tendermint show_genesis_hash

# Check validator key
./shadowy --show-validator-key
```