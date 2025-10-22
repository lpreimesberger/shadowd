package lib

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

// UTXOStore manages the UTXO set with persistent storage
type UTXOStore struct {
	db    *BoltDBAdapter
	mutex sync.RWMutex
	cache sync.Map // In-memory cache for performance (thread-safe)
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
	ValidatorPrefix  = "val:"     // val:{proposer_address_hex} -> wallet_address
)

// NewUTXOStore creates a new UTXO store with the given database path
func NewUTXOStore(dbPath string) (*UTXOStore, error) {
	db, err := NewBoltDBAdapter(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open UTXO database: %w", err)
	}

	return &UTXOStore{
		db: db,
		// cache is sync.Map, no initialization needed
	}, nil
}

// GetUTXO retrieves a UTXO by transaction ID and output index
func (store *UTXOStore) GetUTXO(txID string, outputIndex uint32) (*UTXO, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	key := fmt.Sprintf("%s%s:%d", UTXOPrefix, txID, outputIndex)

	// Check cache first (sync.Map is thread-safe)
	if cached, exists := store.cache.Load(key); exists {
		return cached.(*UTXO), nil
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

	// Cache the UTXO (sync.Map handles concurrency)
	store.cache.Store(key, &utxo)

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
	addrStr := utxo.Output.Address.String()
	addrKey := fmt.Sprintf("%s%s:%s:%d", AddressPrefix, addrStr, utxo.TxID, utxo.OutputIndex)
	if err := store.db.Set([]byte(addrKey), []byte("")); err != nil {
		return fmt.Errorf("failed to store address index: %w", err)
	}

	// Debug logging disabled during sync to improve performance
	// if utxo.OutputIndex == 0 {
	// 	fmt.Printf("[UTXO] Indexed tx %s:0 for address %s (len=%d)\n", utxo.TxID[:16], addrStr[:16], len(addrStr))
	// }

	// Add to height index
	heightKey := fmt.Sprintf("%s%d:%s:%d", HeightPrefix, utxo.BlockHeight, utxo.TxID, utxo.OutputIndex)
	if err := store.db.Set([]byte(heightKey), []byte("")); err != nil {
		return fmt.Errorf("failed to store height index: %w", err)
	}

	// Cache the UTXO
	store.cache.Store(key, utxo)

	return nil
}

// SpendUTXO marks a UTXO as spent
func (store *UTXOStore) SpendUTXO(txID string, outputIndex uint32) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Get the UTXO first (without acquiring lock since we already have it)
	key := fmt.Sprintf("%s%s:%d", UTXOPrefix, txID, outputIndex)

	// Check cache first
	var utxo *UTXO
	if cached, exists := store.cache.Load(key); exists {
		utxo = cached.(*UTXO)
	} else {
		// Check database
		data, err := store.db.Get([]byte(key))
		if err != nil {
			return fmt.Errorf("failed to get UTXO from database: %w", err)
		}
		if data == nil {
			return fmt.Errorf("UTXO not found: %s:%d", txID, outputIndex)
		}

		var u UTXO
		if err := json.Unmarshal(data, &u); err != nil {
			return fmt.Errorf("failed to unmarshal UTXO: %w", err)
		}
		utxo = &u
		store.cache.Store(key, utxo)
	}

	if utxo.IsSpent {
		return fmt.Errorf("UTXO already spent: %s:%d", txID, outputIndex)
	}

	// Mark as spent
	utxo.IsSpent = true

	// Update in database (key already defined above)
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

	// Invalidate cache - force re-read from DB next time to ensure fresh data
	store.cache.Delete(key)

	return nil
}

// GetUTXOsByAddress returns all unspent UTXOs for a given address
func (store *UTXOStore) GetUTXOsByAddress(address Address) ([]*UTXO, error) {
	// Badger handles concurrency - no mutex needed!
	var utxos []*UTXO
	addrStr := address.String()
	prefix := fmt.Sprintf("%s%s:", AddressPrefix, addrStr)

	fmt.Printf("[UTXO Query] Looking for UTXOs with prefix: %s (addr len=%d)\n", prefix[:40], len(addrStr))

	// Iterate through address index
	iterator, err := store.db.Iterator([]byte(prefix), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	matchCount := 0
	for ; iterator.Valid(); iterator.Next() {
		matchCount++
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

	fmt.Printf("[UTXO Query] Found %d matching keys, returning %d unspent UTXOs\n", matchCount, len(utxos))
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
	// Clear sync.Map by deleting all entries
	store.cache.Range(func(key, value interface{}) bool {
		store.cache.Delete(key)
		return true
	})
}

// Close closes the database connection
func (store *UTXOStore) Close() error {
	if store.db != nil {
		return store.db.Close()
	}
	return nil
}

// ProcessTokenTransaction handles token-specific transaction processing (mint/melt/pools)
func (store *UTXOStore) ProcessTokenTransaction(tx *Transaction, tokenRegistry *TokenRegistry, poolRegistry *PoolRegistry, blockHeight int64) error {
	if tx == nil || tokenRegistry == nil {
		return nil
	}

	txID, _ := tx.ID()

	switch tx.TxType {
	case TxTypeMintToken:
		// Extract token metadata from transaction
		var mintData TokenMintData
		if err := json.Unmarshal(tx.Data, &mintData); err != nil {
			return fmt.Errorf("failed to parse mint data: %w", err)
		}

		// Create TokenInfo and register it
		tokenInfo, err := CreateCustomToken(
			mintData.Ticker,
			mintData.Desc,
			mintData.MaxMint,
			mintData.MaxDecimals,
			tx.Outputs[0].Address, // Creator is first output address
		)
		if err != nil {
			return fmt.Errorf("failed to create token info: %w", err)
		}

		// Set token ID to this TX ID
		tokenInfo.SetTokenID(txID)

		// Update the token output to have the correct token ID
		// The output was created with "PENDING" placeholder, now set it to actual TX ID
		for i, output := range tx.Outputs {
			if output.TokenType == "custom" && output.TokenID == "PENDING" {
				tx.Outputs[i].TokenID = txID
				break
			}
		}

		// Register the token
		if err := tokenRegistry.RegisterToken(tokenInfo); err != nil {
			return fmt.Errorf("failed to register token: %w", err)
		}

		fmt.Printf("[TokenRegistry] ✅ Registered token: %s (ID: %s, Supply: %d)\n",
			mintData.Ticker, txID[:16], tokenInfo.TotalSupply)

	case TxTypeMelt:
		fmt.Printf("[TokenRegistry] Processing melt transaction: %s\n", txID[:16])
		// Find the token being melted and update total melted
		for _, output := range tx.Outputs {
			// Find SHADOW output - this tells us how much was melted
			if output.TokenID == GetGenesisToken().TokenID {
				// Figure out which token was melted from inputs
				if len(tx.Inputs) > 0 {
					firstInput := tx.Inputs[0]
					inputUTXO, err := store.GetUTXO(firstInput.PrevTxID, firstInput.OutputIndex)
					if err == nil && inputUTXO != nil {
						tokenID := inputUTXO.Output.TokenID
						// Calculate melted amount (input tokens - output token change)
						meltedAmount := uint64(0)
						for _, input := range tx.Inputs {
							utxo, err := store.GetUTXO(input.PrevTxID, input.OutputIndex)
							if err == nil && utxo != nil && utxo.Output.TokenID == tokenID {
								// Only count inputs of the token being melted (not SHADOW fee inputs)
								meltedAmount += utxo.Output.Amount
							}
						}
						// Subtract any token change
						for _, out := range tx.Outputs {
							if out.TokenID == tokenID {
								meltedAmount -= out.Amount
							}
						}
						// Record the melt - MUST succeed or transaction is invalid
						if err := tokenRegistry.RecordMelt(tokenID, meltedAmount); err != nil {
							fmt.Printf("[TokenRegistry] ❌ Failed to record melt: %v\n", err)
							return fmt.Errorf("melt transaction invalid: %w", err)
						}
						fmt.Printf("[TokenRegistry] ✅ Melted %d tokens (ID: %s)\n", meltedAmount, tokenID[:16])
					} else {
						fmt.Printf("[TokenRegistry] ⚠️  Could not find input UTXO for melt tx\n")
					}
				}
				break
			}
		}

	case TxTypeOffer:
		fmt.Printf("[SwapOffer] Processing offer transaction: %s\n", txID[:16])
		// Offer transactions lock tokens - no special validation needed here
		// The tokens are locked by not creating outputs for them
		// Validation happens in CreateOfferTransaction

	case TxTypeAcceptOffer:
		fmt.Printf("[SwapOffer] Processing accept offer transaction: %s\n", txID[:16])
		// Parse accept data to get offer transaction ID
		var acceptData AcceptOfferData
		if err := json.Unmarshal(tx.Data, &acceptData); err != nil {
			return fmt.Errorf("failed to parse accept data: %w", err)
		}

		// Get the original offer transaction
		offerTx, err := store.GetTransaction(acceptData.OfferTxID)
		if err != nil {
			return fmt.Errorf("failed to get offer transaction: %w", err)
		}

		// Parse offer data
		var offerData OfferData
		if err := json.Unmarshal(offerTx.Data, &offerData); err != nil {
			return fmt.Errorf("failed to parse offer data: %w", err)
		}

		// Mark the offer as consumed by setting its locked UTXOs as spent
		for _, input := range offerTx.Inputs {
			// Only spend the token inputs (not SHADOW fee inputs)
			utxo, err := store.GetUTXO(input.PrevTxID, input.OutputIndex)
			if err == nil && utxo != nil && utxo.Output.TokenID == offerData.HaveTokenID {
				if err := store.SpendUTXO(input.PrevTxID, input.OutputIndex); err != nil {
					fmt.Printf("[SwapOffer] Warning: Failed to spend offer UTXO: %v\n", err)
				}
			}
		}

		fmt.Printf("[SwapOffer] ✅ Accepted offer %s: swapped %d %s for %d %s\n",
			acceptData.OfferTxID[:16], offerData.HaveAmount, offerData.HaveTokenID[:8],
			offerData.WantAmount, offerData.WantTokenID[:8])

	case TxTypeCancelOffer:
		fmt.Printf("[SwapOffer] Processing cancel offer transaction: %s\n", txID[:16])
		// Parse cancel data to get offer transaction ID
		var cancelData CancelOfferData
		if err := json.Unmarshal(tx.Data, &cancelData); err != nil {
			return fmt.Errorf("failed to parse cancel data: %w", err)
		}

		// Get the original offer transaction
		offerTx, err := store.GetTransaction(cancelData.OfferTxID)
		if err != nil {
			return fmt.Errorf("failed to get offer transaction: %w", err)
		}

		// Parse offer data
		var offerData OfferData
		if err := json.Unmarshal(offerTx.Data, &offerData); err != nil {
			return fmt.Errorf("failed to parse offer data: %w", err)
		}

		// Mark the offer as consumed by spending its locked UTXOs
		for _, input := range offerTx.Inputs {
			// Only spend the token inputs (not SHADOW fee inputs)
			utxo, err := store.GetUTXO(input.PrevTxID, input.OutputIndex)
			if err == nil && utxo != nil && utxo.Output.TokenID == offerData.HaveTokenID {
				if err := store.SpendUTXO(input.PrevTxID, input.OutputIndex); err != nil {
					fmt.Printf("[SwapOffer] Warning: Failed to spend offer UTXO: %v\n", err)
				}
			}
		}

		fmt.Printf("[SwapOffer] ✅ Cancelled offer %s\n", cancelData.OfferTxID[:16])

	case TxTypeCreatePool:
		fmt.Printf("[LiquidityPool] ⏳ START processing create pool transaction: %s\n", txID[:16])
		// Parse pool creation data
		var poolData CreatePoolData
		if err := json.Unmarshal(tx.Data, &poolData); err != nil {
			return fmt.Errorf("failed to parse pool data: %w", err)
		}

		// Validate tokens exist in registry
		tokenA, existsA := tokenRegistry.GetToken(poolData.TokenA)
		if !existsA {
			return fmt.Errorf("token A not found: %s", poolData.TokenA)
		}
		tokenB, existsB := tokenRegistry.GetToken(poolData.TokenB)
		if !existsB {
			return fmt.Errorf("token B not found: %s", poolData.TokenB)
		}

		// Calculate LP tokens to mint
		lpTokenAmount := CalculateLPTokens(poolData.AmountA, poolData.AmountB)
		if lpTokenAmount == 0 {
			return fmt.Errorf("LP token amount cannot be zero")
		}

		// Create LP token ticker with pool ID to ensure uniqueness
		lpTokenTicker := GetLPTokenName(tokenA.Ticker, tokenB.Ticker, txID)

		// Calculate MaxMint to satisfy validation: TotalSupply == MaxMint * 10^MaxDecimals
		// For 8 decimals: MaxMint = TotalSupply / 10^8
		lpMaxDecimals := uint8(8)
		divisor := uint64(1)
		for i := uint8(0); i < lpMaxDecimals; i++ {
			divisor *= 10
		}
		lpMaxMint := lpTokenAmount / divisor
		if lpMaxMint == 0 {
			lpMaxMint = 1 // Minimum 1
		}
		// Ensure TotalSupply matches exactly
		expectedSupply := lpMaxMint
		for i := uint8(0); i < lpMaxDecimals; i++ {
			expectedSupply *= 10
		}

		// Create LP token info
		lpTokenInfo := &TokenInfo{
			TokenID:        txID, // Use pool creation tx as LP token ID
			Ticker:         lpTokenTicker,
			Desc:           fmt.Sprintf("%s%sLiquidityPool", tokenA.Ticker, tokenB.Ticker),
			MaxMint:        lpMaxMint,
			MaxDecimals:    lpMaxDecimals,
			TotalSupply:    expectedSupply, // Use calculated value that matches validation
			LockedShadow:   expectedSupply, // Must equal TotalSupply for validation
			TotalMelted:    0,
			MintVersion:    0,
			CreatorAddress: poolData.PoolAddress,
			CreationTime:   blockHeight,
		}

		// Register LP token in token registry
		if err := tokenRegistry.RegisterToken(lpTokenInfo); err != nil {
			return fmt.Errorf("failed to register LP token: %w", err)
		}

		// Create liquidity pool (use expectedSupply for consistency)
		pool := &LiquidityPool{
			PoolID:        txID,
			TokenA:        poolData.TokenA,
			TokenB:        poolData.TokenB,
			ReserveA:      poolData.AmountA,
			ReserveB:      poolData.AmountB,
			LPTokenID:     txID,
			LPTokenSupply: expectedSupply, // Use adjusted supply
			FeePercent:    poolData.FeePercent,
			K:             CalculateK(poolData.AmountA, poolData.AmountB),
			CreatedAt:     uint64(blockHeight),
		}

		// Register pool in pool registry
		if poolRegistry != nil {
			if err := poolRegistry.RegisterPool(pool); err != nil {
				return fmt.Errorf("failed to register pool: %w", err)
			}
		}

		// Create UTXO for LP tokens to pool creator (use expectedSupply)
		lpTokenOutput := CreateTokenOutput(poolData.PoolAddress, expectedSupply, txID, "liquidity_pool", nil)
		lpUTXO := &UTXO{
			TxID:        txID,
			OutputIndex: uint32(len(tx.Outputs)), // Add as next output
			Output:      lpTokenOutput,
			IsSpent:     false,
		}
		if err := store.AddUTXO(lpUTXO); err != nil {
			return fmt.Errorf("failed to create LP token UTXO: %w", err)
		}

		fmt.Printf("[LiquidityPool] ✅ Created pool %s: %s/%s (reserves: %d/%d, LP tokens: %d)\n",
			txID[:16], tokenA.Ticker, tokenB.Ticker, poolData.AmountA, poolData.AmountB, expectedSupply)

	case TxTypeAddLiquidity:
		fmt.Printf("[LiquidityPool] ⏳ START processing add liquidity transaction: %s\n", txID[:16])

		// Parse add liquidity data
		var addData AddLiquidityData
		if err := json.Unmarshal(tx.Data, &addData); err != nil {
			return fmt.Errorf("failed to parse add liquidity data: %w", err)
		}

		// Get the pool
		pool, err := poolRegistry.GetPool(addData.PoolID)
		if err != nil {
			return fmt.Errorf("pool not found: %s", addData.PoolID[:16])
		}

		// Calculate LP tokens to mint based on proportional contribution
		// LP tokens = min(amountA/reserveA, amountB/reserveB) * lpTokenSupply
		var lpTokensToMint uint64
		ratioA := (addData.AmountA * pool.LPTokenSupply) / pool.ReserveA
		ratioB := (addData.AmountB * pool.LPTokenSupply) / pool.ReserveB

		// Use the smaller ratio to ensure pool ratio is maintained
		if ratioA < ratioB {
			lpTokensToMint = ratioA
		} else {
			lpTokensToMint = ratioB
		}

		// Check minimum LP tokens (slippage protection)
		if lpTokensToMint < addData.MinLPTokens {
			return fmt.Errorf("insufficient LP tokens: would receive %d, minimum %d", lpTokensToMint, addData.MinLPTokens)
		}

		// Update pool reserves
		pool.ReserveA += addData.AmountA
		pool.ReserveB += addData.AmountB
		pool.LPTokenSupply += lpTokensToMint
		pool.K = CalculateK(pool.ReserveA, pool.ReserveB)

		// Update pool in registry
		if err := poolRegistry.UpdatePool(pool); err != nil {
			return fmt.Errorf("failed to update pool: %w", err)
		}

		// Update LP token total supply in token registry
		lpToken, exists := tokenRegistry.GetToken(pool.LPTokenID)
		if !exists {
			return fmt.Errorf("LP token not found: %s", pool.LPTokenID[:16])
		}
		lpToken.TotalSupply += lpTokensToMint
		lpToken.LockedShadow += lpTokensToMint // Keep accounting consistent
		if err := tokenRegistry.UpdateToken(lpToken); err != nil {
			return fmt.Errorf("failed to update LP token supply: %w", err)
		}

		// Get liquidity provider address from first output (should be LP token change output)
		var providerAddress Address
		if len(tx.Outputs) > 0 {
			providerAddress = tx.Outputs[0].Address
		} else {
			return fmt.Errorf("no outputs found for LP tokens")
		}

		// Create UTXO for LP tokens to liquidity provider
		lpTokenOutput := CreateTokenOutput(providerAddress, lpTokensToMint, pool.LPTokenID, "liquidity_pool", nil)
		lpUTXO := &UTXO{
			TxID:        txID,
			OutputIndex: uint32(len(tx.Outputs)), // Add as next output
			Output:      lpTokenOutput,
			IsSpent:     false,
		}
		if err := store.AddUTXO(lpUTXO); err != nil {
			return fmt.Errorf("failed to create LP token UTXO: %w", err)
		}

		fmt.Printf("[LiquidityPool] ✅ Added liquidity to pool %s: +%d/%d tokens, minted %d LP tokens\n",
			addData.PoolID[:16], addData.AmountA, addData.AmountB, lpTokensToMint)

	case TxTypeRemoveLiquidity:
		fmt.Printf("[LiquidityPool] ⏳ START processing remove liquidity transaction: %s\n", txID[:16])

		// Parse remove liquidity data
		var removeData RemoveLiquidityData
		if err := json.Unmarshal(tx.Data, &removeData); err != nil {
			return fmt.Errorf("failed to parse remove liquidity data: %w", err)
		}

		// Get the pool
		pool, err := poolRegistry.GetPool(removeData.PoolID)
		if err != nil {
			return fmt.Errorf("pool not found: %s", removeData.PoolID[:16])
		}

		// Calculate tokens to return based on LP tokens being burned
		// amountA = (lpTokens / lpTokenSupply) * reserveA
		// amountB = (lpTokens / lpTokenSupply) * reserveB
		amountAToReturn := (removeData.LPTokens * pool.ReserveA) / pool.LPTokenSupply
		amountBToReturn := (removeData.LPTokens * pool.ReserveB) / pool.LPTokenSupply

		// Check minimum amounts (slippage protection)
		if amountAToReturn < removeData.MinAmountA {
			return fmt.Errorf("insufficient token A: would receive %d, minimum %d", amountAToReturn, removeData.MinAmountA)
		}
		if amountBToReturn < removeData.MinAmountB {
			return fmt.Errorf("insufficient token B: would receive %d, minimum %d", amountBToReturn, removeData.MinAmountB)
		}

		// Update pool reserves
		pool.ReserveA -= amountAToReturn
		pool.ReserveB -= amountBToReturn
		pool.LPTokenSupply -= removeData.LPTokens
		pool.K = CalculateK(pool.ReserveA, pool.ReserveB)

		// Update pool in registry
		if err := poolRegistry.UpdatePool(pool); err != nil {
			return fmt.Errorf("failed to update pool: %w", err)
		}

		// Update LP token total supply in token registry (burn tokens)
		lpToken, exists := tokenRegistry.GetToken(pool.LPTokenID)
		if !exists {
			return fmt.Errorf("LP token not found: %s", pool.LPTokenID[:16])
		}
		lpToken.TotalSupply -= removeData.LPTokens
		lpToken.LockedShadow -= removeData.LPTokens // Keep accounting consistent
		if err := tokenRegistry.UpdateToken(lpToken); err != nil {
			return fmt.Errorf("failed to update LP token supply: %w", err)
		}

		// Get liquidity provider address from first output
		var providerAddress Address
		if len(tx.Outputs) > 0 {
			providerAddress = tx.Outputs[0].Address
		} else {
			return fmt.Errorf("no outputs found for returned tokens")
		}

		// Create UTXOs for returned tokens A and B
		tokenAOutput := CreateTokenOutput(providerAddress, amountAToReturn, pool.TokenA, "liquidity_pool", nil)
		tokenAUTXO := &UTXO{
			TxID:        txID,
			OutputIndex: uint32(len(tx.Outputs)),
			Output:      tokenAOutput,
			IsSpent:     false,
		}
		if err := store.AddUTXO(tokenAUTXO); err != nil {
			return fmt.Errorf("failed to create token A UTXO: %w", err)
		}

		tokenBOutput := CreateTokenOutput(providerAddress, amountBToReturn, pool.TokenB, "liquidity_pool", nil)
		tokenBUTXO := &UTXO{
			TxID:        txID,
			OutputIndex: uint32(len(tx.Outputs) + 1),
			Output:      tokenBOutput,
			IsSpent:     false,
		}
		if err := store.AddUTXO(tokenBUTXO); err != nil {
			return fmt.Errorf("failed to create token B UTXO: %w", err)
		}

		fmt.Printf("[LiquidityPool] ✅ Removed liquidity from pool %s: burned %d LP tokens, returned %d/%d tokens\n",
			removeData.PoolID[:16], removeData.LPTokens, amountAToReturn, amountBToReturn)

	case TxTypeSwap:
		fmt.Printf("[LiquidityPool] ⏳ START processing swap transaction: %s\n", txID[:16])

		// Parse swap data
		var swapData SwapData
		if err := json.Unmarshal(tx.Data, &swapData); err != nil {
			return fmt.Errorf("failed to parse swap data: %w", err)
		}

		// Get the pool
		pool, err := poolRegistry.GetPool(swapData.PoolID)
		if err != nil {
			return fmt.Errorf("pool not found: %s", swapData.PoolID[:16])
		}

		// Determine which token is being swapped
		var tokenOut string
		var reserveIn, reserveOut uint64

		if swapData.TokenIn == pool.TokenA {
			tokenOut = pool.TokenB
			reserveIn = pool.ReserveA
			reserveOut = pool.ReserveB
		} else if swapData.TokenIn == pool.TokenB {
			tokenOut = pool.TokenA
			reserveIn = pool.ReserveB
			reserveOut = pool.ReserveA
		} else {
			return fmt.Errorf("token %s not in pool", swapData.TokenIn[:8])
		}

		// Calculate output amount using constant product formula with fees
		// amountOut = (amountIn * (10000 - fee) * reserveOut) / ((reserveIn * 10000) + (amountIn * (10000 - fee)))
		feeMultiplier := uint64(10000 - pool.FeePercent) // e.g., 9970 for 0.3% fee
		numerator := swapData.AmountIn * feeMultiplier * reserveOut
		denominator := (reserveIn * 10000) + (swapData.AmountIn * feeMultiplier)
		amountOut := numerator / denominator

		// Check minimum output (slippage protection)
		if amountOut < swapData.MinAmountOut {
			return fmt.Errorf("insufficient output: would receive %d, minimum %d", amountOut, swapData.MinAmountOut)
		}

		// Update pool reserves
		if swapData.TokenIn == pool.TokenA {
			pool.ReserveA += swapData.AmountIn
			pool.ReserveB -= amountOut
		} else {
			pool.ReserveB += swapData.AmountIn
			pool.ReserveA -= amountOut
		}
		pool.K = CalculateK(pool.ReserveA, pool.ReserveB)

		// Update pool in registry
		if err := poolRegistry.UpdatePool(pool); err != nil {
			return fmt.Errorf("failed to update pool: %w", err)
		}

		// Get swapper address from first output
		var swapperAddress Address
		if len(tx.Outputs) > 0 {
			swapperAddress = tx.Outputs[0].Address
		} else {
			return fmt.Errorf("no outputs found for swap")
		}

		// Create UTXO for output tokens
		outputTokenOutput := CreateTokenOutput(swapperAddress, amountOut, tokenOut, "swap", nil)
		outputUTXO := &UTXO{
			TxID:        txID,
			OutputIndex: uint32(len(tx.Outputs)),
			Output:      outputTokenOutput,
			IsSpent:     false,
		}
		if err := store.AddUTXO(outputUTXO); err != nil {
			return fmt.Errorf("failed to create output UTXO: %w", err)
		}

		fmt.Printf("[LiquidityPool] ✅ Swapped in pool %s: %d %s -> %d %s\n",
			swapData.PoolID[:16], swapData.AmountIn, swapData.TokenIn[:8], amountOut, tokenOut[:8])
	}

	return nil
}

// StoreTransaction stores a transaction and indexes it by addresses involved
func (store *UTXOStore) StoreTransaction(tx *Transaction, height int64) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	txID, err := tx.ID()
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
		// Get UTXO directly from cache/database (we already hold the write lock)
		key := fmt.Sprintf("%s%s:%d", UTXOPrefix, input.PrevTxID, input.OutputIndex)

		// Check cache first
		var utxo *UTXO
		if cached, exists := store.cache.Load(key); exists {
			utxo = cached.(*UTXO)
		} else {
			// Check database directly (no nested lock)
			data, err := store.db.Get([]byte(key))
			if err == nil && data != nil {
				var u UTXO
				if err := json.Unmarshal(data, &u); err == nil {
					utxo = &u
					store.cache.Store(key, utxo)
				}
			}
		}

		if utxo != nil {
			addressMap[utxo.Output.Address.String()] = true
		}
	}

	// Create address-tx index for each address
	// Format: addrtx:{address}:{height}:{txid}
	// Using negative height for reverse chronological order
	for addrStr := range addressMap {
		addrTxKey := fmt.Sprintf("%s%s:%020d:%s", AddrTxPrefix, addrStr, int64(999999999999999999)-height, txID)
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

// RegisterValidator stores a validator's wallet address for block rewards
func (store *UTXOStore) RegisterValidator(proposerAddr []byte, walletAddr Address) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	key := fmt.Sprintf("%s%x", ValidatorPrefix, proposerAddr)
	data, err := json.Marshal(walletAddr)
	if err != nil {
		return fmt.Errorf("failed to marshal wallet address: %w", err)
	}

	if err := store.db.Set([]byte(key), data); err != nil {
		return fmt.Errorf("failed to store validator registration: %w", err)
	}

	log.Printf("✅ Validator registered: %x -> %s", proposerAddr, walletAddr.String()[:20]+"...")
	return nil
}

// GetValidatorWallet retrieves a validator's registered wallet address
func (store *UTXOStore) GetValidatorWallet(proposerAddr []byte) (*Address, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	key := fmt.Sprintf("%s%x", ValidatorPrefix, proposerAddr)
	data, err := store.db.Get([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("failed to get validator wallet: %w", err)
	}
	if data == nil {
		return nil, nil // Not registered
	}

	var addr Address
	if err := json.Unmarshal(data, &addr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal wallet address: %w", err)
	}

	return &addr, nil
}

// MigrateCoinbaseTransactions creates transaction records for existing coinbase UTXOs
// This is a migration function to backfill transaction history from existing UTXO data
func (store *UTXOStore) MigrateCoinbaseTransactions() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	// Iterate through all UTXOs
	iterator, err := store.db.Iterator([]byte(UTXOPrefix), nil)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iterator.Close()

	txSeen := make(map[string]bool)
	migrated := 0

	for ; iterator.Valid(); iterator.Next() {
		// Get UTXO data
		data := iterator.Value()
		if data == nil {
			continue
		}

		var utxo UTXO
		if err := json.Unmarshal(data, &utxo); err != nil {
			continue
		}

		// Skip if we've already processed this transaction
		if txSeen[utxo.TxID] {
			continue
		}
		txSeen[utxo.TxID] = true

		// Check if transaction already exists
		txKey := fmt.Sprintf("%s%s", TxPrefix, utxo.TxID)
		existing, _ := store.db.Get([]byte(txKey))
		if existing != nil {
			continue // Already have this transaction
		}

		// Reconstruct a coinbase transaction from the UTXO
		// We can only reconstruct coinbase transactions (no inputs)
		tx := &Transaction{
			TxType:    TxTypeCoinbase,
			Version:   1,
			Timestamp: 0, // Unknown, but doesn't matter for display
			LockTime:  0,
			TokenID:   utxo.Output.TokenID,
			Inputs:    []*TxInput{}, // Coinbase has no inputs
			Outputs:   []*TxOutput{utxo.Output},
		}

		// Store the reconstructed transaction
		txData, err := json.Marshal(tx)
		if err != nil {
			continue
		}

		if err := store.db.Set([]byte(txKey), txData); err != nil {
			continue
		}

		// Create address-tx index
		addrTxKey := fmt.Sprintf("%s%s:%020d:%s", AddrTxPrefix, utxo.Output.Address.String(), int64(999999999999999999)-int64(utxo.BlockHeight), utxo.TxID)
		if err := store.db.Set([]byte(addrTxKey), []byte("")); err != nil {
			continue
		}

		migrated++
	}

	if migrated > 0 {
		log.Printf("✅ Migrated %d coinbase transactions from UTXOs", migrated)
	}

	return nil
}
