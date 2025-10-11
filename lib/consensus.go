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
	ConsensusTopic   = "shadowy-consensus"
	ProofTopic       = "shadowy-proofs" // New topic for proof competition
	BlockInterval    = 10 * time.Second // Propose new block every 10 seconds
	ProofWindow      = 8 * time.Second  // Time window to collect proofs before block proposal
	MinVoteThreshold = 0.5              // Need >50% of nodes to vote yes
)

// ConsensusMessage types
type ConsensusMessageType string

const (
	MsgTypeBlockProposal   ConsensusMessageType = "block_proposal"
	MsgTypeBlockVote       ConsensusMessageType = "block_vote"
	MsgTypeBlockCommit     ConsensusMessageType = "block_commit"
	MsgTypeProofSubmission ConsensusMessageType = "proof_submission"
)

// ProofSubmission represents a farmer's proof submission for a block
type ProofSubmission struct {
	BlockHeight   uint64        `json:"block_height"`
	Proof         *ProofOfSpace `json:"proof"`
	RewardAddress Address       `json:"reward_address"` // Where to send block reward
	SubmitterID   string        `json:"submitter_id"`   // Node ID that submitted
}

// ConsensusMessage is the gossip message format for consensus
type ConsensusMessage struct {
	Type            ConsensusMessageType `json:"type"`
	Proposal        *BlockProposal       `json:"proposal,omitempty"`
	Vote            *BlockVote           `json:"vote,omitempty"`
	Block           *Block               `json:"block,omitempty"`
	ProofSubmission *ProofSubmission     `json:"proof_submission,omitempty"`
	Timestamp       int64                `json:"timestamp"`
}

// ConsensusEngine manages blockchain consensus
type ConsensusEngine struct {
	chain         *Blockchain
	mempool       *Mempool
	pubsub        *pubsub.PubSub
	topic         *pubsub.Topic
	sub           *pubsub.Subscription
	proofTopic    *pubsub.Topic        // Topic for proof submissions
	proofSub      *pubsub.Subscription // Subscription to proof topic
	host          host.Host
	nodeID        string
	rewardAddress Address // Address to receive block rewards
	ctx           context.Context
	cancel        context.CancelFunc
	wallet        *NodeWallet // Wallet for signing proofs

	// Consensus state
	isLeader        bool
	leaderLock      sync.RWMutex
	pendingProposal *Block
	proposalVotes   map[string]bool // voter -> vote
	voteLock        sync.RWMutex

	// Proof competition state
	bestProofForHeight map[uint64]*ProofSubmission // Track best proof per height
	proofLock          sync.RWMutex
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(chain *Blockchain, mempool *Mempool, h host.Host, ps *pubsub.PubSub, wallet *NodeWallet, rewardAddr Address) (*ConsensusEngine, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Join consensus topic
	topic, err := ps.Join(ConsensusTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to join consensus topic: %w", err)
	}

	// Subscribe to consensus
	sub, err := topic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe to consensus: %w", err)
	}

	// Join proof competition topic
	proofTopic, err := ps.Join(ProofTopic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to join proof topic: %w", err)
	}

	// Subscribe to proofs
	proofSub, err := proofTopic.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to subscribe to proofs: %w", err)
	}

	ce := &ConsensusEngine{
		chain:              chain,
		mempool:            mempool,
		rewardAddress:      rewardAddr,
		pubsub:             ps,
		topic:              topic,
		sub:                sub,
		proofTopic:         proofTopic,
		proofSub:           proofSub,
		host:               h,
		nodeID:             h.ID().String(),
		wallet:             wallet,
		ctx:                ctx,
		cancel:             cancel,
		isLeader:           false,
		proposalVotes:      make(map[string]bool),
		bestProofForHeight: make(map[uint64]*ProofSubmission),
	}

	// Start listening for consensus messages
	go ce.listenForMessages()

	// Start listening for proof submissions
	go ce.listenForProofs()

	// Start farming loop (generate and submit proofs)
	go ce.farmingLoop()

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
	currentHeight := ce.chain.GetHeight() + 1

	// Get the best proof for this height
	bestProof := ce.GetBestProof(currentHeight)
	if bestProof == nil || bestProof.Proof == nil {
		fmt.Printf("[Consensus] ⚠️  No proof available for height %d, skipping block proposal\n", currentHeight)
		return
	}

	fmt.Printf("[Consensus] 🏆 Using winning proof with distance %d from %s\n",
		bestProof.Proof.Distance, bestProof.SubmitterID[:16])

	// Get transactions from mempool
	txs := ce.mempool.GetTransactions()
	txIDs := []string{}
	totalFees := uint64(0)

	// Calculate total fees from transactions
	for _, tx := range txs {
		txID, err := tx.ID()
		if err != nil {
			continue
		}
		txIDs = append(txIDs, txID)

		// Calculate fee: inputs - outputs
		var inputTotal, outputTotal uint64
		for _, input := range tx.Inputs {
			// Get the UTXO being spent
			utxo, err := ce.chain.GetUTXOStore().GetUTXO(input.PrevTxID, input.OutputIndex)
			if err == nil && utxo != nil {
				inputTotal += utxo.Output.Amount
			}
		}
		for _, output := range tx.Outputs {
			outputTotal += output.Amount
		}
		if inputTotal > outputTotal {
			totalFees += (inputTotal - outputTotal)
		}
	}

	// Limit to first 100 transactions
	if len(txIDs) > 100 {
		txIDs = txIDs[:100]
	}

	// Create coinbase transaction - reward goes to proof WINNER not proposer!
	blockReward := uint64(50_000_000) // 50 SHADOW base reward
	coinbaseTx := NewTxBuilder(TxTypeCoinbase)
	coinbaseTx.SetTimestamp(time.Now().Unix())
	coinbaseTx.AddOutput(bestProof.RewardAddress, blockReward+totalFees, "SHADOW")
	coinbase := coinbaseTx.Build()

	coinbaseID, _ := coinbase.ID()
	txIDs = append([]string{coinbaseID}, txIDs...) // Prepend coinbase

	// Create block proposal (includes coinbase + winning proof)
	block := ce.chain.ProposeBlock(txIDs, ce.nodeID, coinbase)
	block.WinningProof = bestProof.Proof
	block.WinnerAddress = &bestProof.RewardAddress

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

	// Require votes from a majority of nodes (more than half)
	requiredVotes := (peerCount / 2) + 1
	threshold := float64(yesVotes) / float64(totalVotes)
	fmt.Printf("[Consensus] Quorum check: totalVotes=%d >= requiredVotes=%d ? %v, threshold=%.2f > 0.5 ? %v\n",
		totalVotes, requiredVotes, totalVotes >= requiredVotes, threshold, threshold > MinVoteThreshold)

	if totalVotes >= requiredVotes && float64(yesVotes)/float64(totalVotes) > MinVoteThreshold {
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

	// Update mempool with new block height for expiration tracking
	ce.mempool.UpdateBlockHeight(block.Index)

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

	// Update mempool with new block height for expiration tracking
	ce.mempool.UpdateBlockHeight(block.Index)

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

// farmingLoop continuously farms for proofs and submits them
func (ce *ConsensusEngine) farmingLoop() {
	ticker := time.NewTicker(2 * time.Second) // Check every 2 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ce.ctx.Done():
			return
		case <-ticker.C:
			// Get current challenge
			challenge := ce.chain.GetCurrentChallenge()
			currentHeight := ce.chain.GetHeight() + 1

			// Check if we already have plots loaded
			if GetPlotCount() == 0 {
				// No plots available, skip farming
				continue
			}

			// Marshal private key for proof generation
			privKeyBytes, err := ce.wallet.KeyPair.PrivateKey.MarshalBinary()
			if err != nil {
				fmt.Printf("[Farming] Failed to marshal private key: %v\n", err)
				continue
			}

			// Generate proof for this challenge
			proof, err := GenerateProofOfSpace(challenge, privKeyBytes)
			if err != nil {
				fmt.Printf("[Farming] Failed to generate proof: %v\n", err)
				continue
			}

			// Check if this proof is better than what we've seen
			ce.proofLock.RLock()
			bestProof := ce.bestProofForHeight[currentHeight]
			ce.proofLock.RUnlock()

			isBetter := false
			if bestProof == nil || proof.Distance < bestProof.Proof.Distance {
				isBetter = true
			}

			if isBetter {
				fmt.Printf("[Farming] 🌾 Found proof with distance %d for height %d\n", proof.Distance, currentHeight)

				// Submit our proof
				submission := &ProofSubmission{
					BlockHeight:   currentHeight,
					Proof:         proof,
					RewardAddress: ce.rewardAddress,
					SubmitterID:   ce.nodeID,
				}

				// Track locally
				ce.proofLock.Lock()
				ce.bestProofForHeight[currentHeight] = submission
				ce.proofLock.Unlock()

				// Gossip to network
				msg := ConsensusMessage{
					Type:            MsgTypeProofSubmission,
					ProofSubmission: submission,
					Timestamp:       time.Now().Unix(),
				}

				data, err := json.Marshal(msg)
				if err == nil {
					ce.proofTopic.Publish(ce.ctx, data)
				}
			}
		}
	}
}

// listenForProofs listens for proof submissions from other nodes
func (ce *ConsensusEngine) listenForProofs() {
	fmt.Printf("[Farming] 👂 Listening for proof submissions on topic: %s\n", ProofTopic)

	for {
		msg, err := ce.proofSub.Next(ce.ctx)
		if err != nil {
			if ce.ctx.Err() != nil {
				return
			}
			fmt.Printf("[Farming] Error reading proof message: %v\n", err)
			continue
		}

		// Skip our own messages
		if msg.ReceivedFrom == ce.host.ID() {
			continue
		}

		var consensusMsg ConsensusMessage
		if err := json.Unmarshal(msg.Data, &consensusMsg); err != nil {
			fmt.Printf("[Farming] Failed to decode proof message: %v\n", err)
			continue
		}

		if consensusMsg.Type == MsgTypeProofSubmission {
			ce.handleProofSubmission(consensusMsg.ProofSubmission)
		}
	}
}

// handleProofSubmission processes a received proof submission
func (ce *ConsensusEngine) handleProofSubmission(submission *ProofSubmission) {
	if submission == nil || submission.Proof == nil {
		return
	}

	// Validate the proof cryptographically
	if !ValidateProofOfSpace(submission.Proof) {
		fmt.Printf("[Farming] ❌ Invalid proof from %s\n", submission.SubmitterID[:16])
		return
	}

	// Check if this is for current or near-future height
	currentHeight := ce.chain.GetHeight() + 1
	if submission.BlockHeight < currentHeight || submission.BlockHeight > currentHeight+2 {
		// Too old or too far in future
		return
	}

	// Check if this proof is better than what we have
	ce.proofLock.Lock()
	defer ce.proofLock.Unlock()

	bestProof := ce.bestProofForHeight[submission.BlockHeight]
	if bestProof == nil || submission.Proof.Distance < bestProof.Proof.Distance {
		fmt.Printf("[Farming] 🏆 New best proof for height %d: distance=%d from %s\n",
			submission.BlockHeight, submission.Proof.Distance, submission.SubmitterID[:16])
		ce.bestProofForHeight[submission.BlockHeight] = submission
	}
}

// GetBestProof returns the best proof seen for a given height
func (ce *ConsensusEngine) GetBestProof(height uint64) *ProofSubmission {
	ce.proofLock.RLock()
	defer ce.proofLock.RUnlock()
	return ce.bestProofForHeight[height]
}

// Close shuts down the consensus engine
func (ce *ConsensusEngine) Close() error {
	ce.cancel()
	ce.sub.Cancel()
	ce.proofSub.Cancel()
	ce.topic.Close()
	return ce.proofTopic.Close()
}
