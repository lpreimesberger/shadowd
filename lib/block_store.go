package lib

import (
	"encoding/json"
	"fmt"
	"sync"
)

// BlockStore manages persistent storage for blockchain blocks
type BlockStore struct {
	db    *BoltDBAdapter
	mu    sync.RWMutex
	cache map[uint64]*Block // In-memory cache for recent blocks
}

// Database key prefixes
const (
	blockPrefix       = "block:"      // block:{height} -> Block JSON
	blockHashPrefix   = "blockhash:"  // blockhash:{hash} -> height
	latestHeightKey   = "meta:height" // Latest block height
	genesisHashKey    = "meta:genesis_hash"
)

// NewBlockStore creates a new block store with BoltDB
func NewBlockStore(dbPath string) (*BlockStore, error) {
	db, err := NewBoltDBAdapter(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open BoltDB: %w", err)
	}

	return &BlockStore{
		db:    db,
		cache: make(map[uint64]*Block),
	}, nil
}

// SaveBlock persists a block to storage
func (bs *BlockStore) SaveBlock(block *Block) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Serialize block
	data, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}

	// Save block by height
	heightKey := []byte(fmt.Sprintf("%s%d", blockPrefix, block.Index))
	if err := bs.db.Set(heightKey, data); err != nil {
		return fmt.Errorf("failed to save block: %w", err)
	}

	// Save hash -> height mapping for lookups
	hashKey := []byte(fmt.Sprintf("%s%s", blockHashPrefix, block.Hash))
	heightData := []byte(fmt.Sprintf("%d", block.Index))
	if err := bs.db.Set(hashKey, heightData); err != nil {
		return fmt.Errorf("failed to save block hash mapping: %w", err)
	}

	// Update latest height
	latestData := []byte(fmt.Sprintf("%d", block.Index))
	if err := bs.db.Set([]byte(latestHeightKey), latestData); err != nil {
		return fmt.Errorf("failed to update latest height: %w", err)
	}

	// Save genesis hash if this is block 0
	if block.Index == 0 {
		if err := bs.db.Set([]byte(genesisHashKey), []byte(block.Hash)); err != nil {
			return fmt.Errorf("failed to save genesis hash: %w", err)
		}
	}

	// Cache the block
	bs.cache[block.Index] = block

	return nil
}

// GetBlock retrieves a block by height
func (bs *BlockStore) GetBlock(height uint64) (*Block, error) {
	fmt.Printf("[BlockStore] GetBlock(%d): Acquiring RLock...\n", height)
	bs.mu.RLock()
	fmt.Printf("[BlockStore] GetBlock(%d): RLock acquired\n", height)

	// Check cache first
	if block, exists := bs.cache[height]; exists {
		fmt.Printf("[BlockStore] GetBlock(%d): Found in cache\n", height)
		bs.mu.RUnlock()
		return block, nil
	}
	bs.mu.RUnlock()

	fmt.Printf("[BlockStore] GetBlock(%d): Not in cache, calling db.Get()...\n", height)
	key := []byte(fmt.Sprintf("%s%d", blockPrefix, height))
	data, err := bs.db.Get(key)
	fmt.Printf("[BlockStore] GetBlock(%d): db.Get returned, err=%v\n", height, err)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	if data == nil {
		fmt.Printf("[BlockStore] GetBlock(%d): Block not found (expected for new chain)\n", height)
		return nil, nil // Block not found
	}

	var block Block
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	// Cache the block (acquire write lock)
	bs.mu.Lock()
	bs.cache[height] = &block
	bs.mu.Unlock()

	return &block, nil
}

// GetBlockByHash retrieves a block by its hash
func (bs *BlockStore) GetBlockByHash(hash string) (*Block, error) {
	key := []byte(fmt.Sprintf("%s%s", blockHashPrefix, hash))
	data, err := bs.db.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get block hash mapping: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var height uint64
	_, err = fmt.Sscanf(string(data), "%d", &height)
	if err != nil {
		return nil, err
	}

	return bs.GetBlock(height)
}

// GetLatestHeight returns the latest block height stored
func (bs *BlockStore) GetLatestHeight() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	data, err := bs.db.Get([]byte(latestHeightKey))
	if err != nil {
		return 0, fmt.Errorf("failed to get latest height: %w", err)
	}
	if data == nil {
		return 0, nil // No blocks stored yet
	}

	var height uint64
	_, err = fmt.Sscanf(string(data), "%d", &height)
	return height, err
}

// GetBlockRange retrieves a range of blocks (inclusive)
func (bs *BlockStore) GetBlockRange(start, end uint64) ([]*Block, error) {
	blocks := make([]*Block, 0, end-start+1)

	for height := start; height <= end; height++ {
		block, err := bs.GetBlock(height)
		if err != nil {
			return nil, fmt.Errorf("failed to get block %d: %w", height, err)
		}
		if block == nil {
			break // No more blocks
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// GetGenesisHash returns the hash of the genesis block
func (bs *BlockStore) GetGenesisHash() (string, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	data, err := bs.db.Get([]byte(genesisHashKey))
	if err != nil {
		return "", fmt.Errorf("failed to get genesis hash: %w", err)
	}
	if data == nil {
		return "", nil
	}

	return string(data), nil
}

// HasBlock checks if a block exists at the given height
func (bs *BlockStore) HasBlock(height uint64) (bool, error) {
	block, err := bs.GetBlock(height)
	if err != nil {
		return false, err
	}
	return block != nil, nil
}

// Close closes the database
func (bs *BlockStore) Close() error {
	return bs.db.Close()
}
