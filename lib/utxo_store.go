package lib

import (
	"encoding/json"
	"fmt"
	"sync"

	db "github.com/cometbft/cometbft-db"
)

// UTXOStore manages the UTXO set with persistent storage
type UTXOStore struct {
	db    db.DB
	mutex sync.RWMutex
	cache map[string]*UTXO // In-memory cache for performance
}

// Prefixes for different data types in the database
const (
	UTXOPrefix       = "utxo:"    // utxo:{txid}:{index} -> UTXO
	AddressPrefix    = "addr:"    // addr:{address}:{txid}:{index} -> ""
	HeightPrefix     = "height:"  // height:{height}:{txid}:{index} -> ""
	SpentPrefix      = "spent:"   // spent:{txid}:{index} -> ""
	TxPrefix         = "tx:"      // tx:{txid} -> Transaction
	AddrTxPrefix     = "addrtx:"  // addrtx:{address}:{height}:{txid} -> ""
	AddrTxIndexCount = "atxcnt:"  // atxcnt:{address} -> count
)

// NewUTXOStore creates a new UTXO store with the given database
func NewUTXOStore(database db.DB) *UTXOStore {
	return &UTXOStore{
		db:    database,
		cache: make(map[string]*UTXO),
	}
}

// GetUTXO retrieves a UTXO by transaction ID and output index
func (store *UTXOStore) GetUTXO(txID string, outputIndex uint32) (*UTXO, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	key := fmt.Sprintf("%s%s:%d", UTXOPrefix, txID, outputIndex)

	// Check cache first
	if utxo, exists := store.cache[key]; exists {
		return utxo, nil
	}

	// Check database
	data, err := store.db.Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXO from database: %w", err)
	}
	if data == nil {
		return nil, nil // UTXO not found
	}

	var utxo UTXO
	if err := json.Unmarshal(data, &utxo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal UTXO: %w", err)
	}

	// Cache the UTXO
	store.cache[key] = &utxo

	return &utxo, nil
}

// AddUTXO adds a new UTXO to the store
func (store *UTXOStore) AddUTXO(utxo *UTXO) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	key := fmt.Sprintf("%s%s:%d", UTXOPrefix, utxo.TxID, utxo.OutputIndex)

	// Serialize UTXO
	data, err := json.Marshal(utxo)
	if err != nil {
		return fmt.Errorf("failed to marshal UTXO: %w", err)
	}

	// Store in database
	if err := store.db.Set([]byte(key), data); err != nil {
		return fmt.Errorf("failed to store UTXO in database: %w", err)
	}

	// Add to address index
	addrKey := fmt.Sprintf("%s%s:%s:%d", AddressPrefix, utxo.Output.Address.String(), utxo.TxID, utxo.OutputIndex)
	if err := store.db.Set([]byte(addrKey), []byte("")); err != nil {
		return fmt.Errorf("failed to store address index: %w", err)
	}

	// Add to height index
	heightKey := fmt.Sprintf("%s%d:%s:%d", HeightPrefix, utxo.BlockHeight, utxo.TxID, utxo.OutputIndex)
	if err := store.db.Set([]byte(heightKey), []byte("")); err != nil {
		return fmt.Errorf("failed to store height index: %w", err)
	}

	// Cache the UTXO
	store.cache[key] = utxo

	return nil
}

// SpendUTXO marks a UTXO as spent
func (store *UTXOStore) SpendUTXO(txID string, outputIndex uint32) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Get the UTXO first
	utxo, err := store.GetUTXO(txID, outputIndex)
	if err != nil {
		return err
	}
	if utxo == nil {
		return fmt.Errorf("UTXO not found: %s:%d", txID, outputIndex)
	}
	if utxo.IsSpent {
		return fmt.Errorf("UTXO already spent: %s:%d", txID, outputIndex)
	}

	// Mark as spent
	utxo.IsSpent = true

	// Update in database
	key := fmt.Sprintf("%s%s:%d", UTXOPrefix, txID, outputIndex)
	data, err := json.Marshal(utxo)
	if err != nil {
		return fmt.Errorf("failed to marshal UTXO: %w", err)
	}

	if err := store.db.Set([]byte(key), data); err != nil {
		return fmt.Errorf("failed to update UTXO in database: %w", err)
	}

	// Add to spent index
	spentKey := fmt.Sprintf("%s%s:%d", SpentPrefix, txID, outputIndex)
	if err := store.db.Set([]byte(spentKey), []byte("")); err != nil {
		return fmt.Errorf("failed to store spent index: %w", err)
	}

	// Update cache
	store.cache[key] = utxo

	return nil
}

// GetUTXOsByAddress returns all unspent UTXOs for a given address
func (store *UTXOStore) GetUTXOsByAddress(address Address) ([]*UTXO, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	var utxos []*UTXO
	prefix := fmt.Sprintf("%s%s:", AddressPrefix, address.String())

	// Iterate through address index
	iterator, err := store.db.Iterator([]byte(prefix), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		// Parse key to extract txID and outputIndex
		key := string(iterator.Key())
		var txID string
		var outputIndex uint32

		// Key format: addr:{address}:{txid}:{index}
		prefixLen := len(prefix)
		if len(key) <= prefixLen {
			continue // Skip malformed keys
		}
		remainingKey := key[prefixLen:]

		// Find the last colon to separate txID and index
		lastColon := -1
		for i := len(remainingKey) - 1; i >= 0; i-- {
			if remainingKey[i] == ':' {
				lastColon = i
				break
			}
		}

		if lastColon == -1 {
			continue // Skip malformed keys
		}

		txID = remainingKey[:lastColon]
		fmt.Sscanf(remainingKey[lastColon+1:], "%d", &outputIndex)

		// Get the UTXO
		utxo, err := store.GetUTXO(txID, outputIndex)
		if err != nil {
			continue // Skip errored UTXOs
		}
		if utxo != nil && !utxo.IsSpent {
			utxos = append(utxos, utxo)
		}
	}

	return utxos, nil
}

// GetBalance calculates the total balance for an address
func (store *UTXOStore) GetBalance(address Address) (map[string]uint64, error) {
	utxos, err := store.GetUTXOsByAddress(address)
	if err != nil {
		return nil, err
	}

	balances := make(map[string]uint64)
	for _, utxo := range utxos {
		balances[utxo.Output.TokenID] += utxo.Output.Amount
	}

	return balances, nil
}

// GetTotalUTXOs returns the total number of UTXOs in the store
func (store *UTXOStore) GetTotalUTXOs() (int, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	count := 0
	iterator, err := store.db.Iterator([]byte(UTXOPrefix), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		count++
	}

	return count, nil
}

// ValidateTransaction validates a transaction against the UTXO set
func (store *UTXOStore) ValidateTransaction(tx *Transaction) error {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	// Skip validation for coinbase transactions
	if tx.TxType == TxTypeCoinbase {
		return nil
	}

	var totalInput uint64
	var totalOutput uint64

	// Validate inputs
	for _, input := range tx.Inputs {
		utxo, err := store.GetUTXO(input.PrevTxID, input.OutputIndex)
		if err != nil {
			return fmt.Errorf("failed to get UTXO for input %s:%d: %w", input.PrevTxID, input.OutputIndex, err)
		}
		if utxo == nil {
			return fmt.Errorf("UTXO not found for input %s:%d", input.PrevTxID, input.OutputIndex)
		}
		if utxo.IsSpent {
			return fmt.Errorf("UTXO already spent: %s:%d", input.PrevTxID, input.OutputIndex)
		}

		totalInput += utxo.Output.Amount
	}

	// Calculate total output
	for _, output := range tx.Outputs {
		totalOutput += output.Amount
	}

	// Validate that inputs >= outputs (fee is implicit)
	if totalInput < totalOutput {
		return fmt.Errorf("insufficient funds: inputs=%d, outputs=%d", totalInput, totalOutput)
	}

	return nil
}

// ClearCache clears the in-memory cache
func (store *UTXOStore) ClearCache() {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.cache = make(map[string]*UTXO)
}

// Close closes the database connection
func (store *UTXOStore) Close() error {
	if store.db != nil {
		return store.db.Close()
	}
	return nil
}

// StoreTransaction stores a transaction and indexes it by addresses involved
func (store *UTXOStore) StoreTransaction(tx *Transaction, height int64) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	txID, err := tx.CalculateHash()
	if err != nil {
		return fmt.Errorf("failed to calculate transaction hash: %w", err)
	}

	// Store transaction
	txKey := fmt.Sprintf("%s%s", TxPrefix, txID)
	txData, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %w", err)
	}

	if err := store.db.Set([]byte(txKey), txData); err != nil {
		return fmt.Errorf("failed to store transaction: %w", err)
	}

	// Index by addresses involved (both inputs and outputs)
	addressMap := make(map[string]bool)

	// Collect addresses from outputs
	for _, output := range tx.Outputs {
		addressMap[output.Address.String()] = true
	}

	// Collect addresses from inputs (via UTXOs)
	for _, input := range tx.Inputs {
		utxo, _ := store.GetUTXO(input.PrevTxID, input.OutputIndex)
		if utxo != nil {
			addressMap[utxo.Output.Address.String()] = true
		}
	}

	// Create address-tx index for each address
	// Format: addrtx:{address}:{height}:{txid}
	// Using negative height for reverse chronological order
	for addrStr := range addressMap {
		addrTxKey := fmt.Sprintf("%s%s:%020d:%s", AddrTxPrefix, addrStr, 9999999999999999999-height, txID)
		if err := store.db.Set([]byte(addrTxKey), []byte("")); err != nil {
			return fmt.Errorf("failed to store address-tx index: %w", err)
		}
	}

	return nil
}

// GetTransaction retrieves a transaction by its ID
func (store *UTXOStore) GetTransaction(txID string) (*Transaction, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	txKey := fmt.Sprintf("%s%s", TxPrefix, txID)
	data, err := store.db.Get([]byte(txKey))
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}
	if data == nil {
		return nil, nil
	}

	var tx Transaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}

	return &tx, nil
}

// GetTransactionsByAddress returns transactions for an address with pagination
func (store *UTXOStore) GetTransactionsByAddress(address Address, count int, afterTxID string) ([]*Transaction, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	if count <= 0 {
		count = 32
	}

	var transactions []*Transaction
	prefix := fmt.Sprintf("%s%s:", AddrTxPrefix, address.String())

	// If afterTxID is provided, we need to start from that point
	var startKey []byte
	if afterTxID != "" {
		// Find the key for afterTxID to determine where to start
		iterator, err := store.db.Iterator([]byte(prefix), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iterator.Close()

		found := false
		for ; iterator.Valid(); iterator.Next() {
			key := string(iterator.Key())
			if len(key) <= len(prefix) {
				continue
			}
			// Extract txID from key format: addrtx:{address}:{height}:{txid}
			parts := key[len(prefix):]
			lastColon := -1
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i] == ':' {
					lastColon = i
					break
				}
			}
			if lastColon == -1 {
				continue
			}
			txID := parts[lastColon+1:]
			if txID == afterTxID {
				found = true
				// Move to next item
				iterator.Next()
				if iterator.Valid() {
					startKey = iterator.Key()
				}
				break
			}
		}
		if !found {
			return transactions, nil // afterTxID not found, return empty
		}
	} else {
		startKey = []byte(prefix)
	}

	// Iterate from startKey
	iterator, err := store.db.Iterator(startKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	collected := 0
	for ; iterator.Valid() && collected < count; iterator.Next() {
		key := string(iterator.Key())
		if len(key) <= len(prefix) {
			continue
		}
		// Check if key still has our prefix
		if !startsWithPrefix(key, prefix) {
			break
		}

		// Extract txID from key format: addrtx:{address}:{height}:{txid}
		parts := key[len(prefix):]
		lastColon := -1
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == ':' {
				lastColon = i
				break
			}
		}
		if lastColon == -1 {
			continue
		}
		txID := parts[lastColon+1:]

		// Get the transaction
		tx, err := store.GetTransaction(txID)
		if err != nil {
			continue // Skip errored transactions
		}
		if tx != nil {
			transactions = append(transactions, tx)
			collected++
		}
	}

	return transactions, nil
}

// Helper function to check prefix
func startsWithPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
