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
	MempoolTopic = "shadowy-mempool"
)

// Mempool represents a shared transaction mempool
type Mempool struct {
	transactions map[string]*Transaction // txID -> transaction
	txLock       sync.RWMutex
	pubsub       *pubsub.PubSub
	topic        *pubsub.Topic
	sub          *pubsub.Subscription
	ctx          context.Context
	cancel       context.CancelFunc
}

// MempoolMessage is the gossip message format
type MempoolMessage struct {
	Type        string       `json:"type"` // "add_tx"
	Transaction *Transaction `json:"transaction"`
	Timestamp   int64        `json:"timestamp"`
}

// NewMempool creates a new mempool with gossip support
func NewMempool(h host.Host, ps *pubsub.PubSub) (*Mempool, error) {
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
		transactions: make(map[string]*Transaction),
		pubsub:       ps,
		topic:        topic,
		sub:          sub,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start listening for mempool messages
	go mp.listenForMessages()

	fmt.Printf("[Mempool] Created and listening on topic: %s\n", MempoolTopic)
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
				if _, exists := mp.transactions[txID]; !exists {
					mp.transactions[txID] = mempoolMsg.Transaction
					fmt.Printf("[Mempool] Added transaction from gossip: %s (total: %d)\n",
						txID, len(mp.transactions))
				}
				mp.txLock.Unlock()
			}
		}
	}
}

// verifyTransaction checks if a transaction has a valid signature
func (mp *Mempool) verifyTransaction(tx *Transaction) bool {
	// For coinbase transactions, no signature verification needed
	if tx.TxType == TxTypeCoinbase {
		return true
	}

	// Check if transaction is signed
	if len(tx.Signature) == 0 {
		txID, _ := tx.ID()
		fmt.Printf("[Mempool] Transaction %s has no signature\n", txID)
		return false
	}

	// Verify the signature using existing ValidateTransaction function
	if err := ValidateTransaction(tx); err != nil {
		txID, _ := tx.ID()
		fmt.Printf("[Mempool] Transaction %s failed validation: %v\n", txID, err)
		return false
	}

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
	if _, exists := mp.transactions[txID]; exists {
		mp.txLock.Unlock()
		return fmt.Errorf("transaction already in mempool")
	}
	mp.transactions[txID] = tx
	txCount := len(mp.transactions)
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

	txs := make([]*Transaction, 0, len(mp.transactions))
	for _, tx := range mp.transactions {
		txs = append(txs, tx)
	}
	return txs
}

// GetTransaction returns a specific transaction by ID
func (mp *Mempool) GetTransaction(txID string) (*Transaction, bool) {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	tx, exists := mp.transactions[txID]
	return tx, exists
}

// RemoveTransaction removes a transaction from the mempool (e.g., after including in block)
func (mp *Mempool) RemoveTransaction(txID string) {
	mp.txLock.Lock()
	defer mp.txLock.Unlock()

	delete(mp.transactions, txID)
	fmt.Printf("[Mempool] Removed transaction: %s (remaining: %d)\n", txID, len(mp.transactions))
}

// Count returns the number of transactions in the mempool
func (mp *Mempool) Count() int {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()
	return len(mp.transactions)
}

// PrintStatus prints the current mempool status
func (mp *Mempool) PrintStatus() {
	mp.txLock.RLock()
	defer mp.txLock.RUnlock()

	fmt.Printf("\n[Mempool] Status: %d transactions\n", len(mp.transactions))
	for txID, tx := range mp.transactions {
		fmt.Printf("  - %s (type: %d, outputs: %d)\n", txID, tx.TxType, len(tx.Outputs))
	}
	fmt.Println()
}

// Close shuts down the mempool
func (mp *Mempool) Close() error {
	mp.cancel()
	mp.sub.Cancel()
	return mp.topic.Close()
}
