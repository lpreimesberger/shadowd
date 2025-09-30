package lib

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
	db "github.com/cometbft/cometbft-db"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/config"
	tmlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/mempool"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
)

// TendermintConfig holds configuration for the Tendermint node
type TendermintConfig struct {
	HomeDir           string   // Home directory for Tendermint files
	GenesisDoc        string   // Genesis document JSON
	Seeds             []string // Seed nodes
	P2PListenAddr     string   // P2P listen address
	RPCListenAddr     string   // RPC listen address
	LogLevel          string   // Log level
	CreateEmptyBlocks bool     // Whether to create empty blocks
}

// TendermintNode wraps a CometBFT node
type TendermintNode struct {
	node   *node.Node
	config *TendermintConfig
	logger tmlog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	app    *ShadowyApp
}

// DefaultTendermintConfig returns sensible defaults for Tendermint configuration
func DefaultTendermintConfig(blockchainDir string, seeds []string, quiet bool) *TendermintConfig {
	logLevel := "info"
	if quiet {
		// Tendermint has unique logging modules that can be very noisy
		// Common noisy modules: p2p, consensus, state, mempool, blockchain
		logLevel = "error"
	}

	return &TendermintConfig{
		HomeDir:           blockchainDir,
		GenesisDoc:        GetEmbeddedTestnetGenesis(),
		Seeds:             seeds,
		P2PListenAddr:     "tcp://0.0.0.0:26666",
		RPCListenAddr:     "tcp://127.0.0.1:26667",
		LogLevel:          logLevel,
		CreateEmptyBlocks: true, // Enable block production for mining
	}
}

// NewTendermintNode creates a new Tendermint node instance
func NewTendermintNode(tmConfig *TendermintConfig) (*TendermintNode, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create logger
	var logger tmlog.Logger
	if tmConfig.LogLevel == "error" {
		// Redirect Tendermint logs to discard for quiet mode
		logger = tmlog.NewTMLogger(tmlog.NewSyncWriter(io.Discard))
	} else {
		logger = tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))
	}

	// Apply log level filtering
	levelOption, err := tmlog.AllowLevel(tmConfig.LogLevel)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to parse log level: %w", err)
	}
	logger = tmlog.NewFilter(logger, levelOption)

	// Initialize Tendermint configuration
	cfg, err := initializeTendermintConfig(tmConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize Tendermint config: %w", err)
	}

	// Create application (we'll use a default address for now, will be updated later)
	var defaultAddress Address
	app := NewShadowyApp(defaultAddress)

	// Load validator (matches genesis)
	pv := privval.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())

	// Load node key from persistent location (outside blockchain directory)
	persistentNodeKeyFile := "node_key.json"
	nodeKey, err := p2p.LoadOrGenNodeKey(persistentNodeKeyFile)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load node key: %w", err)
	}

	// Copy the persistent node key to the blockchain config directory for Tendermint
	blockchainNodeKeyFile := cfg.NodeKeyFile()
	if err := copyNodeKey(persistentNodeKeyFile, blockchainNodeKeyFile); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to copy node key: %w", err)
	}

	// Create database provider (use in-memory for now to avoid file issues)
	dbProvider := func(ctx *node.DBContext) (db.DB, error) {
		return db.NewMemDB(), nil
	}

	// Create Tendermint node
	tmNode, err := node.NewNode(
		cfg,
		pv,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		node.DefaultGenesisDocProviderFunc(cfg),
		dbProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		logger,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create Tendermint node: %w", err)
	}

	return &TendermintNode{
		node:   tmNode,
		config: tmConfig,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
		app:    app,
	}, nil
}

// Start starts the Tendermint node
func (tn *TendermintNode) Start() error {
	if err := tn.node.Start(); err != nil {
		return fmt.Errorf("failed to start Tendermint node: %w", err)
	}

	// Wait a moment for node to initialize
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Stop stops the Tendermint node
func (tn *TendermintNode) Stop() error {
	tn.cancel()
	if err := tn.node.Stop(); err != nil {
		return fmt.Errorf("failed to stop Tendermint node: %w", err)
	}
	tn.node.Wait()
	return nil
}

// BroadcastTransaction submits a transaction to the mempool via RPC
func (tn *TendermintNode) BroadcastTransaction(txBytes []byte) error {
	if tn.node == nil {
		return fmt.Errorf("Tendermint node not started")
	}

	// Use the node's mempool to add the transaction
	// This will trigger CheckTx for validation
	// Create empty TxInfo for the mempool
	txInfo := mempool.TxInfo{}
	if err := tn.node.Mempool().CheckTx(txBytes, nil, txInfo); err != nil {
		return fmt.Errorf("mempool rejected transaction: %w", err)
	}

	return nil
}

// SetNodeAddress sets the node wallet address for coinbase rewards
func (tn *TendermintNode) SetNodeAddress(address Address) {
	if tn.app != nil {
		tn.app.nodeAddress = address
	}
}

// SetNodePrivateKey sets the node wallet private key for mining
func (tn *TendermintNode) SetNodePrivateKey(privateKey []byte) {
	if tn.app != nil {
		tn.app.nodePrivateKey = privateKey
	}
}

// GetNodeID returns the Tendermint node ID
func (tn *TendermintNode) GetNodeID() (string, error) {
	// Use the same persistent file location
	nodeKey, err := p2p.LoadOrGenNodeKey("node_key.json")
	if err != nil {
		return "", fmt.Errorf("failed to load node key: %w", err)
	}
	return string(nodeKey.ID()), nil
}

// SaveNodeIDToFile saves the node ID to a file
func (tn *TendermintNode) SaveNodeIDToFile(filename string) error {
	nodeID, err := tn.GetNodeID()
	if err != nil {
		return err
	}

	return os.WriteFile(filename, []byte(nodeID), 0644)
}

// initializeTendermintConfig initializes the Tendermint configuration directory and files
func initializeTendermintConfig(tmConfig *TendermintConfig) (*config.Config, error) {
	// Create home directory
	if err := os.MkdirAll(tmConfig.HomeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create home directory: %w", err)
	}

	// Create subdirectories
	dirs := []string{"config", "data"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(tmConfig.HomeDir, dir), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}

	// Initialize Tendermint config
	cfg := config.DefaultConfig()
	cfg.SetRoot(tmConfig.HomeDir)

	// Set P2P configuration
	cfg.P2P.ListenAddress = tmConfig.P2PListenAddr
	// AddrBook path is relative to root, so just use config/addrbook.json
	cfg.P2P.AddrBook = "config/addrbook.json"

	// Set seed nodes
	if len(tmConfig.Seeds) > 0 {
		cfg.P2P.Seeds = ""
		for i, seed := range tmConfig.Seeds {
			if i > 0 {
				cfg.P2P.Seeds += ","
			}
			cfg.P2P.Seeds += seed
		}
	}

	// Set RPC configuration
	cfg.RPC.ListenAddress = tmConfig.RPCListenAddr

	// Set consensus configuration
	cfg.Consensus.CreateEmptyBlocks = tmConfig.CreateEmptyBlocks
	cfg.Consensus.CreateEmptyBlocksInterval = 30 * time.Second
	cfg.Consensus.TimeoutPropose = 3 * time.Second
	cfg.Consensus.TimeoutPrevote = 1 * time.Second
	cfg.Consensus.TimeoutPrecommit = 1 * time.Second
	cfg.Consensus.TimeoutCommit = 1 * time.Second

	// Set mempool configuration
	cfg.Mempool.Size = 5000
	cfg.Mempool.CacheSize = 10000
	cfg.Mempool.MaxTxBytes = 1024 * 1024 // 1MB

	// Set logging
	cfg.LogLevel = tmConfig.LogLevel

	// Load or generate validator key to create matching genesis
	pv := privval.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())
	pubKey, err := pv.GetPubKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get validator public key: %w", err)
	}

	// Generate genesis with the actual validator key
	valAddr := pubKey.Address()
	pubKeyBytes := pubKey.Bytes()
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	minimalGenesis := fmt.Sprintf(`{
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
      "pub_key_types": ["ed25519"]
    },
    "version": {
      "app_version": "1"
    }
  },
  "validators": [
    {
      "address": "%X",
      "pub_key": {
        "type": "tendermint/PubKeyEd25519",
        "value": "%s"
      },
      "power": "10",
      "name": "auto-validator"
    }
  ],
  "app_hash": "",
  "app_state": {}
}`, valAddr, pubKeyB64)

	// Write genesis file
	genesisPath := filepath.Join(tmConfig.HomeDir, "config", "genesis.json")
	if err := os.WriteFile(genesisPath, []byte(minimalGenesis), 0644); err != nil {
		return nil, fmt.Errorf("failed to write genesis file: %w", err)
	}

	// Write config file
	configPath := filepath.Join(tmConfig.HomeDir, "config", "config.toml")
	config.WriteConfigFile(configPath, cfg)

	return cfg, nil
}

// SetupTendermintQuietMode applies additional quiet mode settings to reduce Tendermint noise
func SetupTendermintQuietMode() {
	// Redirect standard Go log output to discard to catch any remaining Tendermint logs
	log.SetOutput(io.Discard)
}

// RestoreTendermintLogging restores normal logging after quiet mode
func RestoreTendermintLogging() {
	log.SetOutput(os.Stderr)
}

// ShadowyApp is a minimal ABCI application for Shadowy blockchain
type ShadowyApp struct {
	// Block height tracking
	height int64
	// Simple state hash for commitment
	stateHash [32]byte
	// UTXO store for persistent state
	utxoStore *UTXOStore
	// Node wallet address for coinbase rewards
	nodeAddress Address
	// Node wallet private key for mining
	nodePrivateKey []byte
}

// NewShadowyApp creates a new Shadowy ABCI application
func NewShadowyApp(nodeAddress Address) *ShadowyApp {
	// Create persistent database for UTXO storage
	// Use LevelDB for production-ready persistence
	var database db.DB
	levelDB, err := db.NewGoLevelDB("utxo", "data")
	if err != nil {
		// Fallback to memory database if file system issues
		log.Printf("Warning: Failed to create persistent database, using memory: %v", err)
		database = db.NewMemDB()
	} else {
		database = levelDB
	}

	return &ShadowyApp{
		utxoStore:   NewUTXOStore(database),
		nodeAddress: nodeAddress,
	}
}

// Info returns application info
func (app *ShadowyApp) Info(req abcitypes.RequestInfo) abcitypes.ResponseInfo {
	return abcitypes.ResponseInfo{
		Data:    "Shadowy Post-Quantum UTXO Blockchain",
		Version: "1.0.0",
	}
}

// InitChain initializes the blockchain
func (app *ShadowyApp) InitChain(req abcitypes.RequestInitChain) abcitypes.ResponseInitChain {
	// Parse genesis state
	var genesisState map[string]interface{}
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		// For now, just log the error but don't fail startup
		fmt.Printf("Warning: failed to parse genesis state: %v\n", err)
	}

	return abcitypes.ResponseInitChain{}
}

// CheckTx validates transactions for mempool
func (app *ShadowyApp) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	// Try to parse as JSON transaction first
	var tx Transaction
	if err := json.Unmarshal(req.Tx, &tx); err == nil {
		// Standard Shadowy transaction - validate it
		if err := ValidateTransaction(&tx); err != nil {
			return abcitypes.ResponseCheckTx{
				Code: 1,
				Log:  fmt.Sprintf("Invalid transaction: %v", err),
			}
		}

		// Validate against UTXO set
		if app.utxoStore != nil {
			if err := app.utxoStore.ValidateTransaction(&tx); err != nil {
				return abcitypes.ResponseCheckTx{
					Code: 1,
					Log:  fmt.Sprintf("UTXO validation failed: %v", err),
				}
			}
		}

		return abcitypes.ResponseCheckTx{Code: 0} // Accept
	}

	// Check if this is a proof-of-space transaction
	if isProofOfSpaceTransaction(req.Tx) {
		if validateProofTransaction(req.Tx) {
			return abcitypes.ResponseCheckTx{Code: 0} // Accept
		} else {
			return abcitypes.ResponseCheckTx{
				Code: 1,
				Log:  "Invalid proof-of-space transaction",
			}
		}
	}

	// Unknown transaction format
	return abcitypes.ResponseCheckTx{
		Code: 1,
		Log:  "Unknown transaction format",
	}
}

// PrepareProposal prepares a proposal for the next block
func (app *ShadowyApp) PrepareProposal(req abcitypes.RequestPrepareProposal) abcitypes.ResponsePrepareProposal {
	return abcitypes.ResponsePrepareProposal{
		Txs: req.Txs, // For now, just include all transactions as-is
	}
}

// ProcessProposal validates a proposed block
func (app *ShadowyApp) ProcessProposal(req abcitypes.RequestProcessProposal) abcitypes.ResponseProcessProposal {
	return abcitypes.ResponseProcessProposal{
		Status: abcitypes.ResponseProcessProposal_ACCEPT,
	}
}

// BeginBlock handles begin block - this is where block rewards and mining happen
func (app *ShadowyApp) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	app.height = req.Header.Height

	// Generate challenge hash for proof of space mining
	challengeHash := generateChallengeHashFromProto(req.Header)

	if farmingDebugMode {
		fmt.Printf("🎯 Block %d challenge hash: %x\n", app.height, challengeHash)
	}

	// Try to generate a mining proof for this block if we have plots
	var bestProof *ProofOfSpace = nil
	if GetPlotCount() > 0 && len(app.nodePrivateKey) > 0 {
		// Generate our mining proof using the node wallet's private key
		proof, err := GenerateProofOfSpace(challengeHash, app.nodePrivateKey)
		if err == nil {
			bestProof = proof
			if farmingDebugMode {
				fmt.Printf("⛏️ Generated mining proof with distance: %d\n", proof.Distance)
			}
		} else if farmingDebugMode {
			fmt.Printf("⚠️ Failed to generate mining proof: %v\n", err)
		}
	} else if farmingDebugMode {
		if GetPlotCount() == 0 {
			fmt.Printf("⚠️ No plot files available for mining\n")
		} else {
			fmt.Printf("⚠️ No node private key available for mining\n")
		}
	}

	if bestProof != nil {
		if farmingDebugMode {
			fmt.Printf("🏆 Best mining proof found with distance: %d\n", bestProof.Distance)
		}

		// Create coinbase transaction for mining reward
		blockReward := uint64(50 * 1e6) // 50 SHADOWY tokens (assuming 6 decimals)

		// Properly derive the miner address from the public key
		minerPubKey := &mldsa87.PublicKey{}
		if err := minerPubKey.UnmarshalBinary(bestProof.MinerPublicKey); err != nil {
			if farmingDebugMode {
				fmt.Printf("❌ Failed to unmarshal miner public key: %v\n", err)
			}
			// Fall back to node address on error
			minerAddress := app.nodeAddress
			coinbaseTx := CreateCoinbaseTransaction(minerAddress, uint64(app.height), blockReward)
			_ = executeTransaction(coinbaseTx, app.height, app.utxoStore)
		} else {
			minerAddress := DeriveAddress(minerPubKey)
			coinbaseTx := CreateCoinbaseTransaction(minerAddress, uint64(app.height), blockReward)

			// Execute the coinbase transaction to award the miner
			if err := executeTransaction(coinbaseTx, app.height, app.utxoStore); err != nil {
				if farmingDebugMode {
					fmt.Printf("❌ Failed to execute coinbase transaction: %v\n", err)
				}
			} else if farmingDebugMode {
				fmt.Printf("💰 Block reward of %d SHADOWY awarded to miner: %s\n", blockReward, minerAddress.String())
			}
		}
	} else {
		// No mining proofs - award block reward to node for testing
		if farmingDebugMode {
			fmt.Printf("⏳ Block %d created - awarding coinbase for testing\n", app.height)
		}

		// Create coinbase transaction for testing (using node wallet address)
		blockReward := uint64(50 * 1e8) // 50 SHADOW tokens (8 decimals)

		// Use the node's wallet address for coinbase rewards
		minerAddress := app.nodeAddress

		coinbaseTx := CreateCoinbaseTransaction(minerAddress, uint64(app.height), blockReward)

		// Execute the coinbase transaction
		if err := executeTransaction(coinbaseTx, app.height, app.utxoStore); err != nil {
			if farmingDebugMode {
				fmt.Printf("❌ Failed to execute test coinbase transaction: %v\n", err)
			}
		} else if farmingDebugMode {
			fmt.Printf("💰 Test block reward of %d awarded to address: %x\n", blockReward, minerAddress[:8])
		}
	}

	return abcitypes.ResponseBeginBlock{}
}

// DeliverTx executes transactions
func (app *ShadowyApp) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	// Try to parse as JSON transaction first
	var tx Transaction
	if err := json.Unmarshal(req.Tx, &tx); err == nil {
		// Standard Shadowy transaction - execute it
		if err := ValidateTransaction(&tx); err != nil {
			return abcitypes.ResponseDeliverTx{
				Code: 1,
				Log:  fmt.Sprintf("Transaction validation failed: %v", err),
			}
		}

		// Execute the transaction (update UTXO set, balances, etc.)
		if err := executeTransaction(&tx, app.height, app.utxoStore); err != nil {
			return abcitypes.ResponseDeliverTx{
				Code: 1,
				Log:  fmt.Sprintf("Transaction execution failed: %v", err),
			}
		}

		if farmingDebugMode {
			fmt.Printf("🔄 Executed transaction: %s\n", GetTransactionSummary(&tx))
		}

		return abcitypes.ResponseDeliverTx{Code: 0}
	}

	// Handle proof-of-space transactions
	if isProofOfSpaceTransaction(req.Tx) {
		if validateProofTransaction(req.Tx) {
			// Process mining proof submission
			if farmingDebugMode {
				fmt.Printf("⛏️ Processed mining proof submission\n")
			}
			return abcitypes.ResponseDeliverTx{Code: 0}
		} else {
			return abcitypes.ResponseDeliverTx{
				Code: 1,
				Log:  "Invalid proof-of-space transaction",
			}
		}
	}

	return abcitypes.ResponseDeliverTx{
		Code: 1,
		Log:  "Unknown transaction format",
	}
}

// EndBlock handles end block
func (app *ShadowyApp) EndBlock(req abcitypes.RequestEndBlock) abcitypes.ResponseEndBlock {
	return abcitypes.ResponseEndBlock{}
}

// Commit commits state changes
func (app *ShadowyApp) Commit() abcitypes.ResponseCommit {
	// Update state hash based on current block height and any state changes
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("block_%d", app.height)))
	hasher.Write(app.stateHash[:])
	newStateHash := sha256.Sum256(hasher.Sum(nil))
	app.stateHash = newStateHash

	if farmingDebugMode {
		fmt.Printf("💾 Committed state for block %d: %x\n", app.height, app.stateHash[:8])
	}

	return abcitypes.ResponseCommit{
		Data: app.stateHash[:],
	}
}

// Query handles state queries
func (app *ShadowyApp) Query(req abcitypes.RequestQuery) abcitypes.ResponseQuery {
	// TODO: Implement state queries
	return abcitypes.ResponseQuery{Code: 0}
}

// Snapshot methods (minimal implementation)
func (app *ShadowyApp) ListSnapshots(req abcitypes.RequestListSnapshots) abcitypes.ResponseListSnapshots {
	return abcitypes.ResponseListSnapshots{}
}

func (app *ShadowyApp) OfferSnapshot(req abcitypes.RequestOfferSnapshot) abcitypes.ResponseOfferSnapshot {
	return abcitypes.ResponseOfferSnapshot{
		Result: abcitypes.ResponseOfferSnapshot_REJECT,
	}
}

func (app *ShadowyApp) LoadSnapshotChunk(req abcitypes.RequestLoadSnapshotChunk) abcitypes.ResponseLoadSnapshotChunk {
	return abcitypes.ResponseLoadSnapshotChunk{}
}

func (app *ShadowyApp) ApplySnapshotChunk(req abcitypes.RequestApplySnapshotChunk) abcitypes.ResponseApplySnapshotChunk {
	return abcitypes.ResponseApplySnapshotChunk{
		Result: abcitypes.ResponseApplySnapshotChunk_UNKNOWN,
	}
}

// generateChallengeHashFromProto creates a challenge hash from ABCI proto header
func generateChallengeHashFromProto(header tmproto.Header) [32]byte {
	// Create challenge from block height, time, and last block hash
	challenge := fmt.Sprintf("%d:%s:%x", header.Height, header.Time, header.LastBlockId.Hash)
	return sha256.Sum256([]byte(challenge))
}

// copyNodeKey copies the persistent node key to the blockchain config directory
func copyNodeKey(srcFile, dstFile string) error {
	// Read the source file
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("failed to read source node key: %w", err)
	}

	// Ensure the destination directory exists
	dstDir := filepath.Dir(dstFile)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Write to the destination file
	if err := os.WriteFile(dstFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write destination node key: %w", err)
	}

	return nil
}

// Mining/Proof-of-Space transaction handling functions

// isProofOfSpaceTransaction checks if a transaction contains a mining proof
func isProofOfSpaceTransaction(tx []byte) bool {
	// Simple check - look for proof-of-space transaction marker
	// In a real implementation, this would parse the transaction structure
	return len(tx) > 10 && string(tx[:10]) == "PROOF_OF_S"
}

// validateProofTransaction validates a proof-of-space transaction
func validateProofTransaction(tx []byte) bool {
	// TODO: Parse the transaction and extract the proof
	// TODO: Validate the proof using ValidateProofOfSpace
	// For now, just return true for demonstration
	return true
}

// findBestProofInTransactions finds the mining proof with the lowest distance
func findBestProofInTransactions(txs [][]byte) *ProofOfSpace {
	var bestProof *ProofOfSpace
	bestDistance := uint64(^uint64(0)) // Max uint64

	for _, tx := range txs {
		if isProofOfSpaceTransaction(tx) {
			// TODO: Parse the transaction to extract the proof
			// proof := parseProofFromTransaction(tx)
			// For now, create a dummy proof for demonstration
			if len(tx) > 0 {
				dummyProof := &ProofOfSpace{
					Distance:       uint64(len(tx) % 1000), // Dummy distance based on tx length
					MinerPublicKey: []byte("dummy_miner_key"),
				}

				if dummyProof.Distance < bestDistance {
					bestDistance = dummyProof.Distance
					bestProof = dummyProof
				}
			}
		}
	}

	return bestProof
}

// executeTransaction processes a transaction and updates state
func executeTransaction(tx *Transaction, blockHeight int64, utxoStore *UTXOStore) error {
	// Validate transaction against UTXO set
	if utxoStore != nil {
		if err := utxoStore.ValidateTransaction(tx); err != nil {
			return fmt.Errorf("UTXO validation failed: %w", err)
		}
	}

	// Validate transaction signatures and structure
	if err := ValidateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Update UTXO set if we have persistent storage
	if utxoStore != nil {
		// Spend input UTXOs (except for coinbase transactions)
		if tx.TxType != TxTypeCoinbase {
			for _, input := range tx.Inputs {
				if err := utxoStore.SpendUTXO(input.PrevTxID, input.OutputIndex); err != nil {
					return fmt.Errorf("failed to spend UTXO %s:%d: %w", input.PrevTxID, input.OutputIndex, err)
				}
			}
		}

		// Create new UTXOs from outputs
		txID, err := tx.ID()
		if err != nil {
			return fmt.Errorf("failed to get transaction ID: %w", err)
		}

		for i, output := range tx.Outputs {
			utxo := &UTXO{
				TxID:        txID,
				OutputIndex: uint32(i),
				Output:      output,
				BlockHeight: uint64(blockHeight),
				IsSpent:     false,
			}

			if err := utxoStore.AddUTXO(utxo); err != nil {
				return fmt.Errorf("failed to add UTXO %s:%d: %w", txID, i, err)
			}
		}

		// Store the transaction
		if err := utxoStore.StoreTransaction(tx, blockHeight); err != nil {
			return fmt.Errorf("failed to store transaction: %w", err)
		}
	}

	return nil
}
