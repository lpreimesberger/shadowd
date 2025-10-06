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
	consensus, err := NewConsensusEngine(chain, mempool, p2p.Host, ps)
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
		Token     string `json:"token"`
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

	// Create transaction
	builder := NewTxBuilder(TxTypeSend)
	builder.SetTimestamp(time.Now().Unix())
	builder.AddOutput(toAddr, req.Amount, req.Token)
	tx := builder.Build()

	// Sign it
	if err := n.Wallet.SignTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to sign: %v", err), http.StatusInternalServerError)
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

// Close shuts down the node
func (n *P2PBlockchainNode) Close() error {
	n.Consensus.Close()
	n.Mempool.Close()
	n.Chain.Close()
	return n.P2P.Close()
}
