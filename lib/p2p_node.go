package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

// P2PBlockchainNode represents a complete blockchain node with P2P and mempool
type P2PBlockchainNode struct {
	P2P       *P2PNode
	Mempool   *Mempool
	Wallet    *NodeWallet
	Chain     *Blockchain
	Consensus *ConsensusEngine
	apiPort   int
	apiKey    string // Optional API key for write endpoints
}

// NewP2PBlockchainNode creates a new blockchain node
func NewP2PBlockchainNode(p2pPort, apiPort int, config *CLIConfig) (*P2PBlockchainNode, error) {
	// Create P2P node
	p2p, err := NewP2PNode(p2pPort)
	if err != nil {
		return nil, fmt.Errorf("failed to create P2P node: %w", err)
	}

	// Create shared gossipsub instance
	ctx := context.Background()
	ps, err := pubsub.NewGossipSub(ctx, p2p.Host)
	if err != nil {
		p2p.Close()
		return nil, fmt.Errorf("failed to create gossipsub: %w", err)
	}

	// Create mempool with expiration and size limits from config
	expiryBlocks := config.MempoolTxExpiryBlocks
	maxSizeMB := config.MempoolMaxSizeMB
	if expiryBlocks <= 0 {
		expiryBlocks = 2048 // Default
	}
	if maxSizeMB <= 0 {
		maxSizeMB = 300 // Default
	}
	mempool, err := NewMempool(p2p.Host, ps, expiryBlocks, maxSizeMB)
	if err != nil {
		p2p.Close()
		return nil, fmt.Errorf("failed to create mempool: %w", err)
	}

	// Create wallet for this node
	wallet, err := LoadOrCreateNodeWallet()
	if err != nil {
		p2p.Close()
		mempool.Close()
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	// Create blockchain with persistent storage
	chain, err := NewBlockchain("blockchain")
	if err != nil {
		p2p.Close()
		mempool.Close()
		return nil, fmt.Errorf("failed to create blockchain: %w", err)
	}

	// Setup sync protocol (for serving blocks to others)
	SetupSyncProtocol(p2p.Host, chain)

	// Wait briefly for peers to connect, then sync if needed
	fmt.Printf("[Node] Waiting for peers to connect...\n")
	time.Sleep(3 * time.Second)

	// Sync from peers if we're behind
	syncClient := NewBlockSyncClient(p2p.Host, chain)
	peers := p2p.Host.Network().Peers()
	if len(peers) > 0 {
		fmt.Printf("[Node] Found %d peers, syncing blockchain...\n", len(peers))
		if err := syncClient.SyncFromBestPeer(); err != nil {
			fmt.Printf("[Node] Warning: sync failed: %v (continuing anyway)\n", err)
		}
	} else {
		fmt.Printf("[Node] No peers available for sync, starting with local chain\n")
	}

	// Create consensus engine with shared gossip (AFTER sync)
	consensus, err := NewConsensusEngine(chain, mempool, p2p.Host, ps, wallet, wallet.Address)
	if err != nil {
		p2p.Close()
		mempool.Close()
		chain.Close()
		return nil, fmt.Errorf("failed to create consensus: %w", err)
	}

	node := &P2PBlockchainNode{
		P2P:       p2p,
		Mempool:   mempool,
		Wallet:    wallet,
		Chain:     chain,
		Consensus: consensus,
		apiPort:   apiPort,
		apiKey:    config.APIKey, // Set from config
	}

	// Start HTTP API
	go node.startAPI()

	fmt.Printf("[Node] Started with P2P on port %d, API on port %d\n", p2pPort, apiPort)
	if node.apiKey != "" {
		fmt.Printf("[Node] 🔒 API key authentication enabled for write endpoints\n")
	}
	fmt.Printf("[Node] Wallet address: %s\n", wallet.Address.String())

	return node, nil
}

// requireAuth is middleware that checks API key for write endpoints
func (n *P2PBlockchainNode) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no API key configured, allow all requests
		if n.apiKey == "" {
			next(w, r)
			return
		}

		// Check X-API-Key header
		providedKey := r.Header.Get("X-API-Key")
		if providedKey != n.apiKey {
			http.Error(w, "Unauthorized: Invalid or missing API key", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// startAPI starts the HTTP API server
func (n *P2PBlockchainNode) startAPI() {
	mux := http.NewServeMux()

	// Submit transaction endpoint (protected)
	mux.HandleFunc("/api/tx/submit", n.requireAuth(n.handleSubmitTransaction))

	// Get mempool endpoint
	mux.HandleFunc("/api/mempool", n.handleGetMempool)

	// Get transaction by ID
	mux.HandleFunc("/api/tx/", n.handleGetTransaction)

	// Create and send transaction endpoint (protected)
	mux.HandleFunc("/api/tx/send", n.requireAuth(n.handleSendTransaction))

	// Peer status endpoint
	mux.HandleFunc("/api/peers", n.handleGetPeers)

	// Chain endpoints
	mux.HandleFunc("/api/chain", n.handleGetChain)
	mux.HandleFunc("/api/chain/height", n.handleGetHeight)
	mux.HandleFunc("/api/chain/block/", n.handleGetBlock)

	// Consensus status
	mux.HandleFunc("/api/consensus/status", n.handleConsensusStatus)

	// Balance and UTXO query
	mux.HandleFunc("/api/balance", n.handleGetBalance)
	mux.HandleFunc("/api/utxos", n.handleGetUTXOs)
	mux.HandleFunc("/api/transactions", n.handleGetTransactions)
	mux.HandleFunc("/api/transactions/send", n.requireAuth(n.handleSendTransaction)) // Alias (protected)

	// Node and wallet info
	mux.HandleFunc("/api/status", n.handleGetStatus)
	mux.HandleFunc("/api/wallet/info", n.handleGetWalletInfo)

	// Token endpoints
	mux.HandleFunc("/api/tokens", n.handleGetTokens)
	mux.HandleFunc("/api/token/info", n.handleGetTokenInfo)
	mux.HandleFunc("/api/token/mint", n.requireAuth(n.handleMintToken))       // Protected
	mux.HandleFunc("/api/token/melt", n.requireAuth(n.handleMeltToken))       // Protected

	// Swap endpoints
	mux.HandleFunc("/api/swap/offer", n.requireAuth(n.handleCreateOffer))     // Protected
	mux.HandleFunc("/api/swap/accept", n.requireAuth(n.handleAcceptOffer))    // Protected
	mux.HandleFunc("/api/swap/cancel", n.requireAuth(n.handleCancelOffer))    // Protected
	mux.HandleFunc("/api/swap/list", n.handleListOffers)

	// Pool endpoints
	mux.HandleFunc("/api/pool/create", n.requireAuth(n.handleCreatePool))              // Protected
	mux.HandleFunc("/api/pool/list", n.handleListPools)
	mux.HandleFunc("/api/pool/add_liquidity", n.requireAuth(n.handleAddLiquidity))     // Protected
	mux.HandleFunc("/api/pool/remove_liquidity", n.requireAuth(n.handleRemoveLiquidity)) // Protected
	mux.HandleFunc("/api/pool/swap", n.requireAuth(n.handleSwap))                      // Protected

	// Mempool management
	mux.HandleFunc("/api/mempool/cancel", n.requireAuth(n.handleCancelMempoolTx)) // Protected

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := fmt.Sprintf(":%d", n.apiPort)
	fmt.Printf("[API] Listening on http://0.0.0.0%s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("[API] Server error: %v\n", err)
	}
}

// handleSubmitTransaction handles transaction submission
func (n *P2PBlockchainNode) handleSubmitTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var tx Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, fmt.Sprintf("Invalid transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool (will verify signature and gossip)
	if err := n.Mempool.AddTransaction(&tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add transaction: %v", err), http.StatusBadRequest)
		return
	}

	txID, _ := tx.ID()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "accepted",
		"tx_id":  txID,
	})
}

// handleGetMempool returns all transactions in the mempool
func (n *P2PBlockchainNode) handleGetMempool(w http.ResponseWriter, r *http.Request) {
	txs := n.Mempool.GetTransactions()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":        len(txs),
		"transactions": txs,
	})
}

// handleGetTransaction returns a specific transaction
func (n *P2PBlockchainNode) handleGetTransaction(w http.ResponseWriter, r *http.Request) {
	// Extract TX ID from path
	txID := r.URL.Path[len("/api/tx/"):]
	if txID == "" {
		http.Error(w, "Transaction ID required", http.StatusBadRequest)
		return
	}

	tx, exists := n.Mempool.GetTransaction(txID)
	if !exists {
		http.Error(w, "Transaction not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tx)
}

// handleCancelMempoolTx allows users to cancel their own pending transactions
func (n *P2PBlockchainNode) handleCancelMempoolTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TxID      string `json:"tx_id"`
		PublicKey []byte `json:"public_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the transaction from mempool
	tx, exists := n.Mempool.GetTransaction(req.TxID)
	if !exists {
		http.Error(w, "Transaction not found in mempool", http.StatusNotFound)
		return
	}

	// Verify the caller owns this transaction by checking the signature public key
	// Extract public key from transaction signature
	if !tx.VerifyOwnership(req.PublicKey) {
		http.Error(w, "Unauthorized: You don't own this transaction", http.StatusForbidden)
		return
	}

	// Remove from mempool
	n.Mempool.RemoveTransaction(req.TxID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Transaction %s cancelled", req.TxID[:16]),
	})
}

// handleSendTransaction creates and sends a transaction
func (n *P2PBlockchainNode) handleSendTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ToAddress string `json:"to_address"`
		Amount    uint64 `json:"amount"`
		Token     string `json:"token"`      // Legacy field
		TokenID   string `json:"token_id"`   // API spec field
		Fee       uint64 `json:"fee"`        // Optional fee
		Memo      string `json:"memo"`       // Optional memo
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Parse destination address
	toAddr, _, err := ParseAddress(req.ToAddress)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Use SHADOW token if not specified
	// Support both "token" (legacy) and "token_id" (API spec)
	tokenID := req.TokenID
	if tokenID == "" {
		tokenID = req.Token
	}
	if tokenID == "" || tokenID == "SHADOW" {
		tokenID = GetGenesisToken().TokenID
	}

	// Get UTXOs for our wallet
	utxos, err := n.Chain.GetUTXOStore().GetUTXOsByAddress(n.Wallet.Address)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if sending custom token (not SHADOW)
	genesisTokenID := GetGenesisToken().TokenID
	isCustomToken := tokenID != genesisTokenID

	// Filter for unspent UTXOs of the requested token
	var availableTokenUTXOs []*UTXO
	var availableShadowUTXOs []*UTXO
	for _, utxo := range utxos {
		if !utxo.IsSpent {
			if utxo.Output.TokenID == tokenID {
				availableTokenUTXOs = append(availableTokenUTXOs, utxo)
			} else if utxo.Output.TokenID == genesisTokenID {
				availableShadowUTXOs = append(availableShadowUTXOs, utxo)
			}
		}
	}

	// Estimate fee first to know how much we need
	estimatedFee := req.Fee
	if estimatedFee == 0 {
		estimatedFee = 11500 // Default minimum fee
	}

	// If sending SHADOW, we need to cover amount + fee from same UTXOs
	requiredAmount := req.Amount
	if tokenID == genesisTokenID {
		requiredAmount = req.Amount + estimatedFee
	}

	// Select token UTXOs to cover the required amount
	var selectedTokenUTXOs []*UTXO
	var tokenTotal uint64
	for _, utxo := range availableTokenUTXOs {
		selectedTokenUTXOs = append(selectedTokenUTXOs, utxo)
		tokenTotal += utxo.Output.Amount
		if tokenTotal >= requiredAmount {
			break
		}
	}

	if tokenTotal < req.Amount {
		http.Error(w, fmt.Sprintf("Insufficient %s balance: have %d, need %d", tokenID[:16], tokenTotal, req.Amount), http.StatusBadRequest)
		return
	}

	// For custom tokens, also need SHADOW for fee
	var selectedShadowUTXOs []*UTXO
	var shadowTotal uint64
	var targetFee uint64

	if isCustomToken {
		// Estimate fee based on token inputs + shadow inputs needed
		if req.Fee > 0 {
			targetFee = req.Fee
		} else {
			// Estimate: token inputs + ~2 shadow inputs + outputs
			targetFee = uint64(len(selectedTokenUTXOs)+2) * 1150
			if targetFee < 11500 {
				targetFee = 11500
			}
		}

		// Select SHADOW UTXOs for fee
		for _, utxo := range availableShadowUTXOs {
			selectedShadowUTXOs = append(selectedShadowUTXOs, utxo)
			shadowTotal += utxo.Output.Amount
			if shadowTotal >= targetFee {
				break
			}
		}

		if shadowTotal < targetFee {
			http.Error(w, fmt.Sprintf("Insufficient SHADOW for fee: have %d, need %d", shadowTotal, targetFee), http.StatusBadRequest)
			return
		}
	} else {
		// Sending SHADOW: fee comes from the same UTXOs
		if req.Fee > 0 {
			targetFee = req.Fee
		} else {
			targetFee = uint64(len(selectedTokenUTXOs)) * 1150
			if targetFee < 11500 {
				targetFee = 11500
			}
		}

		if tokenTotal < req.Amount+targetFee {
			http.Error(w, fmt.Sprintf("Insufficient balance: have %d, need %d (including %d fee)", tokenTotal, req.Amount+targetFee, targetFee), http.StatusBadRequest)
			return
		}
	}

	// Create transaction manually to support memo
	txBuilder := NewTxBuilder(TxTypeSend)
	txBuilder.SetTimestamp(time.Now().Unix())

	// Add token inputs
	for _, utxo := range selectedTokenUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// For custom tokens, also add SHADOW inputs for fee
	if isCustomToken {
		for _, utxo := range selectedShadowUTXOs {
			txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
		}
	}

	// Add output to recipient (token)
	txBuilder.AddOutput(toAddr, req.Amount, tokenID)

	// Add change outputs
	if isCustomToken {
		// Custom token: change is separate for token and SHADOW
		tokenChange := tokenTotal - req.Amount
		if tokenChange > 0 {
			txBuilder.AddOutput(n.Wallet.Address, tokenChange, tokenID)
		}

		shadowChange := shadowTotal - targetFee
		if shadowChange > 0 {
			txBuilder.AddOutput(n.Wallet.Address, shadowChange, genesisTokenID)
		}
	} else {
		// SHADOW: fee is deducted from same UTXOs
		change := tokenTotal - req.Amount - targetFee
		if change > 0 {
			txBuilder.AddOutput(n.Wallet.Address, change, tokenID)
		}
	}

	tx := txBuilder.Build()

	// Add memo if provided
	if req.Memo != "" {
		// Validate memo is ASCII and max 64 bytes
		if len(req.Memo) > 64 {
			http.Error(w, "Memo must be <= 64 bytes", http.StatusBadRequest)
			return
		}
		for _, c := range req.Memo {
			if c > 127 {
				http.Error(w, "Memo must be ASCII only", http.StatusBadRequest)
				return
			}
		}
		tx.Data = []byte(req.Memo)
	}

	// Sign the transaction
	if err := n.Wallet.SignTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to sign transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Add to mempool
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add transaction: %v", err), http.StatusBadRequest)
		return
	}

	txID, _ := tx.ID()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"tx_id":  txID,
		"tx":     tx,
	})
}

// handleGetPeers returns connected peers
func (n *P2PBlockchainNode) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	peers := n.P2P.GetPeers()
	peerStrs := make([]string, len(peers))
	for i, p := range peers {
		peerStrs[i] = p.String()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(peers),
		"peers": peerStrs,
	})
}

// handleGetChain returns the entire blockchain
func (n *P2PBlockchainNode) handleGetChain(w http.ResponseWriter, r *http.Request) {
	blocks := n.Chain.GetBlocks()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"height": len(blocks),
		"blocks": blocks,
	})
}

// handleGetHeight returns the current blockchain height
func (n *P2PBlockchainNode) handleGetHeight(w http.ResponseWriter, r *http.Request) {
	height := n.Chain.GetHeight()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"height": height,
	})
}

// handleGetBlock returns a specific block by index
func (n *P2PBlockchainNode) handleGetBlock(w http.ResponseWriter, r *http.Request) {
	// Extract block index from path
	indexStr := r.URL.Path[len("/api/chain/block/"):]
	if indexStr == "" {
		http.Error(w, "Block index required", http.StatusBadRequest)
		return
	}

	var index uint64
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		http.Error(w, "Invalid block index", http.StatusBadRequest)
		return
	}

	block := n.Chain.GetBlock(index)
	if block == nil {
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(block)
}

// handleConsensusStatus returns consensus status
func (n *P2PBlockchainNode) handleConsensusStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_leader": n.Consensus.IsLeader(),
		"node_id":   n.Consensus.nodeID,
		"height":    n.Chain.GetHeight(),
	})
}

// handleGetBalance returns the balance and UTXOs for an address
func (n *P2PBlockchainNode) handleGetBalance(w http.ResponseWriter, r *http.Request) {
	// Get address from query parameter or use node's own address
	addrStr := r.URL.Query().Get("address")
	if addrStr == "" {
		addrStr = n.Wallet.Address.String()
	}

	// Parse address
	addr, _, err := ParseAddress(addrStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get UTXOs for this address
	utxos, err := n.Chain.GetUTXOStore().GetUTXOsByAddress(addr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Calculate balance by token
	balanceMap := make(map[string]uint64)
	utxoList := []map[string]interface{}{}

	for _, utxo := range utxos {
		if !utxo.IsSpent {
			// Add to balance
			balanceMap[utxo.Output.TokenID] += utxo.Output.Amount

			// Add to UTXO list
			utxoList = append(utxoList, map[string]interface{}{
				"tx_id":        utxo.TxID,
				"output_index": utxo.OutputIndex,
				"amount":       utxo.Output.Amount,
				"token_id":     utxo.Output.TokenID,
				"block_height": utxo.BlockHeight,
			})
		}
	}

	// Convert balance map to array with token details
	balances := []map[string]interface{}{}
	tokenRegistry := GetGlobalTokenRegistry()

	fmt.Printf("[Balance] Token registry has %d tokens registered\n", tokenRegistry.GetTokenCount())

	for tokenID, balance := range balanceMap {
		tokenInfo := map[string]interface{}{
			"token_id": tokenID,
			"balance":  balance,
		}

		// Look up token metadata from registry
		token, exists := tokenRegistry.GetToken(tokenID)
		if exists {
			tokenInfo["name"] = token.Ticker // Use ticker as name
			tokenInfo["ticker"] = token.Ticker
			tokenInfo["decimals"] = token.MaxDecimals
		} else {
			// For unknown tokens, provide defaults
			tokenInfo["name"] = "Unknown Token"
			tokenInfo["ticker"] = "???"
			tokenInfo["decimals"] = 8
		}

		balances = append(balances, tokenInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"address":  addrStr,
		"balances": balances,
		"utxos":    utxoList,
		"count":    len(utxoList),
	})
}

// handleGetUTXOs returns UTXOs for an address
func (n *P2PBlockchainNode) handleGetUTXOs(w http.ResponseWriter, r *http.Request) {
	// Get address from query parameter or use node's own address
	addrStr := r.URL.Query().Get("address")
	if addrStr == "" {
		addrStr = n.Wallet.Address.String()
	}

	// Parse address
	addr, _, err := ParseAddress(addrStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get UTXOs for this address
	utxos, err := n.Chain.GetUTXOStore().GetUTXOsByAddress(addr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Build UTXO list
	utxoList := []map[string]interface{}{}
	for _, utxo := range utxos {
		if !utxo.IsSpent {
			utxoList = append(utxoList, map[string]interface{}{
				"tx_id":        utxo.TxID,
				"output_index": utxo.OutputIndex,
				"amount":       utxo.Output.Amount,
				"token_id":     utxo.Output.TokenID,
				"address":      utxo.Output.Address.String(),
				"block_height": utxo.BlockHeight,
				"is_spent":     utxo.IsSpent,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"address": addrStr,
		"utxos":   utxoList,
		"count":   len(utxoList),
	})
}

// handleGetTransactions returns transaction history for an address
func (n *P2PBlockchainNode) handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	// Get address from query parameter or use node's own address
	addrStr := r.URL.Query().Get("address")
	if addrStr == "" {
		addrStr = n.Wallet.Address.String()
	}

	// Parse address
	addr, _, err := ParseAddress(addrStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid address: %v", err), http.StatusBadRequest)
		return
	}

	// Get UTXOs to find transactions involving this address
	utxos, err := n.Chain.GetUTXOStore().GetUTXOsByAddress(addr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Build transaction list (deduplicate by tx_id)
	txMap := make(map[string]map[string]interface{})
	for _, utxo := range utxos {
		if _, exists := txMap[utxo.TxID]; !exists {
			// Get the full transaction from the block
			block := n.Chain.GetBlock(utxo.BlockHeight)
			if block != nil {
				// For now, just return basic info
				txMap[utxo.TxID] = map[string]interface{}{
					"tx_id":        utxo.TxID,
					"block_height": utxo.BlockHeight,
					"timestamp":    block.Timestamp,
				}
			}
		}
	}

	// Convert map to slice
	txList := []map[string]interface{}{}
	for _, tx := range txMap {
		txList = append(txList, tx)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"address":      addrStr,
		"transactions": txList,
		"count":        len(txList),
	})
}

// handleGetStatus returns node status information
func (n *P2PBlockchainNode) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	peers := n.P2P.GetPeers()
	peerStrs := make([]string, len(peers))
	for i, p := range peers {
		peerStrs[i] = p.String()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id": n.P2P.Host.ID().String(),
		"wallet_info": map[string]string{
			"address": n.Wallet.Address.String(),
		},
		"genesis_token": map[string]interface{}{
			"token_id": GetGenesisToken().TokenID,
			"name":     GetGenesisToken().Ticker,
			"symbol":   GetGenesisToken().Ticker,
			"decimals": GetGenesisToken().MaxDecimals,
		},
		"chain_height":     n.Chain.GetHeight(),
		"peers":            peerStrs,
		"peer_count":       len(peers),
		"http_server_addr": fmt.Sprintf("http://localhost:%d", n.apiPort),
		"is_leader":        n.Consensus.IsLeader(),
	})
}

// handleGetWalletInfo returns wallet information
func (n *P2PBlockchainNode) handleGetWalletInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"address": n.Wallet.Address.String(),
	})
}

// handleGetTokens returns token registry information
func (n *P2PBlockchainNode) handleGetTokens(w http.ResponseWriter, r *http.Request) {
	registry := GetGlobalTokenRegistry()
	tokens := registry.ListTokens()

	tokenList := make([]map[string]interface{}, 0)
	for _, token := range tokens {
		// Skip fully melted tokens from the list (dead tokens that allowed ticker reuse)
		if token.IsFullyMelted() {
			continue
		}
		tokenList = append(tokenList, map[string]interface{}{
			"token_id":     token.TokenID,
			"ticker":       token.Ticker,
			"description":  token.Desc,
			"max_mint":     token.MaxMint,
			"max_decimals": token.MaxDecimals,
			"total_supply": token.TotalSupply,
			"locked_shadow": token.LockedShadow,
			"total_melted":  token.TotalMelted,
			"creator":       token.CreatorAddress.String(),
			"is_shadow":     token.IsBaseToken(),
			"fully_melted":  token.IsFullyMelted(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(tokenList),
		"tokens": tokenList,
	})
}

func (n *P2PBlockchainNode) handleGetTokenInfo(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Query().Get("token_id")
	if tokenID == "" {
		http.Error(w, "token_id parameter required", http.StatusBadRequest)
		return
	}

	registry := GetGlobalTokenRegistry()
	token, exists := registry.GetToken(tokenID)
	if !exists {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token_id":      token.TokenID,
		"ticker":        token.Ticker,
		"description":   token.Desc,
		"max_mint":      token.MaxMint,
		"max_decimals":  token.MaxDecimals,
		"total_supply":  token.TotalSupply,
		"locked_shadow": token.LockedShadow,
		"total_melted":  token.TotalMelted,
		"creator":       token.CreatorAddress.String(),
		"creation_time": token.CreationTime,
		"is_shadow":     token.IsBaseToken(),
		"fully_melted":  token.IsFullyMelted(),
		"supply_formatted": token.FormatSupply(),
	})
}

func (n *P2PBlockchainNode) handleMintToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Ticker      string `json:"ticker"`
		Description string `json:"description"`
		MaxMint     uint64 `json:"max_mint"`
		MaxDecimals uint8  `json:"max_decimals"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get SHADOW UTXOs for staking
	shadowTokenID := GetGenesisToken().TokenID
	utxos, err := n.Chain.utxoStore.GetUTXOsByAddress(n.Wallet.Address)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Filter for SHADOW UTXOs and calculate required amount
	// Calculate total supply and estimated fee first
	totalSupply := req.MaxMint
	for i := uint8(0); i < req.MaxDecimals; i++ {
		totalSupply *= 10
	}

	// Estimate fee (will be recalculated in CreateTokenMintTransaction)
	estimatedFee := CalculateTxFee(TxTypeMintToken, 10, 2, 0) // Estimate ~10 inputs
	requiredAmount := totalSupply + estimatedFee

	// Select only enough UTXOs to cover the required amount
	var shadowUTXOs []*UTXO
	totalSelected := uint64(0)
	for _, utxo := range utxos {
		if utxo.Output.TokenID == shadowTokenID {
			shadowUTXOs = append(shadowUTXOs, utxo)
			totalSelected += utxo.Output.Amount

			// Stop once we have enough (with some buffer for fee adjustment)
			if totalSelected >= requiredAmount*2 {
				break
			}
		}
	}

	// Create mint transaction
	tx, err := CreateTokenMintTransaction(
		n.Wallet.Address,
		shadowUTXOs,
		req.Ticker,
		req.Description,
		req.MaxMint,
		req.MaxDecimals,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create mint transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Sign transaction
	if err := n.Wallet.SignTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("failed to sign transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Broadcast transaction
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("failed to broadcast transaction: %v", err), http.StatusInternalServerError)
		return
	}

	txID, _ := tx.ID()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"tx_id":    txID,
		"token_id": txID, // Token ID = TX ID for minting
		"message":  fmt.Sprintf("Token %s minting transaction broadcast", req.Ticker),
	})
}

func (n *P2PBlockchainNode) handleMeltToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TokenID string `json:"token_id"`
		Amount  uint64 `json:"amount"` // Amount to melt (0 = melt all)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get token UTXOs
	utxos, err := n.Chain.utxoStore.GetUTXOsByAddress(n.Wallet.Address)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get UTXOs: %v", err), http.StatusInternalServerError)
		return
	}

	// Filter for this token
	var tokenUTXOs []*UTXO
	totalTokens := uint64(0)
	for _, utxo := range utxos {
		if utxo.Output.TokenID == req.TokenID {
			tokenUTXOs = append(tokenUTXOs, utxo)
			totalTokens += utxo.Output.Amount
		}
	}

	if len(tokenUTXOs) == 0 {
		http.Error(w, "no UTXOs found for this token", http.StatusBadRequest)
		return
	}

	// If amount is 0, melt everything
	meltAmount := req.Amount
	if meltAmount == 0 {
		meltAmount = totalTokens
	}

	if meltAmount > totalTokens {
		http.Error(w, fmt.Sprintf("insufficient tokens: have %d, want to melt %d", totalTokens, meltAmount), http.StatusBadRequest)
		return
	}

	// Create melt transaction
	tx, err := CreateTokenMeltTransaction(
		tokenUTXOs,
		meltAmount,
		n.Wallet.Address, // Change back to us
		n.Wallet.Address, // Unlocked SHADOW to us
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create melt transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Sign transaction
	if err := n.Wallet.SignTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("failed to sign transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Broadcast transaction
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("failed to broadcast transaction: %v", err), http.StatusInternalServerError)
		return
	}

	txID, _ := tx.ID()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"tx_id":        txID,
		"melted_amount": meltAmount,
		"message":       fmt.Sprintf("Melted %d tokens", meltAmount),
	})
}

// handleCreateOffer creates a new atomic swap offer
func (n *P2PBlockchainNode) handleCreateOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		HaveTokenID    string `json:"have_token_id"`
		WantTokenID    string `json:"want_token_id"`
		HaveAmount     uint64 `json:"have_amount"`
		WantAmount     uint64 `json:"want_amount"`
		ExpiresAtBlock uint64 `json:"expires_at_block"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.HaveTokenID == "" || req.WantTokenID == "" {
		http.Error(w, "have_token_id and want_token_id are required", http.StatusBadRequest)
		return
	}

	if req.HaveAmount == 0 || req.WantAmount == 0 {
		http.Error(w, "amounts must be greater than zero", http.StatusBadRequest)
		return
	}

	currentHeight := n.Chain.GetHeight()
	if req.ExpiresAtBlock == 0 {
		// Default to 2 weeks of blocks (~10 second blocks = 120960 blocks)
		req.ExpiresAtBlock = currentHeight + 120960
	}

	if req.ExpiresAtBlock <= currentHeight {
		http.Error(w, "expires_at_block must be in the future", http.StatusBadRequest)
		return
	}

	// Create offer transaction
	tx, err := CreateOfferTransaction(
		n.Wallet,
		n.Chain.GetUTXOStore(),
		req.HaveTokenID,
		req.WantTokenID,
		req.HaveAmount,
		req.WantAmount,
		req.ExpiresAtBlock,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create offer: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool (gossips automatically)
	txID, _ := tx.ID()
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":     txID,
		"status":    "offer_created",
		"expires_at": req.ExpiresAtBlock,
	})
}

// handleAcceptOffer accepts an existing swap offer
func (n *P2PBlockchainNode) handleAcceptOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OfferTxID string `json:"offer_tx_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.OfferTxID == "" {
		http.Error(w, "offer_tx_id is required", http.StatusBadRequest)
		return
	}

	currentHeight := n.Chain.GetHeight()

	// Create accept transaction
	tx, err := CreateAcceptOfferTransaction(
		n.Wallet,
		n.Chain.GetUTXOStore(),
		req.OfferTxID,
		currentHeight,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to accept offer: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool (gossips automatically)
	txID, _ := tx.ID()
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":      txID,
		"status":     "offer_accepted",
		"offer_tx_id": req.OfferTxID,
	})
}

// handleCancelOffer cancels an existing swap offer
func (n *P2PBlockchainNode) handleCancelOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OfferTxID string `json:"offer_tx_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.OfferTxID == "" {
		http.Error(w, "offer_tx_id is required", http.StatusBadRequest)
		return
	}

	currentHeight := n.Chain.GetHeight()

	// Create cancel transaction
	tx, err := CreateCancelOfferTransaction(
		n.Wallet,
		n.Chain.GetUTXOStore(),
		req.OfferTxID,
		currentHeight,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to cancel offer: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool and gossip
	// Add to mempool (gossips automatically)
	txID, _ := tx.ID()
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":      txID,
		"status":     "offer_cancelled",
		"offer_tx_id": req.OfferTxID,
	})
}

// isOfferConsumed checks if an offer has been accepted or cancelled
func (n *P2PBlockchainNode) isOfferConsumed(offerTxID string, utxoStore *UTXOStore) bool {
	currentHeight := n.Chain.GetHeight()

	// Scan all blocks for accept/cancel transactions referencing this offer
	for i := uint64(0); i < currentHeight; i++ {
		block := n.Chain.GetBlock(i)
		if block == nil {
			continue
		}

		for _, txID := range block.Transactions {
			tx, err := utxoStore.GetTransaction(txID)
			if err != nil || tx == nil {
				continue
			}

			// Check if this is an accept or cancel transaction
			if tx.TxType == TxTypeAcceptOffer {
				var acceptData AcceptOfferData
				if err := json.Unmarshal(tx.Data, &acceptData); err == nil {
					if acceptData.OfferTxID == offerTxID {
						return true
					}
				}
			} else if tx.TxType == TxTypeCancelOffer {
				var cancelData CancelOfferData
				if err := json.Unmarshal(tx.Data, &cancelData); err == nil {
					if cancelData.OfferTxID == offerTxID {
						return true
					}
				}
			}
		}
	}

	return false
}

// handleListOffers lists all active swap offers
func (n *P2PBlockchainNode) handleListOffers(w http.ResponseWriter, r *http.Request) {
	currentHeight := n.Chain.GetHeight()
	utxoStore := n.Chain.GetUTXOStore()

	// Scan blockchain for offer transactions
	offers := make([]map[string]interface{}, 0)

	// Get all blocks (we'll optimize this later if needed)
	for i := uint64(0); i < currentHeight; i++ {
		block := n.Chain.GetBlock(i)
		if block == nil {
			continue
		}

		// Check each transaction in the block
		for _, txID := range block.Transactions {
			tx, err := utxoStore.GetTransaction(txID)
			if err != nil || tx == nil {
				continue
			}

			// Only process offer transactions
			if tx.TxType != TxTypeOffer {
				continue
			}

			// Parse offer data
			var offerData OfferData
			if err := json.Unmarshal(tx.Data, &offerData); err != nil {
				continue
			}

			// Check if offer is expired
			if currentHeight > offerData.ExpiresAtBlock {
				continue
			}

			// Check if offer has been consumed (accepted or cancelled)
			// An offer is consumed if there's an accept/cancel tx referencing it
			isConsumed := n.isOfferConsumed(txID, utxoStore)
			if isConsumed {
				continue
			}

			// This is an active offer!
			offers = append(offers, map[string]interface{}{
				"offer_tx_id":     txID,
				"have_token_id":   offerData.HaveTokenID,
				"want_token_id":   offerData.WantTokenID,
				"have_amount":     offerData.HaveAmount,
				"want_amount":     offerData.WantAmount,
				"expires_at_block": offerData.ExpiresAtBlock,
				"offer_address":   offerData.OfferAddress.String(),
				"block_height":    i,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"offers":        offers,
		"count":         len(offers),
		"current_height": currentHeight,
	})
}

// handleCreatePool handles pool creation requests
func (n *P2PBlockchainNode) handleCreatePool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TokenA     string `json:"token_a"`
		TokenB     string `json:"token_b"`
		AmountA    uint64 `json:"amount_a"`
		AmountB    uint64 `json:"amount_b"`
		FeePercent uint64 `json:"fee_percent"` // Optional, defaults to 30 (0.3%)
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Default fee to 0.3% if not specified
	if req.FeePercent == 0 {
		req.FeePercent = 30
	}

	// Validate fee is in range
	if err := ValidateFeePercent(req.FeePercent); err != nil {
		http.Error(w, fmt.Sprintf("Invalid fee: %v", err), http.StatusBadRequest)
		return
	}

	// Get stores
	utxoStore := n.Chain.GetUTXOStore()
	tokenRegistry := GetGlobalTokenRegistry()
	poolRegistry := n.Chain.GetPoolRegistry()

	// Check if pool already exists for this token pair (in either order)
	existingPools := poolRegistry.GetAllPools()
	for _, pool := range existingPools {
		if (pool.TokenA == req.TokenA && pool.TokenB == req.TokenB) ||
			(pool.TokenA == req.TokenB && pool.TokenB == req.TokenA) {
			http.Error(w, fmt.Sprintf("Pool already exists for this token pair: %s/%s (pool ID: %s)",
				req.TokenA[:8], req.TokenB[:8], pool.PoolID[:16]), http.StatusConflict)
			return
		}
	}

	fmt.Printf("[API] Creating pool transaction: %s/%s amounts: %d/%d fee: %d\n",
		req.TokenA[:8], req.TokenB[:8], req.AmountA, req.AmountB, req.FeePercent)

	// Create pool transaction
	tx, err := CreatePoolTransaction(n.Wallet, utxoStore, tokenRegistry,
		req.TokenA, req.TokenB, req.AmountA, req.AmountB, req.FeePercent)
	if err != nil {
		fmt.Printf("[API] Failed to create pool transaction: %v\n", err)
		http.Error(w, fmt.Sprintf("Failed to create pool transaction: %v", err), http.StatusBadRequest)
		return
	}

	txID, _ := tx.ID()
	fmt.Printf("[API] Created pool transaction: %s (type: %d, inputs: %d, outputs: %d)\n",
		txID[:16], tx.TxType, len(tx.Inputs), len(tx.Outputs))

	// Add to mempool
	fmt.Printf("[API] Adding transaction to mempool: %s\n", txID[:16])
	if err := n.Mempool.AddTransaction(tx); err != nil {
		fmt.Printf("[API] Failed to add to mempool: %v\n", err)
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}
	fmt.Printf("[API] Successfully added transaction to mempool: %s\n", txID[:16])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":  txID,
		"status": "pool_creation_submitted",
		"pool_id": txID, // Pool ID is the creation transaction ID
	})
}

// handleListPools lists all active liquidity pools
func (n *P2PBlockchainNode) handleListPools(w http.ResponseWriter, r *http.Request) {
	poolRegistry := n.Chain.GetPoolRegistry()
	tokenRegistry := GetGlobalTokenRegistry()

	pools := poolRegistry.GetAllPools()

	poolList := make([]map[string]interface{}, 0, len(pools))
	for _, pool := range pools {
		// Get token info for display
		tokenA, _ := tokenRegistry.GetToken(pool.TokenA)
		tokenB, _ := tokenRegistry.GetToken(pool.TokenB)
		lpToken, _ := tokenRegistry.GetToken(pool.LPTokenID)

		// Calculate current exchange rate
		var rateAtoB, rateBtoA float64
		if pool.ReserveB > 0 {
			rateAtoB = float64(pool.ReserveA) / float64(pool.ReserveB)
		}
		if pool.ReserveA > 0 {
			rateBtoA = float64(pool.ReserveB) / float64(pool.ReserveA)
		}

		poolInfo := map[string]interface{}{
			"pool_id":       pool.PoolID,
			"token_a":       pool.TokenA,
			"token_a_ticker": "",
			"token_b":       pool.TokenB,
			"token_b_ticker": "",
			"reserve_a":     pool.ReserveA,
			"reserve_b":     pool.ReserveB,
			"lp_token_id":   pool.LPTokenID,
			"lp_token_ticker": "",
			"lp_token_supply": pool.LPTokenSupply,
			"fee_percent":   pool.FeePercent,
			"k":             pool.K,
			"rate_a_to_b":   rateAtoB,
			"rate_b_to_a":   rateBtoA,
			"created_at":    pool.CreatedAt,
		}

		if tokenA != nil {
			poolInfo["token_a_ticker"] = tokenA.Ticker
		}
		if tokenB != nil {
			poolInfo["token_b_ticker"] = tokenB.Ticker
		}
		if lpToken != nil {
			poolInfo["lp_token_ticker"] = lpToken.Ticker
		}

		poolList = append(poolList, poolInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pools": poolList,
		"count": len(poolList),
	})
}

// handleAddLiquidity handles add liquidity requests
func (n *P2PBlockchainNode) handleAddLiquidity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PoolID      string `json:"pool_id"`
		AmountA     uint64 `json:"amount_a"`
		AmountB     uint64 `json:"amount_b"`
		MinLPTokens uint64 `json:"min_lp_tokens"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get stores
	utxoStore := n.Chain.GetUTXOStore()
	poolRegistry := n.Chain.GetPoolRegistry()

	// Create add liquidity transaction
	tx, err := CreateAddLiquidityTransaction(n.Wallet, utxoStore, poolRegistry,
		req.PoolID, req.AmountA, req.AmountB, req.MinLPTokens)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	txID, _ := tx.ID()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":  txID,
		"status": "add_liquidity_submitted",
	})
}

// handleRemoveLiquidity handles remove liquidity requests
func (n *P2PBlockchainNode) handleRemoveLiquidity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PoolID     string `json:"pool_id"`
		LPTokens   uint64 `json:"lp_tokens"`
		MinAmountA uint64 `json:"min_amount_a"`
		MinAmountB uint64 `json:"min_amount_b"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get stores
	utxoStore := n.Chain.GetUTXOStore()
	poolRegistry := n.Chain.GetPoolRegistry()

	// Create remove liquidity transaction
	tx, err := CreateRemoveLiquidityTransaction(n.Wallet, utxoStore, poolRegistry,
		req.PoolID, req.LPTokens, req.MinAmountA, req.MinAmountB)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	txID, _ := tx.ID()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":  txID,
		"status": "remove_liquidity_submitted",
	})
}

// handleSwap handles token swap requests
func (n *P2PBlockchainNode) handleSwap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PoolID       string `json:"pool_id"`
		TokenIn      string `json:"token_in"`
		AmountIn     uint64 `json:"amount_in"`
		MinAmountOut uint64 `json:"min_amount_out"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get stores
	utxoStore := n.Chain.GetUTXOStore()
	poolRegistry := n.Chain.GetPoolRegistry()

	// Create swap transaction
	tx, err := CreateSwapTransaction(n.Wallet, utxoStore, poolRegistry,
		req.PoolID, req.TokenIn, req.AmountIn, req.MinAmountOut)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create transaction: %v", err), http.StatusBadRequest)
		return
	}

	// Add to mempool
	if err := n.Mempool.AddTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to add to mempool: %v", err), http.StatusInternalServerError)
		return
	}

	txID, _ := tx.ID()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tx_id":  txID,
		"status": "swap_submitted",
	})
}

// Close shuts down the node
func (n *P2PBlockchainNode) Close() error {
	n.Consensus.Close()
	n.Mempool.Close()
	n.Chain.Close()
	return n.P2P.Close()
}
