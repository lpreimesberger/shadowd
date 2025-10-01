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