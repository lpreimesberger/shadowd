package lib

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v4"
)

// BlockStore manages persistent storage for blockchain blocks
type BlockStore struct {
	db    *badger.DB
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

// NewBlockStore creates a new block store with BadgerDB
func NewBlockStore(dbPath string) (*BlockStore, error) {
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // Disable BadgerDB's verbose logging

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
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

	return bs.db.Update(func(txn *badger.Txn) error {
		// Serialize block
		data, err := json.Marshal(block)
		if err != nil {
			return fmt.Errorf("failed to marshal block: %w", err)
		}

		// Save block by height
		heightKey := []byte(fmt.Sprintf("%s%d", blockPrefix, block.Index))
		if err := txn.Set(heightKey, data); err != nil {
			return fmt.Errorf("failed to save block: %w", err)
		}

		// Save hash -> height mapping for lookups
		hashKey := []byte(fmt.Sprintf("%s%s", blockHashPrefix, block.Hash))
		heightData := []byte(fmt.Sprintf("%d", block.Index))
		if err := txn.Set(hashKey, heightData); err != nil {
			return fmt.Errorf("failed to save block hash mapping: %w", err)
		}

		// Update latest height
		latestData := []byte(fmt.Sprintf("%d", block.Index))
		if err := txn.Set([]byte(latestHeightKey), latestData); err != nil {
			return fmt.Errorf("failed to update latest height: %w", err)
		}

		// Save genesis hash if this is block 0
		if block.Index == 0 {
			if err := txn.Set([]byte(genesisHashKey), []byte(block.Hash)); err != nil {
				return fmt.Errorf("failed to save genesis hash: %w", err)
			}
		}

		// Cache the block
		bs.cache[block.Index] = block

		return nil
	})
}

// GetBlock retrieves a block by height
func (bs *BlockStore) GetBlock(height uint64) (*Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	// Check cache first
	if block, exists := bs.cache[height]; exists {
		return block, nil
	}

	var block *Block
	err := bs.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("%s%d", blockPrefix, height))
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // Block not found
			}
			return fmt.Errorf("failed to get block: %w", err)
		}

		return item.Value(func(val []byte) error {
			block = &Block{}
			if err := json.Unmarshal(val, block); err != nil {
				return fmt.Errorf("failed to unmarshal block: %w", err)
			}
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Cache the block
	if block != nil {
		bs.mu.Lock()
		bs.cache[height] = block
		bs.mu.Unlock()
	}

	return block, nil
}

// GetBlockByHash retrieves a block by its hash
func (bs *BlockStore) GetBlockByHash(hash string) (*Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var height uint64
	err := bs.db.View(func(txn *badger.Txn) error {
		key := []byte(fmt.Sprintf("%s%s", blockHashPrefix, hash))
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return fmt.Errorf("failed to get block hash mapping: %w", err)
		}

		return item.Value(func(val []byte) error {
			_, err := fmt.Sscanf(string(val), "%d", &height)
			return err
		})
	})

	if err != nil {
		return nil, err
	}

	return bs.GetBlock(height)
}

// GetLatestHeight returns the latest block height stored
func (bs *BlockStore) GetLatestHeight() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var height uint64
	err := bs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(latestHeightKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // No blocks stored yet
			}
			return fmt.Errorf("failed to get latest height: %w", err)
		}

		return item.Value(func(val []byte) error {
			_, err := fmt.Sscanf(string(val), "%d", &height)
			return err
		})
	})

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

	var hash string
	err := bs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(genesisHashKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return fmt.Errorf("failed to get genesis hash: %w", err)
		}

		return item.Value(func(val []byte) error {
			hash = string(val)
			return nil
		})
	})

	return hash, err
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

// RunGarbageCollection manually triggers BadgerDB's garbage collection
func (bs *BlockStore) RunGarbageCollection() error {
	return bs.db.RunValueLogGC(0.5) // Discard 50% of stale data
}
