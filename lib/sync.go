package lib

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	SyncProtocolID = "/shadowy/sync/1.0.0"
	BlockBatchSize = 100 // Request blocks in batches of 100
)

// SyncRequest is sent to request blocks
type SyncRequest struct {
	Type       string `json:"type"`         // "height" or "blocks"
	StartBlock uint64 `json:"start,omitempty"`
	EndBlock   uint64 `json:"end,omitempty"`
}

// SyncResponse contains the response data
type SyncResponse struct {
	Type   string   `json:"type"`   // "height" or "blocks"
	Height uint64   `json:"height,omitempty"`
	Blocks []*Block `json:"blocks,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// BlockSyncHandler handles incoming sync requests
type BlockSyncHandler struct {
	chain *Blockchain
}

// NewBlockSyncHandler creates a sync handler
func NewBlockSyncHandler(chain *Blockchain) *BlockSyncHandler {
	return &BlockSyncHandler{
		chain: chain,
	}
}

// SetupSyncProtocol registers the sync handler with libp2p
func SetupSyncProtocol(h host.Host, chain *Blockchain) {
	handler := NewBlockSyncHandler(chain)
	h.SetStreamHandler(SyncProtocolID, handler.HandleStream)
	fmt.Printf("[Sync] Registered sync protocol handler\n")
}

// HandleStream processes incoming sync requests
func (h *BlockSyncHandler) HandleStream(s network.Stream) {
	defer s.Close()

	// Read request
	reader := bufio.NewReader(s)
	var req SyncRequest

	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&req); err != nil {
		fmt.Printf("[Sync] Failed to decode request: %v\n", err)
		return
	}

	var resp SyncResponse

	switch req.Type {
	case "height":
		// Return current chain height
		resp = SyncResponse{
			Type:   "height",
			Height: h.chain.GetHeight() - 1, // Return latest block index
		}

	case "blocks":
		// Return requested block range
		if req.EndBlock < req.StartBlock {
			resp = SyncResponse{
				Type:  "blocks",
				Error: "invalid range: end < start",
			}
		} else {
			blocks := h.chain.GetBlockRange(req.StartBlock, req.EndBlock)
			resp = SyncResponse{
				Type:   "blocks",
				Blocks: blocks,
			}
			fmt.Printf("[Sync] Serving blocks %d-%d to peer\n", req.StartBlock, req.EndBlock)
		}

	default:
		resp = SyncResponse{
			Type:  req.Type,
			Error: "unknown request type",
		}
	}

	// Send response
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(resp); err != nil {
		fmt.Printf("[Sync] Failed to send response: %v\n", err)
	}
}

// BlockSyncClient handles requesting blocks from peers
type BlockSyncClient struct {
	host  host.Host
	chain *Blockchain
}

// NewBlockSyncClient creates a sync client
func NewBlockSyncClient(h host.Host, chain *Blockchain) *BlockSyncClient {
	return &BlockSyncClient{
		host:  h,
		chain: chain,
	}
}

// GetPeerHeight requests the height from a peer
func (c *BlockSyncClient) GetPeerHeight(peerID peer.ID) (uint64, error) {
	s, err := c.host.NewStream(context.Background(), peerID, SyncProtocolID)
	if err != nil {
		return 0, fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	// Send request
	req := SyncRequest{Type: "height"}
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(req); err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	var resp SyncResponse
	decoder := json.NewDecoder(s)
	if err := decoder.Decode(&resp); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return 0, fmt.Errorf("peer error: %s", resp.Error)
	}

	return resp.Height, nil
}

// RequestBlocks requests a range of blocks from a peer
func (c *BlockSyncClient) RequestBlocks(peerID peer.ID, start, end uint64) ([]*Block, error) {
	s, err := c.host.NewStream(context.Background(), peerID, SyncProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer s.Close()

	// Send request
	req := SyncRequest{
		Type:       "blocks",
		StartBlock: start,
		EndBlock:   end,
	}
	encoder := json.NewEncoder(s)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	var resp SyncResponse
	decoder := json.NewDecoder(s)
	if err := decoder.Decode(&resp); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("peer closed connection")
		}
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("peer error: %s", resp.Error)
	}

	return resp.Blocks, nil
}

// SyncFromPeer syncs the blockchain from a peer
func (c *BlockSyncClient) SyncFromPeer(peerID peer.ID) error {
	myHeight := c.chain.GetHeight() - 1 // Convert to block index

	// Get peer's height
	peerHeight, err := c.GetPeerHeight(peerID)
	if err != nil {
		return fmt.Errorf("failed to get peer height: %w", err)
	}

	if peerHeight <= myHeight {
		fmt.Printf("[Sync] Already synced (my: %d, peer: %d)\n", myHeight, peerHeight)
		return nil
	}

	fmt.Printf("[Sync] Starting sync from peer %s (my: %d, peer: %d)\n",
		peerID.String()[:16], myHeight, peerHeight)

	// Sync in batches
	blocksNeeded := peerHeight - myHeight
	fmt.Printf("[Sync] Need to download %d blocks\n", blocksNeeded)

	for start := myHeight + 1; start <= peerHeight; start += BlockBatchSize {
		end := start + BlockBatchSize - 1
		if end > peerHeight {
			end = peerHeight
		}

		fmt.Printf("[Sync] Requesting blocks %d-%d...\n", start, end)

		blocks, err := c.RequestBlocks(peerID, start, end)
		if err != nil {
			return fmt.Errorf("failed to get blocks %d-%d: %w", start, end, err)
		}

		if len(blocks) == 0 {
			return fmt.Errorf("peer returned no blocks for range %d-%d", start, end)
		}

		// Add blocks to our chain
		for _, block := range blocks {
			// Check if we already have this block (could have arrived via consensus during sync)
			currentHeight := c.chain.GetHeight() - 1 // Convert to block index
			if block.Index <= currentHeight {
				fmt.Printf("[Sync] Skipping block %d (already have it)\n", block.Index)
				continue
			}

			if err := c.chain.AddBlock(block); err != nil {
				return fmt.Errorf("failed to add block %d: %w", block.Index, err)
			}
			fmt.Printf("[Sync] Added block %d (hash: %s)\n", block.Index, block.Hash[:16])
		}
	}

	fmt.Printf("[Sync] ✓ Sync complete! Chain height now: %d\n", c.chain.GetHeight())
	return nil
}

// SyncFromBestPeer finds the best peer and syncs from them
func (c *BlockSyncClient) SyncFromBestPeer() error {
	peers := c.host.Network().Peers()
	if len(peers) == 0 {
		return fmt.Errorf("no peers available for sync")
	}

	// Try to find peer with highest height
	var bestPeer peer.ID
	var bestHeight uint64

	for _, p := range peers {
		height, err := c.GetPeerHeight(p)
		if err != nil {
			fmt.Printf("[Sync] Failed to get height from %s: %v\n", p.String()[:16], err)
			continue
		}

		if height > bestHeight {
			bestHeight = height
			bestPeer = p
		}
	}

	if bestPeer == "" {
		return fmt.Errorf("no peers responded with height")
	}

	return c.SyncFromPeer(bestPeer)
}
