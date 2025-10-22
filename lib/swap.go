package lib

import (
	"encoding/json"
	"fmt"
)

// OfferData represents the data stored in a TX_OFFER transaction
type OfferData struct {
	HaveTokenID  string `json:"have_token_id"`  // Token being offered
	WantTokenID  string `json:"want_token_id"`  // Token wanted in exchange
	HaveAmount   uint64 `json:"have_amount"`    // Amount of have token
	WantAmount   uint64 `json:"want_amount"`    // Amount of want token
	ExpiresAtBlock uint64 `json:"expires_at_block"` // Block height when offer expires
	OfferAddress Address `json:"offer_address"`  // Address that created the offer
}

// AcceptOfferData represents the data stored in a TX_ACCEPT_OFFER transaction
type AcceptOfferData struct {
	OfferTxID string `json:"offer_tx_id"` // Transaction ID of the offer being accepted
}

// CancelOfferData represents the data stored in a TX_CANCEL_OFFER transaction
type CancelOfferData struct {
	OfferTxID string `json:"offer_tx_id"` // Transaction ID of the offer being cancelled
}

// CreateOfferTransaction creates a transaction that locks tokens for an atomic swap offer
func CreateOfferTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore,
	haveTokenID string, wantTokenID string,
	haveAmount uint64, wantAmount uint64, expiresAtBlock uint64) (*Transaction, error) {

	// Get UTXOs for the token being offered
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	// Filter for unspent UTXOs of the "have" token
	var availableTokenUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO
	genesisTokenID := GetGenesisToken().TokenID

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == haveTokenID {
				availableTokenUTXOs = append(availableTokenUTXOs, utxo)
			} else if utxo.Output.TokenID == genesisTokenID {
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			}
		}
	}

	// Select token UTXOs to cover the offer amount
	var selectedTokenUTXOs []*UTXO
	var tokenTotal uint64
	for _, utxo := range availableTokenUTXOs {
		selectedTokenUTXOs = append(selectedTokenUTXOs, utxo)
		tokenTotal += utxo.Output.Amount
		if tokenTotal >= haveAmount {
			break
		}
	}

	if tokenTotal < haveAmount {
		return nil, fmt.Errorf("insufficient balance: have %d, need %d", tokenTotal, haveAmount)
	}

	// Select SHADOW UTXOs for transaction fee
	estimatedFee := uint64(len(selectedTokenUTXOs)+2) * 1150
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
	txBuilder := NewTxBuilder(TxTypeOffer)

	// Add token inputs (these will be locked by the offer)
	for _, utxo := range selectedTokenUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Add SHADOW inputs for fee
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// No outputs for the locked tokens - they stay locked until accept/cancel
	// Only output is SHADOW change if any
	shadowChange := shadowTotal - estimatedFee
	if shadowChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
	}

	// Add token change if any
	tokenChange := tokenTotal - haveAmount
	if tokenChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, tokenChange, haveTokenID)
	}

	// Create offer data
	offerData := OfferData{
		HaveTokenID:    haveTokenID,
		WantTokenID:    wantTokenID,
		HaveAmount:     haveAmount,
		WantAmount:     wantAmount,
		ExpiresAtBlock: expiresAtBlock,
		OfferAddress:   nodeWallet.Address,
	}

	offerDataBytes, err := json.Marshal(offerData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal offer data: %w", err)
	}

	txBuilder.SetData(offerDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// CreateAcceptOfferTransaction creates a transaction that accepts and executes an atomic swap offer
func CreateAcceptOfferTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore,
	offerTxID string, currentBlockHeight uint64) (*Transaction, error) {

	// Get the offer transaction
	offerTx, err := utxoStore.GetTransaction(offerTxID)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer transaction: %w", err)
	}

	if offerTx.TxType != TxTypeOffer {
		return nil, fmt.Errorf("transaction %s is not an offer", offerTxID)
	}

	// Parse offer data
	var offerData OfferData
	if err := json.Unmarshal(offerTx.Data, &offerData); err != nil {
		return nil, fmt.Errorf("failed to parse offer data: %w", err)
	}

	// Check if offer has expired
	if currentBlockHeight > offerData.ExpiresAtBlock {
		return nil, fmt.Errorf("offer has expired (expired at block %d, current block %d)",
			offerData.ExpiresAtBlock, currentBlockHeight)
	}

	// Get UTXOs for the token wanted by the offer
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	// Filter for unspent UTXOs
	genesisTokenID := GetGenesisToken().TokenID
	wantingShadow := offerData.WantTokenID == genesisTokenID

	// Estimate fee first so we know total SHADOW needed
	estimatedFee := uint64(11500) // Will refine after selecting token UTXOs

	var availableShadowUTXOs []*UTXO
	var availableTokenUTXOs []*UTXO

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == genesisTokenID {
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			} else if utxo.Output.TokenID == offerData.WantTokenID {
				availableTokenUTXOs = append(availableTokenUTXOs, utxo)
			}
		}
	}

	// Select UTXOs based on whether we're trading SHADOW or custom tokens
	var selectedTokenUTXOs []*UTXO
	var selectedShadowUTXOs []*UTXO
	var tokenTotal uint64
	var shadowTotal uint64

	if wantingShadow {
		// We're providing SHADOW - need to cover both want amount AND fee
		totalNeeded := offerData.WantAmount + estimatedFee
		for _, utxo := range availableShadowUTXOs {
			selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
			shadowTotal += utxo.Output.Amount
			if shadowTotal >= totalNeeded {
				break
			}
		}
		// Refine fee estimate
		estimatedFee = uint64(len(selectedShadowUTXOs)+4) * 1150
		if estimatedFee < 11500 {
			estimatedFee = 11500
		}
		totalNeeded = offerData.WantAmount + estimatedFee

		if shadowTotal < totalNeeded {
			return nil, fmt.Errorf("insufficient SHADOW: have %d, need %d (swap) + %d (fee) = %d",
				shadowTotal, offerData.WantAmount, estimatedFee, totalNeeded)
		}
	} else {
		// We're providing custom tokens - select token UTXOs and separate SHADOW for fee
		for _, utxo := range availableTokenUTXOs {
			selectedTokenUTXOs = append(selectedTokenUTXOs, utxo)
			tokenTotal += utxo.Output.Amount
			if tokenTotal >= offerData.WantAmount {
				break
			}
		}

		if tokenTotal < offerData.WantAmount {
			return nil, fmt.Errorf("insufficient %s: have %d, need %d",
				offerData.WantTokenID, tokenTotal, offerData.WantAmount)
		}

		// Refine fee estimate
		estimatedFee = uint64(len(selectedTokenUTXOs)+4) * 1150
		if estimatedFee < 11500 {
			estimatedFee = 11500
		}

		// Select SHADOW UTXOs for fee
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
	txBuilder := NewTxBuilder(TxTypeAcceptOffer)

	// Add inputs (either tokens or SHADOW, depending on what we're trading)
	for _, utxo := range selectedTokenUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Create outputs for the swap:
	// 1. Send offer's "have" tokens to accepter
	txBuilder.AddOutput(nodeWallet.Address, offerData.HaveAmount, offerData.HaveTokenID)

	// 2. Send accepter's "want" tokens to original offerer
	txBuilder.AddOutput(offerData.OfferAddress, offerData.WantAmount, offerData.WantTokenID)

	// 3. Handle change based on what we're trading
	if wantingShadow {
		// We provided SHADOW - calculate change after deducting swap amount AND fee
		shadowChange := shadowTotal - offerData.WantAmount - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		// We provided custom tokens - handle token change and SHADOW change separately
		tokenChange := tokenTotal - offerData.WantAmount
		if tokenChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, tokenChange, offerData.WantTokenID)
		}
		shadowChange := shadowTotal - estimatedFee
		if shadowChange > 0 {
			txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
		}
	}

	// Create accept data
	acceptData := AcceptOfferData{
		OfferTxID: offerTxID,
	}

	acceptDataBytes, err := json.Marshal(acceptData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal accept data: %w", err)
	}

	txBuilder.SetData(acceptDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

// CreateCancelOfferTransaction creates a transaction that cancels an offer and returns locked tokens
func CreateCancelOfferTransaction(nodeWallet *NodeWallet, utxoStore *UTXOStore,
	offerTxID string, currentBlockHeight uint64) (*Transaction, error) {

	// Get the offer transaction
	offerTx, err := utxoStore.GetTransaction(offerTxID)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer transaction: %w", err)
	}

	if offerTx.TxType != TxTypeOffer {
		return nil, fmt.Errorf("transaction %s is not an offer", offerTxID)
	}

	// Parse offer data
	var offerData OfferData
	if err := json.Unmarshal(offerTx.Data, &offerData); err != nil {
		return nil, fmt.Errorf("failed to parse offer data: %w", err)
	}

	// Verify ownership OR expiry
	if offerData.OfferAddress != nodeWallet.Address && currentBlockHeight <= offerData.ExpiresAtBlock {
		return nil, fmt.Errorf("cannot cancel: not owner and offer not expired")
	}

	// Get SHADOW UTXOs for transaction fee
	utxos, err := utxoStore.GetUTXOsByAddress(nodeWallet.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	var availableShadowUTXOs []*UTXO
	genesisTokenID := GetGenesisToken().TokenID

	for _, utxo := range utxos {
		if !utxo.IsSpent && utxo.Output.TokenID == genesisTokenID {
			availableShadowUTXOs = append(availableShadowUTXOs, utxo)
		}
	}

	// Select SHADOW UTXOs for transaction fee
	estimatedFee := uint64(11500)

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
	txBuilder := NewTxBuilder(TxTypeCancelOffer)

	// Add SHADOW inputs for fee
	for _, utxo := range selectedShadowUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Output: Return locked tokens to original offerer
	txBuilder.AddOutput(offerData.OfferAddress, offerData.HaveAmount, offerData.HaveTokenID)

	// Handle SHADOW change
	shadowChange := shadowTotal - estimatedFee
	if shadowChange > 0 {
		txBuilder.AddOutput(nodeWallet.Address, shadowChange, genesisTokenID)
	}

	// Create cancel data
	cancelData := CancelOfferData{
		OfferTxID: offerTxID,
	}

	cancelDataBytes, err := json.Marshal(cancelData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cancel data: %w", err)
	}

	txBuilder.SetData(cancelDataBytes)

	// Build and sign
	tx := txBuilder.Build()
	if err := nodeWallet.SignTransaction(tx); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}
