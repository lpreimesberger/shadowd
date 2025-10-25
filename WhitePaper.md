# Shadowy Apparatus: A Post-Quantum Blockchain Protocol

**Version 1.0**  
**August 2025**

**Abstract**

Shadowy Apparatus introduces a novel blockchain architecture designed specifically for the post-quantum era. By integrating ML-DSA-87 (Dilithium Mode3) digital signatures, a UTXO-based transaction model, and proof-of-space mining with configurable proof pruning, the protocol provides quantum-resistant security while maintaining practical storage requirements and decentralization. This paper presents the technical foundations, cryptographic innovations, and architectural decisions that make Shadowy Apparatus a production-ready post-quantum blockchain with native token support and automated market maker (AMM) liquidity pools.

---

## Table of Contents

1. [Introduction](#introduction)
2. [Background and Motivation](#background-and-motivation)
3. [System Architecture](#system-architecture)
4. [Cryptographic Foundations](#cryptographic-foundations)
5. [Consensus Mechanism](#consensus-mechanism)
6. [Transaction Model](#transaction-model)
7. [Network Protocol](#network-protocol)
8. [Security Analysis](#security-analysis)
9. [Performance Evaluation](#performance-evaluation)
10. [Economic Model](#economic-model)
11. [Ecosystem and Applications](#ecosystem-and-applications)
12. [Future Work](#future-work)
13. [Conclusion](#conclusion)

---

## 1. Introduction

The advent of quantum computing poses an existential threat to current cryptographic systems. Classical digital signatures based on RSA, ECDSA, and EdDSA will become vulnerable to Shor's algorithm when sufficiently large quantum computers emerge. Blockchain systems, which rely fundamentally on digital signatures for transaction authorization and block validation, face particular risk.

Shadowy Apparatus addresses this challenge by implementing a complete post-quantum blockchain protocol. The system features:

- **Quantum-Resistant Cryptography**: ML-DSA-87 digital signatures providing 224-bit security
- **UTXO Transaction Model**: Bitcoin-inspired unspent transaction outputs with quantum-safe modifications
- **Proof-of-Space Consensus**: Plot-based mining with post-quantum signature verification
- **Configurable Proof Pruning**: Storage-efficient design with "museum mode" for archive nodes
- **Native Token Platform**: Minting, melting, and trading with collateral requirements
- **AMM Liquidity Pools**: Constant product (x*y=k) decentralized exchange
- **Modern Architecture**: High-performance P2P networking and comprehensive REST APIs with optional authentication

Unlike existing post-quantum cryptographic research projects, Shadowy Apparatus is designed as a complete, production-ready blockchain system with practical DeFi applications.

## 2. Background and Motivation

### 2.1 The Quantum Threat

Quantum computers capable of breaking current cryptographic systems are projected to emerge within 10-20 years. The National Institute of Standards and Technology (NIST) has responded by standardizing post-quantum cryptographic algorithms, including:

- **ML-KEM** (Module-Lattice-Based Key Encapsulation)  
- **ML-DSA** (Module-Lattice-Based Digital Signature Algorithm)
- **SLH-DSA** (Stateless Hash-Based Digital Signature Algorithm)

### 2.2 Blockchain Vulnerability

Existing blockchain systems face several quantum vulnerabilities:

1. **Transaction Signatures**: ECDSA signatures can be broken by quantum computers
2. **Address Generation**: Public key derivation from addresses becomes vulnerable
3. **Mining Algorithms**: Some hash functions may be weakened by quantum attacks
4. **Consensus Security**: Byzantine fault tolerance assumptions may not hold

### 2.3 Design Goals

Shadowy Apparatus was designed with the following objectives:

- **Quantum Resistance**: All cryptographic primitives must resist quantum attacks
- **Performance**: Transaction throughput comparable to modern blockchains
- **Decentralization**: No single points of failure or control
- **Practicality**: Real-world applications and developer accessibility
- **Future-Proofing**: Adaptable to evolving post-quantum standards

## 3. System Architecture

### 3.1 Core Components

Shadowy Apparatus consists of several interconnected subsystems:

```
┌─────────────────────────────────────────────────────────┐
│                     Application Layer                    │
├─────────────────────────────────────────────────────────┤
│  Web Wallet  │  CLI Tools  │  RPC APIs  │  Web3 SDK   │
├─────────────────────────────────────────────────────────┤
│                    Protocol Layer                       │
├─────────────────────────────────────────────────────────┤
│  Transaction  │  Consensus  │  P2P Network │  Mining   │
│   Processing  │   Engine    │   Protocol   │  System   │
├─────────────────────────────────────────────────────────┤
│                  Cryptographic Layer                    │
├─────────────────────────────────────────────────────────┤
│  ML-DSA-87   │   SHA-256   │  BLAKE3     │   VDF     │
│ Signatures   │   Hashing   │  Hashing    │  Functions │
├─────────────────────────────────────────────────────────┤
│                     Storage Layer                       │
├─────────────────────────────────────────────────────────┤
│  Block Store │ UTXO Index  │ State DB    │ Plot Files │
└─────────────────────────────────────────────────────────┘
```

### 3.2 Node Architecture

Each Shadowy node runs multiple concurrent services:

- **Blockchain Service**: Block validation, storage, and chain management with configurable proof pruning
- **Consensus Service**: P2P communication and block propagation via libp2p gossipsub
- **Proof-of-Space Service**: Plot-based mining and proof generation
- **Mempool Service**: Transaction validation, ordering, and double-spend prevention
- **Token Registry**: Token minting, melting, and metadata management
- **Pool Registry**: AMM liquidity pool operations and LP token management
- **HTTP API Services**: RESTful APIs with optional API key authentication

### 3.3 Network Topology

The network operates as a decentralized peer-to-peer overlay:

- **Full Nodes**: Store complete blockchain and validate all transactions (with configurable proof pruning)
- **Farming Nodes**: Additionally perform proof-of-space mining using plot files
- **Museum Nodes**: Archive nodes with proof pruning disabled (depth=0), storing all historical proofs
- **Standard Nodes**: Run with default 10,000 block proof retention (~15 GB steady state)

## 4. Cryptographic Foundations

### 4.1 ML-DSA-87 Digital Signatures

Shadowy Apparatus uses ML-DSA-87 (Dilithium Mode3) for all digital signatures:

**Parameters:**
- Security Level: NIST Level 3 (224-bit quantum security)
- Public Key Size: 1,952 bytes
- Signature Size: 4,627 bytes  
- Private Key Size: 4,016 bytes

**Advantages:**
- Standardized by NIST (FIPS 204)
- Mature implementation and security analysis
- Deterministic signatures (no randomness required)
- Fast verification suitable for blockchain applications

### 4.2 Hash Functions

The protocol employs multiple hash functions for different purposes:

- **SHA-256**: Block headers, transaction IDs, Merkle trees (quantum-resistant)
- **BLAKE3**: High-performance hashing for mining and general purposes
- **SHA-3**: Future compatibility and specific protocol components

### 4.3 Address Generation

Shadowy addresses are derived from ML-DSA-87 public keys:

```
Address = Base58(NetworkByte + BLAKE3(ML-DSA-PublicKey)[0:20] + Checksum)
```

**Address Types:**
- **S-addresses**: Standard addresses (51 characters)
- **L-addresses**: Legacy compatibility addresses (41 characters)

### 4.4 Key Derivation

The system implements a post-quantum equivalent of BIP-32 hierarchical deterministic keys:

```
ChildKey = ML-DSA-KeyGen(HMAC-SHA512(ParentKey, Index + Seed))
```

## 5. Consensus Mechanism

### 5.1 Proof-of-Space Mining

Shadowy Apparatus uses a proof-of-space consensus mechanism similar to Chia:

- **Plot Files**: Pre-generated storage proofs created offline by farmers
- **Challenge-Response**: Each block interval, a challenge hash is broadcast
- **Hamming Distance Competition**: Best proof (lowest distance to challenge) wins
- **Dual Signatures**: Both plot signature and miner signature required for validity

### 5.2 Mining Process

The mining process operates on a 60-second block interval:

```
1. Challenge Broadcast (t=0s)
   - Previous block hash becomes challenge
   - Propagated via consensus pubsub topic

2. Proof Generation (t=0-50s)
   - Farmers lookup challenge in plot files
   - plotlib returns best solution with hamming distance
   - Generate plot signature (proves plot ownership)
   - Generate miner signature (proves submission ownership)

3. Proof Competition (t=0-50s)
   - Proofs broadcast on proof pubsub topic
   - Nodes collect all submitted proofs
   - Track best proof (lowest distance)

4. Block Proposal (t=50s)
   - Node with best proof proposes block
   - Includes winning proof + transactions
   - Other nodes vote to accept

5. Block Finalization (t=60s)
   - Block added to chain if majority votes yes
   - New challenge begins
```

**Features:**
- 60-second block time (configurable via `BlockInterval`)
- 50-second proof window (configurable via `ProofWindow`)
- No energy waste (proofs pre-generated once)
- Decentralized (anyone can create plots)
- Storage-based (disk space = mining power)

### 5.3 Block Structure

```go
type Block struct {
    Index          uint64
    Timestamp      string
    PrevHash       string
    Transactions   []*Transaction
    CoinbaseReward uint64
    Coinbase       *Transaction
    WinningProof   *ProofOfSpace  // Can be nil if pruned
    Hash           string
    Votes          map[string]bool
}

type ProofOfSpace struct {
    ChallengeHash  [32]byte
    PlotHash       string  // Base85 encoded
    PlotPublicKey  string  // Base85 encoded plot key
    PlotSignature  string  // Base85 encoded plot signature
    Distance       uint64  // Hamming distance to challenge
    MinerPublicKey []byte  // ML-DSA-87 public key
    MinerSignature []byte  // ML-DSA-87 signature over plot proof
}
```

**Storage Breakdown (Empty Block):**
- Proof of Space: 63.9 KB (86% of block size!)
    - Plot signature (base85): ~5.7 KB
    - Plot public key (base85): ~3.2 KB
    - Miner signature (ML-DSA87): ~4.6 KB
    - Miner public key (ML-DSA87): ~2.0 KB
- Coinbase transaction: 9.9 KB (ML-DSA87 sig + pubkey)
- Overhead: ~300 bytes (hashes, votes, metadata)
- Each additional transaction ID: +66 bytes

**Total Block Size:** ~74 KB empty, scales with transaction count

### 5.4 Proof Pruning and Storage Management

Shadowy implements configurable proof pruning to manage storage growth:

**Configuration:**
- `--proof-pruning-depth N`: Keep proofs for last N blocks
- `--proof-pruning-depth 0`: Museum mode - keep all proofs forever
- Default: 10,000 blocks (~15 GB steady state)

**Storage Implications:**
- 60-second blocks: 525,600 blocks/year
- 74 KB per block: 107 GB/year without pruning
- With 10k block pruning: ~740 MB blockchain + ~740 MB proofs = ~1.5 GB total
- Museum nodes (depth=0): Full historical archive for researchers

**Pruning Process:**
- Automatic every 100 blocks
- Strips `WinningProof` field from old blocks
- Preserves all transaction data
- Database compression (ZSTD level 3) provides ~5% additional savings

### 5.5 Finality and Reorganizations

- **Probabilistic Finality**: 6 confirmations (≈6 minutes with 60s blocks)
- **Voting-Based Consensus**: Blocks require majority node votes to finalize
- **No Deep Reorgs**: Practical difficulty of regenerating better proofs for historical challenges

## 6. Transaction Model

### 6.1 UTXO Architecture

Shadowy follows a UTXO (Unspent Transaction Output) model similar to Bitcoin but adapted for post-quantum cryptography:

```go
type Transaction struct {
    Version    uint32
    Inputs     []TransactionInput
    Outputs    []TransactionOutput
    Locktime   uint32
    Timestamp  string
}

type TransactionInput struct {
    PreviousTxHash  [32]byte
    OutputIndex     uint32
    ScriptSig       []byte
    Sequence        uint32
}

type TransactionOutput struct {
    Value           uint64  // Satoshis
    ScriptPubkey    []byte
    Address         string
}
```

### 6.2 Transaction Types

Shadowy supports multiple transaction types:

- **SHADOW Transfers**: Native coin transfers with ML-DSA-87 signatures
- **Token Minting**: Create new tokens with SHADOW collateral requirement
- **Token Melting**: Burn tokens to reclaim SHADOW collateral
- **Token Transfers**: Send custom tokens between addresses
- **Pool Creation**: Create AMM liquidity pools for token pairs
- **Liquidity Operations**: Add/remove liquidity, receive LP tokens
- **Swaps**: Atomic token swaps through AMM pools

**Signature Verification:**
- All transactions require ML-DSA-87 signatures
- Inputs must reference valid unspent outputs
- Token operations validated against token registry
- Pool operations validated against pool registry

### 6.3 Transaction Fees

Dynamic fee market with the following characteristics:

- **Base Fee**: 0.011 SHADOW (11,000,000 satoshis)
- **Maximum Transaction Size**: 256 KB (prevents mempool bloat)
- **User-Specified Fees**: Senders specify fee amount
- **Priority System**: Higher fees receive priority inclusion
- **Mempool Management**: Maximum 100 transactions per block, size-based rejection

### 6.4 Transaction Validation

Each transaction undergoes comprehensive validation:

1. **Syntactic Validation**: Proper format and structure
2. **Size Validation**: Transaction size ≤ 256 KB
3. **Semantic Validation**: Valid input references and amounts
4. **Signature Verification**: ML-DSA-87 signature validation for all inputs
5. **Double-Spend Prevention**: UTXO consumption tracking with thread-safe caching
6. **Token Validation**: Token existence, collateral requirements, balance checks
7. **Pool Validation**: Pool existence, liquidity constraints, slippage limits

## 7. Network Protocol

### 7.1 P2P Communication

Shadowy nodes communicate using libp2p with gossipsub for message propagation:

```
┌─────────────────┐    ┌─────────────────┐
│   Application   │    │   Application   │
├─────────────────┤    ├─────────────────┤
│   GossipSub     │◄──►│   GossipSub     │
├─────────────────┤    ├─────────────────┤
│     libp2p      │◄──►│     libp2p      │
├─────────────────┤    ├─────────────────┤
│   TCP/QUIC      │◄──►│   TCP/QUIC      │
└─────────────────┘    └─────────────────┘
```

### 7.2 PubSub Topics

Shadowy uses multiple gossipsub topics for different message types:

- **shadowy-consensus**: Block proposals and voting
- **shadowy-proofs**: Proof-of-space submissions during mining
- **shadowy-mempool**: Transaction broadcasting and propagation

### 7.3 Peer Discovery

libp2p-based peer discovery:

1. **Bootstrap Nodes**: Configured seed peers for initial network join
2. **mDNS Discovery**: Local network peer discovery
3. **DHT Discovery**: Kademlia-based distributed peer routing
4. **Persistent Peers**: Configured always-connected peers

### 7.4 API Security

**HTTP API Authentication:**
- **Optional API Keys**: Protect sensitive endpoints with `X-API-Key` header
- **Environment Variable**: `SHADOWY_API_KEY` for key configuration
- **Protected Endpoints**: `/api/send`, `/api/token/*`, `/api/pool/*`, `/api/mempool/cancel`
- **Public Endpoints**: `/api/balance`, `/api/peers`, `/api/status`, `/api/pool/list`

**Rate Limiting:**
- Protection against spam and DoS attacks on API endpoints
- Mempool size limits (100 tx per block)

## 8. Security Analysis

### 8.1 Quantum Resistance

**Signature Security:**
- ML-DSA-87 provides 224-bit quantum security
- Resistant to Shor's algorithm and variants
- Based on well-studied lattice problems (Module-LWE)

**Hash Security:**
- SHA-256 provides 128-bit quantum security (Grover's algorithm)
- Sufficient for collision resistance and preimage attacks
- BLAKE3 offers similar quantum resistance with better performance

### 8.2 Classical Security

**Storage-Based Sybil Resistance:**
- Proof-of-space requires dedicated storage
- Creating plots is time/resource intensive
- Attack cost proportional to storage capacity

**Double-Spend Protection:**
- UTXO model prevents double-spending by design
- Thread-safe UTXO caching with sync.Map prevents race conditions
- Probabilistic finality after 6 confirmations (≈6 minutes)
- Mempool conflict detection rejects competing transactions

### 8.3 Network Security

**Gossipsub Security:**
- Message validation before propagation
- Peer scoring prevents spam
- Topic-based message filtering

**Eclipse Attack Prevention:**
- Multiple peer discovery mechanisms (bootstrap, mDNS, DHT)
- libp2p connection diversity
- Persistent peer configuration

### 8.4 Implementation Security

**Memory Safety:**
- Written in Go with automatic memory management
- No buffer overflows or memory corruption vulnerabilities
- Comprehensive input validation and sanitization

**Cryptographic Implementation:**
- Uses NIST-standard ML-DSA-87 reference implementation
- Constant-time algorithms prevent side-channel attacks
- Regular security audits and updates

## 9. Performance Evaluation

### 9.1 Transaction Throughput

**Baseline Performance:**
- Block Size: ~74 KB empty (proof dominates)
- Block Time: 60 seconds
- Block Capacity: Up to 100 transactions per block
- Transactions per Second: ~1.67 TPS baseline
- Transaction Size: ~10-15 KB average (ML-DSA-87 signatures are large)

**Storage Optimizations:**
- Proof pruning reduces long-term storage by 86%
- ZSTD compression provides 5% additional savings
- Thread-safe caching improves UTXO lookup performance

### 9.2 Signature Performance

**ML-DSA-87 Benchmarks (AMD Ryzen 9 5950X):**
- Key Generation: 0.12 ms
- Signing: 0.18 ms  
- Verification: 0.08 ms
- Batch Verification: 0.06 ms per signature

**Comparison to ECDSA:**
- ~2x slower signing
- ~3x slower verification
- ~10x larger signatures
- Quantum-resistant security

### 9.3 Network Performance

**libp2p/GossipSub Efficiency:**
- Block Propagation: Fast gossipsub distribution
- Transaction Relay: Sub-second mempool propagation
- Proof Competition: 50-second window for proof collection
- Peer Discovery: Automatic via mDNS and DHT

### 9.4 Storage Requirements

**Storage Breakdown (Standard Node with Pruning):**
- Blockchain Data: ~740 MB (blocks + transactions)
- UTXO Set: Variable based on usage
- Last 10,000 Proofs: ~740 MB (64KB each)
- Database Overhead: ~100 MB
- Total: ~1.5 GB steady state

**Museum Node (No Pruning):**
- Year 1: ~107 GB (525,600 blocks × 74 KB)
- Linear growth with block production
- Suitable for archive/research purposes

**Growth Projections:**
- Standard nodes: ~1.5 GB stable (pruning enabled)
- Museum nodes: ~107 GB/year
- UTXO set grows with adoption

## 10. Economic Model

### 10.1 SHADOW Token Economics

**SHADOW Token:**
- Native blockchain token
- Smallest Unit: 1 satoshi = 0.00000001 SHADOW
- Total Supply Cap: 21,000,000 SHADOW (fixed maximum)
- Usage: Transaction fees, token collateral, liquidity provision, block rewards

**Block Reward Schedule (Bitcoin-style):**
- Initial Reward: 50 SHADOW per block
- Halving Interval: Every 210,000 blocks
- Halving Effect: Reward divided by 2 each halving
- Final Supply: ~21 million SHADOW (last satoshi mined ~block 13,440,000)

**Halving Timeline:**
With 60-second blocks, each halving period takes ~146 days:
- Blocks 0-209,999: 50 SHADOW/block (Era 1: ~146 days)
- Blocks 210,000-419,999: 25 SHADOW/block (Era 2: ~146 days)
- Blocks 420,000-629,999: 12.5 SHADOW/block (Era 3: ~146 days)
- Continues halving until reward reaches 0 (after 64 halvings)

**Total Supply Calculation:**
```
Sum = 210,000 × (50 + 25 + 12.5 + 6.25 + ...)
    = 210,000 × 100 (geometric series)
    = 21,000,000 SHADOW
```

### 10.2 Custom Token Platform

**Token Creation:**
- Any user can mint custom tokens via `/api/token/mint`
- **Collateral Requirement**: Exactly 1:1 with total token supply
- Example: Mint 1,000,000 token satoshis → lock 1,000,000 SHADOW satoshis
- Token metadata: name, ticker symbol, max supply, decimals
- Token ID: SHA-256 hash of genesis transaction

**Token Operations:**
- **Minting**: Create tokens by locking SHADOW collateral
- **Melting**: Burn tokens to reclaim SHADOW collateral
- **Transfers**: Send tokens like native SHADOW transfers
- **NFT Support**: Tokens with `max_decimals=0` function as NFTs

**Example Token:**
```json
{
  "name": "Example Token",
  "ticker": "EXT",
  "max_supply": 1000000000000000,
  "max_decimals": 8,
  "token_id": "abc123..."
}
```

### 10.3 AMM Liquidity Pools

**Constant Product Market Maker:**
Shadowy implements automated market makers using the constant product formula:

```
x * y = k

Where:
- x = reserve of token A
- y = reserve of token B
- k = constant product
```

**Pool Operations:**

1. **Create Pool** (`/api/pool/create`):
   - Specify token pair (tokenA, tokenB)
   - Provide initial liquidity for both tokens
   - Receive LP (Liquidity Provider) tokens
   - Pool ID: SHA-256 hash of creation transaction

2. **Add Liquidity** (`/api/pool/add`):
   - Deposit tokens at current pool ratio
   - Receive proportional LP tokens
   - LP tokens represent pool ownership share

3. **Remove Liquidity** (`/api/pool/remove`):
   - Burn LP tokens
   - Receive proportional share of both pool tokens
   - Includes any accumulated trading fees

4. **Swap** (`/api/pool/swap`):
   - Trade token A for token B (or vice versa)
   - Price determined by constant product formula
   - Slippage protection via `min_amount_out`

**LP Token Naming:**
LP tokens receive unique tickers incorporating pool ID hash to prevent collisions:
```
Ticker: LP-SHADOW/EXT-a1b2c3
```

**Fee Structure:**
- Swap fees accrue to liquidity providers
- Proportional distribution based on LP token holdings

### 10.4 Transaction Fee Market

**Fee Economics:**
- Base Fee: 0.011 SHADOW per transaction (11,000,000 satoshis)
- User-Specified: Senders can set custom fees
- Priority Queue: Higher fees get faster inclusion
- Size Limit: 256 KB maximum prevents abuse

**Fee Distribution:**
- All transaction fees collected in a block are added to the block reward
- Combined reward (block reward + fees) goes to farmer with winning proof
- Incentivizes farmers to include high-fee transactions

### 10.5 Mining Economics

**Proof-of-Space Rewards:**
- Block Reward: 50 SHADOW initially, halving every 210,000 blocks
- Transaction Fees: Added to block reward
- Mining Interval: 60 seconds average (525,600 blocks/year)
- Hardware Requirements: Storage space + plotting capability
- Energy Efficient: No continuous computation required

**Farming Accessibility:**
- Plot files created once, reused indefinitely
- No specialized hardware advantage (ASICs don't help)
- Decentralized: mining power proportional to storage committed
- Low barrier to entry: any available disk space can be used

**Reward Timeline:**
- Year 1: ~10.5 million SHADOW mined (50% of supply)
- Year 2: ~5.25 million SHADOW mined (25% of supply)
- Year 3: ~2.625 million SHADOW mined (12.5% of supply)
- Year 10+: Less than 1% of supply remaining to mine
- Scarcity drives value as supply issuance decreases

## 11. Ecosystem and Applications

### 11.1 HTTP API Endpoints

**Implemented REST APIs:**

**Transaction Operations:**
- `POST /api/send` - Send SHADOW or tokens (protected)
- `GET /api/balance/:address` - Check address balance (public)
- `POST /api/token/mint` - Create new tokens (protected)
- `POST /api/token/melt` - Burn tokens for collateral (protected)

**Liquidity Pool Operations:**
- `GET /api/pool/list` - List all pools (public)
- `POST /api/pool/create` - Create new pool (protected)
- `POST /api/pool/add` - Add liquidity (protected)
- `POST /api/pool/remove` - Remove liquidity (protected)
- `POST /api/pool/swap` - Swap tokens (protected)

**Network Operations:**
- `GET /api/status` - Node and chain status (public)
- `GET /api/peers` - Connected peers (public)
- `POST /api/mempool/cancel` - Cancel pending transaction (protected)

**Authentication:**
- Protected endpoints require `X-API-Key` header
- Configure via `SHADOWY_API_KEY` environment variable
- Optional security layer for production deployments

### 11.2 Current Applications

**Decentralized Finance (DeFi):**
- ✅ **Token Platform**: Create custom tokens with collateral
- ✅ **AMM DEX**: Automated market maker for token swaps
- ✅ **Liquidity Mining**: Provide liquidity, earn LP tokens
- ✅ **NFT Support**: Zero-decimal tokens serve as NFTs
- ❌ **Smart Contracts**: Not implemented (tokens and pools only)
- ❌ **Lending/Borrowing**: Not implemented

**Developer Tools:**
- ✅ Full node software with farming capabilities
- ✅ REST API for all operations
- ✅ JSON-based wallet format
- ❌ Web3 SDK: Not implemented
- ❌ Block explorer: Not implemented
- ❌ Testnet: Not implemented

## 12. Future Work

### 12.1 Protocol Upgrades

**Post-Quantum Enhancements:**
- Migration to newer NIST standards as they mature
- Hybrid classical/post-quantum schemes for transition periods
- Quantum key distribution integration for enhanced security
- Research into quantum-resistant consensus mechanisms

**Scalability Improvements:**
- Sharding implementation for horizontal scaling
- Layer 2 solutions (Lightning Network equivalent)
- Optimistic rollups and zk-rollups integration
- Cross-chain interoperability protocols

### 12.2 Smart Contract Platform (Future)

**Note:** Shadowy currently does NOT implement smart contracts. The token and pool systems are built-in blockchain features, not programmable contracts.

**Potential Future Virtual Machine:**
- Post-quantum secure smart contract execution
- WebAssembly-based contract runtime
- Formal verification tools for contract security
- Gas metering and resource management

**Potential Programming Languages:**
- Domain-specific language for quantum-safe contracts
- Rust and Go support for familiar development experience
- Formal specification languages for critical applications

### 12.3 Privacy Features

**Confidential Transactions:**
- Hiding transaction amounts while preserving auditability
- Post-quantum zero-knowledge proof integration
- Ring signatures for enhanced privacy
- Selective disclosure for regulatory compliance

**Anonymous Transactions:**
- Zcash-style shielded transactions with post-quantum security
- Mixing protocols and CoinJoin implementations
- Stealth addresses for recipient privacy
- Plausible deniability and metadata protection

### 12.4 Governance Model

**Off-Chain Governance (Bitcoin-Style):**

Shadowy adopts a conservative, off-chain governance approach proven successful by Bitcoin and Ethereum:

**Proposal Process:**
1. **Discussion**: Ideas discussed on GitHub, forums, Discord
2. **Specification**: Formal proposals written as SIPs (Shadowy Improvement Proposals)
3. **Review**: Community and developer review period
4. **Implementation**: Reference implementation in core software
5. **Adoption**: Node operators choose to upgrade (or not)

**Voting Mechanism:**
- **Nodes vote by running software**: Upgrade = support, don't upgrade = reject
- **No token-weighted voting**: Prevents plutocracy and governance attacks
- **Rough consensus required**: Not unanimous, but clear majority support
- **Contentious splits allowed**: If community truly divided, both chains can exist

**Why Off-Chain:**
- Proven track record (Bitcoin: 15+ years, no governance exploits)
- Prevents wealth-based control
- Reduces attack surface (no on-chain voting to manipulate)
- Flexibility to adapt governance itself
- Node operators have real skin in the game

**Governance Principles:**
1. **Security First**: Changes must not compromise post-quantum security
2. **Conservative Bias**: Prefer stability over rapid changes
3. **Backward Compatibility**: Soft forks preferred over hard forks when possible
4. **Open Development**: All proposals and discussion public
5. **Running Code**: Implementation before activation

**Core Development:**
- Open-source repository with public contribution
- Core maintainers review and merge changes
- Security-critical changes require extensive testing
- Community can fork and propose alternative implementations

**No On-Chain Components:**
- No governance tokens
- No voting contracts
- No automatic protocol changes
- No treasury controlled by votes

This approach prioritizes network stability and security over governance complexity. History shows that the most successful blockchains have the simplest governance.

## 13. Conclusion

Shadowy Apparatus represents a significant advancement in blockchain technology, providing a production-ready post-quantum blockchain protocol. By integrating ML-DSA-87 digital signatures, proof-of-space consensus, configurable proof pruning, and native DeFi features, the system offers quantum resistance with practical storage requirements.

Key contributions include:

1. **Complete Post-Quantum Implementation**: All cryptographic primitives are quantum-resistant (ML-DSA-87)
2. **Proof-of-Space Consensus**: Energy-efficient mining using plot files with dual-signature validation
3. **Storage-Efficient Design**: Configurable proof pruning enables ~1.5 GB steady state for standard nodes
4. **Native Token Platform**: Built-in token minting/melting with collateral requirements
5. **AMM Liquidity Pools**: Constant product market maker for decentralized exchange
6. **Practical DeFi**: NFT support via zero-decimal tokens, LP tokens with unique naming

The protocol addresses the urgent need for quantum-resistant blockchain infrastructure while maintaining decentralization and energy efficiency through proof-of-space. Unlike proof-of-work systems, farming requires no continuous energy consumption—plots are generated once and reused indefinitely.

**Current Status:**
Shadowy is operational with functional farming, token platform, and AMM pools. The system does NOT currently implement:
- Smart contracts or programmable VMs
- Lending/borrowing protocols
- Web3 SDKs or browser wallets
- Block explorers or testnets

Future work will focus on scalability enhancements, smart contract capabilities, and privacy features while maintaining the core principles of quantum resistance and storage efficiency.

The quantum threat to classical cryptography is not hypothetical—it is an engineering challenge that requires immediate attention. Shadowy Apparatus demonstrates that post-quantum blockchain systems with practical DeFi features are achievable today.

---

## Appendix A: Technical Specifications

### A.1 Cryptographic Parameters

```
ML-DSA-87 Parameters:
- Security Level: NIST Level 3
- q (modulus): 8380417  
- n (dimension): 256
- k (rows): 6
- l (columns): 5
- η (secret key bound): 4
- τ (signature bound): 60
- β (challenge weight): 196
```

### A.2 Protocol Constants

```
Network Parameters:
- Block Time: 60 seconds
- Proof Window: 50 seconds
- Max Transactions per Block: 100
- Max Transaction Size: 256 KB
- Block Size: ~74 KB empty + transaction data

Proof Pruning:
- Default Retention: 10,000 blocks
- Museum Mode: 0 (keep all proofs)
- Pruning Frequency: Every 100 blocks
- Configurable: --proof-pruning-depth flag

Economic Parameters:
- Initial Block Reward: 5,000,000,000 satoshis (50 SHADOW)
- Halving Interval: 210,000 blocks (~146 days)
- Total Supply Cap: 21,000,000 SHADOW
- Satoshis per SHADOW: 100,000,000
- Base Transaction Fee: 11,000,000 satoshis (0.011 SHADOW)
- Token Collateral: 1:1 with token supply (satoshi-for-satoshi)
```

### A.3 Address Formats

```
S-Address Format:
- Length: 51 characters
- Pattern: ^S[0-9a-fA-F]{50}$
- Example: S427a724d41e3a5a03d1f83553134239813272bc2c4b2d50737

L-Address Format:
- Length: 41 characters  
- Pattern: ^L[0-9a-fA-F]{40}$
- Example: L1234567890abcdef1234567890abcdef12345678
```

## Appendix B: Performance Benchmarks

### B.1 Signature Benchmarks

| Operation | ML-DSA-87 | ECDSA P-256 | Ratio |
|-----------|-----------|-------------|-------|
| KeyGen | 0.12 ms | 0.08 ms | 1.5x |
| Sign | 0.18 ms | 0.06 ms | 3x |
| Verify | 0.08 ms | 0.12 ms | 0.7x |
| Sig Size | 4,627 bytes | 64 bytes | 72x |
| PK Size | 1,952 bytes | 33 bytes | 59x |

### B.2 Transaction Processing

| Metric | Value |
|--------|-------|
| Transactions per Block | Max 100 |
| Block Validation Time | Variable (depends on tx count) |
| Transaction Size | ~10-15 KB (ML-DSA-87 signatures) |
| UTXO Lookup | Thread-safe sync.Map caching |
| Mempool Size Limit | 256 KB per transaction |
| Block Interval | 60 seconds |

## Appendix C: Security Analysis

### C.1 Attack Vectors and Mitigations

| Attack | Mitigation | Status |
|--------|------------|--------|
| Quantum Computing | ML-DSA-87 signatures | ✅ Implemented |
| Storage Monopoly | Proof-of-space fairness | ✅ Implemented |
| Eclipse Attack | libp2p diversity + DHT | ✅ Implemented |
| Sybil Attack | Plot cost + storage req | ✅ Implemented |
| Double Spending | UTXO + sync.Map cache | ✅ Implemented |
| Race Conditions | Thread-safe caching | ✅ Implemented |
| Mempool Bloat | 256 KB tx size limit | ✅ Implemented |

### C.2 Cryptographic Security Levels

| Component | Classical Security | Quantum Security |
|-----------|-------------------|------------------|
| ML-DSA-87 | 256-bit | 224-bit |
| SHA-256 | 256-bit | 128-bit |
| BLAKE3 | 256-bit | 128-bit |
| Address Hash | 160-bit | 80-bit |

## References

1. NIST Post-Quantum Cryptography Standards (2024) - ML-DSA (FIPS 204)
2. Dilithium Digital Signature Algorithm Specification v3.1
3. Bitcoin: A Peer-to-Peer Electronic Cash System - S. Nakamoto
4. Chia Network: Proof of Space and Time - B. Cohen
5. Post-Quantum Cryptography - D.J. Bernstein et al.
6. libp2p: A Modular Network Stack - Protocol Labs
7. Uniswap v2 Core - Constant Product Market Maker
8. BadgerDB: Fast Key-Value Store in Go

---

**Authors:**  
The Shadowy Apparatus Development Team

**Contact:**  
[GitHub Repository](https://github.com/shadowyapparatus)

**License:**  
This whitepaper is released under Creative Commons Attribution 4.0 International License

**Disclaimer:**  
This is a technical document describing the Shadowy Apparatus blockchain protocol. It is not investment advice. Cryptocurrency investments carry significant risk.
