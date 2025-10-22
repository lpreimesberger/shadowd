package lib

import (
	"encoding/json"
	"fmt"
)

// CreatePoolTransaction creates a transaction that creates a new liquidity pool
func CreatePoolTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore, tokenRegistry *TokenRegistry,
	tokenA string, tokenB string, amountA uint64, amountB uint64, feePercent uint64) (*Transaction, error) {

	// Validate fee
	if err := ValidateFeePercent(feePercent); err != nil {
		return nil, err
	}

	// Ensure tokenA and tokenB are different
	if tokenA == tokenB {
		return nil, fmt.Errorf("cannot create pool: tokens must be different")
	}

	// Get UTXOs
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	genesisTokenID := GetGenesisToken().TokenID

	fmt.Printf("[CreatePool] Genesis token ID: %s\n", genesisTokenID)
	fmt.Printf("[CreatePool] Token A: %s\n", tokenA)
	fmt.Printf("[CreatePool] Token B: %s\n", tokenB)
	fmt.Printf("[CreatePool] Total UTXOs to filter: %d\n", len(utxos))

	// Filter UTXOs by token type
	// Special case: if tokenA or tokenB is SHADOW, we need to track them separately
	// because we also need SHADOW for transaction fees
	tokenAIsShadow := tokenA == genesisTokenID
	tokenBIsShadow := tokenB == genesisTokenID

	var availableTokenAUTXOs []*UTXO
	var availableTokenBUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == genesisTokenID {
				// All SHADOW UTXOs go into shadow list
				// We'll allocate them to tokenA/tokenB or fees later
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			} else if utxo.Output.TokenID == tokenA {
				availableTokenAUTXOs = append(availableTokenAUTXOs, utxo)
			} else if utxo.Output.TokenID == tokenB {
				availableTokenBUTXOs = append(availableTokenBUTXOs, utxo)
			}
		}
	}

	fmt.Printf("[CreatePool] Filtered: tokenA=%d, tokenB=%d, shadow=%d\n",
		len(availableTokenAUTXOs), len(availableTokenBUTXOs), len(availableShadowUTXOs))

	// Calculate estimated fee first
	estimatedFee := uint64(11500) // Will refine after selecting UTXOs

	// Select UTXOs for token A
	var selectedTokenAUTXOs []*UTXO
	var tokenATotal uint64

	if tokenAIsShadow {
		// Token A is SHADOW - need to select from shadow UTXOs for amountA + fee
		totalNeeded := amountA + estimatedFee
		for _, utxo := range availableShadowUTXOs {
			selectedTokenAUTXOs = append(selectedTokenAUTXOs, utxo)
			tokenATotal += utxo.Output.Amount
			if tokenATotal >= totalNeeded {
				break
			}
		}
		if tokenATotal < totalNeeded {
			return nil, fmt.Errorf("insufficient SHADOW for pool + fee: have %d, need %d", tokenATotal, totalNeeded)
		}
	} else {
		for _, utxo := range availableTokenAUTXOs {
			selectedTokenAUTXOs = append(selectedTokenAUTXOs, utxo)
			tokenATotal += utxo.Output.Amount
			if tokenATotal >= amountA {
				break
			}
		}
		if tokenATotal < amountA {
			return nil, fmt.Errorf("insufficient token A (%s): have %d, need %d", tokenA[:8], tokenATotal, amountA)
		}
	}

	// Select UTXOs for token B
	var selectedTokenBUTXOs []*UTXO
	var tokenBTotal uint64

	if tokenBIsShadow {
		// Token B is SHADOW - select from remaining shadow UTXOs
		remainingShadow := availableShadowUTXOs
		if tokenAIsShadow {
			// Skip UTXOs already selected for token A
			remainingShadow = availableShadowUTXOs[len(selectedTokenAUTXOs):]
		}
		totalNeeded := amountB
		if !tokenAIsShadow {
			totalNeeded += estimatedFee // Need fee too if tokenA wasn't shadow
		}
		for _, utxo := range remainingShadow {
			selectedTokenBUTXOs = append(selectedTokenBUTXOs, utxo)
			tokenBTotal += utxo.Output.Amount
			if tokenBTotal >= totalNeeded {
				break
			}
		}
		if tokenBTotal < totalNeeded {
			return nil, fmt.Errorf("insufficient SHADOW for pool + fee: have %d, need %d", tokenBTotal, totalNeeded)
		}
	} else {
		for _, utxo := range availableTokenBUTXOs {
			selectedTokenBUTXOs = append(selectedTokenBUTXOs, utxo)
			tokenBTotal += utxo.Output.Amount
			if tokenBTotal >= amountB {
				break
			}
		}
		if tokenBTotal < amountB {
			return nil, fmt.Errorf("insufficient token B (%s): have %d, need %d", tokenB[:8], tokenBTotal, amountB)
		}
	}

	// Refine fee estimate
	estimatedFee = uint64(len(selectedTokenAUTXOs)+len(selectedTokenBUTXOs)+4) * 1150
	if estimatedFee < 11500 {
		estimatedFee = 11500
	}

	// Select SHADOW UTXOs for fee (only if neither token is SHADOW)
	var selectedShadowUTXOs []*UTXO
	var shadowTotal uint64

	if !tokenAIsShadow && !tokenBIsShadow {
		// Neither token is SHADOW, need separate SHADOW for fees
		for _, utxo := range availableShadowUTXOs {
			selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
			shadowTotal += utxo.Output.Amount
			if shadowTotal >= estimatedFee {
				break
			}
		}
		if shadowTotal < estimatedFee {
			return nil, fmt.Errorf("insufficient SHADOW for fee: have %d, need %d", shadowTotal, estimatedFee)
		}
	}
	// If tokenA or tokenB is SHADOW, fee is already included in their selection

	// Build transaction
	txBuilder := NewTxBuilder(TxTypeCreatePool)

	// Add all inputs
	for _, utxo := range selectedTokenAUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedTokenBUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// No outputs for locked tokens - they're locked in the pool
	// Only create change outputs

	// Handle change based on which tokens are SHADOW
	if tokenAIsShadow {
		// Token A is SHADOW - change includes pool change and fee
		shadowChange := tokenATotal - amountA - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		// Token A is not SHADOW - return normal change
		tokenAChange := tokenATotal - amountA
		if tokenAChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, tokenAChange, tokenA)
		}
	}

	if tokenBIsShadow {
		// Token B is SHADOW
		if tokenAIsShadow {
			// Both are SHADOW - this shouldn't happen (caught earlier)
			return nil, fmt.Errorf("cannot create pool: tokens must be different")
		}
		// Token B is SHADOW - change includes pool change and fee
		shadowChange := tokenBTotal - amountB - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		// Token B is not SHADOW - return normal change
		tokenBChange := tokenBTotal - amountB
		if tokenBChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, tokenBChange, tokenB)
		}
	}

	// SHADOW change (only if neither token was SHADOW)
	if !tokenAIsShadow && !tokenBIsShadow {
		shadowChange := shadowTotal - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	}

	// Create pool data
	poolData := CreatePoolData{
		TokenA:      tokenA,
		TokenB:      tokenB,
		AmountA:     amountA,
		AmountB:     amountB,
		FeePercent:  feePercent,
		PoolAddress: nodeWallet.Address,
	}

	poolDataBytes, err := json.Marshal(poolData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pool data: %w", err)
	}

	txBuilder.SetData(poolDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// CreateAddLiquidityTransaction creates a transaction that adds liquidity to an existing pool
func CreateAddLiquidityTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore, poolRegistry *PoolRegistry,
	poolID string, amountA uint64, amountB uint64, minLPTokens uint64) (*Transaction, error) {

	// Get the pool
	pool, err := poolRegistry.GetPool(poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// Validate that amounts maintain the pool ratio (within 1% tolerance)
	if !ValidatePoolRatio(amountA, amountB, pool.ReserveA, pool.ReserveB, 1) {
		expectedB := CalculateProportionalAmount(amountA, pool.ReserveA, pool.ReserveB)
		return nil, fmt.Errorf("amounts don't match pool ratio: provided %d/%d, expected %d/%d",
			amountA, amountB, amountA, expectedB)
	}

	// Get UTXOs
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	genesisTokenID := GetGenesisToken().TokenID

	// Check if tokenA or tokenB is SHADOW
	tokenAIsShadow := pool.TokenA == genesisTokenID
	tokenBIsShadow := pool.TokenB == genesisTokenID

	// Filter and select UTXOs (similar to CreatePoolTransaction)
	var availableTokenAUTXOs []*UTXO
	var availableTokenBUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == genesisTokenID {
				// All SHADOW UTXOs go into shadow list
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			} else if utxo.Output.TokenID == pool.TokenA {
				availableTokenAUTXOs = append(availableTokenAUTXOs, utxo)
			} else if utxo.Output.TokenID == pool.TokenB {
				availableTokenBUTXOs = append(availableTokenBUTXOs, utxo)
			}
		}
	}

	// Calculate estimated fee first
	estimatedFee := uint64(11500)

	// Select token A UTXOs
	var selectedTokenAUTXOs []*UTXO
	var tokenATotal uint64

	if tokenAIsShadow {
		// Token A is SHADOW - select from shadow UTXOs for amountA + fee
		totalNeeded := amountA + estimatedFee
		for _, utxo := range availableShadowUTXOs {
			selectedTokenAUTXOs = append(selectedTokenAUTXOs, utxo)
			tokenATotal += utxo.Output.Amount
			if tokenATotal >= totalNeeded {
				break
			}
		}
		if tokenATotal < totalNeeded {
			return nil, fmt.Errorf("insufficient SHADOW for liquidity + fee: have %d, need %d", tokenATotal, totalNeeded)
		}
	} else {
		for _, utxo := range availableTokenAUTXOs {
			selectedTokenAUTXOs = append(selectedTokenAUTXOs, utxo)
			tokenATotal += utxo.Output.Amount
			if tokenATotal >= amountA {
				break
			}
		}
		if tokenATotal < amountA {
			return nil, fmt.Errorf("insufficient token A: have %d, need %d", tokenATotal, amountA)
		}
	}

	// Select token B UTXOs
	var selectedTokenBUTXOs []*UTXO
	var tokenBTotal uint64

	if tokenBIsShadow {
		// Token B is SHADOW - select from remaining shadow UTXOs
		remainingShadow := availableShadowUTXOs
		if tokenAIsShadow {
			// Skip UTXOs already selected for token A
			remainingShadow = availableShadowUTXOs[len(selectedTokenAUTXOs):]
		}
		totalNeeded := amountB
		if !tokenAIsShadow {
			totalNeeded += estimatedFee // Need fee too if tokenA wasn't shadow
		}
		for _, utxo := range remainingShadow {
			selectedTokenBUTXOs = append(selectedTokenBUTXOs, utxo)
			tokenBTotal += utxo.Output.Amount
			if tokenBTotal >= totalNeeded {
				break
			}
		}
		if tokenBTotal < totalNeeded {
			return nil, fmt.Errorf("insufficient SHADOW for liquidity + fee: have %d, need %d", tokenBTotal, totalNeeded)
		}
	} else {
		for _, utxo := range availableTokenBUTXOs {
			selectedTokenBUTXOs = append(selectedTokenBUTXOs, utxo)
			tokenBTotal += utxo.Output.Amount
			if tokenBTotal >= amountB {
				break
			}
		}
		if tokenBTotal < amountB {
			return nil, fmt.Errorf("insufficient token B: have %d, need %d", tokenBTotal, amountB)
		}
	}

	// Refine fee estimate
	estimatedFee = uint64(len(selectedTokenAUTXOs)+len(selectedTokenBUTXOs)+4) * 1150
	if estimatedFee < 11500 {
		estimatedFee = 11500
	}

	// Select SHADOW for fee (only if neither token is SHADOW)
	var selectedShadowUTXOs []*UTXO
	var shadowTotal uint64

	if !tokenAIsShadow && !tokenBIsShadow {
		for _, utxo := range availableShadowUTXOs {
			selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
			shadowTotal += utxo.Output.Amount
			if shadowTotal >= estimatedFee {
				break
			}
		}
		if shadowTotal < estimatedFee {
			return nil, fmt.Errorf("insufficient SHADOW for fee: have %d, need %d", shadowTotal, estimatedFee)
		}
	}

	// Build transaction
	txBuilder := NewTxBuilder(TxTypeAddLiquidity)

	// Add inputs
	for _, utxo := range selectedTokenAUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedTokenBUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Create change outputs (handle SHADOW specially)
	if tokenAIsShadow {
		shadowChange := tokenATotal - amountA - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		tokenAChange := tokenATotal - amountA
		if tokenAChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, tokenAChange, pool.TokenA)
		}
	}

	if tokenBIsShadow {
		if tokenAIsShadow {
			return nil, fmt.Errorf("cannot add liquidity: both tokens are SHADOW")
		}
		shadowChange := tokenBTotal - amountB - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		tokenBChange := tokenBTotal - amountB
		if tokenBChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, tokenBChange, pool.TokenB)
		}
	}

	// SHADOW change (only if neither token was SHADOW)
	if !tokenAIsShadow && !tokenBIsShadow {
		shadowChange := shadowTotal - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	}

	// Create add liquidity data
	addData := AddLiquidityData{
		PoolID:      poolID,
		AmountA:     amountA,
		AmountB:     amountB,
		MinLPTokens: minLPTokens,
	}

	addDataBytes, err := json.Marshal(addData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal add liquidity data: %w", err)
	}

	txBuilder.SetData(addDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// CreateRemoveLiquidityTransaction creates a transaction that removes liquidity from a pool
func CreateRemoveLiquidityTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore, poolRegistry *PoolRegistry,
	poolID string, lpTokens uint64, minAmountA uint64, minAmountB uint64) (*Transaction, error) {

	// Get the pool
	pool, err := poolRegistry.GetPool(poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// Get UTXOs
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	genesisTokenID := GetGenesisToken().TokenID

	// Find LP token UTXOs and SHADOW for fees
	var availableLPUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == pool.LPTokenID {
				availableLPUTXOs = append(availableLPUTXOs, utxo)
			} else if utxo.Output.TokenID == genesisTokenID {
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			}
		}
	}

	// Select LP token UTXOs
	var selectedLPUTXOs []*UTXO
	var lpTokenTotal uint64
	for _, utxo := range availableLPUTXOs {
		selectedLPUTXOs = append(selectedLPUTXOs, utxo)
		lpTokenTotal += utxo.Output.Amount
		if lpTokenTotal >= lpTokens {
			break
		}
	}

	if lpTokenTotal < lpTokens {
		return nil, fmt.Errorf("insufficient LP tokens: have %d, need %d", lpTokenTotal, lpTokens)
	}

	// Calculate fee and select SHADOW
	estimatedFee := uint64(len(selectedLPUTXOs)+4) * 1150
	if estimatedFee < 11500 {
		estimatedFee = 11500
	}

	var selectedShadowUTXOs []*UTXO
	var shadowTotal uint64
	for _, utxo := range availableShadowUTXOs {
		selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
		shadowTotal += utxo.Output.Amount
		if shadowTotal >= estimatedFee {
			break
		}
	}

	if shadowTotal < estimatedFee {
		return nil, fmt.Errorf("insufficient SHADOW for fee: have %d, need %d", shadowTotal, estimatedFee)
	}

	// Build transaction
	txBuilder := NewTxBuilder(TxTypeRemoveLiquidity)

	// Add inputs
	for _, utxo := range selectedLPUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// LP token change
	lpTokenChange := lpTokenTotal - lpTokens
	if lpTokenChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, lpTokenChange, pool.LPTokenID)
	}

	// SHADOW change
	shadowChange := shadowTotal - estimatedFee
	if shadowChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
	}

	// Create remove liquidity data
	removeData := RemoveLiquidityData{
		PoolID:     poolID,
		LPTokens:   lpTokens,
		MinAmountA: minAmountA,
		MinAmountB: minAmountB,
	}

	removeDataBytes, err := json.Marshal(removeData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal remove liquidity data: %w", err)
	}

	txBuilder.SetData(removeDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// CreateSwapTransaction creates a transaction that swaps tokens through a liquidity pool
func CreateSwapTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore, poolRegistry *PoolRegistry,
	poolID string, tokenIn string, amountIn uint64, minAmountOut uint64) (*Transaction, error) {

	// Get the pool
	pool, err := poolRegistry.GetPool(poolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	// Validate that tokenIn is one of the pool's tokens
	if tokenIn != pool.TokenA && tokenIn != pool.TokenB {
		return nil, fmt.Errorf("token %s is not in pool (pool has %s/%s)", tokenIn[:8], pool.TokenA[:8], pool.TokenB[:8])
	}

	// Get UTXOs
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	genesisTokenID := GetGenesisToken().TokenID

	// Filter UTXOs
	var availableTokenInUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == tokenIn {
				availableTokenInUTXOs = append(availableTokenInUTXOs, utxo)
			} else if utxo.Output.TokenID == genesisTokenID {
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			}
		}
	}

	// Select input token UTXOs
	var selectedTokenInUTXOs []*UTXO
	var tokenInTotal uint64
	for _, utxo := range availableTokenInUTXOs {
		selectedTokenInUTXOs = append(selectedTokenInUTXOs, utxo)
		tokenInTotal += utxo.Output.Amount
		if tokenInTotal >= amountIn {
			break
		}
	}

	if tokenInTotal < amountIn {
		return nil, fmt.Errorf("insufficient input token: have %d, need %d", tokenInTotal, amountIn)
	}

	// Calculate fee and select SHADOW
	estimatedFee := uint64(len(selectedTokenInUTXOs)+4) * 1150
	if estimatedFee < 11500 {
		estimatedFee = 11500
	}

	var selectedShadowUTXOs []*UTXO
	var shadowTotal uint64
	for _, utxo := range availableShadowUTXOs {
		selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
		shadowTotal += utxo.Output.Amount
		if shadowTotal >= estimatedFee {
			break
		}
	}

	if shadowTotal < estimatedFee {
		return nil, fmt.Errorf("insufficient SHADOW for fee: have %d, need %d", shadowTotal, estimatedFee)
	}

	// Build transaction
	txBuilder := NewTxBuilder(TxTypeSwap)

	// Add inputs
	for _, utxo := range selectedTokenInUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Token in change
	tokenInChange := tokenInTotal - amountIn
	if tokenInChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, tokenInChange, tokenIn)
	}

	// SHADOW change
	shadowChange := shadowTotal - estimatedFee
	if shadowChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
	}

	// Create swap data
	swapData := SwapData{
		PoolID:       poolID,
		TokenIn:      tokenIn,
		AmountIn:     amountIn,
		MinAmountOut: minAmountOut,
	}

	swapDataBytes, err := json.Marshal(swapData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal swap data: %w", err)
	}

	txBuilder.SetData(swapDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}
