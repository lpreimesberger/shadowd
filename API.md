# Shadowy Blockchain HTTP API Documentation

This document describes the HTTP API endpoints available for interacting with the Shadowy post-quantum blockchain.

## Base URL

When running a node with `-node` flag, the HTTP server starts on an automatically selected port:
```
http://localhost:[PORT]
```

The port is displayed when the node starts. You can also check `/api/status` to see the current configuration.

## Authentication

Currently, no authentication is required for API endpoints.

---

## Node Information

### Get Node Status
Returns comprehensive information about the node's current state.

**Endpoint:** `GET /api/status`

**Response:**
```json
{
  "node_address": "tcp://127.0.0.1:26667",
  "node_id": "c1664df26a9bdf2e5b14f88ebacd6e12e42a761e",
  "seed_connect_string": "c1664df26a9bdf2e5b14f88ebacd6e12e42a761e@127.0.0.1:26666",
  "wallet_info": {
    "address": "a8b033b8fde716ee...",
    "public_key": "...",
    "created_at": "2025-09-29T16:39:30Z"
  },
  "token_count": 1,
  "genesis_token": {
    "token_id": "SHADOW",
    "name": "Shadow Token",
    "symbol": "SHADOW",
    "decimals": 8
  },
  "plot_count": 0,
  "farming_enabled": true,
  "configuration": "blockchain_dir: ./blockchain, seeds: [], dirs: [/tmp/farming/test-plots]",
  "seeds": [],
  "directories": ["/tmp/farming/test-plots"],
  "http_server_addr": "http://localhost:8080",
  "uptime": "5m30s"
}
```

### Health Check
Simple health check endpoint.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "ok"
}
```

---

## Wallet Information

### Get Wallet Info
Returns information about the node's wallet.

**Endpoint:** `GET /api/wallet/info`

**Response:**
```json
{
  "address": "a8b033b8fde716ee1234567890abcdef12345678",
  "public_key": "034f0764fb8cc5c2d7b5c8f5e3a8b7c6d9e2f1a0...",
  "created_at": "2025-09-29T16:39:30Z"
}
```

### Get Wallet Balance (Legacy)
Legacy endpoint that returns placeholder balance information.

**Endpoint:** `GET /api/wallet/balance`

**Response:**
```json
{
  "balance": 0,
  "confirmed": 0,
  "pending": 0
}
```

---

## UTXO and Balance Queries

### Get Address Balance
Returns the current balance for any address by querying the UTXO set.

**Endpoint:** `GET /api/balance`

**Query Parameters:**
- `address` (optional): The address to query. If not provided, uses the node's wallet address.

**Example:**
```bash
curl "http://localhost:8080/api/balance?address=a8b033b8fde716ee1234567890abcdef12345678"
```

**Response:**
```json
{
  "address": "a8b033b8fde716ee1234567890abcdef12345678",
  "balances": [
    {
      "token_id": "a1b2c3d4e5f6...",
      "name": "Shadow",
      "balance": 5000000000
    },
    {
      "token_id": "f6e5d4c3b2a1...",
      "name": "Custom Token",
      "balance": 1000000
    }
  ]
}
```

**Response Fields:**
- `address`: The queried address
- `balances`: Array of token balances
  - `token_id`: The unique hash identifier for the token
  - `name`: Token name ("Shadow" for SHADOW base currency, or custom name for minted tokens)
  - `balance`: Amount in smallest units (8 decimal places)

**Balance Format:**
- All amounts are in the smallest unit (8 decimal places for SHADOW)
- `5000000000` = 50.00000000 SHADOW tokens

### Get Address UTXOs
Returns all unspent transaction outputs (UTXOs) for an address.

**Endpoint:** `GET /api/utxos`

**Query Parameters:**
- `address` (optional): The address to query. If not provided, uses the node's wallet address.

**Example:**
```bash
curl "http://localhost:8080/api/utxos?address=a8b033b8fde716ee1234567890abcdef12345678"
```

**Response:**
```json
{
  "address": "a8b033b8fde716ee1234567890abcdef12345678",
  "utxos": [
    {
      "tx_id": "abc123def456...",
      "output_index": 0,
      "amount": 5000000000,
      "token_id": "SHADOW",
      "address": "a8b033b8fde716ee1234567890abcdef12345678",
      "block_height": 42,
      "is_spent": false
    }
  ],
  "count": 1
}
```

---

## Transaction Operations

### Send Transaction
Creates, signs, and submits a simple send transaction.

**Endpoint:** `POST /api/transactions/send`

**Request Body:**
```json
{
  "to_address": "b9c144c9fed827ff2345678901bcdef123456789",
  "amount": 1000000000,
  "fee": 1000
}
```

**Parameters:**
- `to_address` (required): Recipient address
- `amount` (required): Amount to send in smallest units
- `fee` (optional): Transaction fee in smallest units. Default: 1000

**Example:**
```bash
curl -X POST http://localhost:8080/api/transactions/send \
  -H "Content-Type: application/json" \
  -d '{
    "to_address": "b9c144c9fed827ff2345678901bcdef123456789",
    "amount": 1000000000,
    "fee": 1000
  }'
```

**Response:**
```json
{
  "transaction_id": "def789abc123...",
  "status": "submitted",
  "message": "Transaction submitted to mempool"
}
```

**Error Response:**
```json
{
  "error": "insufficient funds: have 0, need 1000001000"
}
```

### Submit Raw Transaction
Submits a pre-signed transaction to the mempool.

**Endpoint:** `POST /api/transactions/submit`

**Request Body:**
```json
{
  "transaction": {
    "tx_type": 1,
    "version": 1,
    "timestamp": 1727632770,
    "lock_time": 0,
    "token_id": "SHADOW",
    "inputs": [
      {
        "prev_tx_id": "abc123...",
        "output_index": 0,
        "script_sig": "...",
        "sequence": 4294967295
      }
    ],
    "outputs": [
      {
        "amount": 1000000000,
        "address": "b9c144c9fed827ff...",
        "token_id": "SHADOW",
        "token_type": "native",
        "script_pub_key": "..."
      }
    ],
    "signatures": ["..."],
    "data": null
  }
}
```

**Response:**
```json
{
  "transaction_id": "def789abc123...",
  "status": "submitted",
  "message": "Transaction submitted to mempool"
}
```

---

## Token Information

### Get Token Registry
Returns information about registered tokens.

**Endpoint:** `GET /api/tokens`

**Response:**
```json
{
  "count": 1,
  "genesis_token": {
    "token_id": "SHADOW",
    "name": "Shadow Token",
    "symbol": "SHADOW",
    "decimals": 8,
    "total_supply": "21000000.00000000",
    "description": "The native token of the Shadowy post-quantum blockchain"
  }
}
```

---

## Transaction Types

The blockchain supports several transaction types:

### Transaction Type Constants
- `0` - **Coinbase**: Mining rewards (block generation)
- `1` - **Send**: Transfer tokens between addresses
- `2` - **Mint Token**: Create new custom tokens
- `3` - **Melt**: Destroy/burn tokens

### Amount Format
All amounts use 8 decimal places:
- `100000000` = 1.00000000 tokens
- `1000000` = 0.01000000 tokens
- `1000` = 0.00001000 tokens

---

## Error Codes

### HTTP Status Codes
- `200` - Success
- `400` - Bad Request (invalid parameters)
- `405` - Method Not Allowed
- `500` - Internal Server Error
- `503` - Service Unavailable (node not ready)

### Common Error Messages
- `"Invalid address: ..."` - Malformed address format
- `"insufficient funds: have X, need Y"` - Not enough balance
- `"UTXO validation failed: ..."` - Transaction references non-existent or spent UTXOs
- `"Node not initialized"` - Tendermint node not ready
- `"Transaction validation failed: ..."` - Invalid transaction structure

---

## Usage Examples

### Check Your Balance
```bash
curl http://localhost:8080/api/balance
```

### Send 1 SHADOW Token
```bash
curl -X POST http://localhost:8080/api/transactions/send \
  -H "Content-Type: application/json" \
  -d '{
    "to_address": "b9c144c9fed827ff2345678901bcdef123456789",
    "amount": 100000000
  }'
```

### Check Transaction Pool Status
```bash
curl http://localhost:8080/api/status | jq '.wallet_info'
```

### Monitor UTXOs
```bash
curl http://localhost:8080/api/utxos | jq '.count'
```

---

## Integration Notes

1. **Mining Rewards**: Nodes automatically receive coinbase transactions when they successfully mine blocks
2. **Mempool**: Transactions are validated against the current UTXO set before entering the mempool
3. **Persistence**: All UTXO state is persisted to LevelDB for crash recovery
4. **P2P**: Transactions broadcast across the network through Tendermint's P2P layer
5. **Consensus**: All state changes require consensus through the ABCI interface

---

## Development Tools

### Using with curl
```bash
# Set your node's port
export NODE_PORT=8080

# Check status
curl http://localhost:$NODE_PORT/api/status | jq

# Check balance
curl http://localhost:$NODE_PORT/api/balance | jq

# Send transaction
curl -X POST http://localhost:$NODE_PORT/api/transactions/send \
  -H "Content-Type: application/json" \
  -d '{"to_address":"RECIPIENT_ADDRESS", "amount":100000000}' | jq
```

### Using with httpie
```bash
# Check balance
http GET localhost:8080/api/balance

# Send transaction
http POST localhost:8080/api/transactions/send \
  to_address=RECIPIENT_ADDRESS \
  amount:=100000000
```