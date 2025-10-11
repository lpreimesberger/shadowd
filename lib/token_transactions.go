package lib

import (
	"encoding/json"
	"fmt"
)

// TokenMintData represents the metadata stored in TX_MINT transaction Data field
type TokenMintData struct {
	Ticker      string `json:"ticker"`       // 3-32 chars, [A-Za-z0-9]
	Desc        string `json:"desc"`         // 0-64 chars, [A-Za-z0-9]
	MaxMint     uint64 `json:"max_mint"`     // Max base units (1 to 21M)
	MaxDecimals uint8  `json:"max_decimals"` // 0-8 decimals
	MintVersion uint8  `json:"mint_version"` // Currently 0
}

// CreateTokenMintTransaction creates a TX_MINT transaction per spec
// Inputs: SHADOW UTXOs totaling (MAX_MINT * 10^MAX_DECIMALS) + fee
// Outputs: Single token UTXO with full supply sent to creator
func CreateTokenMintTransaction(
	creator Address,
	shadowUTXOs []*UTXO,
	ticker string,
	desc string,
	maxMint uint64,
	maxDecimals uint8,
) (*Transaction, error) {
	// Calculate total supply
	totalSupply := maxMint
	for i := uint8(0); i < maxDecimals; i++ {
		totalSupply *= 10
	}

	// Validate ticker/desc format
	if len(ticker) < 3 || len(ticker) > 32 {
		return nil, fmt.Errorf("ticker must be 3-32 characters")
	}
	if len(desc) > 64 {
		return nil, fmt.Errorf("desc must be 0-64 characters")
	}
	if maxMint == 0 || maxMint > 21_000_000 {
		return nil, fmt.Errorf("max_mint must be 1 to 21,000,000")
	}
	if maxDecimals > 8 {
		return nil, fmt.Errorf("max_decimals cannot exceed 8")
	}

	builder := NewTxBuilder(TxTypeMintToken)

	// Add SHADOW UTXOs as inputs
	totalShadowInput := uint64(0)
	for _, utxo := range shadowUTXOs {
		if utxo.Output.TokenID != GetGenesisToken().TokenID {
			return nil, fmt.Errorf("mint transaction can only use SHADOW inputs")
		}
		builder.AddInput(utxo.TxID, utxo.OutputIndex)
		totalShadowInput += utxo.Output.Amount
	}

	// Calculate fee
	fee := CalculateTxFee(TxTypeMintToken, len(builder.inputs), 2, 0) // Token output + change

	// Check we have enough SHADOW for staking + fee
	requiredShadow := totalSupply + fee
	if totalShadowInput < requiredShadow {
		return nil, fmt.Errorf("insufficient SHADOW: have %d, need %d (stake %d + fee %d)",
			totalShadowInput, requiredShadow, totalSupply, fee)
	}

	// Create token metadata
	mintData := TokenMintData{
		Ticker:      ticker,
		Desc:        desc,
		MaxMint:     maxMint,
		MaxDecimals: maxDecimals,
		MintVersion: 0,
	}

	mintDataBytes, err := json.Marshal(mintData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mint data: %w", err)
	}

	builder.SetData(mintDataBytes)

	// Build transaction to get TX ID (token ID will be this TX's ID)
	tx := builder.Build()

	// Get transaction ID
	txID, err := tx.ID()
	if err != nil {
		return nil, fmt.Errorf("failed to calculate TX ID: %w", err)
	}

	// NOW we can add the token output (with the correct token ID = this TX ID)
	tokenOutput := &TxOutput{
		Amount:       totalSupply,
		Address:      creator,
		TokenID:      txID, // Token ID is the TX ID of this minting transaction
		TokenType:    "custom",
		LockedShadow: totalSupply, // 1:1 SHADOW locked
		ScriptPubKey: CreateP2PKHScript(creator),
	}

	// Rebuild with token output
	builder.AddCustomOutput(tokenOutput)

	// Add SHADOW change output if any
	shadowChange := totalShadowInput - totalSupply - fee
	if shadowChange > 0 {
		shadowChangeOutput := CreateShadowOutput(creator, shadowChange)
		builder.AddCustomOutput(shadowChangeOutput)
	}

	return builder.Build(), nil
}

// CreateTokenMeltTransaction creates a TX_MELT transaction per spec
// Inputs: Token UTXOs to melt
// Outputs: Unlocked SHADOW (proportional to melted amount) + token change (if partial melt)
func CreateTokenMeltTransaction(
	tokenUTXOs []*UTXO,
	meltAmount uint64,
	changeAddress Address,
	shadowRecipient Address,
) (*Transaction, error) {
	if len(tokenUTXOs) == 0 {
		return nil, fmt.Errorf("no token UTXOs to melt")
	}

	// Verify all UTXOs are same token
	tokenID := tokenUTXOs[0].Output.TokenID
	genesisTokenID := GetGenesisToken().TokenID

	if tokenID == genesisTokenID {
		return nil, fmt.Errorf("cannot melt SHADOW token")
	}

	builder := NewTxBuilder(TxTypeMelt)

	// Add token inputs and calculate totals
	totalTokens := uint64(0)
	totalLockedShadow := uint64(0)

	for _, utxo := range tokenUTXOs {
		if utxo.Output.TokenID != tokenID {
			return nil, fmt.Errorf("all token UTXOs must be same token ID")
		}

		builder.AddInput(utxo.TxID, utxo.OutputIndex)
		totalTokens += utxo.Output.Amount
		totalLockedShadow += utxo.Output.LockedShadow
	}

	if totalTokens < meltAmount {
		return nil, fmt.Errorf("insufficient tokens to melt: have %d, want %d", totalTokens, meltAmount)
	}

	// Calculate proportional SHADOW to unlock
	unlockedShadow := (meltAmount * totalLockedShadow) / totalTokens

	// Add unlocked SHADOW output
	shadowOutput := CreateShadowOutput(shadowRecipient, unlockedShadow)
	builder.AddCustomOutput(shadowOutput)

	// Add token change output if partial melt
	tokenChange := totalTokens - meltAmount
	if tokenChange > 0 {
		// Proportional locked SHADOW for change
		changeLockedShadow := totalLockedShadow - unlockedShadow

		tokenChangeOutput := &TxOutput{
			Amount:       tokenChange,
			Address:      changeAddress,
			TokenID:      tokenID,
			TokenType:    "custom",
			LockedShadow: changeLockedShadow,
			ScriptPubKey: CreateP2PKHScript(changeAddress),
		}
		builder.AddCustomOutput(tokenChangeOutput)
	}

	// Add melt metadata
	meltData := fmt.Sprintf("melt_%s_amount_%d", tokenID, meltAmount)
	builder.SetData([]byte(meltData))

	return builder.Build(), nil
}

// ValidateTokenMintTransaction validates a TX_MINT transaction per spec
func ValidateTokenMintTransaction(tx *Transaction, registry *TokenRegistry) error {
	if tx.TxType != TxTypeMintToken {
		return fmt.Errorf("not a mint transaction")
	}

	// Parse mint data
	var mintData TokenMintData
	if err := json.Unmarshal(tx.Data, &mintData); err != nil {
		return fmt.Errorf("invalid mint data: %w", err)
	}

	// Validate mint parameters
	if len(mintData.Ticker) < 3 || len(mintData.Ticker) > 32 {
		return fmt.Errorf("invalid ticker length")
	}
	if len(mintData.Desc) > 64 {
		return fmt.Errorf("invalid desc length")
	}
	if mintData.MaxMint == 0 || mintData.MaxMint > 21_000_000 {
		return fmt.Errorf("invalid max_mint: %d", mintData.MaxMint)
	}
	if mintData.MaxDecimals > 8 {
		return fmt.Errorf("max_decimals exceeds 8")
	}
	if mintData.MintVersion != 0 {
		return fmt.Errorf("mint_version must be 0")
	}

	// Check ticker availability
	if err := registry.CheckTickerAvailable(mintData.Ticker); err != nil {
		return err
	}

	// Calculate expected total supply
	totalSupply := mintData.MaxMint
	for i := uint8(0); i < mintData.MaxDecimals; i++ {
		totalSupply *= 10
	}

	// Validate outputs - should have exactly one token output
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("mint transaction must have at least one output")
	}

	// Find token output
	var tokenOutput *TxOutput
	for _, output := range tx.Outputs {
		if output.TokenType == "custom" {
			if tokenOutput != nil {
				return fmt.Errorf("mint transaction can only create one token type")
			}
			tokenOutput = output
		}
	}

	if tokenOutput == nil {
		return fmt.Errorf("no token output found")
	}

	// Validate token output
	txID, _ := tx.ID()
	if tokenOutput.TokenID != txID {
		return fmt.Errorf("token ID must equal TX ID")
	}

	if tokenOutput.Amount != totalSupply {
		return fmt.Errorf("token output amount (%d) doesn't match total supply (%d)",
			tokenOutput.Amount, totalSupply)
	}

	if tokenOutput.LockedShadow != totalSupply {
		return fmt.Errorf("locked SHADOW (%d) must equal total supply (%d)",
			tokenOutput.LockedShadow, totalSupply)
	}

	return nil
}

// ValidateTokenMeltTransaction validates a TX_MELT transaction per spec
func ValidateTokenMeltTransaction(tx *Transaction, utxoStore *UTXOStore) error {
	if tx.TxType != TxTypeMelt {
		return fmt.Errorf("not a melt transaction")
	}

	if len(tx.Inputs) == 0 {
		return fmt.Errorf("melt transaction must have inputs")
	}

	// Get token being melted (from first input)
	firstInput := tx.Inputs[0]
	firstUTXO, err := utxoStore.GetUTXO(firstInput.PrevTxID, firstInput.OutputIndex)
	if err != nil || firstUTXO == nil {
		return fmt.Errorf("input UTXO not found: %v", err)
	}

	tokenID := firstUTXO.Output.TokenID
	genesisTokenID := GetGenesisToken().TokenID

	// Cannot melt SHADOW
	if tokenID == genesisTokenID {
		return fmt.Errorf("cannot melt SHADOW token")
	}

	// Verify all inputs are same token and calculate totals
	totalTokens := uint64(0)
	totalLockedShadow := uint64(0)

	for _, input := range tx.Inputs {
		utxo, err := utxoStore.GetUTXO(input.PrevTxID, input.OutputIndex)
		if err != nil || utxo == nil {
			return fmt.Errorf("input UTXO not found: %s:%d - %v", input.PrevTxID, input.OutputIndex, err)
		}

		if utxo.Output.TokenID != tokenID {
			return fmt.Errorf("all inputs must be same token")
		}

		totalTokens += utxo.Output.Amount
		totalLockedShadow += utxo.Output.LockedShadow
	}

	// Verify outputs - should have SHADOW output, optionally token change
	shadowOutput := uint64(0)
	tokenChange := uint64(0)
	tokenChangeLocked := uint64(0)

	for _, output := range tx.Outputs {
		if output.TokenID == genesisTokenID {
			shadowOutput += output.Amount
		} else if output.TokenID == tokenID {
			tokenChange += output.Amount
			tokenChangeLocked += output.LockedShadow
		} else {
			return fmt.Errorf("unexpected token in output: %s", output.TokenID)
		}
	}

	// Melted amount = total tokens - token change
	meltedTokens := totalTokens - tokenChange

	// Verify proportional SHADOW unlocked
	expectedShadow := (meltedTokens * totalLockedShadow) / totalTokens
	if shadowOutput != expectedShadow {
		return fmt.Errorf("incorrect SHADOW unlocked: got %d, expected %d",
			shadowOutput, expectedShadow)
	}

	// Verify token change has proportional locked SHADOW
	if tokenChange > 0 {
		expectedChangeLocked := totalLockedShadow - expectedShadow
		if tokenChangeLocked != expectedChangeLocked {
			return fmt.Errorf("incorrect locked SHADOW in change: got %d, expected %d",
				tokenChangeLocked, expectedChangeLocked)
		}
	}

	return nil
}
