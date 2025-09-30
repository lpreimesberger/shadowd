# Command Line Interface

## Overview

The Shadowy node provides a command-line interface for configuration and network connectivity.

## Basic Usage

```bash
# Build the binary
go build -o shadowy main.go

# Run with default settings
./shadowy

# Get help
./shadowy --help
```

## Command Line Flags

### `--quiet`
Suppresses verbose output, especially useful for reducing Tendermint debug information.

```bash
# Quiet mode - minimal output
./shadowy --quiet
```

**When to use:**
- Production deployments
- When integrating with Tendermint (reduces log noise)
- Automated scripts where minimal output is preferred

### `--seeds`
Specifies comma-delimited list of seed nodes for P2P network connectivity.

```bash
# Single seed node
./shadowy --seeds=abc123...def456@192.168.1.100

# Multiple seed nodes
./shadowy --seeds=abc123...def456@192.168.1.100,fed987...cba654@node2.example.com:26657

# With custom ports
./shadowy --seeds=abc123...def456@192.168.1.100:26657,fed987...cba654@[::1]:26656
```

### `--dirs`
Specifies comma-delimited list of directories for plot and proof file storage.

```bash
# Single directory
./shadowy --dirs=./plots

# Multiple directories
./shadowy --dirs=./plots,./proofs,/mnt/storage/farming

# Absolute and relative paths
./shadowy --dirs=/mnt/ssd/plots,./local_plots,../shared/proofs
```

**Directory Behavior:**
- Directories are created automatically if they don't exist
- Relative paths are converted to absolute paths
- Permissions set to 0755 (rwxr-xr-x)

## Seed Node Format

### Specification
```
nodeid@ip_address[:port]
```

### Components

#### Node ID
- **Length**: Exactly 40 hexadecimal characters
- **Format**: Tendermint node identifier (20 bytes hex-encoded)
- **Example**: `abc123def456789012345678901234567890abcd`

#### IP Address
- **IPv4**: Standard dotted notation (`192.168.1.100`)
- **IPv6**: Bracketed format (`[::1]`, `[2001:db8::1]`)
- **Hostname**: DNS names (`node.example.com`, `localhost`)

#### Port (Optional)
- **Default**: 26656 (standard Tendermint port)
- **Range**: 1-65535
- **Format**: `:port` suffix

### Valid Examples

```bash
# IPv4 with default port
abc123def456789012345678901234567890abcd@192.168.1.100

# IPv4 with custom port
abc123def456789012345678901234567890abcd@192.168.1.100:26657

# Hostname with port
abc123def456789012345678901234567890abcd@node.example.com:26657

# IPv6 with brackets
abc123def456789012345678901234567890abcd@[::1]

# IPv6 with port
abc123def456789012345678901234567890abcd@[2001:db8::1]:26657

# Localhost
abc123def456789012345678901234567890abcd@localhost
```

### Invalid Examples

```bash
# Node ID too short
short@192.168.1.100

# Missing @ separator
abc123def456789012345678901234567890abcd192.168.1.100

# Missing IP address
abc123def456789012345678901234567890abcd@

# Invalid port (0)
abc123def456789012345678901234567890abcd@192.168.1.100:0

# Non-hex characters in node ID
gggggg456789012345678901234567890abcd@192.168.1.100
```

## Validation

### Node ID Validation
- Must be exactly 40 characters
- Must contain only hexadecimal characters (0-9, a-f, A-F)
- Case insensitive

### IP Address Validation
- IPv4: Standard format validation
- IPv6: Must be enclosed in brackets
- Hostname: Basic format checking

### Port Validation
- Must be between 1-65535
- Cannot be 0 or empty
- Must be numeric

## Usage Examples

### Development
```bash
# Local development with verbose output
./shadowy

# Connect to local testnet
./shadowy --seeds=abc123...@localhost
```

### Production
```bash
# Production deployment with minimal logging
./shadowy --quiet --seeds=node1...@seed1.network.com,node2...@seed2.network.com:26657

# Behind load balancer
./shadowy --quiet --seeds=cluster...@internal.network:26656
```

### Testnet
```bash
# Join public testnet
./shadowy --seeds=testnet1...@testnet.shadowy.org,testnet2...@backup.shadowy.org

# Development testnet
./shadowy --quiet --seeds=dev1...@192.168.1.10,dev2...@192.168.1.11,dev3...@192.168.1.12
```

## Configuration Display

When starting with verbose output, the node displays:

```
🌑 Shadowy - Post-Quantum UTXO Blockchain Node
===========================================
   Configuration: config(quiet=false, seeds=2)
   Seed nodes: 2 configured
     1. abc123def456789012345678901234567890abcd@192.168.1.100
     2. fed987cba654321098765432109876543210fedc@node.example.com:26657
```

## Error Handling

### Invalid Seed Format
```bash
$ ./shadowy --seeds=invalid_seed
Error parsing command line: failed to parse seeds: invalid seed 1 (invalid_seed): seed must be in format 'nodeid@ip_address[:port]' where nodeid is 40 hex characters

Usage: ./shadowy [options]
[... usage information displayed ...]
```

### Missing Required Fields
```bash
$ ./shadowy --seeds=abc123@
Error parsing command line: failed to parse seeds: invalid seed 1 (abc123@): seed must be in format 'nodeid@ip_address[:port]' where nodeid is 40 hex characters
```

## Integration with Tendermint

### Seed Configuration
Seeds configured via CLI are used to:
1. Bootstrap P2P network discovery
2. Find initial peers for block synchronization
3. Maintain network connectivity

### Tendermint Config Generation
```go
// Example of how seeds are used in Tendermint config
config := &TendermintConfig{
    P2P: &P2PConfig{
        Seeds: strings.Join(cliConfig.Seeds, ","),
    },
}
```

## Environment Variables

While not currently implemented, future versions may support:

```bash
# Future environment variable support
export SHADOWY_QUIET=true
export SHADOWY_SEEDS="node1...@seed1.com,node2...@seed2.com"
./shadowy
```

## Configuration Files

### shadow.json
Shadowy automatically creates and uses a `shadow.json` configuration file in the current working directory:

```json
{
  "quiet": false,
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

### Configuration Precedence
1. **Command line flags** (highest priority)
2. **Configuration file values**
3. **Default values** (lowest priority)

### Auto-Creation
- Configuration file is created automatically if missing
- Default values: `quiet=false`, `seeds=[]`, `dirs=["./plots"]`
- Command line flags update the configuration file values in memory
- Configuration file is not modified by command line flags

### Manual Editing
You can manually edit `shadow.json` to set default values:

```bash
# Edit configuration
nano shadow.json

# Run with config file values
./shadowy

# Override specific values
./shadowy --quiet --dirs=./custom_plots
```

## Debugging

### Verbose Mode
Default mode shows detailed initialization:
- Token system initialization
- Wallet loading/creation
- Transaction demonstrations
- Network configuration

### Quiet Mode
Minimal output shows only:
- Critical errors
- Essential startup messages
- Network connectivity status

### Troubleshooting
```bash
# Check if binary works
./shadowy --help

# Test with minimal config
./shadowy --quiet

# Validate seed format
./shadowy --seeds=abc123def456789012345678901234567890abcd@localhost

# Debug network connectivity
./shadowy --seeds=known_good_seed@reliable.node.com
```

## Security Considerations

### Seed Node Trust
- Only connect to trusted seed nodes
- Seed nodes can see your IP address
- Use multiple seed nodes for redundancy

### Network Discovery
- Seed nodes help bootstrap peer discovery
- Additional peers discovered through gossip protocol
- No sensitive data shared with seed nodes

### Node ID Generation
```bash
# Generate Tendermint node ID (future utility)
tendermint show_node_id
```

## Performance

### Startup Time
- Quiet mode: ~100ms faster startup
- Seed validation: <1ms per seed
- Network connection: depends on seed responsiveness

### Memory Usage
- CLI parsing: Minimal overhead
- Seed storage: ~100 bytes per seed
- Configuration: <1KB total