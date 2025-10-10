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
}

// NewP2PBlockchainNode creates a new blockchain node
func NewP2PBlockchainNode(p2pPort, apiPort int) (*P2PBlockchainNode, error) {
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

	// Create mempool with shared gossip
	mempool, err := NewMempool(p2p.Host, ps)
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
	}

	// Start HTTP API
	go node.startAPI()

	fmt.Printf("[Node] Started with P2P on port %d, API on port %d\n", p2pPort, apiPort)
	fmt.Printf("[Node] Wallet address: %s\n", wallet.Address.String())

	return node, nil
}

// startAPI starts the HTTP API server
func (n *P2PBlockchainNode) startAPI() {
	mux := http.NewServeMux()

	// Submit transaction endpoint
	mux.HandleFunc("/api/tx/submit", n.handleSubmitTransaction)

	// Get mempool endpoint
	mux.HandleFunc("/api/mempool", n.handleGetMempool)

	// Get transaction by ID
	mux.HandleFunc("/api/tx/", n.handleGetTransaction)

	// Create and send transaction endpoint
	mux.HandleFunc("/api/tx/send", n.handleSendTransaction)

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
	mux.HandleFunc("/api/transactions/send", n.handleSendTransaction) // Alias for /api/tx/send

	// Node and wallet info
	mux.HandleFunc("/api/status", n.handleGetStatus)
	mux.HandleFunc("/api/wallet/info", n.handleGetWalletInfo)
	mux.HandleFunc("/api/tokens", n.handleGetTokens)

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

	// Filter for unspent UTXOs of the requested token
	var availableUTXOs []*UTXO
	for _, utxo := range utxos {
		if !utxo.IsSpent && utxo.Output.TokenID == tokenID {
			availableUTXOs = append(availableUTXOs, utxo)
		}
	}

	// Determine the fee to use
	// If custom fee provided, use it. Otherwise estimate based on inputs
	var targetFee uint64
	if req.Fee > 0 {
		targetFee = req.Fee
	} else {
		targetFee = 1000 // Default from API spec
	}

	// Select UTXOs to cover the amount + fee
	var selectedUTXOs []*UTXO
	var total uint64
	for _, utxo := range availableUTXOs {
		selectedUTXOs = append(selectedUTXOs, utxo)
		total += utxo.Output.Amount
		// If no custom fee, recalculate based on inputs selected
		if req.Fee == 0 {
			targetFee = uint64(len(selectedUTXOs)) * 1150
			if targetFee < 11500 {
				targetFee = 11500
			}
		}
		if total >= req.Amount+targetFee {
			break
		}
	}

	// Final check with actual fee
	if req.Fee == 0 {
		targetFee = uint64(len(selectedUTXOs)) * 1150
		if targetFee < 11500 {
			targetFee = 11500
		}
	}
	if total < req.Amount+targetFee {
		http.Error(w, fmt.Sprintf("Insufficient balance: have %d, need %d (including %d fee)", total, req.Amount+targetFee, targetFee), http.StatusBadRequest)
		return
	}

	// Create transaction manually to support memo
	txBuilder := NewTxBuilder(TxTypeSend)
	txBuilder.SetTimestamp(time.Now().Unix())

	// Add inputs
	for _, utxo := range selectedUTXOs {
		txBuilder.AddInput(utxo.TxID, utxo.OutputIndex)
	}

	// Add output to recipient
	txBuilder.AddOutput(toAddr, req.Amount, tokenID)

	// Add change output if needed
	change := total - req.Amount - targetFee
	if change > 0 {
		txBuilder.AddOutput(n.Wallet.Address, change, tokenID)
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
	for tokenID, balance := range balanceMap {
		tokenInfo := map[string]interface{}{
			"token_id": tokenID,
			"balance":  balance,
		}

		// Add token metadata (for now, only support SHADOW genesis token)
		if tokenID == GetGenesisToken().TokenID || tokenID == "SHADOW" {
			genesis := GetGenesisToken()
			tokenInfo["name"] = genesis.Name
			tokenInfo["ticker"] = genesis.Ticker
			tokenInfo["decimals"] = genesis.Decimals
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
			"name":     GetGenesisToken().Name,
			"symbol":   GetGenesisToken().Ticker,
			"decimals": GetGenesisToken().Decimals,
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
	genesisToken := GetGenesisToken()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": 1,
		"genesis_token": map[string]interface{}{
			"token_id":    genesisToken.TokenID,
			"name":        genesisToken.Name,
			"symbol":      genesisToken.Ticker,
			"decimals":    genesisToken.Decimals,
			"description": "The native token of the Shadowy post-quantum blockchain",
		},
	})
}

// Close shuts down the node
func (n *P2PBlockchainNode) Close() error {
	n.Consensus.Close()
	n.Mempool.Close()
	n.Chain.Close()
	return n.P2P.Close()
}
