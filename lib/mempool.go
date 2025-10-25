package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
)

const (
	MempoolTopic       = "shadowy-mempool"
	MaxTransactionSize = 256 * 1024 // 256 KB max per transaction
)

// MempoolEntry tracks a transaction and its metadata
type MempoolEntry struct {
	Tx             *Transaction
	AddedAtBlock   uint64    // Block height when tx was added
	AddedTimestamp time.Time // Timestamp when tx was added
	SizeBytes      int       // Approximate size in bytes
}

// Mempool represents a shared transaction mempool
type Mempool struct {
	entries       map[string]*MempoolEntry // txID -> entry
	txLock        sync.RWMutex
	pubsub        *pubsub.PubSub
	topic         *pubsub.Topic
	sub           *pubsub.Subscription
	ctx           context.Context
	cancel        context.CancelFunc
	expiryBlocks  int // Transactions expire after this many blocks
	maxSizeBytes  int // Maximum mempool size in bytes
	currentHeight uint64
}

// MempoolMessage is the gossip message format
type MempoolMessage struct {
	Type        string       `json:"type"` // "add_tx"
	Transaction *Transaction `json:"transaction"`
	Timestamp   int64        `json:"timestamp"`
}

// NewMempool creates a new mempool with gossip support
func NewMempool(h host.Host, ps *pubsub.PubSub, expiryBlocks int, maxSizeMB int) (*Mempool, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Join the mempool topic
	topic, err := ps.Join(MempoolTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to join topic: %w", err)
	}

	// Subscribe to the topic
	sub, err := topic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	mp := &Mempool{
		entries:       make(map[string]*MempoolEntry),
		pubsub:        ps,
		topic:         topic,
		sub:           sub,
		ctx:           ctx,
		cancel:        cancel,
		expiryBlocks:  expiryBlocks,
		maxSizeBytes:  maxSizeMB * 1024 * 1024, // Convert MB to bytes
		currentHeight: 0,
	}

	// Start listening for mempool messages
	go mp.listenForMessages()

	fmt.Printf("[Mempool] Created: expiry=%d blocks, maxSize=%dMB\n", expiryBlocks, maxSizeMB)
	return mp, nil
}

// listenForMessages processes incoming mempool messages
func (mp *Mempool) listenForMessages() {
	for {
		msg, err := mp.sub.Next(mp.ctx)
		if err != nil {
			if mp.ctx.Err() != nil {
				// Context cancelled, shutting down
				return
			}
			fmt.Printf("[Mempool] Error reading message: %v\n", err)
			continue
		}

		// Decode message
		var mempoolMsg MempoolMessage
		if err := json.Unmarshal(msg.Data, &mempoolMsg); err != nil {
			fmt.Printf("[Mempool] Failed to decode message: %v\n", err)
			continue
		}

		// Process based on type
		switch mempoolMsg.Type {
		case "add_tx":
			if mempoolMsg.Transaction != nil {
				// Get transaction ID
				txID, err := mempoolMsg.Transaction.ID()
				if err != nil {
					fmt.Printf("[Mempool] Failed to get transaction ID: %v\n", err)
					continue
				}

				// Verify signature before adding to mempool
				if !mp.verifyTransaction(mempoolMsg.Transaction) {
					fmt.Printf("[Mempool] Rejected invalid transaction: %s\n", txID)
					continue
				}

				mp.txLock.Lock()
				// Only add if we don't already have it (avoid duplicates)
				if _, exists := mp.entries[txID]; !exists {
					txSize := mp.estimateTxSize(mempoolMsg.Transaction)
					entry := &MempoolEntry{
						Tx:             mempoolMsg.Transaction,
						AddedAtBlock:   mp.currentHeight,
						AddedTimestamp: time.Now(),
						SizeBytes:      txSize,
					}
					mp.entries[txID] = entry
					fmt.Printf("[Mempool] Added transaction from gossip: %s (total: %d)\n",
						txID, len(mp.entries))

					// Check if we need to evict old transactions
					mp.enforceMemoryLimitLocked()
				}
				mp.txLock.Unlock()
			}
		}
	}
}

// verifyTransaction checks if a transaction has a valid signature
func (mp *Mempool) verifyTransaction(tx *Transaction) bool {
	txID, _ := tx.ID()

	// For coinbase transactions, no signature verification needed
	if tx.TxType == TxTypeCoinbase {
		fmt.Printf("[Mempool] Accepting coinbase transaction %s\n", txID[:16])
		return true
	}

	fmt.Printf("[Mempool] Verifying transaction %s (type: %s)\n", txID[:16], tx.TxType.String())

	// Check if transaction is signed
	if len(tx.Signature) == 0 {
		fmt.Printf("[Mempool] Transaction %s has no signature\n", txID[:16])
		return false
	}

	// Verify the signature using existing ValidateTransaction function
	if err := ValidateTransaction(tx); err != nil {
		fmt.Printf("[Mempool] Transaction %s failed validation: %v\n", txID[:16], err)
		return false
	}

	fmt.Printf("[Mempool] Transaction %s validation passed\n", txID[:16])
	return true
}

// AddTransaction adds a transaction to the mempool and gossips it
func (mp *Mempool) AddTransaction(tx *Transaction) error {
	// Get transaction ID
	txID, err := tx.ID()
	if err != nil {
		return fmt.Errorf("failed to get transaction ID: %w", err)
	}

	// Verify signature first
	if !mp.verifyTransaction(tx) {
		return fmt.Errorf("invalid transaction signature")
	}

	mp.txLock.Lock()
	// Check if we already have it
	if _, exists := mp.entries[txID]; exists {
		mp.txLock.Unlock()
		return fmt.Errorf("transaction already in mempool")
	}

	// Check transaction size limit
	txSize := mp.estimateTxSize(tx)
	if txSize > MaxTransactionSize {
		mp.txLock.Unlock()
		return fmt.Errorf("transaction too large: %d bytes (max %d KB)", txSize, MaxTransactionSize/1024)
	}

	// Check for double-spend: reject if any input is already used by pending tx
	for _, input := range tx.Inputs {
		inputKey := fmt.Sprintf("%s:%d", input.PrevTxID, input.OutputIndex)
		for existingTxID, entry := range mp.entries {
			for _, existingInput := range entry.Tx.Inputs {
				existingKey := fmt.Sprintf("%s:%d", existingInput.PrevTxID, existingInput.OutputIndex)
				if inputKey == existingKey {
					mp.txLock.Unlock()
					return fmt.Errorf("double-spend detected: input %s already used by pending tx %s", inputKey[:16], existingTxID[:16])
				}
			}
		}
	}
	entry := &MempoolEntry{
		Tx:             tx,
		AddedAtBlock:   mp.currentHeight,
		AddedTimestamp: time.Now(),
		SizeBytes:      txSize,
	}
	mp.entries[txID] = entry
	txCount := len(mp.entries)

	// Check if we need to evict old transactions
	mp.enforceMemoryLimitLocked()
	mp.txLock.Unlock()

	fmt.Printf("[Mempool] Added transaction locally: %s (total: %d)\n", txID, txCount)

	// Gossip to other nodes
	msg := MempoolMessage{
		Type:        "add_tx",
		Transaction: tx,
		Timestamp:   time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := mp.topic.Publish(mp.ctx, data); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	fmt.Printf("[Mempool] Gossiped transaction to network: %s\n", txID)
	return nil
}

// GetTransactions returns all transactions in the mempool
func (mp *Mempool) GetTransactions() []*Transaction {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	txs := make([]*Transaction, 0, len(mp.entries))
	for _, entry := range mp.entries {
		txs = append(txs, entry.Tx)
	}
	return txs
}

// GetTransaction returns a specific transaction by ID
func (mp *Mempool) GetTransaction(txID string) (*Transaction, bool) {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	entry, exists := mp.entries[txID]
	if !exists {
		return nil, false
	}
	return entry.Tx, true
}

// HasTransaction checks if a transaction exists in the mempool
func (mp *Mempool) HasTransaction(txID string) bool {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	_, exists := mp.entries[txID]
	return exists
}

// RemoveTransaction removes a transaction from the mempool (e.g., after including in block)
func (mp *Mempool) RemoveTransaction(txID string) {
	mp.txLock.Lock()
	defer mp.txLock.Unlock()

	delete(mp.entries, txID)
	fmt.Printf("[Mempool] Removed transaction: %s (remaining: %d)\n", txID, len(mp.entries))
}

// Count returns the number of transactions in the mempool
func (mp *Mempool) Count() int {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()
	return len(mp.entries)
}

// PrintStatus prints the current mempool status
func (mp *Mempool) PrintStatus() {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	totalSize := 0
	for _, entry := range mp.entries {
		totalSize += entry.SizeBytes
	}

	fmt.Printf("\n[Mempool] Status: %d transactions, %.2f MB / %d MB\n",
		len(mp.entries), float64(totalSize)/(1024*1024), mp.maxSizeBytes/(1024*1024))
	for txID, entry := range mp.entries {
		age := mp.currentHeight - entry.AddedAtBlock
		fmt.Printf("  - %s (type: %d, outputs: %d, age: %d blocks)\n",
			txID, entry.Tx.TxType, len(entry.Tx.Outputs), age)
	}
	fmt.Println()
}

// Close shuts down the mempool
func (mp *Mempool) Close() error {
	mp.cancel()
	mp.sub.Cancel()
	return mp.topic.Close()
}

// UpdateBlockHeight updates the current block height and triggers expiration cleanup
func (mp *Mempool) UpdateBlockHeight(height uint64) {
	mp.txLock.Lock()
	defer mp.txLock.Unlock()

	mp.currentHeight = height
	mp.cleanupExpiredTransactionsLocked()
}

// PurgeInvalidTransactions removes transactions with spent inputs
// Should be called after each block is added
func (mp *Mempool) PurgeInvalidTransactions(utxoStore *UTXOStore) {
	mp.txLock.Lock()
	defer mp.txLock.Unlock()

	beforeCount := len(mp.entries)
	var invalidTxs []string

	for txID, entry := range mp.entries {
		// Check if all inputs are still unspent
		for _, input := range entry.Tx.Inputs {
			utxo, err := utxoStore.GetUTXO(input.PrevTxID, input.OutputIndex)
			if err != nil || utxo == nil || utxo.IsSpent {
				// Input no longer available - transaction is invalid
				invalidTxs = append(invalidTxs, txID)
				break
			}
		}
	}

	if len(invalidTxs) > 0 {
		for _, txID := range invalidTxs {
			delete(mp.entries, txID)
		}
		fmt.Printf("[Mempool] 🧹 Purged %d transactions with spent inputs (%d -> %d remaining)\n",
			len(invalidTxs), beforeCount, len(mp.entries))
	} else if beforeCount > 0 {
		fmt.Printf("[Mempool] 🧹 Checked %d transactions, none invalid\n", beforeCount)
	}
}

// cleanupExpiredTransactionsLocked removes transactions older than expiryBlocks
// Must be called with txLock held
func (mp *Mempool) cleanupExpiredTransactionsLocked() {
	if mp.expiryBlocks <= 0 {
		return
	}

	var expiredTxs []string
	for txID, entry := range mp.entries {
		age := mp.currentHeight - entry.AddedAtBlock
		if age >= uint64(mp.expiryBlocks) {
			expiredTxs = append(expiredTxs, txID)
		}
	}

	if len(expiredTxs) > 0 {
		for _, txID := range expiredTxs {
			delete(mp.entries, txID)
		}
		fmt.Printf("[Mempool] Expired %d transactions (age >= %d blocks)\n", len(expiredTxs), mp.expiryBlocks)
	}
}

// enforceMemoryLimitLocked evicts oldest transactions if mempool exceeds size limit
// Must be called with txLock held
func (mp *Mempool) enforceMemoryLimitLocked() {
	if mp.maxSizeBytes <= 0 {
		return
	}

	// Calculate current size
	currentSize := 0
	for _, entry := range mp.entries {
		currentSize += entry.SizeBytes
	}

	if currentSize <= mp.maxSizeBytes {
		return
	}

	// Need to evict - sort entries by timestamp (oldest first)
	type entryWithID struct {
		txID  string
		entry *MempoolEntry
	}

	entries := make([]entryWithID, 0, len(mp.entries))
	for txID, entry := range mp.entries {
		entries = append(entries, entryWithID{txID, entry})
	}

	// Sort by timestamp (oldest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].entry.AddedTimestamp.After(entries[j].entry.AddedTimestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Evict oldest until we're under the limit
	evictedCount := 0
	for _, e := range entries {
		if currentSize <= mp.maxSizeBytes {
			break
		}
		delete(mp.entries, e.txID)
		currentSize -= e.entry.SizeBytes
		evictedCount++
	}

	if evictedCount > 0 {
		fmt.Printf("[Mempool] Evicted %d oldest transactions to enforce %d MB limit\n",
			evictedCount, mp.maxSizeBytes/(1024*1024))
	}
}

// estimateTxSize estimates the size of a transaction in bytes
func (mp *Mempool) estimateTxSize(tx *Transaction) int {
	// Rough estimate: count inputs, outputs, and signature
	size := 100 // Base overhead

	// Inputs (UTXO references)
	size += len(tx.Inputs) * 100

	// Outputs
	for _, output := range tx.Outputs {
		size += 100 + len(output.Address)
	}

	// Signature
	size += len(tx.Signature)

	return size
}
