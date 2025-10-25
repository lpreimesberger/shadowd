package lib

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Block represents a single block in the blockchain
type Block struct {
	Index         uint64        `json:"index"`
	Timestamp     int64         `json:"timestamp"`
	Transactions  []string      `json:"transactions"` // Transaction IDs
	Coinbase      *Transaction  `json:"coinbase"`     // Coinbase transaction for block reward
	PreviousHash  string        `json:"previous_hash"`
	Hash          string        `json:"hash"`
	Proposer      string        `json:"proposer"`                 // Node that proposed this block
	Votes         []string      `json:"votes"`                    // Signatures from nodes that approved
	WinningProof  *ProofOfSpace `json:"winning_proof"`            // Proof of space that won this block
	WinnerAddress *Address      `json:"winner_address,omitempty"` // Address to receive block reward
}

// Blockchain represents the chain of blocks
type Blockchain struct {
	blocks            []*Block
	store             *BlockStore
	utxoStore         *UTXOStore
	poolRegistry      *PoolRegistry
	chainLock         sync.RWMutex
	proofPruningDepth int // Keep proofs for last N blocks, 0 = keep all
}

// NewBlockchain creates a new blockchain with a genesis block
func NewBlockchain(storePath string) (*Blockchain, error) {
	blockStorePath := storePath + ".db"
	utxoStorePath := storePath + "_utxo.db"

	fmt.Printf("[Chain] Opening block store at %s...\n", blockStorePath)
	// Open persistent storage
	store, err := NewBlockStore(blockStorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create block store: %w", err)
	}
	fmt.Printf("[Chain] Block store opened successfully\n")

	fmt.Printf("[Chain] Opening UTXO store at %s...\n", utxoStorePath)
	// Open UTXO store
	utxoStore, err := NewUTXOStore(utxoStorePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create UTXO store: %w", err)
	}
	fmt.Printf("[Chain] UTXO store opened successfully\n")

	// Create pool registry
	poolRegistry := NewPoolRegistry()

	bc := &Blockchain{
		blocks:       make([]*Block, 0),
		store:        store,
		utxoStore:    utxoStore,
		poolRegistry: poolRegistry,
	}

	// Try to load existing chain from storage
	fmt.Printf("[Chain] Getting latest height from store...\n")
	latestHeight, err := store.GetLatestHeight()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest height: %w", err)
	}
	fmt.Printf("[Chain] Latest height: %d\n", latestHeight)

	// Check if genesis exists in storage
	fmt.Printf("[Chain] Checking for genesis block...\n")
	hasGenesis, err := store.HasBlock(0)
	if err != nil {
		return nil, fmt.Errorf("failed to check for genesis: %w", err)
	}
	fmt.Printf("[Chain] Has genesis: %v\n", hasGenesis)

	if hasGenesis {
		// Load existing blockchain from storage
		fmt.Printf("[Chain] Loading existing blockchain from storage (height: %d)...\n", latestHeight+1)
		for i := uint64(0); i <= latestHeight; i++ {
			block, err := store.GetBlock(i)
			if err != nil {
				return nil, fmt.Errorf("failed to load block %d: %w", i, err)
			}
			if block == nil {
				return nil, fmt.Errorf("missing block %d in storage", i)
			}
			bc.blocks = append(bc.blocks, block)
		}
		fmt.Printf("[Chain] Loaded %d blocks from storage, latest hash: %s\n",
			len(bc.blocks), bc.blocks[len(bc.blocks)-1].Hash[:16])

		// Rebuild token registry from blockchain
		fmt.Printf("[Chain] Rebuilding token registry from blockchain...\n")
		if err := bc.rebuildTokenRegistry(); err != nil {
			fmt.Printf("[Chain] Warning: Failed to rebuild token registry: %v\n", err)
		}

		// Rebuild pool registry from blockchain
		fmt.Printf("[Chain] Rebuilding pool registry from blockchain...\n")
		if err := bc.rebuildPoolRegistry(); err != nil {
			fmt.Printf("[Chain] Warning: Failed to rebuild pool registry: %v\n", err)
		}
	} else {
		// Create new genesis block
		genesis := &Block{
			Index:        0,
			Timestamp:    1704067200, // Fixed: Jan 1, 2024 00:00:00 UTC
			Transactions: []string{},
			PreviousHash: "0",
			Proposer:     "genesis",
			Votes:        []string{},
		}
		genesis.Hash = bc.calculateBlockHash(genesis)
		bc.blocks = append(bc.blocks, genesis)

		// Save genesis to storage
		if err := store.SaveBlock(genesis); err != nil {
			return nil, fmt.Errorf("failed to save genesis block: %w", err)
		}

		fmt.Printf("[Chain] Created new blockchain with genesis block: %s\n", genesis.Hash)
	}

	return bc, nil
}

// calculateBlockHash computes the hash of a block
func (bc *Blockchain) calculateBlockHash(block *Block) string {
	// Hash everything except the hash itself and votes
	record := fmt.Sprintf("%d%d%v%s%s",
		block.Index,
		block.Timestamp,
		block.Transactions,
		block.PreviousHash,
		block.Proposer,
	)
	h := sha256.Sum256([]byte(record))
	return hex.EncodeToString(h[:])
}

// GetLatestBlock returns the most recent block
func (bc *Blockchain) GetLatestBlock() *Block {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	if len(bc.blocks) == 0 {
		return nil
	}
	return bc.blocks[len(bc.blocks)-1]
}

// GetBlock returns a block by index
func (bc *Blockchain) GetBlock(index uint64) *Block {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	if index >= uint64(len(bc.blocks)) {
		return nil
	}
	return bc.blocks[index]
}

// GetHeight returns the current blockchain height
func (bc *Blockchain) GetHeight() uint64 {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	return uint64(len(bc.blocks))
}

// SetProofPruningDepth configures proof pruning
func (bc *Blockchain) SetProofPruningDepth(depth int) {
	bc.chainLock.Lock()
	defer bc.chainLock.Unlock()
	bc.proofPruningDepth = depth
	if depth == 0 {
		fmt.Printf("[Chain] Proof pruning disabled (museum mode - keeping all proofs)\n")
	} else {
		fmt.Printf("[Chain] Proof pruning enabled: keeping last %d blocks of proofs\n", depth)
	}
}

// PruneOldProofs removes proofs from blocks older than proofPruningDepth
func (bc *Blockchain) PruneOldProofs() error {
	bc.chainLock.Lock()
	defer bc.chainLock.Unlock()

	if bc.proofPruningDepth == 0 {
		return nil // Museum mode - don't prune
	}

	currentHeight := uint64(len(bc.blocks))
	if currentHeight <= uint64(bc.proofPruningDepth) {
		return nil // Not enough blocks yet
	}

	pruneBeforeHeight := currentHeight - uint64(bc.proofPruningDepth)
	prunedCount := 0

	for _, block := range bc.blocks {
		if block.Index < pruneBeforeHeight && block.WinningProof != nil {
			// Strip the proof but keep the block
			block.WinningProof = nil

			// Update in database
			if err := bc.store.SaveBlock(block); err != nil {
				return fmt.Errorf("failed to save pruned block %d: %w", block.Index, err)
			}
			prunedCount++
		}
	}

	if prunedCount > 0 {
		fmt.Printf("[Chain] Pruned proofs from %d blocks (kept last %d blocks)\n",
			prunedCount, bc.proofPruningDepth)
	}

	return nil
}

// ProposeBlock creates a new block proposal
func (bc *Blockchain) ProposeBlock(txIDs []string, proposer string, coinbase *Transaction) *Block {
	latest := bc.GetLatestBlock()

	block := &Block{
		Index:        latest.Index + 1,
		Timestamp:    time.Now().Unix(),
		Transactions: txIDs,
		Coinbase:     coinbase,
		PreviousHash: latest.Hash,
		Proposer:     proposer,
		Votes:        []string{},
	}
	block.Hash = bc.calculateBlockHash(block)

	fmt.Printf("[Chain] Proposed block %d with %d transactions, hash: %s\n",
		block.Index, len(txIDs), block.Hash[:16])

	return block
}

// ValidateBlock checks if a block is valid
func (bc *Blockchain) ValidateBlock(block *Block) error {
	latest := bc.GetLatestBlock()

	// Check index
	if block.Index != latest.Index+1 {
		return fmt.Errorf("invalid block index: expected %d, got %d", latest.Index+1, block.Index)
	}

	// Check previous hash
	if block.PreviousHash != latest.Hash {
		return fmt.Errorf("invalid previous hash: expected %s, got %s", latest.Hash, block.PreviousHash)
	}

	// Verify hash
	expectedHash := bc.calculateBlockHash(block)
	if block.Hash != expectedHash {
		return fmt.Errorf("invalid block hash: expected %s, got %s", expectedHash, block.Hash)
	}

	return nil
}

// AddBlock adds a validated block to the chain
func (bc *Blockchain) AddBlock(block *Block, mempool *Mempool) error {
	// Validate first
	if err := bc.ValidateBlock(block); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	bc.chainLock.Lock()
	defer bc.chainLock.Unlock()

	// Process coinbase transaction if present
	if block.Coinbase != nil {
		if err := bc.utxoStore.StoreTransaction(block.Coinbase, int64(block.Index)); err != nil {
			return fmt.Errorf("failed to store coinbase transaction: %w", err)
		}

		// Create UTXOs for coinbase outputs
		coinbaseID, _ := block.Coinbase.ID()
		for i, output := range block.Coinbase.Outputs {
			utxo := &UTXO{
				TxID:        coinbaseID,
				OutputIndex: uint32(i),
				Output:      output,
				BlockHeight: block.Index,
				IsSpent:     false,
			}
			if err := bc.utxoStore.AddUTXO(utxo); err != nil {
				return fmt.Errorf("failed to add coinbase UTXO: %w", err)
			}
		}
		// Logging disabled for sync performance
		// fmt.Printf("[Chain] Processed coinbase tx for block %d: %s\n", block.Index, coinbaseID[:16])
	}

	// Process regular transactions from mempool
	tokenRegistry := GetGlobalTokenRegistry()
	for _, txID := range block.Transactions {
		// Get transaction from mempool first, then try storage
		var tx *Transaction
		if mempool != nil {
			tx, _ = mempool.GetTransaction(txID)
		}
		if tx == nil {
			// Try storage as fallback (for syncing old blocks)
			tx, _ = bc.utxoStore.GetTransaction(txID)
		}
		if tx == nil {
			fmt.Printf("[Chain] Warning: Transaction %s not found in mempool or storage, skipping\n", txID[:16])
			continue
		}

		// Store transaction at this block height
		if err := bc.utxoStore.StoreTransaction(tx, int64(block.Index)); err != nil {
			fmt.Printf("[Chain] Warning: Failed to store transaction %s: %v\n", txID[:16], err)
			continue
		}

		// Handle token-specific operations FIRST (updates tx.Outputs[].TokenID from PENDING to actual)
		if err := bc.utxoStore.ProcessTokenTransaction(tx, tokenRegistry, bc.poolRegistry, int64(block.Index)); err != nil {
			fmt.Printf("[Chain] Warning: Failed to process token transaction %s: %v\n", txID[:16], err)
		}

		// Spend inputs (mark UTXOs as spent)
		for _, input := range tx.Inputs {
			if err := bc.utxoStore.SpendUTXO(input.PrevTxID, input.OutputIndex); err != nil {
				fmt.Printf("[Chain] Warning: Failed to spend UTXO %s:%d: %v\n", input.PrevTxID[:16], input.OutputIndex, err)
			}
		}

		// Create new UTXOs from outputs (after ProcessTokenTransaction fixed the TokenID)
		for i, output := range tx.Outputs {
			utxo := &UTXO{
				TxID:        txID,
				OutputIndex: uint32(i),
				Output:      output,
				BlockHeight: block.Index,
				IsSpent:     false,
			}
			if err := bc.utxoStore.AddUTXO(utxo); err != nil {
				fmt.Printf("[Chain] Warning: Failed to add UTXO: %v\n", err)
			}
		}

		// Logging disabled for sync performance
		// fmt.Printf("[Chain] Applied transaction %s (type: %s)\n", txID[:16], tx.TxType.String())
	}

	// Persist to storage
	if err := bc.store.SaveBlock(block); err != nil {
		return fmt.Errorf("failed to persist block: %w", err)
	}

	bc.blocks = append(bc.blocks, block)
	fmt.Printf("🟢 [BLOCK ADDED] Height: %d | TxCount: %d | Hash: %s | Proposer: %s\n",
		block.Index, len(block.Transactions), block.Hash[:16], block.Proposer[:16])

	// Purge mempool transactions with now-spent inputs
	if mempool != nil {
		mempool.PurgeInvalidTransactions(bc.utxoStore)
	}

	// Prune old proofs every 100 blocks to avoid overhead
	if bc.proofPruningDepth > 0 && block.Index%100 == 0 {
		go func() {
			if err := bc.PruneOldProofs(); err != nil {
				fmt.Printf("[Chain] Warning: Proof pruning failed: %v\n", err)
			}
		}()
	}

	return nil
}

// rebuildTokenRegistry scans all blocks and rebuilds the token registry from mint transactions
func (bc *Blockchain) rebuildTokenRegistry() error {
	tokenRegistry := GetGlobalTokenRegistry()
	tokenCount := 0

	// Scan all blocks for mint transactions
	for _, block := range bc.blocks {
		// Process all transactions in the block
		for _, txID := range block.Transactions {
			tx, err := bc.utxoStore.GetTransaction(txID)
			if err != nil || tx == nil {
				continue
			}

			// Only process mint transactions
			if tx.TxType == TxTypeMintToken {
				// Extract token metadata
				var mintData TokenMintData
				if err := json.Unmarshal(tx.Data, &mintData); err != nil {
					fmt.Printf("[Chain] Warning: Failed to parse mint data for tx %s: %v\n", txID[:16], err)
					continue
				}

				// Get token creator from first output
				if len(tx.Outputs) == 0 {
					continue
				}
				creator := tx.Outputs[0].Address

				// Create TokenInfo
				tokenInfo, err := CreateCustomToken(
					mintData.Ticker,
					mintData.Desc,
					mintData.MaxMint,
					mintData.MaxDecimals,
					creator,
				)
				if err != nil {
					fmt.Printf("[Chain] Warning: Failed to create token info for %s: %v\n", mintData.Ticker, err)
					continue
				}

				// Set token ID to transaction ID
				tokenInfo.SetTokenID(txID)

				// Register the token
				if err := tokenRegistry.RegisterToken(tokenInfo); err != nil {
					// Ignore duplicate registration errors (token already exists)
					continue
				}

				tokenCount++
				fmt.Printf("[Chain] Restored token: %s (ID: %s)\n", mintData.Ticker, txID[:16])
			}
		}
	}

	fmt.Printf("[Chain] Token registry rebuilt: %d custom tokens restored\n", tokenCount)
	return nil
}

// rebuildPoolRegistry scans all blocks and rebuilds the pool registry from create pool transactions
func (bc *Blockchain) rebuildPoolRegistry() error {
	poolCount := 0

	// Scan all blocks for create pool transactions
	for _, block := range bc.blocks {
		// Process all transactions in the block
		for _, txID := range block.Transactions {
			tx, err := bc.utxoStore.GetTransaction(txID)
			if err != nil || tx == nil {
				continue
			}

			// Only process create pool transactions
			if tx.TxType == TxTypeCreatePool {
				// Extract pool metadata
				var poolData CreatePoolData
				if err := json.Unmarshal(tx.Data, &poolData); err != nil {
					fmt.Printf("[Chain] Warning: Failed to parse pool data for tx %s: %v\n", txID[:16], err)
					continue
				}

				// Calculate LP tokens (same logic as in utxo_store.go)
				lpTokenAmount := CalculateLPTokens(poolData.AmountA, poolData.AmountB)

				// Adjust to match validation (MaxMint * 10^MaxDecimals)
				lpMaxDecimals := uint8(8)
				divisor := uint64(1)
				for i := uint8(0); i < lpMaxDecimals; i++ {
					divisor *= 10
				}
				lpMaxMint := lpTokenAmount / divisor
				if lpMaxMint == 0 {
					lpMaxMint = 1
				}
				expectedSupply := lpMaxMint
				for i := uint8(0); i < lpMaxDecimals; i++ {
					expectedSupply *= 10
				}

				// Create liquidity pool
				pool := &LiquidityPool{
					PoolID:        txID,
					TokenA:        poolData.TokenA,
					TokenB:        poolData.TokenB,
					ReserveA:      poolData.AmountA,
					ReserveB:      poolData.AmountB,
					LPTokenID:     txID,
					LPTokenSupply: expectedSupply,
					FeePercent:    poolData.FeePercent,
					K:             CalculateK(poolData.AmountA, poolData.AmountB),
					CreatedAt:     block.Index,
				}

				// Get token info for LP token ticker generation
				tokenRegistry := GetGlobalTokenRegistry()
				tokenA, existsA := tokenRegistry.GetToken(poolData.TokenA)
				tokenB, existsB := tokenRegistry.GetToken(poolData.TokenB)

				if !existsA || !existsB {
					fmt.Printf("[Chain] Warning: Cannot restore pool %s - tokens not found\n", txID[:16])
					continue
				}

				// Create LP token ticker with pool ID to ensure uniqueness
				lpTokenTicker := GetLPTokenName(tokenA.Ticker, tokenB.Ticker, txID)

				// Create LP token info
				lpTokenInfo := &TokenInfo{
					TokenID:        txID,
					Ticker:         lpTokenTicker,
					Desc:           fmt.Sprintf("%s%sLiquidityPool", tokenA.Ticker, tokenB.Ticker),
					MaxMint:        lpMaxMint,
					MaxDecimals:    lpMaxDecimals,
					TotalSupply:    expectedSupply,
					LockedShadow:   expectedSupply,
					TotalMelted:    0,
					MintVersion:    0,
					CreatorAddress: poolData.PoolAddress,
					CreationTime:   int64(block.Index),
				}

				// Register LP token in token registry
				if err := tokenRegistry.RegisterToken(lpTokenInfo); err != nil {
					// Ignore duplicate registration errors
					fmt.Printf("[Chain] Warning: Failed to register LP token for pool %s: %v\n", txID[:16], err)
				}

				// Register pool
				if err := bc.poolRegistry.RegisterPool(pool); err != nil {
					// Ignore duplicate registration errors
					continue
				}

				poolCount++
				fmt.Printf("[Chain] Restored pool: %s (ID: %s, LP token: %s)\n",
					txID[:16], txID[:16], lpTokenTicker)
			}
		}
	}

	fmt.Printf("[Chain] Pool registry rebuilt: %d pools restored\n", poolCount)
	return nil
}

// AddVote adds a vote signature to a block
func (bc *Blockchain) AddVote(blockHash string, vote string) error {
	bc.chainLock.Lock()
	defer bc.chainLock.Unlock()

	// Find the block by hash
	for _, block := range bc.blocks {
		if block.Hash == blockHash {
			// Check if vote already exists
			for _, v := range block.Votes {
				if v == vote {
					return fmt.Errorf("vote already exists")
				}
			}
			block.Votes = append(block.Votes, vote)
			return nil
		}
	}

	return fmt.Errorf("block not found")
}

// GetBlocks returns all blocks (for debugging/API)
func (bc *Blockchain) GetBlocks() []*Block {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	blocks := make([]*Block, len(bc.blocks))
	copy(blocks, bc.blocks)
	return blocks
}

// GetBlockRange returns a range of blocks (for sync)
func (bc *Blockchain) GetBlockRange(start, end uint64) []*Block {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	if start >= uint64(len(bc.blocks)) {
		return []*Block{}
	}

	if end >= uint64(len(bc.blocks)) {
		end = uint64(len(bc.blocks)) - 1
	}

	blocks := make([]*Block, 0, end-start+1)
	for i := start; i <= end; i++ {
		blocks = append(blocks, bc.blocks[i])
	}
	return blocks
}

// PrintChain prints the blockchain state
func (bc *Blockchain) PrintChain() {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	fmt.Printf("\n[Chain] Blockchain State (height: %d)\n", len(bc.blocks))
	for i, block := range bc.blocks {
		fmt.Printf("  Block %d: hash=%s, txs=%d, votes=%d, proposer=%s\n",
			i, block.Hash[:16], len(block.Transactions), len(block.Votes), block.Proposer)
	}
	fmt.Println()
}

// GetUTXOStore returns the UTXO store for this blockchain
func (bc *Blockchain) GetUTXOStore() *UTXOStore {
	return bc.utxoStore
}

// GetPoolRegistry returns the pool registry for this blockchain
func (bc *Blockchain) GetPoolRegistry() *PoolRegistry {
	return bc.poolRegistry
}

// Close closes the blockchain and its storage
func (bc *Blockchain) Close() error {
	if bc.utxoStore != nil {
		bc.utxoStore.Close()
	}
	if bc.store != nil {
		return bc.store.Close()
	}
	return nil
}

// BlockProposal represents a block proposal message for gossip
type BlockProposal struct {
	Block     *Block `json:"block"`
	Proposer  string `json:"proposer"`
	Timestamp int64  `json:"timestamp"`
}

// BlockVote represents a vote on a proposed block
type BlockVote struct {
	BlockHash  string `json:"block_hash"`
	BlockIndex uint64 `json:"block_index"`
	Voter      string `json:"voter"`
	Vote       bool   `json:"vote"` // true = approve, false = reject
	Timestamp  int64  `json:"timestamp"`
}

// Marshal block to JSON
func (b *Block) ToJSON() ([]byte, error) {
	return json.Marshal(b)
}

// Unmarshal block from JSON
func BlockFromJSON(data []byte) (*Block, error) {
	var block Block
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, err
	}
	return &block, nil
}
