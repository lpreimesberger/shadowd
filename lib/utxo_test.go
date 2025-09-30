package lib

import (
	"testing"
)

func TestTxType(t *testing.T) {
	tests := []struct {
		txType   TxType
		expected string
	}{
		{TxTypeCoinbase, "coinbase"},
		{TxTypeSend, "send"},
		{TxTypeMintToken, "mint_token"},
		{TxTypeMelt, "melt"},
		{TxType(99), "unknown(99)"},
	}

	for _, test := range tests {
		result := test.txType.String()
		if result != test.expected {
			t.Errorf("TxType(%d).String() = %s, expected %s", int(test.txType), result, test.expected)
		}
	}
}

func TestCreateShadowOutput(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	address := kp.Address()
	amount := uint64(100000000) // 1.00 SHADOW

	output := CreateShadowOutput(address, amount)

	if output.Amount != amount {
		t.Errorf("Expected amount %d, got %d", amount, output.Amount)
	}

	if output.Address != address {
		t.Errorf("Address mismatch")
	}

	if output.TokenID != "SHADOW" {
		t.Errorf("Expected TokenID 'SHADOW', got %s", output.TokenID)
	}

	if output.TokenType != "native" {
		t.Errorf("Expected TokenType 'native', got %s", output.TokenType)
	}

	if len(output.ScriptPubKey) != 33 {
		t.Errorf("Expected script length 33, got %d", len(output.ScriptPubKey))
	}
}

func TestCreateTokenOutput(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	address := kp.Address()
	amount := uint64(1000)
	tokenID := "MYTOKEN"
	tokenType := "custom"
	metadata := []byte("token metadata")

	output := CreateTokenOutput(address, amount, tokenID, tokenType, metadata)

	if output.Amount != amount {
		t.Errorf("Expected amount %d, got %d", amount, output.Amount)
	}

	if output.TokenID != tokenID {
		t.Errorf("Expected TokenID %s, got %s", tokenID, output.TokenID)
	}

	if output.TokenType != tokenType {
		t.Errorf("Expected TokenType %s, got %s", tokenType, output.TokenType)
	}

	if string(output.Data) != string(metadata) {
		t.Errorf("Metadata mismatch")
	}

	if !output.IsTokenOutput() {
		t.Error("Should be identified as token output")
	}
}

func TestUTXO(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	output := CreateShadowOutput(kp.Address(), 50000000)
	utxo := &UTXO{
		TxID:        "test_tx_id_123",
		OutputIndex: 0,
		Output:      output,
		BlockHeight: 100,
		IsSpent:     false,
	}

	// Test outpoint
	outpoint := utxo.GetOutPoint()
	if outpoint.TxID != utxo.TxID {
		t.Error("Outpoint TxID mismatch")
	}
	if outpoint.Index != utxo.OutputIndex {
		t.Error("Outpoint index mismatch")
	}

	// Test spendable check
	if !utxo.IsSpendableBy(kp.Address()) {
		t.Error("UTXO should be spendable by owner")
	}

	// Generate different address
	otherKp, _ := GenerateKeyPair()
	if utxo.IsSpendableBy(otherKp.Address()) {
		t.Error("UTXO should not be spendable by non-owner")
	}

	// Test spent UTXO
	utxo.IsSpent = true
	if utxo.IsSpendableBy(kp.Address()) {
		t.Error("Spent UTXO should not be spendable")
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		amount   uint64
		expected string
	}{
		{0, "0.00000000"},
		{1, "0.00000001"},
		{100000000, "1.00000000"},
		{150000000, "1.50000000"},
		{250000001, "2.50000001"},
	}

	for _, test := range tests {
		result := FormatAmount(test.amount)
		if result != test.expected {
			t.Errorf("FormatAmount(%d) = %s, expected %s", test.amount, result, test.expected)
		}
	}
}

func TestParseAmount(t *testing.T) {
	tests := []struct {
		amountStr string
		expected  uint64
		shouldErr bool
	}{
		{"0.00000000", 0, false},
		{"1.00000000", 100000000, false},
		{"2.50000001", 250000001, false},
		{"invalid", 0, true},
		{"1.2.3", 0, true},
	}

	for _, test := range tests {
		result, err := ParseAmount(test.amountStr)

		if test.shouldErr {
			if err == nil {
				t.Errorf("ParseAmount(%s) should have failed", test.amountStr)
			}
			continue
		}

		if err != nil {
			t.Errorf("ParseAmount(%s) failed: %v", test.amountStr, err)
			continue
		}

		if result != test.expected {
			t.Errorf("ParseAmount(%s) = %d, expected %d", test.amountStr, result, test.expected)
		}
	}
}

func TestCalculateTxFee(t *testing.T) {
	tests := []struct {
		txType      TxType
		inputCount  int
		outputCount int
		dataSize    int
		minExpected uint64
	}{
		{TxTypeCoinbase, 0, 1, 0, 0},        // No fee for coinbase
		{TxTypeSend, 1, 2, 0, 1750},         // Base fee + input + outputs
		{TxTypeMintToken, 0, 1, 100, 11000}, // Higher fee for minting
		{TxTypeMelt, 2, 0, 0, 1500},         // Base fee + inputs
	}

	for _, test := range tests {
		result := CalculateTxFee(test.txType, test.inputCount, test.outputCount, test.dataSize)

		if test.txType == TxTypeCoinbase && result != 0 {
			t.Errorf("Coinbase transaction should have no fee, got %d", result)
		} else if test.txType != TxTypeCoinbase && result < test.minExpected {
			t.Errorf("Fee too low for %s: got %d, expected at least %d",
				test.txType.String(), result, test.minExpected)
		}
	}
}

func TestNewTxInput(t *testing.T) {
	prevTxID := "previous_transaction_id"
	outputIndex := uint32(1)

	input := NewTxInput(prevTxID, outputIndex)

	if input.PrevTxID != prevTxID {
		t.Errorf("Expected PrevTxID %s, got %s", prevTxID, input.PrevTxID)
	}

	if input.OutputIndex != outputIndex {
		t.Errorf("Expected OutputIndex %d, got %d", outputIndex, input.OutputIndex)
	}

	if input.Sequence != 0xFFFFFFFF {
		t.Errorf("Expected final sequence number, got %d", input.Sequence)
	}

	// Test outpoint
	outpoint := input.GetOutPoint()
	if outpoint.TxID != prevTxID {
		t.Error("Input outpoint TxID mismatch")
	}
	if outpoint.Index != outputIndex {
		t.Error("Input outpoint index mismatch")
	}
}

func TestCreateP2PKHScript(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	address := kp.Address()
	script := CreateP2PKHScript(address)

	if len(script) != 33 {
		t.Errorf("Expected script length 33, got %d", len(script))
	}

	if script[0] != 0x76 {
		t.Errorf("Expected OP_P2PKH marker, got 0x%x", script[0])
	}

	// Extract address from script
	var scriptAddr Address
	copy(scriptAddr[:], script[1:])

	if scriptAddr != address {
		t.Error("Script address doesn't match original address")
	}
}

func TestValidateScript(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	address := kp.Address()
	scriptPubKey := CreateP2PKHScript(address)
	publicKeyBytes, _ := PublicKeyToBytes(kp.PublicKey)
	txHash := []byte("test_transaction_hash")

	// Valid script validation
	if !ValidateScript([]byte{}, scriptPubKey, txHash, publicKeyBytes) {
		t.Error("Valid script should pass validation")
	}

	// Invalid script (wrong length)
	invalidScript := []byte{0x76, 0x00} // Too short
	if ValidateScript([]byte{}, invalidScript, txHash, publicKeyBytes) {
		t.Error("Invalid script should fail validation")
	}

	// Wrong public key
	otherKp, _ := GenerateKeyPair()
	otherPublicKeyBytes, _ := PublicKeyToBytes(otherKp.PublicKey)
	if ValidateScript([]byte{}, scriptPubKey, txHash, otherPublicKeyBytes) {
		t.Error("Script with wrong public key should fail validation")
	}
}

func TestTxOutputMethods(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Test SHADOW output
	shadowOutput := CreateShadowOutput(kp.Address(), 100000000)
	if shadowOutput.IsTokenOutput() {
		t.Error("SHADOW output should not be identified as token output")
	}

	// Test custom token output
	tokenOutput := CreateTokenOutput(kp.Address(), 1000, "CUSTOM", "erc20", []byte("metadata"))
	if !tokenOutput.IsTokenOutput() {
		t.Error("Custom token output should be identified as token output")
	}

	// Test metadata
	metadata := tokenOutput.GetTokenMetadata()
	if metadata["token_id"] != "CUSTOM" {
		t.Error("Token metadata mismatch")
	}

	if metadata["amount"] != uint64(1000) {
		t.Error("Token amount metadata mismatch")
	}

	if metadata["data"] != "6d65746164617461" { // hex encoded "metadata"
		t.Error("Token data metadata mismatch")
	}
}
