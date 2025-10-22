package lib

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// CLIConfig holds the parsed command line configuration
type CLIConfig struct {
	Quiet                 bool     `mapstructure:"quiet" json:"quiet"`                                       // Suppress verbose output (especially Tendermint debug info)
	Seeds                 []string `mapstructure:"seeds" json:"seeds"`                                       // List of seed nodes in format nodeid@ip_address
	Dirs                  []string `mapstructure:"dirs" json:"dirs"`                                         // Directories containing plot/proof files
	NodeMode              bool     `mapstructure:"node_mode" json:"node_mode"`                               // Run in node mode (HTTP server + console)
	BlockchainDir         string   `mapstructure:"blockchain_dir" json:"blockchain_dir"`                     // Directory for blockchain data (Tendermint files)
	P2PPort               int      `mapstructure:"p2p_port" json:"p2p_port"`                                 // P2P listen port
	APIPort               int      `mapstructure:"api_port" json:"api_port"`                                 // API/HTTP listen port
	MempoolTxExpiryBlocks int      `mapstructure:"mempool_tx_expiry_blocks" json:"mempool_tx_expiry_blocks"` // Blocks before tx expires from mempool (default: 2048)
	MempoolMaxSizeMB      int      `mapstructure:"mempool_max_size_mb" json:"mempool_max_size_mb"`           // Maximum mempool size in MB (default: 300)
	APIKey                string   `mapstructure:"api_key" json:"api_key"`                                   // Optional API key for write endpoints (env: SHADOWY_API_KEY)
}

// SeedNode represents a parsed seed node
type SeedNode struct {
	NodeID    string // Tendermint node ID (hex string)
	IPAddress string // IP address or hostname
	Port      string // Port (defaults to 26656 if not specified)
}

// String returns the seed node in standard format
func (sn SeedNode) String() string {
	if sn.Port != "" && sn.Port != "26656" {
		return fmt.Sprintf("%s@%s:%s", sn.NodeID, sn.IPAddress, sn.Port)
	}
	return fmt.Sprintf("%s@%s", sn.NodeID, sn.IPAddress)
}

// ParseCLI parses command line arguments and configuration file, returns configuration
func ParseCLI() (*CLIConfig, error) {
	config := &CLIConfig{}

	// Initialize viper
	viper.SetConfigName("shadow")
	viper.SetConfigType("json")
	viper.AddConfigPath(".") // Look for config file in current directory

	// Enable environment variable support
	viper.SetEnvPrefix("SHADOWY")
	viper.AutomaticEnv()

	// Set defaults
	viper.SetDefault("quiet", false)
	viper.SetDefault("seeds", []string{})
	viper.SetDefault("dirs", []string{})
	viper.SetDefault("node_mode", false)
	viper.SetDefault("blockchain_dir", "./blockchain")
	viper.SetDefault("p2p_port", 9000)
	viper.SetDefault("api_port", 8080)
	viper.SetDefault("mempool_tx_expiry_blocks", 2048)
	viper.SetDefault("mempool_max_size_mb", 300)
	viper.SetDefault("api_key", "") // No API key by default

	// Define command line flags
	quietFlag := flag.Bool("quiet", false, "Suppress verbose output (especially Tendermint debug info)")
	seedsFlag := flag.String("seeds", "", "Comma-delimited list of seed nodes (format: nodeid@ip_address[:port])")
	dirsFlag := flag.String("dirs", "", "Comma-delimited list of directories containing plot/proof files")
	nodeFlag := flag.Bool("node", false, "Run in node mode (starts HTTP server, Tendermint, and interactive console)")
	blockchainDirFlag := flag.String("blockchain-dir", "", "Directory for blockchain data (Tendermint files), defaults to ./blockchain")
	p2pPortFlag := flag.Int("p2p-port", 9000, "P2P listen port (default: 9000)")
	apiPortFlag := flag.Int("api-port", 8080, "API/HTTP listen port (default: 8080)")
	apiKeyFlag := flag.String("api-key", "", "API key for write endpoints (or set SHADOWY_API_KEY env var)")

	// Parse command line
	flag.Parse()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create default one
			if err := createDefaultConfig(); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			// Config file was found but another error was produced
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Command line flags override config file values
	if *quietFlag {
		viper.Set("quiet", true)
	}

	if *nodeFlag {
		viper.Set("node_mode", true)
	}

	if *seedsFlag != "" {
		seeds, err := parseSeeds(*seedsFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to parse seeds: %w", err)
		}
		viper.Set("seeds", seeds)
	}

	if *dirsFlag != "" {
		dirs, err := parseDirs(*dirsFlag)
		if err != nil {
			return nil, fmt.Errorf("failed to parse dirs: %w", err)
		}
		viper.Set("dirs", dirs)
	}

	if *blockchainDirFlag != "" {
		viper.Set("blockchain_dir", *blockchainDirFlag)
	}

	if *p2pPortFlag != 9000 {
		viper.Set("p2p_port", *p2pPortFlag)
	}

	if *apiPortFlag != 8080 {
		viper.Set("api_port", *apiPortFlag)
	}

	if *apiKeyFlag != "" {
		viper.Set("api_key", *apiKeyFlag)
	}

	// Unmarshal config into struct
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// createDefaultConfig creates a default shadow.json configuration file
func createDefaultConfig() error {
	defaultConfig := &CLIConfig{
		Quiet:                 false,
		Seeds:                 []string{},
		Dirs:                  []string{"./plots"},
		NodeMode:              false,
		BlockchainDir:         "./blockchain",
		MempoolTxExpiryBlocks: 2048,
		MempoolMaxSizeMB:      300,
	}

	viper.Set("quiet", defaultConfig.Quiet)
	viper.Set("seeds", defaultConfig.Seeds)
	viper.Set("dirs", defaultConfig.Dirs)
	viper.Set("node_mode", defaultConfig.NodeMode)
	viper.Set("blockchain_dir", defaultConfig.BlockchainDir)
	viper.Set("mempool_tx_expiry_blocks", defaultConfig.MempoolTxExpiryBlocks)
	viper.Set("mempool_max_size_mb", defaultConfig.MempoolMaxSizeMB)

	// Write config file
	if err := viper.WriteConfigAs("shadow.json"); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// parseDirs parses a comma-delimited list of directories
func parseDirs(dirsStr string) ([]string, error) {
	if dirsStr == "" {
		return nil, nil
	}

	// Split by comma and trim whitespace
	rawDirs := strings.Split(dirsStr, ",")
	var dirs []string

	for i, rawDir := range rawDirs {
		dir := strings.TrimSpace(rawDir)
		if dir == "" {
			continue
		}

		// Validate directory path
		if err := validateDirectoryPath(dir); err != nil {
			return nil, fmt.Errorf("invalid directory %d (%s): %w", i+1, dir, err)
		}

		// Convert to absolute path
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for %s: %w", dir, err)
		}

		dirs = append(dirs, absDir)
	}

	return dirs, nil
}

// validateDirectoryPath validates a directory path
func validateDirectoryPath(dir string) error {
	// Check for invalid characters
	if strings.Contains(dir, "\x00") {
		return fmt.Errorf("directory path contains null character")
	}

	// Check if path is too long (reasonable limit)
	if len(dir) > 4096 {
		return fmt.Errorf("directory path too long (max 4096 characters)")
	}

	// Check if directory exists or can be created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Try to create directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}

	return nil
}

// parseSeeds parses a comma-delimited list of seed nodes
func parseSeeds(seedsStr string) ([]string, error) {
	if seedsStr == "" {
		return nil, nil
	}

	// Split by comma and trim whitespace
	rawSeeds := strings.Split(seedsStr, ",")
	var seeds []string

	for i, rawSeed := range rawSeeds {
		seed := strings.TrimSpace(rawSeed)
		if seed == "" {
			continue
		}

		// Validate seed format
		if err := ValidateSeedFormat(seed); err != nil {
			return nil, fmt.Errorf("invalid seed %d (%s): %w", i+1, seed, err)
		}

		seeds = append(seeds, seed)
	}

	return seeds, nil
}

// ValidateSeedFormat validates that a seed follows the nodeid@ip_address[:port] format
func ValidateSeedFormat(seed string) error {
	// Regex for seed format: nodeid@ip_address[:port]
	// NodeID should be 40 hex characters (20 bytes)
	// IP can be IPv4, IPv6 (with brackets), or hostname
	// Port is optional, defaults to 26656
	seedRegex := regexp.MustCompile(`^[0-9a-fA-F]{40}@(?:[a-zA-Z0-9.-]+|\[[0-9a-fA-F:]+\])(?::[0-9]{1,5})?$`)

	if !seedRegex.MatchString(seed) {
		return fmt.Errorf("seed must be in format 'nodeid@ip_address[:port]' where nodeid is 40 hex characters")
	}

	// Additional validation for port range if present
	parts := strings.Split(seed, "@")
	if len(parts) != 2 {
		return fmt.Errorf("seed must contain exactly one '@' character")
	}

	nodeID := parts[0]
	if len(nodeID) != 40 {
		return fmt.Errorf("node ID must be exactly 40 hex characters, got %d", len(nodeID))
	}

	// Validate hex characters in node ID
	for _, char := range nodeID {
		if !isHexChar(char) {
			return fmt.Errorf("node ID must contain only hex characters (0-9, a-f, A-F)")
		}
	}

	addressPart := parts[1]

	// Handle IPv6 addresses in brackets [::1]:port
	if strings.HasPrefix(addressPart, "[") {
		closeBracket := strings.Index(addressPart, "]")
		if closeBracket == -1 {
			return fmt.Errorf("IPv6 address missing closing bracket")
		}

		// Check if there's a port after the bracket
		afterBracket := addressPart[closeBracket+1:]
		if afterBracket != "" {
			if !strings.HasPrefix(afterBracket, ":") {
				return fmt.Errorf("invalid IPv6 address format")
			}
			port := afterBracket[1:]
			if port == "0" || len(port) == 0 {
				return fmt.Errorf("port cannot be 0 or empty")
			}
			if len(port) > 5 {
				return fmt.Errorf("port number too long")
			}
		}
	} else if strings.Contains(addressPart, ":") {
		// Regular IPv4 or hostname with port
		lastColon := strings.LastIndex(addressPart, ":")
		port := addressPart[lastColon+1:]
		if port == "0" || len(port) == 0 {
			return fmt.Errorf("port cannot be 0 or empty")
		}
		if len(port) > 5 {
			return fmt.Errorf("port number too long")
		}
	}

	return nil
}

// isHexChar checks if a character is a valid hexadecimal digit
func isHexChar(c rune) bool {
	return (c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'f') ||
		(c >= 'A' && c <= 'F')
}

// ParseSeedNode parses a seed string into a SeedNode struct
func ParseSeedNode(seed string) (*SeedNode, error) {
	if err := ValidateSeedFormat(seed); err != nil {
		return nil, err
	}

	parts := strings.Split(seed, "@")
	nodeID := parts[0]
	addressPart := parts[1]

	var ipAddress, port string

	// Handle IPv6 addresses in brackets
	if strings.HasPrefix(addressPart, "[") {
		closeBracket := strings.Index(addressPart, "]")
		ipAddress = addressPart[:closeBracket+1] // Include brackets
		afterBracket := addressPart[closeBracket+1:]
		if afterBracket != "" && strings.HasPrefix(afterBracket, ":") {
			port = afterBracket[1:]
		} else {
			port = "26656"
		}
	} else if strings.Contains(addressPart, ":") {
		// Regular IPv4 or hostname with port
		lastColon := strings.LastIndex(addressPart, ":")
		ipAddress = addressPart[:lastColon]
		port = addressPart[lastColon+1:]
	} else {
		ipAddress = addressPart
		port = "26656" // Default Tendermint port
	}

	return &SeedNode{
		NodeID:    nodeID,
		IPAddress: ipAddress,
		Port:      port,
	}, nil
}

// GetSeedNodes parses all seeds into SeedNode structs
func (config *CLIConfig) GetSeedNodes() ([]*SeedNode, error) {
	var seedNodes []*SeedNode

	for _, seed := range config.Seeds {
		node, err := ParseSeedNode(seed)
		if err != nil {
			return nil, fmt.Errorf("failed to parse seed %s: %w", seed, err)
		}
		seedNodes = append(seedNodes, node)
	}

	return seedNodes, nil
}

// PrintUsage prints command line usage information
func PrintUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n🌑 Shadowy - Post-Quantum UTXO Blockchain Node\n")
	fmt.Fprintf(os.Stderr, "============================================\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s --quiet\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --seeds=abc123...@192.168.1.100,def456...@node2.example.com:26657\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --dirs=./plots,./proofs,/mnt/storage/farming\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --quiet --seeds=abc123...@192.168.1.100 --dirs=./plots\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nConfiguration:\n")
	fmt.Fprintf(os.Stderr, "  Config file: shadow.json (created automatically if missing)\n")
	fmt.Fprintf(os.Stderr, "  Command line flags override config file values\n")
	fmt.Fprintf(os.Stderr, "\nSeed Format:\n")
	fmt.Fprintf(os.Stderr, "  Seeds must be in format: nodeid@ip_address[:port]\n")
	fmt.Fprintf(os.Stderr, "  - nodeid: 40 hex characters (Tendermint node ID)\n")
	fmt.Fprintf(os.Stderr, "  - ip_address: IPv4, IPv6, or hostname\n")
	fmt.Fprintf(os.Stderr, "  - port: optional, defaults to 26656\n")
	fmt.Fprintf(os.Stderr, "\nDirectories:\n")
	fmt.Fprintf(os.Stderr, "  Plot/proof directories are created automatically if missing\n")
	fmt.Fprintf(os.Stderr, "  Paths are converted to absolute paths\n")
}

// ValidateConfig performs additional validation on the parsed configuration
func (config *CLIConfig) ValidateConfig() error {
	// Validate seed nodes
	if len(config.Seeds) > 0 {
		_, err := config.GetSeedNodes()
		if err != nil {
			return fmt.Errorf("seed validation failed: %w", err)
		}
	}

	// Validate directories
	for i, dir := range config.Dirs {
		if err := validateDirectoryPath(dir); err != nil {
			return fmt.Errorf("directory validation failed for dir %d (%s): %w", i+1, dir, err)
		}
	}

	return nil
}

// String returns a string representation of the CLI configuration
func (config *CLIConfig) String() string {
	var parts []string

	if config.Quiet {
		parts = append(parts, "quiet=true")
	}

	if len(config.Seeds) > 0 {
		parts = append(parts, fmt.Sprintf("seeds=%d", len(config.Seeds)))
	}

	if len(config.Dirs) > 0 {
		parts = append(parts, fmt.Sprintf("dirs=%d", len(config.Dirs)))
	}

	if config.NodeMode {
		parts = append(parts, "node_mode=true")
	}

	if config.BlockchainDir != "./blockchain" {
		parts = append(parts, fmt.Sprintf("blockchain_dir=%s", config.BlockchainDir))
	}

	if len(parts) == 0 {
		return "default configuration"
	}

	return fmt.Sprintf("config(%s)", strings.Join(parts, ", "))
}
