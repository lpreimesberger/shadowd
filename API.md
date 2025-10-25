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

## Address Format and Validation

Shadowy uses a robust address format with multiple layers of validation to prevent errors and improve usability.

### Address Structure

Addresses consist of 66 characters with the following format:
```
[TYPE_PREFIX][64_HEX_CHARS][LUHN_CHECKSUM]
```

**Components:**
1. **Type Prefix** (1 char): Indicates address type
   - `S` - Standard wallet addresses (most common)
   - `L` - Liquidity pool addresses
   - `X` - Exchange/swap addresses
   - `N` - NFT/special purpose addresses

2. **Hex Portion** (64 chars): 32-byte address hash encoded as hexadecimal
   - Derived from BLAKE2b-256 hash of ML-DSA-87 public key
   - Uses EIP-55 style mixed-case checksum (optional but recommended)

3. **Luhn Checksum** (1 char): Final validation character
   - Calculated on normalized (lowercase) address
   - Mandatory for all addresses

### Example Valid Addresses

```
SBBB69aaa4a1537de49FEFde036b2f55809d2B868797B2fD20E7811203E233D3d4
│└────────────────────────────────────────────────────────────────┘│
│                    64 hex chars (with EIP-55 checksum)             │
│                                                                   Luhn
Type (S = Standard wallet)
```

### Validation Rules

Clients should validate addresses **before** submitting to the API to avoid rejected transactions:

1. **Length Check**: Must be exactly 66 characters
2. **Type Prefix**: First character must be `S`, `L`, `X`, or `N`
3. **Hex Validation**: Characters 2-65 must be valid hexadecimal (0-9, a-f, A-F)
4. **Luhn Checksum**: Last character must match calculated Luhn checksum
5. **EIP-55 Checksum** (optional): If mixed case is used, it must be correct

### Client-Side Validation

**JavaScript Example:**
```javascript
function validateAddress(addr) {
  // Length check
  if (addr.length !== 66) return false;

  // Type prefix check
  const validTypes = ['S', 'L', 'X', 'N'];
  if (!validTypes.includes(addr[0])) return false;

  // Hex portion check
  const hexPortion = addr.substring(1, 65);
  if (!/^[0-9a-fA-F]{64}$/.test(hexPortion)) return false;

  // Luhn checksum validation (implement calculateLuhn function)
  const normalized = addr.substring(0, 65).toLowerCase();
  const expectedLuhn = calculateLuhn(normalized);
  if (addr[65] !== expectedLuhn) return false;

  return true;
}
```

**Python Example:**
```python
def validate_address(addr: str) -> bool:
    # Length check
    if len(addr) != 66:
        return False

    # Type prefix check
    if addr[0] not in ['S', 'L', 'X', 'N']:
        return False

    # Hex portion check
    hex_portion = addr[1:65]
    try:
        int(hex_portion, 16)
    except ValueError:
        return False

    # Luhn checksum validation
    normalized = addr[:65].lower()
    expected_luhn = calculate_luhn(normalized)
    return addr[65] == expected_luhn
```

### Common Validation Errors

- `"address too short"` - Address must be exactly 66 characters
- `"invalid address type prefix"` - First character must be S, L, X, or N
- `"invalid Luhn checksum"` - Final checksum character is incorrect
- `"invalid EIP-55 checksum"` - Mixed case is used but incorrect
- `"address must be 32 bytes"` - Hex portion doesn't decode to 32 bytes

### Best Practices

1. **Always validate addresses client-side** before API submission
2. **Display addresses with mixed case** to enable EIP-55 validation
3. **Use copy-paste** for addresses to avoid transcription errors
4. **Implement QR codes** for mobile/cross-device address sharing
5. **Show clear error messages** indicating which validation failed

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
    "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
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
  "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
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

## Block Explorer APIs

### Get Recent Blocks (Paginated)
Returns a paginated list of recent blocks with summary information.

**Endpoint:** `GET /api/blocks`

**Query Parameters:**
- `limit` (optional): Number of blocks to return. Default: 20. Maximum: 100.
- `offset` (optional): Number of blocks to skip. Default: 0.

**Example:**
```bash
# Get latest 20 blocks
curl http://localhost:8080/api/blocks

# Get next 20 blocks
curl "http://localhost:8080/api/blocks?limit=20&offset=20"

# Get 50 blocks
curl "http://localhost:8080/api/blocks?limit=50"
```

**Response:**
```json
{
  "height": 4350,
  "blocks": [
    {
      "index": 4349,
      "hash": "abc123def456...",
      "prev_hash": "def789abc123...",
      "timestamp": "2025-10-24T12:34:56Z",
      "tx_count": 15,
      "reward": 5000000000,
      "has_proof": true,
      "proof_distance": 42
    },
    {
      "index": 4348,
      "hash": "def789abc123...",
      "prev_hash": "ghi012jkl345...",
      "timestamp": "2025-10-24T12:33:56Z",
      "tx_count": 8,
      "reward": 5000000000,
      "has_proof": false
    }
  ],
  "limit": 20,
  "offset": 0,
  "count": 20
}
```

**Response Fields:**
- `height`: Current blockchain height
- `blocks`: Array of block summaries (newest first)
  - `index`: Block height/index
  - `hash`: Block hash
  - `prev_hash`: Previous block hash
  - `timestamp`: Block timestamp
  - `tx_count`: Number of transactions in block
  - `reward`: Coinbase reward amount
  - `has_proof`: Whether block has proof-of-space (may be pruned)
  - `proof_distance`: Hamming distance (if proof present)
- `limit`: Requested limit
- `offset`: Requested offset
- `count`: Number of blocks returned

**Notes:**
- Blocks are returned in reverse chronological order (newest first)
- This endpoint provides summary information only - use `/api/chain/block/:index` for full block details
- Useful for building block list pages in explorers

### Get Block by Index
Returns full details of a specific block by its height/index.

**Endpoint:** `GET /api/chain/block/:index`

**Example:**
```bash
curl http://localhost:8080/api/chain/block/4344
```

**Response:**
```json
{
  "index": 4344,
  "timestamp": "2025-10-24T12:30:00Z",
  "prev_hash": "abc123...",
  "transactions": [
    "tx_hash_1",
    "tx_hash_2",
    "tx_hash_3"
  ],
  "coinbase_reward": 5000000000,
  "coinbase": {
    "tx_type": 0,
    "version": 1,
    "outputs": [{
      "address": "S...",
      "amount": 5000000000,
      "token_id": "SHADOW"
    }]
  },
  "winning_proof": {
    "challenge_hash": "...",
    "plot_hash": "...",
    "plot_public_key": "...",
    "plot_signature": "...",
    "distance": 42,
    "miner_public_key": "...",
    "miner_signature": "..."
  },
  "hash": "def456...",
  "votes": {
    "node_id_1": true,
    "node_id_2": true
  }
}
```

**Notes:**
- Returns complete block structure including all transactions and proof
- `winning_proof` may be `null` if block was pruned (check with `/api/status` for pruning depth)
- `transactions` array contains transaction hashes - use `/api/transaction/:hash` to get details

### Get Block by Hash
Returns full details of a specific block by its hash.

**Endpoint:** `GET /api/block/hash/:hash`

**Example:**
```bash
curl http://localhost:8080/api/block/hash/abc123def456...
```

**Response:**
Same as "Get Block by Index" above.

**Notes:**
- Searches through blockchain to find block with matching hash
- Returns 404 if hash not found
- Slower than lookup by index (requires linear search)

### Get Transaction Details
Returns comprehensive details about a specific transaction including confirmation status.

**Endpoint:** `GET /api/transaction/:hash`

**Example:**
```bash
curl http://localhost:8080/api/transaction/abc123def456...
```

**Response:**
```json
{
  "tx_hash": "abc123def456...",
  "tx_type": 1,
  "version": 1,
  "locktime": 0,
  "timestamp": "2025-10-24T12:30:00Z",
  "inputs": [
    {
      "prev_tx_id": "def789...",
      "output_index": 0,
      "script_sig": "...",
      "sequence": 4294967295
    }
  ],
  "outputs": [
    {
      "address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
      "amount": 1000000000,
      "token_id": "SHADOW",
      "token_type": "native",
      "script_pub_key": "..."
    }
  ],
  "confirmed": true,
  "block_height": 4344,
  "block_hash": "def456...",
  "block_timestamp": "2025-10-24T12:30:00Z",
  "confirmations": 6
}
```

**Response Fields (Confirmed Transaction):**
- `tx_hash`: Transaction hash/ID
- `tx_type`: Transaction type (see Transaction Types section)
- `version`: Transaction version
- `locktime`: Lock time (if any)
- `timestamp`: Transaction creation timestamp
- `inputs`: Array of transaction inputs
- `outputs`: Array of transaction outputs
- `confirmed`: `true` if included in a block
- `block_height`: Height of block containing transaction
- `block_hash`: Hash of block containing transaction
- `block_timestamp`: Timestamp of block
- `confirmations`: Number of confirmations (current_height - block_height)
- `data`: Additional data (for special transaction types)

**Response Fields (Unconfirmed Transaction):**
```json
{
  "tx_hash": "abc123def456...",
  "tx_type": 1,
  "version": 1,
  "locktime": 0,
  "timestamp": "2025-10-24T12:35:00Z",
  "inputs": [...],
  "outputs": [...],
  "confirmed": false,
  "in_mempool": true
}
```

**Notes:**
- Searches both confirmed blocks and mempool
- Returns 404 if transaction not found anywhere
- Use `confirmations` field to determine transaction finality (6+ confirmations recommended)
- Special transaction types (mint, melt, pool operations) include parsed `data` field

### Get Chain Info
Returns overall blockchain statistics.

**Endpoint:** `GET /api/chain`

**Example:**
```bash
curl http://localhost:8080/api/chain
```

**Response:**
```json
{
  "height": 4350,
  "blocks": [...]
}
```

**Notes:**
- Returns ALL blocks - use `/api/blocks` for paginated results
- Primarily for testing/debugging - not recommended for production explorers

### Get Current Height
Returns just the current blockchain height.

**Endpoint:** `GET /api/chain/height`

**Example:**
```bash
curl http://localhost:8080/api/chain/height
```

**Response:**
```json
{
  "height": 4350
}
```

---

## UTXO and Balance Queries

### Get Mempool Transactions
Returns all pending transactions currently in the mempool waiting to be included in a block.

**Endpoint:** `GET /api/mempool`

**Example:**
```bash
curl http://localhost:8080/api/mempool
```

**Response:**
```json
{
  "count": 2,
  "transactions": [
    {
      "tx_id": "abc123def456...",
      "tx_type": 1,
      "timestamp": 1727632800,
      "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
      "inputs": [
        {
          "prev_tx_id": "def789...",
          "output_index": 0,
          "sequence": 4294967295
        }
      ],
      "outputs": [
        {
          "address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
          "amount": 1000000000,
          "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
          "token_type": "native"
        }
      ],
      "memo": "Payment #123"
    }
  ]
}
```

**Response Fields:**
- `count`: Number of pending transactions
- `transactions`: Array of transaction objects
  - `tx_id`: Transaction hash
  - `tx_type`: Transaction type (0=Coinbase, 1=Send, 2=Mint, 3=Melt)
  - `timestamp`: Unix timestamp when transaction was created
  - `token_id`: Primary token being transferred
  - `inputs`: Array of UTXOs being spent
  - `outputs`: Array of new UTXOs being created
  - `memo`: Optional memo/tag if present

**Notes:**
- Transactions remain in mempool until included in a block
- Mempool has size limit (default 5000 transactions)
- Invalid transactions are rejected during CheckTx and won't appear here
- Transaction order may not reflect inclusion order in next block

### Get Address Balance
Returns the current balance for any address by querying the UTXO set.

**Endpoint:** `GET /api/balance`

**Query Parameters:**
- `address` (optional): The address to query. If not provided, uses the node's wallet address.

**Example:**
```bash
curl "http://localhost:8080/api/balance?address=SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a"
```

**Response:**
```json
{
  "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
  "balances": [
    {
      "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
      "name": "Shadow",
      "ticker": "SHADOW",
      "decimals": 8,
      "balance": 5000000000
    },
    {
      "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
      "name": "Custom Token",
      "ticker": "CUST",
      "decimals": 6,
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
  - `ticker`: Token ticker symbol (e.g., "SHADOW", "CUST")
  - `decimals`: Number of decimal places for display formatting
  - `balance`: Amount in smallest units (base units, not decimalized)

**Balance Format:**
- Amounts are always in the smallest base unit (atomic units)
- Use the `decimals` field to format for display
- Example with `decimals: 8`: `5000000000` base units = `50.00000000` tokens
- Example with `decimals: 6`: `1000000` base units = `1.000000` tokens

**Client-Side Formatting:**
```javascript
function formatBalance(balance, decimals) {
  return (balance / Math.pow(10, decimals)).toFixed(decimals);
}

// Example: formatBalance(5000000000, 8) => "50.00000000"
```

### Get Address Transactions
Returns paginated transaction history for an address.

**Endpoint:** `GET /api/transactions`

**Query Parameters:**
- `address` (optional): The address to query. If not provided, uses the node's wallet address.
- `count` (optional): Number of transactions to return. Default: 32. Must be > 0.
- `after` (optional): Transaction ID to paginate from. Returns transactions after this ID. If not provided, returns the latest transactions.

**Example:**
```bash
# Get latest 10 transactions
curl "http://localhost:8080/api/transactions?count=10"

# Get next page after a specific transaction
curl "http://localhost:8080/api/transactions?count=10&after=abc123def456..."

# Get transactions for a specific address
curl "http://localhost:8080/api/transactions?address=SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a&count=20"
```

**Response:**
```json
{
  "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
  "transactions": [
    {
      "tx_id": "abc123def456...",
      "tx_type": 0,
      "timestamp": 1727632770,
      "inputs": [],
      "outputs": [
        {
          "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
          "amount": 5000000000,
          "token_id": "a1b2c3d4e5f6...",
          "token_type": "native"
        }
      ]
    },
    {
      "tx_id": "def789abc123...",
      "tx_type": 1,
      "timestamp": 1727632800,
      "inputs": [
        {
          "prev_tx_id": "abc123def456...",
          "output_index": 0,
          "sequence": 4294967295
        }
      ],
      "outputs": [
        {
          "address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
          "amount": 1000000000,
          "token_id": "a1b2c3d4e5f6...",
          "token_type": "native"
        }
      ]
    }
  ],
  "count": 2
}
```

**Response Fields:**
- `address`: The queried address
- `transactions`: Array of transaction objects
  - `tx_id`: Unique transaction identifier
  - `tx_type`: Transaction type (0=Coinbase, 1=Send, 2=Mint, 3=Melt)
  - `timestamp`: Unix timestamp
  - `inputs`: Array of transaction inputs (empty for coinbase transactions)
  - `outputs`: Array of transaction outputs with addresses, amounts, and token info
- `count`: Number of transactions returned

**Pagination:**
To paginate through results, use the `tx_id` of the last transaction in the current page as the `after` parameter for the next request.

### Get Address UTXOs
Returns all unspent transaction outputs (UTXOs) for an address.

**Endpoint:** `GET /api/utxos`

**Query Parameters:**
- `address` (optional): The address to query. If not provided, uses the node's wallet address.

**Example:**
```bash
curl "http://localhost:8080/api/utxos?address=SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a"
```

**Response:**
```json
{
  "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
  "utxos": [
    {
      "tx_id": "abc123def456...",
      "output_index": 0,
      "amount": 5000000000,
      "token_id": "SHADOW",
      "address": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
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
  "to_address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
  "amount": 1000000000,
  "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
  "fee": 1000,
  "memo": "Payment for services"
}
```

**Parameters:**
- `to_address` (required): Recipient address
- `amount` (required): Amount to send in smallest units
- `token_id` (optional): Token identifier hash. Defaults to SHADOW base token if not provided or set to "SHADOW"
- `fee` (optional): Transaction fee in smallest units. Default: 1000
- `memo` (optional): ASCII-only memo/tag up to 64 bytes for transaction identification

**Examples:**
```bash
# Send SHADOW base currency
curl -X POST http://localhost:8080/api/transactions/send \
  -H "Content-Type: application/json" \
  -d '{
    "to_address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
    "amount": 1000000000,
    "fee": 1000
  }'

# Send custom token with memo
curl -X POST http://localhost:8080/api/transactions/send \
  -H "Content-Type: application/json" \
  -d '{
    "to_address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
    "amount": 500000000,
    "token_id": "f6e5d4c3b2a1...",
    "fee": 2000,
    "memo": "Invoice #12345"
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
        "address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
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

## Admin Endpoints (Testing Only)

### Shutdown Node
**⚠️ WARNING: This endpoint should be REMOVED before production deployment!**

Performs a graceful shutdown of the node. Useful for automated testing to avoid corrupting Tendermint data.

**Endpoint:** `GET /api/admin/shutdown` or `POST /api/admin/shutdown`

**Example:**
```bash
curl http://localhost:8080/api/admin/shutdown
```

**Response:**
```json
{
  "status": "shutting_down",
  "message": "Node is performing graceful shutdown"
}
```

**Shutdown Process:**
1. Sends response to client
2. Stops Tendermint node (flushes all data)
3. Stops HTTP server with 5s timeout
4. Closes shell
5. Waits for all goroutines to complete
6. Exits cleanly

**Security Note:** This endpoint allows anyone with network access to shut down the node. It exists only for testing automation and **MUST BE REMOVED** before any production or mainnet deployment.

---

## Token Information

### List All Tokens
Returns information about all registered tokens in the token registry.

**Endpoint:** `GET /api/tokens`

**Response:**
```json
{
  "count": 2,
  "tokens": [
    {
      "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626",
      "ticker": "SHADOW",
      "description": "Base token for Shadow Network",
      "max_mint": 21000000,
      "max_decimals": 8,
      "total_supply": 2100000000000000,
      "locked_shadow": 0,
      "total_melted": 0,
      "creator": "S00000000000000000000000000000000000000000000000000000000000000000",
      "is_shadow": true,
      "fully_melted": false
    },
    {
      "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
      "ticker": "MYTOKEN",
      "description": "My custom token",
      "max_mint": 1000000,
      "max_decimals": 6,
      "total_supply": 1000000000000,
      "locked_shadow": 1000000000000,
      "total_melted": 0,
      "creator": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
      "is_shadow": false,
      "fully_melted": false
    }
  ]
}
```

**Response Fields:**
- `count`: Number of registered tokens
- `tokens`: Array of token objects
  - `token_id`: Unique token identifier (hash of minting transaction)
  - `ticker`: 3-32 character ticker symbol (A-Z, a-z, 0-9 only)
  - `description`: Optional 0-64 character description
  - `max_mint`: Maximum base units (before decimals), max 21 million
  - `max_decimals`: Number of decimal places (0-8)
  - `total_supply`: Total token supply in smallest unit (max_mint × 10^max_decimals)
  - `locked_shadow`: SHADOW satoshis locked (1:1 with total_supply for custom tokens)
  - `total_melted`: Total tokens that have been melted/burned
  - `creator`: Address that created this token
  - `is_shadow`: Whether this is the base SHADOW token
  - `fully_melted`: Whether all tokens have been melted

### Get Token Info
Returns detailed information about a specific token.

**Endpoint:** `GET /api/token/info`

**Query Parameters:**
- `token_id` (required): The token identifier hash

**Example:**
```bash
curl "http://localhost:8080/api/token/info?token_id=f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6"
```

**Response:**
```json
{
  "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
  "ticker": "MYTOKEN",
  "description": "My custom token",
  "max_mint": 1000000,
  "max_decimals": 6,
  "total_supply": 1000000000000,
  "locked_shadow": 1000000000000,
  "total_melted": 0,
  "creator": "SA8b033b8fDe716eE1234567890aBcdEF12345678901234567890aBcdEf123456a",
  "creation_time": 1727632800,
  "is_shadow": false,
  "fully_melted": false,
  "supply_formatted": "1000000.000000 MYTOKEN"
}
```

**Error Response:**
```json
{
  "error": "token not found"
}
```

### Mint Token
Creates a new custom token by locking SHADOW as collateral.

**Endpoint:** `POST /api/token/mint`

**Request Body:**
```json
{
  "ticker": "MYTOKEN",
  "description": "My custom token",
  "max_mint": 1000000,
  "max_decimals": 6
}
```

**Parameters:**
- `ticker` (required): 3-32 character ticker symbol (A-Z, a-z, 0-9 only)
- `description` (optional): 0-64 character description (A-Z, a-z, 0-9 only)
- `max_mint` (required): Maximum base units (1 to 21,000,000)
- `max_decimals` (required): Number of decimal places (0-8)

**SHADOW Staking Requirement:**
Minting requires locking SHADOW at a 1:1 ratio with the total token supply:
- `locked_shadow = max_mint × 10^max_decimals`
- Example: `max_mint=1000000, max_decimals=6` requires `1000000000000` SHADOW satoshis locked

**Example:**
```bash
# Mint 1 million tokens with 6 decimals (requires 1,000,000.000000 SHADOW locked)
curl -X POST http://localhost:8080/api/token/mint \
  -H "Content-Type: application/json" \
  -d '{
    "ticker": "MYTOKEN",
    "description": "My custom token",
    "max_mint": 1000000,
    "max_decimals": 6
  }'
```

**Response:**
```json
{
  "tx_id": "abc123def456...",
  "token_id": "abc123def456...",
  "ticker": "MYTOKEN",
  "total_supply": 1000000000000,
  "locked_shadow": 1000000000000,
  "message": "Token minted successfully"
}
```

**Response Fields:**
- `tx_id`: Transaction ID of the minting transaction
- `token_id`: Token identifier (same as tx_id)
- `ticker`: Token ticker symbol
- `total_supply`: Total token supply in smallest units
- `locked_shadow`: Amount of SHADOW locked
- `message`: Success message

**Error Responses:**
```json
// Insufficient SHADOW
{
  "error": "insufficient SHADOW for staking: have 500000000000, need 1000000000000"
}

// Invalid ticker
{
  "error": "invalid token info: ticker must be 3-32 characters, got 2"
}

// Ticker already in use
{
  "error": "ticker MYTOKEN already in use by token abc123..."
}
```

**Important Notes:**
- The token_id is set to the transaction ID of the minting transaction
- SHADOW is locked in the UTXO set and cannot be spent until tokens are melted
- Ticker symbols must be unique across all active (non-fully-melted) tokens
- Once a token is fully melted, its ticker can be reused

### Melt Token
Destroys custom tokens and unlocks the proportional SHADOW collateral.

**Endpoint:** `POST /api/token/melt`

**Request Body:**
```json
{
  "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
  "amount": 500000000000
}
```

**Parameters:**
- `token_id` (required): The token identifier hash
- `amount` (required): Amount to melt in smallest units. Use `0` to melt all available tokens.

**SHADOW Unlock Calculation:**
Melting returns proportional SHADOW:
- `unlocked_shadow = (melted_amount / total_supply) × locked_shadow`
- Example: Melt 50% of tokens → unlock 50% of locked SHADOW

**Examples:**
```bash
# Melt specific amount
curl -X POST http://localhost:8080/api/token/melt \
  -H "Content-Type: application/json" \
  -d '{
    "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
    "amount": 500000000000
  }'

# Melt all tokens
curl -X POST http://localhost:8080/api/token/melt \
  -H "Content-Type: application/json" \
  -d '{
    "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
    "amount": 0
  }'
```

**Response:**
```json
{
  "tx_id": "def789abc123...",
  "token_id": "f6e5d4c3b2a1a9b8c7d6e5f4a3b2c1d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
  "melted_amount": 500000000000,
  "shadow_unlocked": 500000000000,
  "message": "Token melted successfully"
}
```

**Response Fields:**
- `tx_id`: Transaction ID of the melting transaction
- `token_id`: Token identifier
- `melted_amount`: Amount of tokens destroyed
- `shadow_unlocked`: Amount of SHADOW unlocked
- `message`: Success message

**Error Responses:**
```json
// No tokens to melt
{
  "error": "no token UTXOs found for address"
}

// Insufficient tokens
{
  "error": "insufficient token balance: have 100000000000, need 500000000000"
}

// Invalid token
{
  "error": "token not found"
}

// Cannot melt SHADOW
{
  "error": "cannot melt SHADOW base token"
}
```

**Important Notes:**
- You can only melt tokens you own
- Melting is irreversible - tokens are permanently destroyed
- SHADOW is unlocked proportionally to the amount melted
- Use `amount: 0` to melt all available tokens in one transaction
- When a token is fully melted (across all holders), its ticker can be reused

---

## Atomic Swaps

SHADOW supports trustless peer-to-peer token trading through atomic swap offers.

**See [SWAPS.md](SWAPS.md) for complete documentation.**

### Quick Reference

**Create Offer:**
```bash
POST /api/swap/offer
{
  "have_token_id": "abc...",
  "want_token_id": "def...",
  "have_amount": 1000,
  "want_amount": 5000,
  "expires_at_block": 125000  // optional
}
```

**Accept Offer:**
```bash
POST /api/swap/accept
{
  "offer_tx_id": "xyz..."
}
```

**Cancel Offer:**
```bash
POST /api/swap/cancel
{
  "offer_tx_id": "xyz..."
}
```

**List Active Offers:**
```bash
GET /api/swap/list
```

Returns all active offers (not accepted, not cancelled, not expired).

---

## Liquidity Pools

SHADOW supports AMM-style constant product liquidity pools for decentralized token trading.

### Create Pool

Create a new liquidity pool for a token pair:

```bash
POST /api/pool/create
Content-Type: application/json

{
  "token_a": "7abf97c6d93541347a766a339fde7fc175a77ef8953b33953f5c7dc022993132",
  "token_b": "d1b55d0b5a4fcf20380f39c24d67710239b09d2718e5bd65a6a8f2f37952c692",
  "amount_a": 1000000000,
  "amount_b": 5000000,
  "fee_percent": 30  // Optional: 30 = 0.3%, defaults to 30, range: 10-1000 (0.1%-10%)
}
```

**Response:**
```json
{
  "tx_id": "abc123...",
  "status": "pool_creation_submitted",
  "pool_id": "abc123..."
}
```

**Notes:**
- Pool ID is the transaction ID of the pool creation
- LP tokens are minted using formula: `sqrt(amount_a × amount_b)`, then adjusted to match `max_mint × 10^8` validation
- LP token ticker format: `{TOKEN_A}{TOKEN_B}LP` (alphanumeric only, e.g., "SHADOWBOOBSLP")
- LP token description format: `{TOKEN_A}{TOKEN_B}LiquidityPool`
- Locked tokens are held by the pool (no outputs created for them)
- LP tokens are sent to pool creator
- Only one pool per token pair allowed (checked at API level)

### List Pools

Get all active liquidity pools:

```bash
GET /api/pool/list
```

**Response:**
```json
{
  "pools": [
    {
      "pool_id": "abc123...",
      "token_a": "token_id_a",
      "token_a_ticker": "SHADOW",
      "token_b": "token_id_b",
      "token_b_ticker": "BOOBS",
      "reserve_a": 1000000000,
      "reserve_b": 5000000,
      "lp_token_id": "abc123...",
      "lp_token_ticker": "SHADOW-BOOBS LP",
      "lp_token_supply": 70710678,
      "fee_percent": 30,
      "k": 5000000000000000,
      "rate_a_to_b": 200.0,
      "rate_b_to_a": 0.005,
      "created_at": 12345
    }
  ],
  "count": 1
}
```

**Pool Info Fields:**
- `reserve_a`, `reserve_b`: Current token reserves in the pool
- `k`: Constant product (reserve_a × reserve_b)
- `rate_a_to_b`: Current exchange rate (how much A per 1 B)
- `rate_b_to_a`: Current exchange rate (how much B per 1 A)
- `fee_percent`: Trading fee in basis points (30 = 0.3%)

### Add Liquidity

Add liquidity to an existing pool:

```bash
POST /api/pool/add_liquidity
Content-Type: application/json

{
  "pool_id": "abc123...",
  "amount_a": 100000000,
  "amount_b": 500000,
  "min_lp_tokens": 7000000  // Slippage protection
}
```

**Response:**
```json
{
  "tx_id": "def456...",
  "status": "add_liquidity_submitted"
}
```

**Notes:**
- Amounts must maintain the pool ratio (within 1% tolerance)
- LP tokens minted = `min(amount_a/reserve_a, amount_b/reserve_b) × lp_supply`
- Transaction fails if LP tokens received < `min_lp_tokens`
- Locks token A and B, mints LP tokens to provider

### Remove Liquidity

Remove liquidity from a pool by burning LP tokens:

```bash
POST /api/pool/remove_liquidity
Content-Type: application/json

{
  "pool_id": "abc123...",
  "lp_tokens": 7000000,
  "min_amount_a": 95000000,  // Slippage protection
  "min_amount_b": 475000     // Slippage protection
}
```

**Response:**
```json
{
  "tx_id": "ghi789...",
  "status": "remove_liquidity_submitted"
}
```

**Notes:**
- Returns proportional amounts: `(lp_tokens / lp_supply) × reserve`
- Transaction fails if returned amounts < minimums
- Burns LP tokens, returns token A and B to provider

### Swap

Swap tokens through a liquidity pool:

```bash
POST /api/pool/swap
Content-Type: application/json

{
  "pool_id": "abc123...",
  "token_in": "token_id_a",
  "amount_in": 10000000,
  "min_amount_out": 49000  // Slippage protection
}
```

**Response:**
```json
{
  "tx_id": "jkl012...",
  "status": "swap_submitted"
}
```

**Notes:**
- Uses constant product AMM formula: `x × y = k`
- Output calculation: `amountOut = (amountIn × (10000 - fee) × reserveOut) / ((reserveIn × 10000) + (amountIn × (10000 - fee)))`
- Transaction fails if output < `min_amount_out`
- Fee is taken from input token
- Locks input token, returns output token

---

## Transaction Types

The blockchain supports several transaction types:

### Transaction Type Constants
- `0` - **Coinbase**: Mining rewards (block generation)
- `1` - **Send**: Transfer tokens between addresses
- `2` - **Mint Token**: Create new custom tokens
- `3` - **Melt**: Destroy/burn tokens
- `4` - **Register Validator**: Register validator wallet address
- `5` - **Offer**: Create atomic swap offer (locks tokens)
- `6` - **Accept Offer**: Execute atomic swap
- `7` - **Cancel Offer**: Cancel swap offer (reclaim tokens)
- `8` - **Create Pool**: Create liquidity pool (locks tokens, mints LP tokens)
- `9` - **Add Liquidity**: Add liquidity to pool (mints LP tokens)
- `10` - **Remove Liquidity**: Remove liquidity from pool (burns LP tokens)
- `11` - **Swap**: Swap tokens through liquidity pool

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
    "to_address": "SB9c144C9Fed827fF2345678901BcdEF12345678901234567890bCdEf123456b",
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