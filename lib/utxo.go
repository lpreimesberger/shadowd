package lib

import (
	"encoding/hex"
	"fmt"
)

// TxType represents the type of transaction
type TxType int

const (
	// TxTypeCoinbase creates new SHADOW tokens (mining/block rewards)
	TxTypeCoinbase TxType = 0

	// TxTypeSend transfers SHADOW tokens between addresses
	TxTypeSend TxType = 1

	// TxTypeMintToken creates new custom tokens on the blockchain
	TxTypeMintToken TxType = 2

	// TxTypeMelt explicitly destroys tokens/assets and unlocks collateral
	TxTypeMelt TxType = 3

	// TxTypeRegisterValidator registers a validator's wallet address for block rewards
	TxTypeRegisterValidator TxType = 4

	// TxTypeOffer creates an atomic swap offer (locks tokens for exchange)
	TxTypeOffer TxType = 5

	// TxTypeAcceptOffer accepts and executes an atomic swap offer
	TxTypeAcceptOffer TxType = 6

	// TxTypeCancelOffer cancels an offer and returns locked tokens
	TxTypeCancelOffer TxType = 7

	// TxTypeCreatePool creates a liquidity pool for a token pair
	TxTypeCreatePool TxType = 8

	// TxTypeAddLiquidity adds liquidity to an existing pool
	TxTypeAddLiquidity TxType = 9

	// TxTypeRemoveLiquidity removes liquidity from a pool
	TxTypeRemoveLiquidity TxType = 10

	// TxTypeSwap swaps tokens through a liquidity pool
	TxTypeSwap TxType = 11
)

// String returns the string representation of a transaction type
func (tt TxType) String() string {
	switch tt {
	case TxTypeCoinbase:
		return "coinbase"
	case TxTypeSend:
		return "send"
	case TxTypeMintToken:
		return "mint_token"
	case TxTypeMelt:
		return "melt"
	case TxTypeRegisterValidator:
		return "register_validator"
	case TxTypeOffer:
		return "offer"
	case TxTypeAcceptOffer:
		return "accept_offer"
	case TxTypeCancelOffer:
		return "cancel_offer"
	case TxTypeCreatePool:
		return "create_pool"
	case TxTypeAddLiquidity:
		return "add_liquidity"
	case TxTypeRemoveLiquidity:
		return "remove_liquidity"
	case TxTypeSwap:
		return "swap"
	default:
		return fmt.Sprintf("unknown(%d)", int(tt))
	}
}

// TxInput represents an input to a transaction (spending a UTXO)
type TxInput struct {
	// Reference to previous transaction output being spent
	PrevTxID    string `json:"prev_tx_id"`   // Transaction ID containing the UTXO
	OutputIndex uint32 `json:"output_index"` // Index of output in that transaction

	// Spending authorization
	ScriptSig []byte `json:"script_sig"` // Script signature (for future smart contracts)
	Sequence  uint32 `json:"sequence"`   // Sequence number (for time locks, etc.)
}

// TxOutput represents an output of a transaction (creating a UTXO)
type TxOutput struct {
	// Value and recipient
	Amount  uint64  `json:"amount"`  // Amount of tokens (in smallest unit)
	Address Address `json:"address"` // Recipient address

	// Token information
	TokenID   string `json:"token_id"`   // Token identifier (genesis hash for SHADOW, TX ID for custom tokens)
	TokenType string `json:"token_type"` // Token type descriptor

	// Token staking (for custom tokens only)
	LockedShadow uint64 `json:"locked_shadow,omitempty"` // Proportional SHADOW locked to this token UTXO

	// Locking script
	ScriptPubKey []byte `json:"script_pub_key"` // Locking script (for future smart contracts)

	// Additional metadata
	Data []byte `json:"data,omitempty"` // Optional data payload
}

// UTXO represents an Unspent Transaction Output
type UTXO struct {
	TxID        string    `json:"tx_id"`        // Transaction that created this UTXO
	OutputIndex uint32    `json:"output_index"` // Index in that transaction's outputs
	Output      *TxOutput `json:"output"`       // The actual output
	BlockHeight uint64    `json:"block_height"` // Block height when created
	IsSpent     bool      `json:"is_spent"`     // Whether this UTXO has been spent
}

// OutPoint represents a reference to a transaction output
type OutPoint struct {
	TxID  string `json:"tx_id"`
	Index uint32 `json:"index"`
}

// String returns a string representation of the outpoint
func (op OutPoint) String() string {
	return fmt.Sprintf("%s:%d", op.TxID[:16]+"...", op.Index)
}

// String returns a string representation of the UTXO
func (utxo *UTXO) String() string {
	return fmt.Sprintf("UTXO{%s:%d, %s %s, %s}",
		utxo.TxID[:16]+"...",
		utxo.OutputIndex,
		FormatAmount(utxo.Output.Amount),
		utxo.Output.TokenID,
		utxo.Output.Address.String()[:8]+"...")
}

// GetOutPoint returns the outpoint for this UTXO
func (utxo *UTXO) GetOutPoint() OutPoint {
	return OutPoint{
		TxID:  utxo.TxID,
		Index: utxo.OutputIndex,
	}
}

// IsSpendableBy checks if a UTXO can be spent by the given address
func (utxo *UTXO) IsSpendableBy(addr Address) bool {
	return !utxo.IsSpent && utxo.Output.Address == addr
}

// CreateShadowOutput creates a standard SHADOW token output
func CreateShadowOutput(address Address, amount uint64) *TxOutput {
	genesisToken := GetGenesisToken()
	return &TxOutput{
		Amount:       amount,
		Address:      address,
		TokenID:      genesisToken.TokenID,
		TokenType:    "native",
		ScriptPubKey: CreateP2PKHScript(address), // Pay-to-PubKey-Hash
	}
}

// CreateTokenOutput creates a custom token output
func CreateTokenOutput(address Address, amount uint64, tokenID, tokenType string, data []byte) *TxOutput {
	return &TxOutput{
		Amount:       amount,
		Address:      address,
		TokenID:      tokenID,
		TokenType:    tokenType,
		ScriptPubKey: CreateP2PKHScript(address),
		Data:         data,
	}
}

// CreateP2PKHScript creates a Pay-to-PubKey-Hash script
func CreateP2PKHScript(address Address) []byte {
	// Simple script: OP_DUP OP_HASH160 <address> OP_EQUALVERIFY OP_CHECKSIG
	// For now, we'll just store the address directly
	script := make([]byte, 1+32) // 1 byte opcode + 32 byte address
	script[0] = 0x76             // OP_P2PKH marker
	copy(script[1:], address[:])
	return script
}

// ValidateScript validates a script (simplified for now)
func ValidateScript(scriptSig, scriptPubKey []byte, txHash []byte, publicKey []byte) bool {
	// For now, just validate that it's a P2PKH script and the address matches
	if len(scriptPubKey) != 33 || scriptPubKey[0] != 0x76 {
		return false
	}

	// Extract address from script
	var scriptAddr Address
	copy(scriptAddr[:], scriptPubKey[1:])

	// Derive address from public key
	pk, err := PublicKeyFromBytes(publicKey)
	if err != nil {
		return false
	}

	derivedAddr := DeriveAddress(pk)
	return scriptAddr == derivedAddr
}

// FormatAmount formats an amount with proper decimal places
func FormatAmount(amount uint64) string {
	// SHADOW has 8 decimal places (like Bitcoin)
	if amount == 0 {
		return "0.00000000"
	}

	whole := amount / 100000000
	fractional := amount % 100000000

	return fmt.Sprintf("%d.%08d", whole, fractional)
}

// ParseAmount parses a formatted amount string back to uint64
func ParseAmount(amountStr string) (uint64, error) {
	// This is a simplified parser - in production you'd want proper decimal handling
	var whole, fractional uint64
	n, err := fmt.Sscanf(amountStr, "%d.%d", &whole, &fractional)
	if err != nil || n != 2 {
		return 0, fmt.Errorf("invalid amount format: %s", amountStr)
	}

	return whole*100000000 + fractional, nil
}

// CalculateTxFee calculates the transaction fee based on size and type
func CalculateTxFee(txType TxType, inputCount, outputCount int, dataSize int) uint64 {
	baseFee := uint64(1000) // 0.00001000 SHADOW base fee

	// Different fees for different transaction types
	switch txType {
	case TxTypeCoinbase:
		return 0 // No fee for coinbase transactions
	case TxTypeSend:
		return baseFee + uint64(inputCount)*500 + uint64(outputCount)*250
	case TxTypeMintToken:
		return baseFee*10 + uint64(dataSize)*10 // Higher fee for token minting
	case TxTypeMelt:
		return baseFee + uint64(inputCount)*250 // Lower fee for melting
	default:
		return baseFee
	}
}

// NewTxInput creates a new transaction input
func NewTxInput(prevTxID string, outputIndex uint32) *TxInput {
	return &TxInput{
		PrevTxID:    prevTxID,
		OutputIndex: outputIndex,
		ScriptSig:   []byte{},   // Empty for now, will be filled during signing
		Sequence:    0xFFFFFFFF, // Final sequence number
	}
}

// GetOutPoint returns the outpoint this input is spending
func (ti *TxInput) GetOutPoint() OutPoint {
	return OutPoint{
		TxID:  ti.PrevTxID,
		Index: ti.OutputIndex,
	}
}

// String returns a string representation of the transaction input
func (ti *TxInput) String() string {
	return fmt.Sprintf("Input{%s:%d}", ti.PrevTxID[:16]+"...", ti.OutputIndex)
}

// String returns a string representation of the transaction output
func (to *TxOutput) String() string {
	return fmt.Sprintf("Output{%s %s -> %s}",
		FormatAmount(to.Amount),
		to.TokenID,
		to.Address.String()[:8]+"...")
}

// IsTokenOutput returns true if this is a custom token output (not SHADOW)
func (to *TxOutput) IsTokenOutput() bool {
	genesisToken := GetGenesisToken()
	return to.TokenID != genesisToken.TokenID
}

// GetTokenMetadata returns metadata for token outputs
func (to *TxOutput) GetTokenMetadata() map[string]interface{} {
	metadata := map[string]interface{}{
		"token_id":   to.TokenID,
		"token_type": to.TokenType,
		"amount":     to.Amount,
	}

	if len(to.Data) > 0 {
		metadata["data"] = hex.EncodeToString(to.Data)
	}

	return metadata
}
