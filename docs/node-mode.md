# Node Mode

## Overview

Node mode (`--node`) starts Shadowy as a full daemon with:
- **HTTP Server** for wallet operations and API access
- **Interactive Console** with readline support
- **Background Services** (future: Tendermint integration)

## Starting Node Mode

```bash
# Basic node mode
./shadowy --node

# Node mode with custom configuration
./shadowy --node --quiet --seeds=abc123...@192.168.1.100 --dirs=./plots

# Using configuration file
echo '{"node_mode": true, "quiet": true}' > shadow.json
./shadowy
```

## HTTP Server

### Base URL
```
http://localhost:8080
```

### API Endpoints

#### Health Check
```bash
curl http://localhost:8080/health
```
Response:
```json
{"status": "ok"}
```

#### Node Status
```bash
curl http://localhost:8080/api/status
```
Response:
```json
{
  "node_address": "a8b033b8fde716ee88528ff8e17ad7b764fb06e26e7caf92f5d5bb775be13918",
  "wallet_info": {
    "path": "/home/user/.sn/default.json"
  },
  "token_count": 1,
  "genesis_token": {
    "name": "Shadow",
    "ticker": "SHADOW",
    "token_id": "ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626"
  },
  "configuration": "config(node_mode=true)",
  "seeds": [],
  "directories": ["./plots"],
  "http_server_addr": "http://localhost:8080"
}
```

#### Wallet Information
```bash
curl http://localhost:8080/api/wallet/info
```

#### Wallet Balance
```bash
curl http://localhost:8080/api/wallet/balance
```

#### Token Registry
```bash
curl http://localhost:8080/api/tokens
```

## Interactive Console

### Available Commands

#### `status`
Display comprehensive node status information.
```
shadowy> status
🌑 Shadowy Node Status
====================
Node Address: a8b033b8fde716ee88528ff8e17ad7b764fb06e26e7caf92f5d5bb775be13918
Wallet File: /home/user/.sn/default.json
HTTP Server: http://localhost:8080
Configuration: config(node_mode=true)
Token Registry: 1 tokens
Genesis Token: Shadow (SHADOW)
```

#### `shutdown`
Shutdown the node cleanly with state dump.
```
shadowy> shutdown
🌑 Initiating shutdown...
[... status information ...]
✅ Node state dumped. Shutting down...
```

#### `wallet`
Display wallet information.
```
shadowy> wallet
Wallet Address: a8b033b8fde716ee88528ff8e17ad7b764fb06e26e7caf92f5d5bb775be13918
Wallet File: /home/user/.sn/default.json
Key Type: ML-DSA87 (Post-Quantum)
```

#### `tokens`
Display token registry information.
```
shadowy> tokens
Token Registry: 1 tokens
Genesis Token: Shadow (SHADOW)
Token ID: ee5ccf1bab2fa5ce60bbaec533faf8332a637045b5c6d47803dce25e1591b626
```

#### `config`
Display current configuration.
```
shadowy> config
Configuration: config(node_mode=true, seeds=1, dirs=2)
Seed Nodes (1):
  1. abc123...@192.168.1.100
Directories (2):
  1. /home/user/plots
  2. /home/user/proofs
```

#### `help`
Show available commands.
```
shadowy> help
Commands:
  config     Display current configuration
  help       display help
  shutdown   Shutdown the node cleanly
  status     Display node status information
  tokens     Display token registry information
  wallet     Display wallet information
```

#### `exit`
Exit the interactive console (same as `shutdown`).

### Console Features

- **Readline Support**: History, tab completion, line editing
- **Command History**: Previous commands accessible with up/down arrows
- **Tab Completion**: Command name completion
- **Color Output**: Syntax highlighting and colored status messages
- **Signal Handling**: Graceful shutdown on Ctrl+C

## Process Management

### Graceful Shutdown

Node mode handles shutdown signals gracefully:

```bash
# Send SIGINT (Ctrl+C)
kill -INT $PID

# Send SIGTERM
kill -TERM $PID
```

Shutdown sequence:
1. Stop accepting new HTTP requests
2. Close HTTP server with 5-second timeout
3. Close interactive console
4. Wait for background goroutines
5. Clean exit

### Background Services

Current services:
- HTTP API server
- Interactive console

Future services:
- Tendermint blockchain node
- P2P networking
- Transaction mempool
- Block validation

## Configuration

### Node Mode in Config File

```json
{
  "node_mode": true,
  "quiet": true,
  "seeds": [
    "abc123def456789012345678901234567890abcd@seed1.network.com",
    "fed987cba654321098765432109876543210fedc@seed2.network.com:26657"
  ],
  "dirs": [
    "./plots",
    "./proofs",
    "/mnt/storage/farming"
  ]
}
```

### Environment Integration

```bash
# Production deployment
./shadowy --node --quiet

# Development with verbose logging
./shadowy --node

# Custom configuration
./shadowy --node --seeds=local@localhost --dirs=./dev_plots
```

## Security Considerations

### HTTP Server
- **Local only**: Binds to localhost:8080 by default
- **No authentication**: Currently open (add auth in production)
- **Read-only APIs**: Most endpoints are informational
- **Wallet access**: Sensitive operations require additional security

### Interactive Console
- **Local access**: Console runs on local terminal only
- **Command validation**: All commands are validated
- **Safe shutdown**: Graceful exit preserves state

## Monitoring

### Health Checks
```bash
# Simple health check
curl -f http://localhost:8080/health

# Detailed status
curl http://localhost:8080/api/status | jq .
```

### Logs
- HTTP server logs to stderr
- Console messages to stdout
- Use `--quiet` to minimize output

### Process Monitoring
```bash
# Check if node is running
pgrep -f "shadowy.*--node"

# Monitor HTTP endpoints
watch -n 5 'curl -s http://localhost:8080/api/status | jq .uptime'
```

## Development

### Adding New Commands

```go
// In lib/node.go addShellCommands()
ns.shell.AddCmd(&ishell.Cmd{
    Name: "newcommand",
    Help: "Description of new command",
    Func: func(c *ishell.Context) {
        c.Println("Command output")
    },
})
```

### Adding New API Endpoints

```go
// In lib/node.go startHTTPServer()
mux.HandleFunc("/api/newendpoint", ns.handleNewEndpoint)

func (ns *NodeServer) handleNewEndpoint(w http.ResponseWriter, r *http.Request) {
    data := map[string]interface{}{"key": "value"}
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}
```

## Troubleshooting

### Port Already in Use
```
HTTP server error: listen tcp :8080: bind: address already in use
```
Solution: Change port or kill existing process:
```bash
sudo lsof -i :8080
kill <PID>
```

### Console Not Responding
If the interactive console becomes unresponsive:
1. Try Ctrl+C for graceful shutdown
2. Use `kill -TERM <PID>` from another terminal
3. Force kill with `kill -9 <PID>` as last resort

### API Endpoints Not Working
1. Check if HTTP server started: `curl http://localhost:8080/health`
2. Verify node is in node mode: Check configuration output
3. Check logs for HTTP server errors