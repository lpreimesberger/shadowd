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
	Transactions  []string      `json:"transactions"`  // Transaction IDs
	Coinbase      *Transaction  `json:"coinbase"`      // Coinbase transaction for block reward
	PreviousHash  string        `json:"previous_hash"`
	Hash          string        `json:"hash"`
	Proposer      string        `json:"proposer"`      // Node that proposed this block
	Votes         []string      `json:"votes"`         // Signatures from nodes that approved
	WinningProof  *ProofOfSpace `json:"winning_proof"` // Proof of space that won this block
	WinnerAddress *Address      `json:"winner_address,omitempty"` // Address to receive block reward
}

// Blockchain represents the chain of blocks
type Blockchain struct {
	blocks    []*Block
	store     *BlockStore
	utxoStore *UTXOStore
	chainLock sync.RWMutex
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

	bc := &Blockchain{
		blocks:    make([]*Block, 0),
		store:     store,
		utxoStore: utxoStore,
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
func (bc *Blockchain) AddBlock(block *Block) error {
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
		fmt.Printf("[Chain] Processed coinbase tx for block %d: %s\n", block.Index, coinbaseID[:16])
	}

	// Persist to storage
	if err := bc.store.SaveBlock(block); err != nil {
		return fmt.Errorf("failed to persist block: %w", err)
	}

	bc.blocks = append(bc.blocks, block)
	fmt.Printf("[Chain] Added block %d to chain, height now: %d\n", block.Index, len(bc.blocks))

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
	BlockHash string `json:"block_hash"`
	BlockIndex uint64 `json:"block_index"`
	Voter     string `json:"voter"`
	Vote      bool   `json:"vote"` // true = approve, false = reject
	Timestamp int64  `json:"timestamp"`
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
