package main

import (
	"fmt"
	"log"
	"os"

	"shadowy/lib"
)

// conditionalPrintf prints only if not in quiet mode
func conditionalPrintf(config *lib.CLIConfig, format string, args ...interface{}) {
	if !config.Quiet {
		fmt.Printf(format, args...)
	}
}

// conditionalPrintln prints only if not in quiet mode
func conditionalPrintln(config *lib.CLIConfig, args ...interface{}) {
	if !config.Quiet {
		fmt.Println(args...)
	}
}

func main() {
	// Parse command line arguments
	config, err := lib.ParseCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing command line: %v\n", err)
		lib.PrintUsage()
		os.Exit(1)
	}

	// Validate configuration
	if err := config.ValidateConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Check if running in node mode
	if config.NodeMode {
		if err := lib.StartNode(config); err != nil {
			fmt.Fprintf(os.Stderr, "Node mode failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Print startup banner (unless quiet mode)
	if !config.Quiet {
		fmt.Println("🌑 Shadowy - Post-Quantum UTXO Blockchain Node")
		fmt.Println("===========================================")
		fmt.Printf("   Configuration: %s\n", config.String())

		// Show seed nodes if configured
		if len(config.Seeds) > 0 {
			seedNodes, _ := config.GetSeedNodes()
			fmt.Printf("   Seed nodes: %d configured\n", len(seedNodes))
			for i, node := range seedNodes {
				fmt.Printf("     %d. %s\n", i+1, node.String())
			}
		}

		// Show plot/proof directories if configured
		if len(config.Dirs) > 0 {
			fmt.Printf("   Plot/proof directories: %d configured\n", len(config.Dirs))
			for i, dir := range config.Dirs {
				fmt.Printf("     %d. %s\n", i+1, dir)
			}
		}
		fmt.Println()
	}

	// Initialize token registry with genesis SHADOW token
	if !config.Quiet {
		fmt.Println("1. Initializing token system...")
	}
	lib.InitializeTokenRegistry()
	tokenRegistry := lib.GetGlobalTokenRegistry()

	genesisToken := lib.GetGenesisToken()
	if !config.Quiet {
		fmt.Printf("   Genesis Token: %s (%s)\n", genesisToken.Name, genesisToken.Ticker)
		fmt.Printf("   Token ID: %s\n", genesisToken.TokenID)
		fmt.Printf("   Total Supply: %s\n", genesisToken.FormatSupply())
		fmt.Printf("   Token registry initialized with %d tokens\n", tokenRegistry.GetTokenCount())

		// Initialize node wallet (loads from ~/.sn/default.json or creates if missing)
		fmt.Println("\n2. Initializing node wallet...")
	}
	err = lib.InitializeGlobalWallet()
	if err != nil {
		log.Fatal("Failed to initialize node wallet:", err)
	}

	nodeWallet := lib.GetGlobalWallet()
	if !config.Quiet {
		fmt.Printf("   Node address: %s\n", nodeWallet.GetAddressString())

		walletInfo := nodeWallet.GetWalletInfo()
		fmt.Printf("   Wallet file: %s\n", walletInfo["path"])
		fmt.Printf("   Node ready for mining rewards! 💎\n")

		// Create coinbase transaction (mining reward)
		fmt.Println("\n3. Creating coinbase transaction (mining reward)...")
	}
	blockHeight := uint64(12345)
	reward := uint64(5000000000) // 50 SHADOW

	coinbaseTx, err := nodeWallet.CreateAndSignCoinbaseTransaction(blockHeight, reward)
	if err != nil {
		log.Fatal("Failed to create coinbase transaction:", err)
	}

	if !config.Quiet {
		fmt.Printf("   ✓ Coinbase transaction: %s\n", lib.GetTransactionSummary(coinbaseTx))
		fmt.Printf("   Block reward: %s\n", lib.FormatAmount(reward))
		fmt.Printf("   Token ID in tx: %s\n", coinbaseTx.Outputs[0].TokenID)

		// Generate recipient for demonstration
		fmt.Println("\n4. Generating recipient key pair...")
	}
	recipient, err := lib.GenerateKeyPair()
	if err != nil {
		log.Fatal("Failed to generate recipient key pair:", err)
	}
	if !config.Quiet {
		fmt.Printf("   Recipient address: %s\n", recipient.Address().String())

		// Simulate UTXOs from previous transactions (in real blockchain, these would come from UTXO set)
		fmt.Println("\n4. Creating mock UTXOs for sending...")
	}
	utxo1 := &lib.UTXO{
		TxID:        "mock_tx_1",
		OutputIndex: 0,
		Output:      lib.CreateShadowOutput(nodeWallet.GetAddress(), 300000000), // 3 SHADOW
		BlockHeight: blockHeight - 1,
		IsSpent:     false,
	}

	utxo2 := &lib.UTXO{
		TxID:        "mock_tx_2",
		OutputIndex: 1,
		Output:      lib.CreateShadowOutput(nodeWallet.GetAddress(), 200000000), // 2 SHADOW
		BlockHeight: blockHeight - 2,
		IsSpent:     false,
	}

	nodeUTXOs := []*lib.UTXO{utxo1, utxo2}
	conditionalPrintf(config, "   Created 2 mock UTXOs totaling %s SHADOW\n", lib.FormatAmount(500000000))

	// Create send transaction
	conditionalPrintln(config, "\n5. Creating send transaction...")
	sendAmount := uint64(150000000) // 1.5 SHADOW

	sendTx, err := nodeWallet.CreateAndSignSendTransaction(nodeUTXOs, recipient.Address(), sendAmount)
	if err != nil {
		log.Fatal("Failed to create send transaction:", err)
	}

	conditionalPrintf(config, "   ✓ Send transaction: %s\n", lib.GetTransactionSummary(sendTx))
	conditionalPrintf(config, "   Sending: %s SHADOW to %s\n",
		lib.FormatAmount(sendAmount), recipient.Address().String()[:16]+"...")

	// Validate UTXO transaction
	conditionalPrintln(config, "\n6. Validating UTXO transactions...")

	// Validate coinbase
	if coinbaseTx.IsValid() {
		conditionalPrintf(config, "   ✓ Coinbase transaction is valid (Type: %s)\n", coinbaseTx.TxType.String())
	} else {
		conditionalPrintln(config, "   ✗ Coinbase transaction is invalid!")
	}

	// Validate send transaction
	if sendTx.IsValid() {
		conditionalPrintf(config, "   ✓ Send transaction is valid (Type: %s)\n", sendTx.TxType.String())
		conditionalPrintf(config, "     Inputs: %d, Outputs: %d\n", len(sendTx.Inputs), len(sendTx.Outputs))
	} else {
		conditionalPrintln(config, "   ✗ Send transaction is invalid!")
	}

	// Create custom token with proper TokenInfo
	conditionalPrintln(config, "\n7. Creating custom token with SHAKE256 ID...")

	// Create TokenInfo for the custom token
	customTokenInfo, err := lib.CreateCustomToken(
		"ShadowCoin",            // name
		"SCOIN",                 // ticker
		100000000000,            // total supply (1000 tokens with 8 decimals)
		8,                       // decimals
		50000000,                // melt value per token (0.5 SHADOW per token)
		nodeWallet.GetAddress(), // creator
	)
	if err != nil {
		log.Fatal("Failed to create custom token info:", err)
	}

	conditionalPrintf(config, "   Custom Token: %s (%s)\n", customTokenInfo.Name, customTokenInfo.Ticker)
	conditionalPrintf(config, "   Token ID: %s\n", customTokenInfo.TokenID)
	conditionalPrintf(config, "   Total Supply: %s\n", customTokenInfo.FormatSupply())
	conditionalPrintf(config, "   Melt Value: %s SHADOW per token\n", lib.FormatAmount(customTokenInfo.MeltValuePerToken))

	// Register the token
	err = tokenRegistry.RegisterToken(customTokenInfo)
	if err != nil {
		log.Fatal("Failed to register custom token:", err)
	}

	// Create mint transaction using TokenInfo
	mintAmount := uint64(100000000) // 1 SCOIN
	tokenTx := lib.CreateMintTokenTransactionFromTokenInfo(customTokenInfo, mintAmount, recipient.Address())

	err = tokenTx.Sign(nodeWallet.KeyPair)
	if err != nil {
		log.Fatal("Failed to sign token mint transaction:", err)
	}

	conditionalPrintf(config, "   ✓ Token mint transaction: %s\n", lib.GetTransactionSummary(tokenTx))
	if tokenTx.IsValid() {
		conditionalPrintf(config, "   ✓ Token transaction is valid (Type: %s)\n", tokenTx.TxType.String())
		conditionalPrintf(config, "   Minted: %s to %s\n",
			lib.FormatAmount(mintAmount), recipient.Address().String()[:16]+"...")

		stakingReq := customTokenInfo.CalculateStakingRequirement(mintAmount)
		conditionalPrintf(config, "   Staking required: %s SHADOW\n", lib.FormatAmount(stakingReq))
	}

	conditionalPrintf(config, "   Token registry now has %d tokens\n", tokenRegistry.GetTokenCount())

	// Create melt transaction (token destruction)
	conditionalPrintln(config, "\n8. Creating token melt transaction...")

	// Create a mock token UTXO to melt
	tokenUTXO := &lib.UTXO{
		TxID:        "token_tx_1",
		OutputIndex: 0,
		Output:      lib.CreateTokenOutput(nodeWallet.GetAddress(), 100, "OLDTOKEN", "deprecated", nil),
		BlockHeight: blockHeight - 3,
		IsSpent:     false,
	}

	meltTx := nodeWallet.CreateMeltTransaction([]*lib.UTXO{tokenUTXO}, "token_deprecation")
	if err := meltTx.Sign(nodeWallet.KeyPair); err != nil {
		log.Fatal("Failed to sign melt transaction:", err)
	}

	conditionalPrintf(config, "   ✓ Melt transaction: %s\n", lib.GetTransactionSummary(meltTx))
	if meltTx.IsValid() {
		conditionalPrintf(config, "   ✓ Melt transaction is valid (Type: %s)\n", meltTx.TxType.String())
		conditionalPrintln(config, "   Destroyed: 100 OLDTOKEN units")
	}

	// Demonstrate transaction IDs and hashing
	conditionalPrintln(config, "\n9. Transaction details...")

	coinbaseID, _ := coinbaseTx.ID()
	sendID, _ := sendTx.ID()
	tokenID, _ := tokenTx.ID()
	meltID, _ := meltTx.ID()

	conditionalPrintf(config, "   Coinbase TX ID: %s...\n", coinbaseID[:16])
	conditionalPrintf(config, "   Send TX ID:     %s...\n", sendID[:16])
	conditionalPrintf(config, "   Token TX ID:    %s...\n", tokenID[:16])
	conditionalPrintf(config, "   Melt TX ID:     %s...\n", meltID[:16])

	conditionalPrintln(config, "\n🎉 Post-quantum UTXO blockchain demo completed!")
	conditionalPrintln(config, "\nFeatures demonstrated:")
	conditionalPrintln(config, "- ✅ UTXO-based transaction model")
	conditionalPrintln(config, "- ✅ Multiple transaction types (coinbase, send, mint, melt)")
	conditionalPrintln(config, "- ✅ ML-DSA87 post-quantum signatures")
	conditionalPrintln(config, "- ✅ Custom token creation and destruction")
	conditionalPrintln(config, "- ✅ Mining rewards and fee calculation")
	conditionalPrintln(config, "- ✅ Address derivation from public key hashes")
	conditionalPrintln(config, "- ✅ Comprehensive transaction validation")

	walletInfo := nodeWallet.GetWalletInfo()
	conditionalPrintf(config, "\n💡 Your node address: %s\n", nodeWallet.GetAddressString())
	conditionalPrintf(config, "💡 Wallet location: %s\n", walletInfo["path"])
	conditionalPrintln(config, "💡 Ready to participate in the Shadowy network!")
}
