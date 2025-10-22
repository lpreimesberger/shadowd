package lib

import (
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/blake2b"
	"time"
)

// Transaction represents a blockchain transaction using UTXO model
type Transaction struct {
	// Transaction metadata
	TxType     TxType `json:"tx_type"`               // Required: type of transaction
	Version    uint32 `json:"version"`               // Transaction version
	Timestamp  int64  `json:"timestamp"`             // Transaction timestamp
	LockTime   uint32 `json:"lock_time"`             // Lock time (0 = immediate)
	MempoolTTL uint32 `json:"mempool_ttl,omitempty"` // Block epoch to discard tx if no match
	TokenID    string `json:"token_id"`              // Hash of token being operated on
	// UTXO inputs and outputs
	Inputs  []*TxInput  `json:"inputs"`  // Transaction inputs (UTXOs being spent)
	Outputs []*TxOutput `json:"outputs"` // Transaction outputs (new UTXOs being created)

	// Transaction-specific data
	Data []byte `json:"data,omitempty"` // Optional transaction data

	// Signature fields (for backward compatibility and simple validation)
	PublicKey []byte `json:"public_key,omitempty"` // Public key of primary signer
	Signature []byte `json:"signature,omitempty"`  // Primary signature

	// Legacy fields (deprecated but kept for migration)
	From   *Address `json:"from,omitempty"`   // Deprecated: use inputs instead
	To     *Address `json:"to,omitempty"`     // Deprecated: use outputs instead
	Amount *uint64  `json:"amount,omitempty"` // Deprecated: use outputs instead
	Fee    *uint64  `json:"fee,omitempty"`    // Deprecated: calculated from inputs/outputs
	Nonce  *uint64  `json:"nonce,omitempty"`  // Deprecated: not needed in UTXO model
}

// TxBuilder helps construct UTXO-based transactions
type TxBuilder struct {
	txType    TxType
	version   uint32
	timestamp int64
	lockTime  uint32
	inputs    []*TxInput
	outputs   []*TxOutput
	data      []byte
}

// NewTxBuilder creates a new transaction builder
func NewTxBuilder(txType TxType) *TxBuilder {
	return &TxBuilder{
		txType:    txType,
		version:   1,
		timestamp: time.Now().Unix(),
		lockTime:  0,
		inputs:    make([]*TxInput, 0),
		outputs:   make([]*TxOutput, 0),
	}
}

// AddInput adds an input to the transaction
func (tb *TxBuilder) AddInput(prevTxID string, outputIndex uint32) *TxBuilder {
	input := NewTxInput(prevTxID, outputIndex)
	tb.inputs = append(tb.inputs, input)
	return tb
}

// AddOutput adds an output to the transaction
func (tb *TxBuilder) AddOutput(address Address, amount uint64, tokenID string) *TxBuilder {
	var output *TxOutput
	genesisTokenID := GetGenesisToken().TokenID

	if tokenID == "" || tokenID == "SHADOW" || tokenID == genesisTokenID {
		output = CreateShadowOutput(address, amount)
	} else {
		output = CreateTokenOutput(address, amount, tokenID, "custom", nil)
	}
	tb.outputs = append(tb.outputs, output)
	return tb
}

// AddCustomOutput adds a custom output with full control
func (tb *TxBuilder) AddCustomOutput(output *TxOutput) *TxBuilder {
	tb.outputs = append(tb.outputs, output)
	return tb
}

// SetData sets optional transaction data
func (tb *TxBuilder) SetData(data []byte) *TxBuilder {
	if data != nil {
		tb.data = make([]byte, len(data))
		copy(tb.data, data)
	}
	return tb
}

// SetTimestamp sets the transaction timestamp
func (tb *TxBuilder) SetTimestamp(ts int64) *TxBuilder {
	tb.timestamp = ts
	return tb
}

// SetLockTime sets the transaction lock time
func (tb *TxBuilder) SetLockTime(lockTime uint32) *TxBuilder {
	tb.lockTime = lockTime
	return tb
}

// Build creates an unsigned transaction
func (tb *TxBuilder) Build() *Transaction {
	tx := &Transaction{
		TxType:    tb.txType,
		Version:   tb.version,
		Timestamp: tb.timestamp,
		LockTime:  tb.lockTime,
		Inputs:    make([]*TxInput, len(tb.inputs)),
		Outputs:   make([]*TxOutput, len(tb.outputs)),
	}

	// Deep copy inputs and outputs
	copy(tx.Inputs, tb.inputs)
	copy(tx.Outputs, tb.outputs)

	if tb.data != nil {
		tx.Data = make([]byte, len(tb.data))
		copy(tx.Data, tb.data)
	}

	return tx
}

// Hash computes the transaction hash (for signing)
func (tx *Transaction) Hash() ([]byte, error) {
	// Create a copy without signature fields for hashing
	unsignedTx := &Transaction{
		TxType:    tx.TxType,
		Version:   tx.Version,
		Timestamp: tx.Timestamp,
		LockTime:  tx.LockTime,
		Inputs:    tx.Inputs,
		Outputs:   tx.Outputs,
		Data:      tx.Data,
		// Exclude signature fields from hash
	}

	bytes, err := json.Marshal(unsignedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	hash := blake2b.Sum256(bytes)
	return hash[:], nil
}

// Sign signs the transaction with the given key pair (simplified signing)
func (tx *Transaction) Sign(kp *KeyPair) error {
	// Get the transaction hash
	hash, err := tx.Hash()
	if err != nil {
		return fmt.Errorf("failed to compute transaction hash: %w", err)
	}

	// Sign the hash
	signature, err := kp.Sign(hash)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Serialize the public key
	pkBytes, err := PublicKeyToBytes(kp.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to serialize public key: %w", err)
	}

	// Set signature fields (simplified - in full implementation each input would have its own signature)
	tx.PublicKey = pkBytes
	tx.Signature = signature

	return nil
}

// ValidateTransaction validates a complete UTXO transaction
func ValidateTransaction(tx *Transaction) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	// Validate transaction type
	if tx.TxType < TxTypeCoinbase || tx.TxType > TxTypeSwap {
		return fmt.Errorf("invalid transaction type: %d", int(tx.TxType))
	}

	// Type-specific validation
	switch tx.TxType {
	case TxTypeCoinbase:
		return validateCoinbaseTransaction(tx)
	case TxTypeSend:
		return validateSendTransaction(tx)
	case TxTypeMintToken:
		return validateMintTokenTransaction(tx)
	case TxTypeMelt:
		return validateMeltTransaction(tx)
	case TxTypeRegisterValidator:
		return validateRegisterValidatorTransaction(tx)
	case TxTypeOffer:
		return validateOfferTransaction(tx)
	case TxTypeAcceptOffer:
		return validateAcceptOfferTransaction(tx)
	case TxTypeCancelOffer:
		return validateCancelOfferTransaction(tx)
	case TxTypeCreatePool:
		return validateCreatePoolTransaction(tx)
	case TxTypeAddLiquidity:
		return validateAddLiquidityTransaction(tx)
	case TxTypeRemoveLiquidity:
		return validateRemoveLiquidityTransaction(tx)
	case TxTypeSwap:
		return validateSwapTransaction(tx)
	default:
		return fmt.Errorf("unsupported transaction type: %s", tx.TxType.String())
	}
}

// validateRegisterValidatorTransaction validates validator registration transactions
func validateRegisterValidatorTransaction(tx *Transaction) error {
	// Validator registration should have no inputs or outputs (state change only)
	if len(tx.Inputs) != 0 {
		return fmt.Errorf("validator registration must have no inputs")
	}
	if len(tx.Outputs) != 0 {
		return fmt.Errorf("validator registration must have no outputs")
	}
	// Data should contain proposer address (20 bytes) + wallet address (32 bytes)
	if len(tx.Data) != 52 {
		return fmt.Errorf("validator registration data must be 52 bytes (20 + 32), got %d", len(tx.Data))
	}
	return nil
}

// validateCoinbaseTransaction validates coinbase (mining reward) transactions
func validateCoinbaseTransaction(tx *Transaction) error {
	// Coinbase transactions should have no inputs (money creation)
	if len(tx.Inputs) != 0 {
		return fmt.Errorf("coinbase transaction must have no inputs")
	}

	// Must have at least one output
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("coinbase transaction must have at least one output")
	}

	// All outputs should be SHADOW tokens
	genesisTokenID := GetGenesisToken().TokenID
	for i, output := range tx.Outputs {
		if output.TokenID != genesisTokenID {
			return fmt.Errorf("coinbase output %d must be SHADOW tokens, got %s", i, output.TokenID)
		}
		if output.Amount == 0 {
			return fmt.Errorf("coinbase output %d must have non-zero amount", i)
		}
	}

	return nil
}

// validateSendTransaction validates regular send transactions
func validateSendTransaction(tx *Transaction) error {
	// Must have inputs and outputs
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("send transaction must have inputs")
	}
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("send transaction must have outputs")
	}

	// Validate signature (simplified validation)
	if len(tx.PublicKey) == 0 {
		return fmt.Errorf("send transaction must include public key")
	}
	if len(tx.Signature) == 0 {
		return fmt.Errorf("send transaction must be signed")
	}

	// Verify signature
	publicKey, err := PublicKeyFromBytes(tx.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}

	hash, err := tx.Hash()
	if err != nil {
		return fmt.Errorf("failed to compute transaction hash: %w", err)
	}

	if !VerifySignature(hash, tx.Signature, publicKey) {
		return fmt.Errorf("invalid transaction signature")
	}

	return nil
}

// validateMintTokenTransaction validates token minting transactions
func validateMintTokenTransaction(tx *Transaction) error {
	// Basic validation - must have at least one output
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("mint token transaction must have outputs")
	}

	// Must have Data field with mint parameters
	if len(tx.Data) == 0 {
		return fmt.Errorf("mint transaction must have token metadata in Data field")
	}

	// At least one output should be a custom token
	hasCustomToken := false
	for _, output := range tx.Outputs {
		if output.IsTokenOutput() {
			hasCustomToken = true
			break
		}
	}

	if !hasCustomToken {
		return fmt.Errorf("mint token transaction must create at least one custom token")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("mint token transaction must be signed")
	}

	return nil
}

// validateMeltTransaction validates token melting (destruction) transactions
func validateMeltTransaction(tx *Transaction) error {
	// Must have inputs (tokens to destroy)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("melt transaction must have inputs")
	}

	// May or may not have outputs (could destroy everything or have change)

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("melt transaction must be signed")
	}

	return nil
}

// ValidateTransactionWithContext validates a transaction with full blockchain context
// This includes UTXO validation and token registry checks
func ValidateTransactionWithContext(tx *Transaction, utxoStore *UTXOStore, tokenRegistry *TokenRegistry) error {
	// First do basic validation
	if err := ValidateTransaction(tx); err != nil {
		return err
	}

	// Type-specific context validation
	switch tx.TxType {
	case TxTypeMintToken:
		// Use the comprehensive mint validation
		if tokenRegistry != nil {
			return ValidateTokenMintTransaction(tx, tokenRegistry)
		}
	case TxTypeMelt:
		// Use the comprehensive melt validation
		if utxoStore != nil {
			return ValidateTokenMeltTransaction(tx, utxoStore)
		}
	case TxTypeSend:
		// TODO: Validate UTXO inputs exist and are spendable
		// For now, basic validation is enough
	}

	return nil
}

// IsValid returns true if the transaction is valid
func (tx *Transaction) IsValid() bool {
	return ValidateTransaction(tx) == nil
}

// ID returns a unique identifier for the transaction
func (tx *Transaction) ID() (string, error) {
	// Include signature in ID calculation for uniqueness
	idData := struct {
		Hash      []byte
		Signature []byte
	}{}

	hash, err := tx.Hash()
	if err != nil {
		return "", fmt.Errorf("failed to compute hash for ID: %w", err)
	}

	idData.Hash = hash
	idData.Signature = tx.Signature

	bytes, err := json.Marshal(idData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ID data: %w", err)
	}

	idHash := blake2b.Sum256(bytes)
	return fmt.Sprintf("%x", idHash), nil
}

// String returns a human-readable representation of the transaction
func (tx *Transaction) String() string {
	id, _ := tx.ID()
	inputCount := len(tx.Inputs)
	outputCount := len(tx.Outputs)

	return fmt.Sprintf("Transaction{ID: %s, Type: %s, Inputs: %d, Outputs: %d}",
		id[:16]+"...", tx.TxType.String(), inputCount, outputCount)
}

// VerifyOwnership checks if the provided public key matches the transaction's signer
func (tx *Transaction) VerifyOwnership(publicKeyBytes []byte) bool {
	// Compare the provided public key with the transaction's public key
	if len(tx.PublicKey) == 0 || len(publicKeyBytes) == 0 {
		return false
	}

	if len(tx.PublicKey) != len(publicKeyBytes) {
		return false
	}

	for i := range tx.PublicKey {
		if tx.PublicKey[i] != publicKeyBytes[i] {
			return false
		}
	}

	return true
}

// GetTotalInputAmount calculates total input amount (requires UTXO lookup)
func (tx *Transaction) GetTotalInputAmount() uint64 {
	// Note: In a real implementation, this would require looking up the actual UTXOs
	// For now, we'll just return 0 as a placeholder
	return 0
}

// GetTotalOutputAmount calculates total output amount for SHADOW tokens
func (tx *Transaction) GetTotalOutputAmount() uint64 {
	total := uint64(0)
	genesisTokenID := GetGenesisToken().TokenID
	for _, output := range tx.Outputs {
		if output.TokenID == genesisTokenID {
			total += output.Amount
		}
	}
	return total
}

// CalculateFee calculates the transaction fee (inputs - outputs)
func (tx *Transaction) CalculateFee() uint64 {
	if tx.TxType == TxTypeCoinbase {
		return 0 // Coinbase transactions don't pay fees
	}

	// Note: This is simplified. Real implementation would lookup input UTXOs
	return CalculateTxFee(tx.TxType, len(tx.Inputs), len(tx.Outputs), len(tx.Data))
}

// GetOutputsForAddress returns all outputs going to a specific address
func (tx *Transaction) GetOutputsForAddress(address Address) []*TxOutput {
	var outputs []*TxOutput
	for _, output := range tx.Outputs {
		if output.Address == address {
			outputs = append(outputs, output)
		}
	}
	return outputs
}

// HasTokenOutputs returns true if transaction creates any custom tokens
func (tx *Transaction) HasTokenOutputs() bool {
	for _, output := range tx.Outputs {
		if output.IsTokenOutput() {
			return true
		}
	}
	return false
}

// GetTokenTypes returns all unique token types in outputs
func (tx *Transaction) GetTokenTypes() []string {
	tokenMap := make(map[string]bool)
	for _, output := range tx.Outputs {
		tokenMap[output.TokenID] = true
	}

	var tokens []string
	for token := range tokenMap {
		tokens = append(tokens, token)
	}
	return tokens
}

// validateOfferTransaction validates atomic swap offer transactions
func validateOfferTransaction(tx *Transaction) error {
	// Must have inputs (tokens being locked)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("offer transaction must have inputs")
	}

	// Must have Data field with offer metadata
	if len(tx.Data) == 0 {
		return fmt.Errorf("offer transaction must have offer metadata in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("offer transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateAcceptOfferTransaction validates atomic swap accept transactions
func validateAcceptOfferTransaction(tx *Transaction) error {
	// Must have inputs (tokens being exchanged)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("accept offer transaction must have inputs")
	}

	// Must have outputs (tokens being distributed to both parties)
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("accept offer transaction must have outputs")
	}

	// Must have Data field with reference to offer transaction
	if len(tx.Data) == 0 {
		return fmt.Errorf("accept offer transaction must have offer reference in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("accept offer transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateCancelOfferTransaction validates atomic swap cancel transactions
func validateCancelOfferTransaction(tx *Transaction) error {
	// Must have inputs (for fee payment)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("cancel offer transaction must have inputs")
	}

	// Must have outputs (returning locked tokens)
	if len(tx.Outputs) == 0 {
		return fmt.Errorf("cancel offer transaction must have outputs")
	}

	// Must have Data field with reference to offer transaction
	if len(tx.Data) == 0 {
		return fmt.Errorf("cancel offer transaction must have offer reference in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("cancel offer transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateCreatePoolTransaction validates pool creation transactions
func validateCreatePoolTransaction(tx *Transaction) error {
	// Must have inputs (tokens being locked)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("create pool transaction must have inputs")
	}

	// Must have Data field with pool metadata
	if len(tx.Data) == 0 {
		return fmt.Errorf("create pool transaction must have pool metadata in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("create pool transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateAddLiquidityTransaction validates add liquidity transactions
func validateAddLiquidityTransaction(tx *Transaction) error {
	// Must have inputs (tokens being added)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("add liquidity transaction must have inputs")
	}

	// Must have Data field with pool reference and amounts
	if len(tx.Data) == 0 {
		return fmt.Errorf("add liquidity transaction must have pool data in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("add liquidity transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateRemoveLiquidityTransaction validates remove liquidity transactions
func validateRemoveLiquidityTransaction(tx *Transaction) error {
	// Must have inputs (LP tokens being burned)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("remove liquidity transaction must have inputs")
	}

	// Must have Data field with pool reference and LP token amount
	if len(tx.Data) == 0 {
		return fmt.Errorf("remove liquidity transaction must have pool data in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("remove liquidity transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}

// validateSwapTransaction validates swap transactions
func validateSwapTransaction(tx *Transaction) error {
	// Must have inputs (tokens being swapped)
	if len(tx.Inputs) == 0 {
		return fmt.Errorf("swap transaction must have inputs")
	}

	// Must have Data field with pool reference and swap details
	if len(tx.Data) == 0 {
		return fmt.Errorf("swap transaction must have swap data in Data field")
	}

	// Must be signed
	if len(tx.Signature) == 0 {
		return fmt.Errorf("swap transaction must be signed")
	}

	// Validate signature
	if len(tx.PublicKey) > 0 {
		publicKey, err := PublicKeyFromBytes(tx.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key: %w", err)
		}

		hash, err := tx.Hash()
		if err != nil {
			return fmt.Errorf("failed to compute transaction hash: %w", err)
		}

		if !VerifySignature(hash, tx.Signature, publicKey) {
			return fmt.Errorf("invalid transaction signature")
		}
	}

	return nil
}
