package lib

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

const (
	ServiceTag = "shadowy-p2p"
)

// P2PNode represents a libp2p network node
type P2PNode struct {
	Host     host.Host
	ctx      context.Context
	cancel   context.CancelFunc
	peers    map[peer.ID]peer.AddrInfo
	peerLock sync.RWMutex
}

// discoveryNotifee implements the mdns.Notifee interface for peer discovery
type discoveryNotifee struct {
	node *P2PNode
}

func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	// Skip if it's ourselves
	if pi.ID == n.node.Host.ID() {
		return
	}

	// Check if already connected
	if n.node.Host.Network().Connectedness(pi.ID) == 1 { // 1 = Connected
		return
	}

	n.node.peerLock.Lock()
	n.node.peers[pi.ID] = pi
	n.node.peerLock.Unlock()

	fmt.Printf("[P2P] Discovered peer: %s\n", pi.ID.String())

	// Try to connect with retries (to handle simultaneous dial issues)
	go func() {
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			if n.node.Host.Network().Connectedness(pi.ID) == 1 {
				fmt.Printf("[P2P] Successfully connected to peer: %s\n", pi.ID.String())
				return
			}

			err := n.node.Host.Connect(context.Background(), pi)
			if err == nil {
				fmt.Printf("[P2P] Successfully connected to peer: %s\n", pi.ID.String())
				return
			}

			if i == maxRetries-1 {
				fmt.Printf("[P2P] Failed to connect to peer %s after %d attempts: %v\n", pi.ID.String(), maxRetries, err)
				return
			}

			// Wait before retry (exponential backoff)
			backoff := time.Duration(100*(i+1)) * time.Millisecond
			time.Sleep(backoff)
		}
	}()
}

// NewP2PNode creates a new libp2p node
func NewP2PNode(listenPort int) (*P2PNode, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create multiaddr for listening
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create listen address: %w", err)
	}

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrs(listenAddr),
		libp2p.DisableRelay(), // We don't need relay for local network
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	node := &P2PNode{
		Host:   h,
		ctx:    ctx,
		cancel: cancel,
		peers:  make(map[peer.ID]peer.AddrInfo),
	}

	// Setup mDNS discovery (for local network)
	discoveryService := mdns.NewMdnsService(h, ServiceTag, &discoveryNotifee{node})
	if err := discoveryService.Start(); err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("failed to start mDNS discovery: %w", err)
	}

	fmt.Printf("[P2P] Node started with ID: %s\n", h.ID().String())
	fmt.Printf("[P2P] Listening on: %v\n", h.Addrs())

	return node, nil
}

// GetPeers returns the list of connected peers
func (n *P2PNode) GetPeers() []peer.ID {
	n.peerLock.RLock()
	defer n.peerLock.RUnlock()

	peers := make([]peer.ID, 0, len(n.peers))
	for id := range n.peers {
		// Only include if we're actually connected
		if n.Host.Network().Connectedness(id) == 1 { // 1 = Connected
			peers = append(peers, id)
		}
	}
	return peers
}

// PrintPeerStatus prints current peer connection status
func (n *P2PNode) PrintPeerStatus() {
	peers := n.GetPeers()
	fmt.Printf("\n[P2P] Connected to %d peers:\n", len(peers))
	for _, p := range peers {
		fmt.Printf("  - %s\n", p.String())
	}
	fmt.Println()
}

// WaitForPeers waits until we have at least minPeers connected
func (n *P2PNode) WaitForPeers(minPeers int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for peers (have %d, want %d)", len(n.GetPeers()), minPeers)
		}

		peers := n.GetPeers()
		if len(peers) >= minPeers {
			return nil
		}

		select {
		case <-ticker.C:
			fmt.Printf("[P2P] Waiting for peers... (have %d, want %d)\n", len(peers), minPeers)
		case <-n.ctx.Done():
			return fmt.Errorf("context cancelled")
		}
	}
}

// ConnectToPeer manually connects to a peer by multiaddr string
func (n *P2PNode) ConnectToPeer(addrStr string) error {
	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("failed to parse peer info: %w", err)
	}

	if err := n.Host.Connect(n.ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	n.peerLock.Lock()
	n.peers[peerInfo.ID] = *peerInfo
	n.peerLock.Unlock()

	fmt.Printf("[P2P] Manually connected to peer: %s\n", peerInfo.ID.String())
	return nil
}

// Close shuts down the P2P node
func (n *P2PNode) Close() error {
	n.cancel()
	return n.Host.Close()
}
