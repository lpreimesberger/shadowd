# Shadowy - Post-Quantum UTXO Blockchain

🌑 A post-quantum blockchain implementation using ML-DSA87 signatures and Tendermint consensus.

## Overview

Shadowy is a modern blockchain that combines:
- **Post-quantum cryptography** (ML-DSA87 signatures from FIPS 204)
- **UTXO transaction model** (Bitcoin-style with multiple transaction types)
- **Tendermint consensus** (Byzantine fault tolerant consensus)
- **Multi-token support** (Native SHADOW token + custom tokens)

## Key Features

### 🔐 Post-Quantum Security
- **ML-DSA87 signatures** (Module-Lattice-Based Digital Signature Algorithm)
- **FIPS 204 compliant** post-quantum cryptography
- **Future-proof** against quantum computer attacks

### 💰 UTXO Transaction Model
- **4 Transaction Types**: Coinbase, Send, Mint Token, Melt
- **Fee system** based on transaction type and size
- **Script foundation** for future smart contracts

### 🌐 Multi-Token Support
- **Native SHADOW token** with 8 decimal places (21M max supply)
- **Custom tokens** with SHAKE256 deterministic IDs
- **Token staking requirements** (economic security)
- **Token melting** (controlled destruction)

### ⚡ Tendermint Integration
- **BFT consensus** with instant finality
- **Genesis block** with embedded testnet configuration
- **P2P networking** with seed node support

## Address Derivation

```
Address = BLAKE2b-256(PublicKey)
```

- **Input**: ML-DSA87 public key (post-quantum)
- **Hash**: BLAKE2b-256 (32 bytes output)
- **Format**: 64-character hex string

This enables transaction validation without pre-storing public keys in the blockchain state.

## Project Structure

```
shadowy/
├── main.go              # Node entry point with CLI
├── lib/
│   ├── crypto.go        # ML-DSA87 cryptography & address derivation
│   ├── wallet.go        # Node wallet management (~/.sn/default.json)
│   ├── transaction.go   # UTXO transaction types & validation
│   ├── utxo.go         # UTXO data structures & fee calculation
│   ├── tokeninfo.go    # Token metadata with SHAKE256 IDs
│   ├── genesis.go      # Tendermint genesis block creation
│   └── cli.go          # Command line parsing & seed validation
├── docs/               # Documentation
└── testnet_genesis.json # Generated testnet genesis
```

## Quick Start

### Build
```bash
go build -o shadowy main.go
```

### Basic Usage
```bash
# Run with full output (creates shadow.json config file)
./shadowy

# Quiet mode (suppress verbose output)
./shadowy --quiet

# Connect to seed nodes
./shadowy --seeds=abc123...def456@192.168.1.100,fed987...cba654@node2.example.com:26657

# Configure plot/proof directories
./shadowy --dirs=./plots,./proofs,/mnt/storage/farming

# Run in node mode (HTTP server + interactive console)
./shadowy --node

# Combined options
./shadowy --node --quiet --seeds=abc123...@192.168.1.100 --dirs=./plots
```

### Configuration File
Shadowy automatically creates `shadow.json` in the current directory:
```json
{
  "quiet": false,
  "seeds": [],
  "dirs": ["./plots"],
  "node_mode": false
}
```

### Help
```bash
./shadowy --help
```

## CLI Reference

### Flags
- `--quiet` - Suppress verbose output (especially Tendermint debug info)
- `--seeds` - Comma-delimited list of seed nodes
- `--dirs` - Comma-delimited list of directories for plot/proof files
- `--node` - Run in node mode (HTTP server + interactive console)

### Seed Node Format
```
nodeid@ip_address[:port]
```
- **nodeid**: 40 hex characters (Tendermint node ID)
- **ip_address**: IPv4, IPv6, or hostname
- **port**: Optional, defaults to 26656

### Examples
```bash
# Valid seed formats
abc123...def456@192.168.1.100         # IPv4
abc123...def456@node.example.com:26657 # Hostname with port
abc123...def456@localhost              # Hostname default port
```

## Transaction Types

### 1. Coinbase (Type 0)
- **Purpose**: Mining rewards / block creation
- **Inputs**: None (money creation)
- **Outputs**: SHADOW tokens to miner
- **Fee**: None

### 2. Send (Type 1)
- **Purpose**: Transfer SHADOW tokens
- **Inputs**: UTXOs being spent
- **Outputs**: Recipients + change
- **Fee**: Base + input/output counts

### 3. Mint Token (Type 2)
- **Purpose**: Create custom tokens
- **Inputs**: Optional staking inputs
- **Outputs**: New token outputs
- **Fee**: Higher (token creation cost)

### 4. Melt (Type 3)
- **Purpose**: Destroy tokens/assets
- **Inputs**: Tokens to destroy
- **Outputs**: Optional change/refund
- **Fee**: Lower (cleanup operation)

## Token System

### Genesis Token (SHADOW)
```json
{
  "name": "Shadow",
  "ticker": "SHADOW",
  "total_supply": 2100000000000000,
  "decimals": 8,
  "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626"
}
```

### Custom Tokens
- **Token ID**: SHAKE256 hash of token metadata
- **Staking Required**: Economic security mechanism
- **Melt Value**: Redemption value in SHADOW tokens

## Wallet Management

### Default Location
```
~/.sn/default.json
```

### Auto-Creation
- Creates new wallet if missing
- Generates ML-DSA87 key pair
- Saves encrypted to disk
- Loads existing wallet on restart

## Genesis Block

### Testnet Configuration
- **Chain ID**: `shadowy-testnet-1`
- **Consensus**: Tendermint with ML-DSA87 validators
- **Pre-mine**: 10,000 SHADOW to genesis validator
- **Block Time**: 1 second minimum
- **Block Size**: 1MB maximum

### Embedded Genesis
Access via `lib.GetEmbeddedTestnetGenesis()` for easy testnet deployment.

## Development Status

✅ **Completed Features:**
- ML-DSA87 post-quantum signatures
- UTXO transaction model with 4 types
- Multi-token support with economic staking
- Tendermint genesis block creation
- Command line interface with seed support
- Persistent wallet management

🚧 **Next Steps:**
- Tendermint ABCI integration
- P2P networking implementation
- Transaction mempool
- Block validation and mining
- REST API for blockchain queries

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]