package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abiosoft/ishell"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// NodeServer represents the Shadowy node server
type NodeServer struct {
	config        *CLIConfig
	httpServer    *http.Server
	httpPort      int
	shell         *ishell.Shell
	nodeWallet    *NodeWallet
	tokenRegistry *TokenRegistry
	tendermint    *TendermintNode
	shutdown      chan bool
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	startTime     time.Time
}

// NodeStatus represents the current status of the node
type NodeStatus struct {
	NodeAddress    string                 `json:"node_address"`
	NodeID         string                 `json:"node_id"`
	SeedConnectStr string                 `json:"seed_connect_string"`
	WalletInfo     map[string]interface{} `json:"wallet_info"`
	TokenCount     int                    `json:"token_count"`
	GenesisToken   *TokenInfo             `json:"genesis_token"`
	PlotCount      int                    `json:"plot_count"`
	FarmingEnabled bool                   `json:"farming_enabled"`
	Configuration  string                 `json:"configuration"`
	Seeds          []string               `json:"seeds"`
	Directories    []string               `json:"directories"`
	HTTPServerAddr string                 `json:"http_server_addr"`
	Uptime         time.Duration          `json:"uptime"`
	StartTime      time.Time              `json:"start_time"`
}

// NewNodeServer creates a new node server instance
func NewNodeServer(config *CLIConfig) (*NodeServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize components that are needed
	if err := InitializeGlobalWallet(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize wallet: %w", err)
	}

	InitializeTokenRegistry()

	node := &NodeServer{
		config:        config,
		nodeWallet:    GetGlobalWallet(),
		tokenRegistry: GetGlobalTokenRegistry(),
		shutdown:      make(chan bool, 1),
		ctx:           ctx,
		cancel:        cancel,
		startTime:     time.Now(),
	}

	return node, nil
}

// StartNode starts the node in daemon mode
func StartNode(config *CLIConfig) error {
	if !config.Quiet {
		fmt.Println("🌑 Starting Shadowy Node...")
		fmt.Println("========================")
	}

	// Initialize plot manager FIRST (before any other services)
	plotDir := "./plots"
	if !config.Quiet {
		fmt.Printf("📊 Initializing plot manager from: %s\n", plotDir)
	}

	if err := InitializePlotManager(plotDir); err != nil {
		if !config.Quiet {
			fmt.Printf("⚠️  Warning: Failed to load plots from %s: %v\n", plotDir, err)
			fmt.Printf("   Continuing without farming capability...\n")
		}
	}

	// Create node server
	node, err := NewNodeServer(config)
	if err != nil {
		return fmt.Errorf("failed to create node server: %w", err)
	}

	// Start Tendermint blockchain (before shell loads)
	if err := node.startTendermint(); err != nil {
		return fmt.Errorf("failed to start Tendermint: %w", err)
	}

	// Start HTTP server
	if err := node.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start interactive shell
	if err := node.startShell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Handle shutdown signals
	node.handleSignals()

	// Wait for shutdown
	<-node.shutdown

	if !config.Quiet {
		fmt.Println("\n🌑 Shutting down Shadowy Node...")
	}

	// Clean shutdown
	return node.Stop()
}

// findFreePort finds a free port starting from the preferred port
func findFreePort(preferredPort int) int {
	// Try the preferred port first
	if isPortFree(preferredPort) {
		return preferredPort
	}

	// If preferred port is busy, try random high ports
	for attempts := 0; attempts < 10; attempts++ {
		// Random port between 8000-9999
		randomPort := 8000 + rand.Intn(2000)
		if isPortFree(randomPort) {
			return randomPort
		}
	}

	// Fallback: let the OS pick a free port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return preferredPort // Return preferred port as last resort
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

// isPortFree checks if a port is available
func isPortFree(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// startHTTPServer starts the HTTP server for wallet operations
func (ns *NodeServer) startHTTPServer() error {
	// Find a free port starting from 9090
	ns.httpPort = findFreePort(9090)

	mux := http.NewServeMux()

	// Wallet API endpoints
	mux.HandleFunc("/api/status", ns.handleStatus)
	mux.HandleFunc("/api/wallet/info", ns.handleWalletInfo)
	mux.HandleFunc("/api/wallet/balance", ns.handleWalletBalance)
	mux.HandleFunc("/api/tokens", ns.handleTokens)

	// Transaction API endpoints
	mux.HandleFunc("/api/transactions/send", ns.handleSendTransaction)
	mux.HandleFunc("/api/transactions/submit", ns.handleSubmitTransaction)
	mux.HandleFunc("/api/transactions", ns.handleGetTransactions)

	// UTXO and balance endpoints
	mux.HandleFunc("/api/utxos", ns.handleGetUTXOs)
	mux.HandleFunc("/api/balance", ns.handleGetBalance)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	ns.httpServer = &http.Server{
		Addr:    ":" + strconv.Itoa(ns.httpPort),
		Handler: mux,
	}

	ns.wg.Add(1)
	go func() {
		defer ns.wg.Done()
		if !ns.config.Quiet {
			fmt.Printf("   HTTP Server started on http://localhost:%d\n", ns.httpPort)
		}
		if err := ns.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// startTendermint starts the Tendermint blockchain node
func (ns *NodeServer) startTendermint() error {
	// Set up quiet mode for Tendermint if needed
	if ns.config.Quiet {
		SetupTendermintQuietMode()
	}

	// Create Tendermint configuration
	tmConfig := DefaultTendermintConfig(ns.config.BlockchainDir, ns.config.Seeds, ns.config.Quiet)

	// Create Tendermint node
	tmNode, err := NewTendermintNode(tmConfig)
	if err != nil {
		if ns.config.Quiet {
			RestoreTendermintLogging()
		}
		return fmt.Errorf("failed to create Tendermint node: %w", err)
	}

	ns.tendermint = tmNode

	// Set the node wallet address for coinbase rewards
	ns.tendermint.SetNodeAddress(ns.nodeWallet.GetAddress())

	// Set the node wallet private key for mining
	ns.tendermint.SetNodePrivateKey(ns.nodeWallet.GetPrivateKeyBytes())

	// Start the Tendermint node
	if err := ns.tendermint.Start(); err != nil {
		if ns.config.Quiet {
			RestoreTendermintLogging()
		}
		return fmt.Errorf("failed to start Tendermint node: %w", err)
	}

	// Save node ID to file
	if err := ns.tendermint.SaveNodeIDToFile("node_id.txt"); err != nil {
		if !ns.config.Quiet {
			fmt.Printf("   Warning: failed to save node ID to file: %v\n", err)
		}
	}

	if !ns.config.Quiet {
		nodeID, _ := ns.tendermint.GetNodeID()
		fmt.Printf("   Tendermint node started (ID: %s)\n", nodeID)
		fmt.Printf("   Seed connect string: %s@0.0.0.0:26666\n", nodeID)
		fmt.Printf("   Blockchain data: %s\n", ns.config.BlockchainDir)
		fmt.Printf("   Node ID saved to: node_id.txt\n")
	}

	return nil
}

// startShell starts the interactive shell
func (ns *NodeServer) startShell() error {
	shell := ishell.New()
	ns.shell = shell

	// Configure shell
	shell.Println("🌑 Shadowy Interactive Console")
	shell.Println("=============================")
	shell.Println("Type 'help' for available commands or 'exit' to quit.")

	// Add commands
	ns.addShellCommands()

	ns.wg.Add(1)
	go func() {
		defer ns.wg.Done()
		shell.Run()
	}()

	return nil
}

// addShellCommands adds commands to the interactive shell
func (ns *NodeServer) addShellCommands() {
	// Status command
	ns.shell.AddCmd(&ishell.Cmd{
		Name: "status",
		Help: "Display node status information",
		Func: func(c *ishell.Context) {
			status := ns.getNodeStatus()
			ns.printStatus(c, status)
		},
	})

	// Shutdown command
	ns.shell.AddCmd(&ishell.Cmd{
		Name: "shutdown",
		Help: "Shutdown the node cleanly",
		Func: func(c *ishell.Context) {
			c.Println("🌑 Initiating shutdown...")
			status := ns.getNodeStatus()
			ns.printStatus(c, status)
			c.Println("\n✅ Node state dumped. Shutting down...")
			ns.shutdown <- true
		},
	})

	// Wallet info command
	ns.shell.AddCmd(&ishell.Cmd{
		Name: "wallet",
		Help: "Display wallet information",
		Func: func(c *ishell.Context) {
			walletInfo := ns.nodeWallet.GetWalletInfo()
			c.Printf("Wallet Address: %s\n", ns.nodeWallet.GetAddressString())
			c.Printf("Wallet File: %s\n", walletInfo["path"])
			c.Printf("Key Type: ML-DSA87 (Post-Quantum)\n")
		},
	})

	// Token info command
	ns.shell.AddCmd(&ishell.Cmd{
		Name: "tokens",
		Help: "Display token registry information",
		Func: func(c *ishell.Context) {
			c.Printf("Token Registry: %d tokens\n", ns.tokenRegistry.GetTokenCount())
			genesisToken := GetGenesisToken()
			c.Printf("Genesis Token: %s (%s)\n", genesisToken.Name, genesisToken.Ticker)
			c.Printf("Token ID: %s\n", genesisToken.TokenID)
		},
	})

	// Config command
	ns.shell.AddCmd(&ishell.Cmd{
		Name: "config",
		Help: "Display current configuration",
		Func: func(c *ishell.Context) {
			c.Printf("Configuration: %s\n", ns.config.String())
			if len(ns.config.Seeds) > 0 {
				c.Printf("Seed Nodes (%d):\n", len(ns.config.Seeds))
				for i, seed := range ns.config.Seeds {
					c.Printf("  %d. %s\n", i+1, seed)
				}
			}
			if len(ns.config.Dirs) > 0 {
				c.Printf("Directories (%d):\n", len(ns.config.Dirs))
				for i, dir := range ns.config.Dirs {
					c.Printf("  %d. %s\n", i+1, dir)
				}
			}
		},
	})
}

// getNodeStatus returns the current node status
func (ns *NodeServer) getNodeStatus() *NodeStatus {
	walletInfo := ns.nodeWallet.GetWalletInfo()
	genesisToken := GetGenesisToken()

	var nodeID, seedConnectStr string
	if ns.tendermint != nil {
		nodeID, _ = ns.tendermint.GetNodeID()
		seedConnectStr = fmt.Sprintf("%s@0.0.0.0:26666", nodeID)
	}

	plotCount := GetPlotCount()
	farmingEnabled := plotCount > 0

	return &NodeStatus{
		NodeAddress:    ns.nodeWallet.GetAddressString(),
		NodeID:         nodeID,
		SeedConnectStr: seedConnectStr,
		WalletInfo:     walletInfo,
		TokenCount:     ns.tokenRegistry.GetTokenCount(),
		GenesisToken:   genesisToken,
		PlotCount:      plotCount,
		FarmingEnabled: farmingEnabled,
		Configuration:  ns.config.String(),
		Seeds:          ns.config.Seeds,
		Directories:    ns.config.Dirs,
		HTTPServerAddr: fmt.Sprintf("http://localhost:%d", ns.httpPort),
		StartTime:      ns.startTime,
		Uptime:         time.Since(ns.startTime),
	}
}

// printStatus prints the node status to the console
func (ns *NodeServer) printStatus(c *ishell.Context, status *NodeStatus) {
	c.Println("🌑 Shadowy Node Status")
	c.Println("====================")
	c.Printf("Node Address: %s\n", status.NodeAddress)
	c.Printf("Node ID: %s\n", status.NodeID)
	c.Printf("Seed Connect String: %s\n", status.SeedConnectStr)
	c.Printf("Wallet File: %s\n", status.WalletInfo["path"].(string))
	c.Printf("HTTP Server: %s\n", status.HTTPServerAddr)
	c.Printf("Configuration: %s\n", status.Configuration)
	c.Printf("Token Registry: %d tokens\n", status.TokenCount)
	c.Printf("Genesis Token: %s (%s)\n", status.GenesisToken.Name, status.GenesisToken.Ticker)
	c.Printf("Plot Files: %d loaded\n", status.PlotCount)
	if status.FarmingEnabled {
		c.Printf("Farming: ✅ ENABLED\n")
	} else {
		c.Printf("Farming: ❌ DISABLED (no plots)\n")
	}

	if len(status.Seeds) > 0 {
		c.Printf("Seed Nodes: %d configured\n", len(status.Seeds))
		for i, seed := range status.Seeds {
			c.Printf("  %d. %s\n", i+1, seed)
		}
	}

	if len(status.Directories) > 0 {
		c.Printf("Directories: %d configured\n", len(status.Directories))
		for i, dir := range status.Directories {
			c.Printf("  %d. %s\n", i+1, dir)
		}
	}
}

// HTTP Handlers

// handleStatus returns the node status as JSON
func (ns *NodeServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := ns.getNodeStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleWalletInfo returns wallet information
func (ns *NodeServer) handleWalletInfo(w http.ResponseWriter, r *http.Request) {
	walletInfo := ns.nodeWallet.GetWalletInfo()
	walletInfo["address"] = ns.nodeWallet.GetAddressString()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(walletInfo)
}

// handleWalletBalance returns wallet balance (placeholder)
func (ns *NodeServer) handleWalletBalance(w http.ResponseWriter, r *http.Request) {
	// Placeholder - in real implementation would check UTXO set
	balance := map[string]interface{}{
		"address": ns.nodeWallet.GetAddressString(),
		"balance": "0.00000000 SHADOW",
		"note":    "Balance calculation requires UTXO set implementation",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

// handleTokens returns token registry information
func (ns *NodeServer) handleTokens(w http.ResponseWriter, r *http.Request) {
	genesisToken := GetGenesisToken()
	tokens := map[string]interface{}{
		"count":         ns.tokenRegistry.GetTokenCount(),
		"genesis_token": genesisToken,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

// SendTransactionRequest represents a transaction send request
type SendTransactionRequest struct {
	ToAddress string `json:"to_address"`
	Amount    uint64 `json:"amount"`
	Fee       uint64 `json:"fee,omitempty"`
}

// SubmitTransactionRequest represents a raw transaction submission
type SubmitTransactionRequest struct {
	Transaction *Transaction `json:"transaction"`
}

// handleSendTransaction creates and broadcasts a simple send transaction
func (ns *NodeServer) handleSendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SendTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Parse address
	toAddress, _, err := ParseAddress(req.ToAddress)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Set default fee if not provided
	if req.Fee == 0 {
		req.Fee = 1000 // Default fee
	}

	// Create and sign transaction using wallet
	// TODO: Get UTXOs from UTXO set when implemented
	// For now, this will fail with "insufficient funds" which is expected
	tx, err := ns.nodeWallet.CreateAndSignSendTransaction(nil, toAddress, req.Amount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Submit transaction to mempool
	if err := ns.submitTransactionToMempool(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to submit transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Return transaction details
	txID, _ := tx.ID()
	response := map[string]interface{}{
		"transaction_id": txID,
		"status":         "submitted",
		"message":        "Transaction submitted to mempool",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSubmitTransaction submits a pre-signed transaction to the mempool
func (ns *NodeServer) handleSubmitTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SubmitTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Transaction == nil {
		http.Error(w, "Transaction is required", http.StatusBadRequest)
		return
	}

	// Validate transaction
	if err := ValidateTransaction(req.Transaction); err != nil {
		http.Error(w, fmt.Sprintf("Invalid transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Submit to mempool
	if err := ns.submitTransactionToMempool(req.Transaction); err != nil {
		http.Error(w, fmt.Sprintf("Failed to submit transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	txID, _ := req.Transaction.ID()
	response := map[string]interface{}{
		"transaction_id": txID,
		"status":         "submitted",
		"message":        "Transaction submitted to mempool",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// submitTransactionToMempool submits a transaction to Tendermint's mempool
func (ns *NodeServer) submitTransactionToMempool(tx *Transaction) error {
	if ns.tendermint == nil {
		return fmt.Errorf("Tendermint node not initialized")
	}

	// Serialize transaction to JSON
	txBytes, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Use Tendermint's BroadcastTxAsync to submit to mempool
	// This will call our CheckTx ABCI method for validation
	if err := ns.tendermint.BroadcastTransaction(txBytes); err != nil {
		return fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if !ns.config.Quiet {
		fmt.Printf("📡 Transaction submitted to mempool: %s\n", GetTransactionSummary(tx))
	}

	return nil
}

// handleGetUTXOs returns UTXOs for a given address
func (ns *NodeServer) handleGetUTXOs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addressStr := r.URL.Query().Get("address")
	if addressStr == "" {
		// Default to node's own address
		addressStr = ns.nodeWallet.GetAddressString()
	}

	_, _, err := ParseAddress(addressStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get UTXO store from Tendermint app
	if ns.tendermint == nil || ns.tendermint.app == nil {
		http.Error(w, "Node not initialized", http.StatusServiceUnavailable)
		return
	}

	address, _, err := ParseAddress(addressStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get actual UTXOs from store
	utxos, err := ns.tendermint.app.utxoStore.GetUTXOsByAddress(address)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert UTXOs to API format
	utxoData := make([]map[string]interface{}, len(utxos))
	for i, utxo := range utxos {
		utxoData[i] = map[string]interface{}{
			"tx_id":        utxo.TxID,
			"output_index": utxo.OutputIndex,
			"amount":       utxo.Output.Amount,
			"token_id":     utxo.Output.TokenID,
			"address":      utxo.Output.Address.String(),
			"block_height": utxo.BlockHeight,
			"is_spent":     utxo.IsSpent,
		}
	}

	response := map[string]interface{}{
		"address": addressStr,
		"utxos":   utxoData,
		"count":   len(utxos),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetBalance returns balance for a given address
func (ns *NodeServer) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addressStr := r.URL.Query().Get("address")
	if addressStr == "" {
		// Default to node's own address
		addressStr = ns.nodeWallet.GetAddressString()
	}

	_, _, err := ParseAddress(addressStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get UTXO store from Tendermint app
	if ns.tendermint == nil || ns.tendermint.app == nil {
		http.Error(w, "Node not initialized", http.StatusServiceUnavailable)
		return
	}

	address, _, err := ParseAddress(addressStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get actual balance from UTXO store
	balances, err := ns.tendermint.app.utxoStore.GetBalance(address)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get balance: %v", err), http.StatusInternalServerError)
		return
	}

	// Get genesis token for base currency
	genesisToken := GetGenesisToken()

	// Build balance array with only base currency for now
	balanceArray := []map[string]interface{}{}
	if amount, exists := balances[genesisToken.TokenID]; exists {
		balanceArray = append(balanceArray, map[string]interface{}{
			"token_id": genesisToken.TokenID,
			"name":     genesisToken.Name,
			"balance":  amount,
		})
	}

	response := map[string]interface{}{
		"address":  addressStr,
		"balances": balanceArray,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetTransactions returns paginated transactions for a given address
func (ns *NodeServer) handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addressStr := r.URL.Query().Get("address")
	if addressStr == "" {
		// Default to node's own address
		addressStr = ns.nodeWallet.GetAddressString()
	}

	address, _, err := ParseAddress(addressStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Parse count parameter (default 32)
	count := 32
	if countStr := r.URL.Query().Get("count"); countStr != "" {
		parsedCount, err := strconv.Atoi(countStr)
		if err != nil || parsedCount <= 0 {
			http.Error(w, "Invalid count parameter", http.StatusBadRequest)
			return
		}
		count = parsedCount
	}

	// Parse after parameter (optional)
	afterTxID := r.URL.Query().Get("after")

	// Get UTXO store from Tendermint app
	if ns.tendermint == nil || ns.tendermint.app == nil {
		http.Error(w, "Node not initialized", http.StatusServiceUnavailable)
		return
	}

	// Get transactions from UTXO store
	transactions, err := ns.tendermint.app.utxoStore.GetTransactionsByAddress(address, count, afterTxID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response with all useful transaction info
	txResponses := make([]map[string]interface{}, 0, len(transactions))
	for _, tx := range transactions {
		txID, _ := tx.ID()

		// Build inputs info
		inputs := make([]map[string]interface{}, len(tx.Inputs))
		for i, input := range tx.Inputs {
			inputs[i] = map[string]interface{}{
				"prev_tx_id":     input.PrevTxID,
				"output_index":   input.OutputIndex,
				"sequence":       input.Sequence,
			}
		}

		// Build outputs info
		outputs := make([]map[string]interface{}, len(tx.Outputs))
		for i, output := range tx.Outputs {
			outputs[i] = map[string]interface{}{
				"address":   output.Address.String(),
				"amount":    output.Amount,
				"token_id":  output.TokenID,
				"token_type": output.TokenType,
			}
		}

		txResponse := map[string]interface{}{
			"tx_id":     txID,
			"tx_type":   tx.TxType,
			"timestamp": tx.Timestamp,
			"inputs":    inputs,
			"outputs":   outputs,
		}

		txResponses = append(txResponses, txResponse)
	}

	response := map[string]interface{}{
		"address":      addressStr,
		"transactions": txResponses,
		"count":        len(txResponses),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSignals sets up signal handling for graceful shutdown
func (ns *NodeServer) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		if !ns.config.Quiet {
			fmt.Println("\n🌑 Received shutdown signal...")
		}
		ns.shutdown <- true
	}()
}

// Stop stops the node server
func (ns *NodeServer) Stop() error {
	// Cancel context
	ns.cancel()

	// Stop Tendermint node
	if ns.tendermint != nil {
		if err := ns.tendermint.Stop(); err != nil {
			log.Printf("Tendermint shutdown error: %v", err)
		}
	}

	// Restore logging if we were in quiet mode
	if ns.config.Quiet {
		RestoreTendermintLogging()
	}

	// Stop HTTP server
	if ns.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ns.httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}

	// Stop shell
	if ns.shell != nil {
		ns.shell.Close()
	}

	// Wait for goroutines
	ns.wg.Wait()

	if !ns.config.Quiet {
		fmt.Println("✅ Node stopped cleanly")
	}

	return nil
}
