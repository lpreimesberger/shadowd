package lib

import (
	"fmt"
	"time"
)

// CreateCoinbaseTransaction creates a coinbase transaction for mining rewards
func CreateCoinbaseTransaction(minerAddress Address, blockHeight uint64, reward uint64, blockTimestamp int64) *Transaction {
	builder := NewTxBuilder(TxTypeCoinbase)

	// Set deterministic timestamp from block height (critical for consensus)
	// Using block height instead of wall-clock time ensures perfect determinism
	builder.SetTimestamp(int64(blockHeight))

	// Add mining reward output
	builder.AddOutput(minerAddress, reward, "SHADOW")

	// Add block height data
	blockData := fmt.Sprintf("block_height_%d", blockHeight)
	builder.SetData([]byte(blockData))

	return builder.Build()
}

// CreateSendTransaction creates a regular send transaction
func CreateSendTransaction(inputs []*TxInput, outputs []*TxOutput) *Transaction {
	builder := NewTxBuilder(TxTypeSend)

	// Add all inputs
	for _, input := range inputs {
		builder.AddInput(input.PrevTxID, input.OutputIndex)
	}

	// Add all outputs
	for _, output := range outputs {
		builder.AddCustomOutput(output)
	}

	return builder.Build()
}

// CreateSimpleSendTransaction creates a simple send from one address to another
func CreateSimpleSendTransaction(fromUTXOs []*UTXO, toAddress Address, amount uint64, changeAddress Address) (*Transaction, error) {
	if len(fromUTXOs) == 0 {
		return nil, fmt.Errorf("no UTXOs to spend")
	}

	builder := NewTxBuilder(TxTypeSend)

	// Determine which token we're dealing with (use first UTXO's token)
	var tokenID string
	if len(fromUTXOs) > 0 {
		tokenID = fromUTXOs[0].Output.TokenID
	}

	// Add inputs from UTXOs
	totalInput := uint64(0)
	for _, utxo := range fromUTXOs {
		// Only spend UTXOs of the same token type
		if utxo.Output.TokenID != tokenID {
			continue
		}

		builder.AddInput(utxo.TxID, utxo.OutputIndex)
		totalInput += utxo.Output.Amount

		// Break when we have enough to cover the amount + fee
		estimatedFee := CalculateTxFee(TxTypeSend, len(builder.inputs)+1, 2, 0)
		if totalInput >= amount+estimatedFee {
			break
		}
	}

	// Calculate final fee
	fee := CalculateTxFee(TxTypeSend, len(builder.inputs), 2, 0)

	// Check if we have enough
	if totalInput < amount+fee {
		return nil, fmt.Errorf("insufficient funds: have %d, need %d", totalInput, amount+fee)
	}

	// Add recipient output (use the token ID from UTXOs)
	builder.AddOutput(toAddress, amount, tokenID)

	// Add change output if needed
	change := totalInput - amount - fee
	if change > 0 {
		builder.AddOutput(changeAddress, change, tokenID)
	}

	return builder.Build(), nil
}

// CreateMintTokenTransaction creates a token minting transaction from TokenInfo
func CreateMintTokenTransactionFromTokenInfo(tokenInfo *TokenInfo, mintAmount uint64, recipientAddress Address) *Transaction {
	builder := NewTxBuilder(TxTypeMintToken)

	// Create the token output using the TokenInfo
	tokenOutput := &TxOutput{
		Amount:       mintAmount,
		Address:      recipientAddress,
		TokenID:      tokenInfo.TokenID,
		TokenType:    "custom",
		ScriptPubKey: CreateP2PKHScript(recipientAddress),
		Data:         []byte(fmt.Sprintf("token:%s", tokenInfo.Ticker)),
	}

	builder.AddCustomOutput(tokenOutput)

	// Add token creation data with staking requirement
	stakingRequired := tokenInfo.CalculateStakingRequirement()
	tokenData := fmt.Sprintf("token_mint_%s_%d_stake_%d", tokenInfo.TokenID, mintAmount, stakingRequired)
	builder.SetData([]byte(tokenData))

	// Set the TokenID in the transaction for tracking
	tx := builder.Build()
	tx.TokenID = tokenInfo.TokenID

	return tx
}

// CreateMintTokenTransaction creates a token minting transaction (legacy)
func CreateMintTokenTransaction(tokenID, tokenType string, mintAmount uint64, recipientAddress Address, metadata []byte) *Transaction {
	builder := NewTxBuilder(TxTypeMintToken)

	// Create the token output
	tokenOutput := &TxOutput{
		Amount:       mintAmount,
		Address:      recipientAddress,
		TokenID:      tokenID,
		TokenType:    tokenType,
		ScriptPubKey: CreateP2PKHScript(recipientAddress),
		Data:         metadata,
	}

	builder.AddCustomOutput(tokenOutput)

	// Add token creation data
	tokenData := fmt.Sprintf("token_mint_%s_%d", tokenID, mintAmount)
	builder.SetData([]byte(tokenData))

	tx := builder.Build()
	tx.TokenID = tokenID

	return tx
}

// CreateMeltTransaction creates a token melting (destruction) transaction
func CreateMeltTransaction(inputUTXOs []*UTXO, meltReason string) *Transaction {
	builder := NewTxBuilder(TxTypeMelt)

	// Add all inputs to be melted
	for _, utxo := range inputUTXOs {
		builder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Add melt reason data
	meltData := fmt.Sprintf("melt_reason_%s", meltReason)
	builder.SetData([]byte(meltData))

	return builder.Build()
}

// CreatePartialMeltTransaction melts some tokens but returns change
func CreatePartialMeltTransaction(inputUTXOs []*UTXO, meltAmount uint64, changeAddress Address, meltReason string) (*Transaction, error) {
	builder := NewTxBuilder(TxTypeMelt)

	totalToMelt := uint64(0)
	tokenID := ""

	// Add inputs and calculate total
	for _, utxo := range inputUTXOs {
		builder.AddInput(utxo.TxID, utxo.OutputIndex)
		totalToMelt += utxo.Output.Amount

		if tokenID == "" {
			tokenID = utxo.Output.TokenID
		} else if tokenID != utxo.Output.TokenID {
			return nil, fmt.Errorf("cannot melt different token types in same transaction")
		}
	}

	if totalToMelt < meltAmount {
		return nil, fmt.Errorf("insufficient tokens to melt: have %d, want to melt %d", totalToMelt, meltAmount)
	}

	// Return change if any
	change := totalToMelt - meltAmount
	if change > 0 {
		changeOutput := &TxOutput{
			Amount:       change,
			Address:      changeAddress,
			TokenID:      tokenID,
			TokenType:    "custom",
			ScriptPubKey: CreateP2PKHScript(changeAddress),
		}
		builder.AddCustomOutput(changeOutput)
	}

	// Add melt data
	meltData := fmt.Sprintf("melt_%s_amount_%d_reason_%s", tokenID, meltAmount, meltReason)
	builder.SetData([]byte(meltData))

	return builder.Build(), nil
}

// CreateSwapOfferTransaction creates a swap offer (special mint transaction)
func CreateSwapOfferTransaction(offerAddress Address, offerTokenID string, offerAmount uint64, requestTokenID string, requestAmount uint64) *Transaction {
	builder := NewTxBuilder(TxTypeMintToken)

	// Create swap offer token
	swapOfferID := fmt.Sprintf("SWAP_%s_%d_FOR_%s_%d_%d", offerTokenID, offerAmount, requestTokenID, requestAmount, time.Now().Unix())

	swapOutput := &TxOutput{
		Amount:       1, // One swap offer token
		Address:      offerAddress,
		TokenID:      swapOfferID,
		TokenType:    "swap_offer",
		ScriptPubKey: CreateP2PKHScript(offerAddress),
		Data:         []byte(fmt.Sprintf("offer:%s:%d:request:%s:%d", offerTokenID, offerAmount, requestTokenID, requestAmount)),
	}

	builder.AddCustomOutput(swapOutput)
	builder.SetData([]byte(fmt.Sprintf("swap_offer_%s", swapOfferID)))

	return builder.Build()
}

// CreateRevokeSwapTransaction creates a melt transaction to revoke a swap offer
func CreateRevokeSwapTransaction(swapOfferUTXO *UTXO) *Transaction {
	if swapOfferUTXO.Output.TokenType != "swap_offer" {
		panic("not a swap offer UTXO")
	}

	return CreateMeltTransaction([]*UTXO{swapOfferUTXO}, "revoke_swap_offer")
}

// CreateMultiTokenMintTransaction creates a transaction that mints multiple token types
func CreateMultiTokenMintTransaction(mints []TokenMint, creatorAddress Address) *Transaction {
	builder := NewTxBuilder(TxTypeMintToken)

	for _, mint := range mints {
		tokenOutput := &TxOutput{
			Amount:       mint.Amount,
			Address:      mint.RecipientAddress,
			TokenID:      mint.TokenID,
			TokenType:    mint.TokenType,
			ScriptPubKey: CreateP2PKHScript(mint.RecipientAddress),
			Data:         mint.Metadata,
		}
		builder.AddCustomOutput(tokenOutput)
	}

	// Add creation data
	builder.SetData([]byte(fmt.Sprintf("multi_token_mint_by_%s", creatorAddress.String()[:16])))

	return builder.Build()
}

// TokenMint represents a token to be minted
type TokenMint struct {
	TokenID          string
	TokenType        string
	Amount           uint64
	RecipientAddress Address
	Metadata         []byte
}

// CreateBatchSendTransaction creates a transaction that sends to multiple recipients
func CreateBatchSendTransaction(inputUTXOs []*UTXO, recipients []SendRecipient, changeAddress Address) (*Transaction, error) {
	builder := NewTxBuilder(TxTypeSend)

	// Add all inputs
	totalInput := uint64(0)
	for _, utxo := range inputUTXOs {
		if utxo.Output.TokenID != "SHADOW" {
			continue // Only handle SHADOW for batch sends
		}
		builder.AddInput(utxo.TxID, utxo.OutputIndex)
		totalInput += utxo.Output.Amount
	}

	// Calculate total to send
	totalToSend := uint64(0)
	for _, recipient := range recipients {
		totalToSend += recipient.Amount
	}

	// Calculate fee
	outputCount := len(recipients)
	if totalInput > totalToSend {
		outputCount++ // Change output
	}
	fee := CalculateTxFee(TxTypeSend, len(builder.inputs), outputCount, 0)

	// Check if we have enough
	if totalInput < totalToSend+fee {
		return nil, fmt.Errorf("insufficient funds for batch send: have %d, need %d",
			totalInput, totalToSend+fee)
	}

	// Add recipient outputs
	for _, recipient := range recipients {
		builder.AddOutput(recipient.Address, recipient.Amount, "SHADOW")
	}

	// Add change output if needed
	change := totalInput - totalToSend - fee
	if change > 0 {
		builder.AddOutput(changeAddress, change, "SHADOW")
	}

	return builder.Build(), nil
}

// SendRecipient represents a recipient in a batch send
type SendRecipient struct {
	Address Address
	Amount  uint64
}

// GetTransactionSummary returns a human-readable summary of a transaction
func GetTransactionSummary(tx *Transaction) string {
	switch tx.TxType {
	case TxTypeCoinbase:
		totalReward := tx.GetTotalOutputAmount()
		return fmt.Sprintf("Coinbase: Mining reward of %s SHADOW", FormatAmount(totalReward))

	case TxTypeSend:
		totalOut := tx.GetTotalOutputAmount()
		return fmt.Sprintf("Send: %s SHADOW (%d inputs → %d outputs)",
			FormatAmount(totalOut), len(tx.Inputs), len(tx.Outputs))

	case TxTypeMintToken:
		tokenTypes := tx.GetTokenTypes()
		return fmt.Sprintf("Mint: Created %d token types: %v", len(tokenTypes), tokenTypes)

	case TxTypeMelt:
		return fmt.Sprintf("Melt: Destroyed tokens (%d inputs → %d outputs)",
			len(tx.Inputs), len(tx.Outputs))

	default:
		return fmt.Sprintf("Unknown transaction type: %s", tx.TxType.String())
	}
}
