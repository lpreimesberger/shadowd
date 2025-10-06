package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ConsensusTopic  = "shadowy-consensus"
	BlockInterval   = 10 * time.Second // Propose new block every 10 seconds
	MinVoteThreshold = 0.5              // Need >50% of nodes to vote yes
)

// ConsensusMessage types
type ConsensusMessageType string

const (
	MsgTypeBlockProposal ConsensusMessageType = "block_proposal"
	MsgTypeBlockVote     ConsensusMessageType = "block_vote"
	MsgTypeBlockCommit   ConsensusMessageType = "block_commit"
)

// ConsensusMessage is the gossip message format for consensus
type ConsensusMessage struct {
	Type      ConsensusMessageType `json:"type"`
	Proposal  *BlockProposal       `json:"proposal,omitempty"`
	Vote      *BlockVote           `json:"vote,omitempty"`
	Block     *Block               `json:"block,omitempty"`
	Timestamp int64                `json:"timestamp"`
}

// ConsensusEngine manages blockchain consensus
type ConsensusEngine struct {
	chain      *Blockchain
	mempool    *Mempool
	pubsub     *pubsub.PubSub
	topic      *pubsub.Topic
	sub        *pubsub.Subscription
	host       host.Host
	nodeID     string
	ctx        context.Context
	cancel     context.CancelFunc

	// Consensus state
	isLeader         bool
	leaderLock       sync.RWMutex
	pendingProposal  *Block
	proposalVotes    map[string]bool // voter -> vote
	voteLock         sync.RWMutex
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(chain *Blockchain, mempool *Mempool, h host.Host, ps *pubsub.PubSub) (*ConsensusEngine, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Join consensus topic
	topic, err := ps.Join(ConsensusTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to join topic: %w", err)
	}

	// Subscribe
	sub, err := topic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	ce := &ConsensusEngine{
		chain:      chain,
		mempool:    mempool,
		pubsub:     ps,
		topic:      topic,
		sub:        sub,
		host:       h,
		nodeID:     h.ID().String(),
		ctx:        ctx,
		cancel:     cancel,
		isLeader:   false,
		proposalVotes: make(map[string]bool),
	}

	// Start listening for consensus messages
	go ce.listenForMessages()

	// Start simple leader election (for now, just use peer ID comparison)
	go ce.leaderElection()

	// Start block proposal loop (if leader)
	go ce.blockProposalLoop()

	fmt.Printf("[Consensus] Started consensus engine, node ID: %s\n", ce.nodeID[:16])
	fmt.Printf("[Consensus] Waiting 5 seconds for gossipsub mesh to form...\n")

	// Give gossipsub mesh time to form before starting consensus
	time.Sleep(5 * time.Second)

	fmt.Printf("[Consensus] Mesh formation complete, consensus active!\n")
	return ce, nil
}

// leaderElection implements a simple leader election
// For now: lowest peer ID is the leader (deterministic)
func (ce *ConsensusEngine) leaderElection() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ce.ctx.Done():
			return
		case <-ticker.C:
			// Get connected peers
			peers := ce.host.Network().Peers()

			// Include ourselves
			allPeers := append([]peer.ID{ce.host.ID()}, peers...)

			// Find lowest peer ID (simple deterministic leader)
			var leader peer.ID
			for _, p := range allPeers {
				if leader == "" || p.String() < leader.String() {
					leader = p
				}
			}

			wasLeader := ce.IsLeader()
			ce.leaderLock.Lock()
			ce.isLeader = (leader == ce.host.ID())
			ce.leaderLock.Unlock()

			if ce.isLeader && !wasLeader {
				fmt.Printf("[Consensus] 👑 I am now the leader!\n")
			} else if !ce.isLeader && wasLeader {
				fmt.Printf("[Consensus] Lost leader status\n")
			}
		}
	}
}

// IsLeader returns whether this node is currently the leader
func (ce *ConsensusEngine) IsLeader() bool {
	ce.leaderLock.RLock()
	defer ce.leaderLock.RUnlock()
	return ce.isLeader
}

// blockProposalLoop proposes new blocks periodically (if leader)
func (ce *ConsensusEngine) blockProposalLoop() {
	ticker := time.NewTicker(BlockInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ce.ctx.Done():
			return
		case <-ticker.C:
			if ce.IsLeader() {
				ce.proposeBlock()
			}
		}
	}
}

// proposeBlock creates and proposes a new block
func (ce *ConsensusEngine) proposeBlock() {
	// Get transactions from mempool
	txs := ce.mempool.GetTransactions()
	txIDs := make([]string, 0, len(txs))
	for _, tx := range txs {
		txID, err := tx.ID()
		if err != nil {
			continue
		}
		txIDs = append(txIDs, txID)
	}

	// Limit to first 100 transactions
	if len(txIDs) > 100 {
		txIDs = txIDs[:100]
	}

	// Create block proposal
	block := ce.chain.ProposeBlock(txIDs, ce.nodeID)

	// Store as pending proposal
	ce.voteLock.Lock()
	ce.pendingProposal = block
	ce.proposalVotes = make(map[string]bool)
	// Vote for our own proposal
	ce.proposalVotes[ce.nodeID] = true
	ce.voteLock.Unlock()

	// Gossip proposal
	proposal := &BlockProposal{
		Block:     block,
		Proposer:  ce.nodeID,
		Timestamp: time.Now().Unix(),
	}

	msg := ConsensusMessage{
		Type:      MsgTypeBlockProposal,
		Proposal:  proposal,
		Timestamp: time.Now().Unix(),
	}

	ce.publishMessage(msg)

	fmt.Printf("[Consensus] Proposed block %d with %d transactions\n", block.Index, len(txIDs))
}

// listenForMessages processes incoming consensus messages
func (ce *ConsensusEngine) listenForMessages() {
	fmt.Printf("[Consensus] 👂 Listening for messages on topic: %s\n", ConsensusTopic)

	for {
		msg, err := ce.sub.Next(ce.ctx)
		if err != nil {
			if ce.ctx.Err() != nil {
				fmt.Printf("[Consensus] Context cancelled, stopping listener\n")
				return
			}
			fmt.Printf("[Consensus] Error reading message: %v\n", err)
			continue
		}

		fmt.Printf("[Consensus] 📨 Received message from: %s (self: %s)\n",
			msg.ReceivedFrom.String()[:16], ce.host.ID().String()[:16])

		// Skip our own messages
		if msg.ReceivedFrom == ce.host.ID() {
			fmt.Printf("[Consensus] Skipping own message\n")
			continue
		}

		var consensusMsg ConsensusMessage
		if err := json.Unmarshal(msg.Data, &consensusMsg); err != nil {
			fmt.Printf("[Consensus] Failed to decode message: %v\n", err)
			continue
		}

		fmt.Printf("[Consensus] Message type: %s\n", consensusMsg.Type)
		ce.handleMessage(&consensusMsg)
	}
}

// handleMessage processes a consensus message
func (ce *ConsensusEngine) handleMessage(msg *ConsensusMessage) {
	switch msg.Type {
	case MsgTypeBlockProposal:
		ce.handleBlockProposal(msg.Proposal)
	case MsgTypeBlockVote:
		ce.handleBlockVote(msg.Vote)
	case MsgTypeBlockCommit:
		ce.handleBlockCommit(msg.Block)
	}
}

// handleBlockProposal handles a new block proposal
func (ce *ConsensusEngine) handleBlockProposal(proposal *BlockProposal) {
	if proposal == nil || proposal.Block == nil {
		return
	}

	block := proposal.Block
	fmt.Printf("[Consensus] Received block proposal %d from %s\n", block.Index, proposal.Proposer[:16])

	// Validate block
	if err := ce.chain.ValidateBlock(block); err != nil {
		fmt.Printf("[Consensus] Invalid block proposal: %v\n", err)
		return
	}

	// Store as pending
	ce.voteLock.Lock()
	ce.pendingProposal = block
	ce.proposalVotes = make(map[string]bool)
	ce.voteLock.Unlock()

	// Vote yes
	ce.voteOnBlock(block, true)
}

// voteOnBlock casts a vote on a block
func (ce *ConsensusEngine) voteOnBlock(block *Block, approve bool) {
	vote := &BlockVote{
		BlockHash:  block.Hash,
		BlockIndex: block.Index,
		Voter:      ce.nodeID,
		Vote:       approve,
		Timestamp:  time.Now().Unix(),
	}

	// Record our own vote
	ce.voteLock.Lock()
	ce.proposalVotes[ce.nodeID] = approve
	ce.voteLock.Unlock()

	// Gossip vote
	msg := ConsensusMessage{
		Type:      MsgTypeBlockVote,
		Vote:      vote,
		Timestamp: time.Now().Unix(),
	}

	ce.publishMessage(msg)
	fmt.Printf("[Consensus] Voted %v on block %d\n", approve, block.Index)
}

// handleBlockVote processes a vote on a block
func (ce *ConsensusEngine) handleBlockVote(vote *BlockVote) {
	if vote == nil {
		return
	}

	ce.voteLock.Lock()
	defer ce.voteLock.Unlock()

	// Check if we have a pending proposal matching this vote
	if ce.pendingProposal == nil || ce.pendingProposal.Hash != vote.BlockHash {
		return
	}

	// Record vote
	ce.proposalVotes[vote.Voter] = vote.Vote

	yesVotes := 0
	totalVotes := len(ce.proposalVotes)
	for _, v := range ce.proposalVotes {
		if v {
			yesVotes++
		}
	}

	fmt.Printf("[Consensus] Block %d votes: %d yes / %d total\n", vote.BlockIndex, yesVotes, totalVotes)

	// Check if we have quorum (need majority + need at least 1 peer)
	peerCount := len(ce.host.Network().Peers()) + 1 // +1 for ourselves
	if peerCount < 2 {
		peerCount = 2 // Minimum for testing
	}

	if totalVotes >= peerCount && float64(yesVotes)/float64(totalVotes) > MinVoteThreshold {
		fmt.Printf("[Consensus] ✓ Block %d approved! Committing...\n", ce.pendingProposal.Index)
		ce.commitBlock(ce.pendingProposal)
	}
}

// commitBlock adds the block to the chain and broadcasts commit
func (ce *ConsensusEngine) commitBlock(block *Block) {
	// Add to chain
	if err := ce.chain.AddBlock(block); err != nil {
		fmt.Printf("[Consensus] Failed to add block: %v\n", err)
		return
	}

	// Remove transactions from mempool
	for _, txID := range block.Transactions {
		ce.mempool.RemoveTransaction(txID)
	}

	// Clear pending proposal
	ce.pendingProposal = nil
	ce.proposalVotes = make(map[string]bool)

	// Broadcast commit
	msg := ConsensusMessage{
		Type:      MsgTypeBlockCommit,
		Block:     block,
		Timestamp: time.Now().Unix(),
	}
	ce.publishMessage(msg)
}

// handleBlockCommit processes a block commit
func (ce *ConsensusEngine) handleBlockCommit(block *Block) {
	if block == nil {
		return
	}

	// Check if we already have this block
	existing := ce.chain.GetBlock(block.Index)
	if existing != nil && existing.Hash == block.Hash {
		return
	}

	// Add to our chain
	if err := ce.chain.AddBlock(block); err != nil {
		fmt.Printf("[Consensus] Failed to add committed block: %v\n", err)
		return
	}

	// Remove transactions from mempool
	for _, txID := range block.Transactions {
		ce.mempool.RemoveTransaction(txID)
	}

	fmt.Printf("[Consensus] Committed block %d from network\n", block.Index)
}

// publishMessage publishes a consensus message
func (ce *ConsensusEngine) publishMessage(msg ConsensusMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[Consensus] Failed to marshal message: %v\n", err)
		return
	}

	if err := ce.topic.Publish(ce.ctx, data); err != nil {
		fmt.Printf("[Consensus] Failed to publish message: %v\n", err)
	}
}

// Close shuts down the consensus engine
func (ce *ConsensusEngine) Close() error {
	ce.cancel()
	ce.sub.Cancel()
	return ce.topic.Close()
}
