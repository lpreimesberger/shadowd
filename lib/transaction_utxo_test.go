package lib

import (
	"fmt"
	"testing"
)

func TestNewTxBuilder(t *testing.T) {
	builder := NewTxBuilder(TxTypeSend)

	if builder.txType != TxTypeSend {
		t.Errorf("Expected TxType %s, got %s", TxTypeSend.String(), builder.txType.String())
	}

	if builder.version != 1 {
		t.Errorf("Expected version 1, got %d", builder.version)
	}

	if len(builder.inputs) != 0 {
		t.Error("New builder should have empty inputs")
	}

	if len(builder.outputs) != 0 {
		t.Error("New builder should have empty outputs")
	}
}

func TestTxBuilderAddInput(t *testing.T) {
	builder := NewTxBuilder(TxTypeSend)
	prevTxID := "test_tx_id"
	outputIndex := uint32(0)

	builder.AddInput(prevTxID, outputIndex)

	if len(builder.inputs) != 1 {
		t.Fatal("Expected 1 input after AddInput")
	}

	input := builder.inputs[0]
	if input.PrevTxID != prevTxID {
		t.Errorf("Expected PrevTxID %s, got %s", prevTxID, input.PrevTxID)
	}

	if input.OutputIndex != outputIndex {
		t.Errorf("Expected OutputIndex %d, got %d", outputIndex, input.OutputIndex)
	}
}

func TestTxBuilderAddOutput(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	builder := NewTxBuilder(TxTypeSend)
	address := kp.Address()
	amount := uint64(100000000)

	// Add SHADOW output
	builder.AddOutput(address, amount, "SHADOW")

	if len(builder.outputs) != 1 {
		t.Fatal("Expected 1 output after AddOutput")
	}

	output := builder.outputs[0]
	if output.Address != address {
		t.Error("Output address mismatch")
	}

	if output.Amount != amount {
		t.Errorf("Expected amount %d, got %d", amount, output.Amount)
	}

	if output.TokenID != "SHADOW" {
		t.Errorf("Expected TokenID SHADOW, got %s", output.TokenID)
	}

	// Add custom token output
	builder.AddOutput(address, 1000, "CUSTOM")

	if len(builder.outputs) != 2 {
		t.Fatal("Expected 2 outputs after second AddOutput")
	}

	customOutput := builder.outputs[1]
	if customOutput.TokenID != "CUSTOM" {
		t.Errorf("Expected TokenID CUSTOM, got %s", customOutput.TokenID)
	}
}

func TestCreateCoinbaseTransaction(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	minerAddress := kp.Address()
	blockHeight := uint64(12345)
	reward := uint64(5000000000) // 50 SHADOW

	tx := CreateCoinbaseTransaction(minerAddress, blockHeight, reward)

	// Validate basic structure
	if tx.TxType != TxTypeCoinbase {
		t.Errorf("Expected TxType coinbase, got %s", tx.TxType.String())
	}

	if len(tx.Inputs) != 0 {
		t.Errorf("Coinbase should have no inputs, got %d", len(tx.Inputs))
	}

	if len(tx.Outputs) != 1 {
		t.Errorf("Expected 1 output, got %d", len(tx.Outputs))
	}

	// Validate output
	output := tx.Outputs[0]
	if output.Address != minerAddress {
		t.Error("Output address should match miner address")
	}

	if output.Amount != reward {
		t.Errorf("Expected reward %d, got %d", reward, output.Amount)
	}

	if output.TokenID != "SHADOW" {
		t.Errorf("Expected TokenID SHADOW, got %s", output.TokenID)
	}

	// Validate transaction
	if err := ValidateTransaction(tx); err != nil {
		t.Errorf("Coinbase transaction should be valid: %v", err)
	}

	if !tx.IsValid() {
		t.Error("Coinbase transaction should be valid")
	}

	// Check data field
	expectedData := fmt.Sprintf("block_height_%d", blockHeight)
	if string(tx.Data) != expectedData {
		t.Errorf("Expected data %s, got %s", expectedData, string(tx.Data))
	}
}

func TestCreateSimpleSendTransaction(t *testing.T) {
	// Create key pairs
	senderKp, _ := GenerateKeyPair()
	recipientKp, _ := GenerateKeyPair()

	senderAddr := senderKp.Address()
	recipientAddr := recipientKp.Address()

	// Create mock UTXOs
	utxo1 := &UTXO{
		TxID:        "utxo_tx_1",
		OutputIndex: 0,
		Output:      CreateShadowOutput(senderAddr, 300000000), // 3 SHADOW
		BlockHeight: 100,
		IsSpent:     false,
	}

	utxo2 := &UTXO{
		TxID:        "utxo_tx_2",
		OutputIndex: 1,
		Output:      CreateShadowOutput(senderAddr, 200000000), // 2 SHADOW
		BlockHeight: 101,
		IsSpent:     false,
	}

	utxos := []*UTXO{utxo1, utxo2}
	sendAmount := uint64(150000000) // 1.5 SHADOW
	changeAddr := senderAddr

	tx, err := CreateSimpleSendTransaction(utxos, recipientAddr, sendAmount, changeAddr)
	if err != nil {
		t.Fatalf("Failed to create send transaction: %v", err)
	}

	// Validate basic structure
	if tx.TxType != TxTypeSend {
		t.Errorf("Expected TxType send, got %s", tx.TxType.String())
	}

	// Should use inputs until we have enough
	if len(tx.Inputs) == 0 {
		t.Error("Send transaction should have inputs")
	}

	// Should have recipient output and potentially change output
	if len(tx.Outputs) == 0 {
		t.Error("Send transaction should have outputs")
	}

	// Find recipient output
	var recipientOutput *TxOutput
	for _, output := range tx.Outputs {
		if output.Address == recipientAddr {
			recipientOutput = output
			break
		}
	}

	if recipientOutput == nil {
		t.Fatal("No output found for recipient")
	}

	if recipientOutput.Amount != sendAmount {
		t.Errorf("Expected recipient amount %d, got %d", sendAmount, recipientOutput.Amount)
	}

	// Validate transaction
	if err := ValidateTransaction(tx); err != nil {
		t.Errorf("Send transaction should be valid: %v", err)
	}
}

func TestCreateMintTokenTransaction(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	recipientAddr := kp.Address()
	tokenID := "MYTOKEN"
	tokenType := "erc20"
	mintAmount := uint64(1000)
	metadata := []byte("token metadata")

	tx := CreateMintTokenTransaction(tokenID, tokenType, mintAmount, recipientAddr, metadata)

	// Validate basic structure
	if tx.TxType != TxTypeMintToken {
		t.Errorf("Expected TxType mint_token, got %s", tx.TxType.String())
	}

	if len(tx.Outputs) == 0 {
		t.Error("Mint transaction should have outputs")
	}

	// Find token output
	var tokenOutput *TxOutput
	for _, output := range tx.Outputs {
		if output.TokenID == tokenID {
			tokenOutput = output
			break
		}
	}

	if tokenOutput == nil {
		t.Fatal("No token output found")
	}

	if tokenOutput.Amount != mintAmount {
		t.Errorf("Expected mint amount %d, got %d", mintAmount, tokenOutput.Amount)
	}

	if tokenOutput.TokenType != tokenType {
		t.Errorf("Expected token type %s, got %s", tokenType, tokenOutput.TokenType)
	}

	if string(tokenOutput.Data) != string(metadata) {
		t.Error("Token metadata mismatch")
	}

	// Validate transaction
	if err := ValidateTransaction(tx); err != nil {
		t.Errorf("Mint token transaction should be valid: %v", err)
	}

	// Check if it has token outputs
	if !tx.HasTokenOutputs() {
		t.Error("Mint transaction should have token outputs")
	}

	tokenTypes := tx.GetTokenTypes()
	if len(tokenTypes) == 0 {
		t.Error("Should have token types")
	}
}

func TestCreateMeltTransaction(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	ownerAddr := kp.Address()

	// Create mock token UTXO to melt
	tokenUTXO := &UTXO{
		TxID:        "token_tx_1",
		OutputIndex: 0,
		Output:      CreateTokenOutput(ownerAddr, 500, "MELTTOKEN", "custom", nil),
		BlockHeight: 200,
		IsSpent:     false,
	}

	utxos := []*UTXO{tokenUTXO}
	meltReason := "revoke_swap_offer"

	tx := CreateMeltTransaction(utxos, meltReason)

	// Validate basic structure
	if tx.TxType != TxTypeMelt {
		t.Errorf("Expected TxType melt, got %s", tx.TxType.String())
	}

	if len(tx.Inputs) != 1 {
		t.Errorf("Expected 1 input, got %d", len(tx.Inputs))
	}

	// Melt transaction can have 0 outputs (complete destruction)
	if len(tx.Outputs) > 1 {
		t.Error("Melt transaction should have 0 or 1 outputs")
	}

	// Check input
	input := tx.Inputs[0]
	if input.PrevTxID != tokenUTXO.TxID {
		t.Error("Input should reference token UTXO")
	}

	// Check data
	expectedData := fmt.Sprintf("melt_reason_%s", meltReason)
	if string(tx.Data) != expectedData {
		t.Errorf("Expected data %s, got %s", expectedData, string(tx.Data))
	}

	// Validate transaction
	if err := ValidateTransaction(tx); err != nil {
		t.Errorf("Melt transaction should be valid: %v", err)
	}
}

func TestTransactionValidation(t *testing.T) {
	// Test nil transaction
	if ValidateTransaction(nil) == nil {
		t.Error("Nil transaction should fail validation")
	}

	// Test invalid transaction type
	invalidTx := &Transaction{TxType: TxType(999)}
	if ValidateTransaction(invalidTx) == nil {
		t.Error("Invalid transaction type should fail validation")
	}

	// Test coinbase validation
	kp, _ := GenerateKeyPair()
	validCoinbase := CreateCoinbaseTransaction(kp.Address(), 100, 5000000000)
	if ValidateTransaction(validCoinbase) != nil {
		t.Error("Valid coinbase should pass validation")
	}

	// Test coinbase with inputs (should fail)
	invalidCoinbase := CreateCoinbaseTransaction(kp.Address(), 100, 5000000000)
	invalidCoinbase.Inputs = append(invalidCoinbase.Inputs, NewTxInput("test", 0))
	if ValidateTransaction(invalidCoinbase) == nil {
		t.Error("Coinbase with inputs should fail validation")
	}
}

func TestTransactionSigning(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create a simple coinbase transaction
	tx := CreateCoinbaseTransaction(kp.Address(), 100, 5000000000)

	// Sign the transaction
	if err := tx.Sign(kp); err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	// Check signature fields are set
	if len(tx.PublicKey) == 0 {
		t.Error("Public key should be set after signing")
	}

	if len(tx.Signature) == 0 {
		t.Error("Signature should be set after signing")
	}

	// Validate signed transaction
	if err := ValidateTransaction(tx); err != nil {
		t.Errorf("Signed transaction should be valid: %v", err)
	}

	// Test hash consistency
	hash1, _ := tx.Hash()
	hash2, _ := tx.Hash()

	if len(hash1) != len(hash2) {
		t.Error("Hash length should be consistent")
	}

	for i := range hash1 {
		if hash1[i] != hash2[i] {
			t.Error("Hash should be deterministic")
			break
		}
	}

	// Test transaction ID
	id1, _ := tx.ID()
	id2, _ := tx.ID()

	if id1 != id2 {
		t.Error("Transaction ID should be deterministic")
	}

	if len(id1) != 64 { // 32 bytes * 2 hex chars
		t.Errorf("Transaction ID should be 64 characters, got %d", len(id1))
	}
}

func TestTransactionMethods(t *testing.T) {
	kp, _ := GenerateKeyPair()
	recipientKp, _ := GenerateKeyPair()

	// Create transaction with multiple outputs
	tx := CreateCoinbaseTransaction(kp.Address(), 100, 5000000000)

	// Add another output
	tx.Outputs = append(tx.Outputs, CreateShadowOutput(recipientKp.Address(), 1000000000))

	// Test GetTotalOutputAmount
	totalOutput := tx.GetTotalOutputAmount()
	expected := uint64(6000000000) // 50 + 10 SHADOW
	if totalOutput != expected {
		t.Errorf("Expected total output %d, got %d", expected, totalOutput)
	}

	// Test GetOutputsForAddress
	ownerOutputs := tx.GetOutputsForAddress(kp.Address())
	if len(ownerOutputs) != 1 {
		t.Errorf("Expected 1 output for owner, got %d", len(ownerOutputs))
	}

	recipientOutputs := tx.GetOutputsForAddress(recipientKp.Address())
	if len(recipientOutputs) != 1 {
		t.Errorf("Expected 1 output for recipient, got %d", len(recipientOutputs))
	}

	// Test CalculateFee
	fee := tx.CalculateFee()
	if fee != 0 {
		t.Errorf("Coinbase transaction should have 0 fee, got %d", fee)
	}

	// Test HasTokenOutputs
	if tx.HasTokenOutputs() {
		t.Error("SHADOW-only transaction should not have token outputs")
	}

	// Test GetTokenTypes
	tokenTypes := tx.GetTokenTypes()
	if len(tokenTypes) != 1 || tokenTypes[0] != "SHADOW" {
		t.Error("Should have only SHADOW token type")
	}

	// Test String method
	txString := tx.String()
	if txString == "" {
		t.Error("Transaction string should not be empty")
	}
}

func TestGetTransactionSummary(t *testing.T) {
	kp, _ := GenerateKeyPair()

	// Test coinbase summary
	coinbaseTx := CreateCoinbaseTransaction(kp.Address(), 100, 5000000000)
	coinbaseSummary := GetTransactionSummary(coinbaseTx)
	if coinbaseSummary == "" {
		t.Error("Coinbase summary should not be empty")
	}

	// Test mint token summary
	mintTx := CreateMintTokenTransaction("TEST", "custom", 1000, kp.Address(), nil)
	mintSummary := GetTransactionSummary(mintTx)
	if mintSummary == "" {
		t.Error("Mint token summary should not be empty")
	}

	// Test melt summary
	utxo := &UTXO{
		TxID:        "test",
		OutputIndex: 0,
		Output:      CreateTokenOutput(kp.Address(), 100, "TEST", "custom", nil),
		BlockHeight: 1,
	}
	meltTx := CreateMeltTransaction([]*UTXO{utxo}, "test")
	meltSummary := GetTransactionSummary(meltTx)
	if meltSummary == "" {
		t.Error("Melt summary should not be empty")
	}
}
