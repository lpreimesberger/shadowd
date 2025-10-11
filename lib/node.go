package lib

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// StartNode starts the blockchain node in node mode
func StartNode(config *CLIConfig) error {
	// Use ports from config (defaults: 9000 for P2P, 8080 for API)
	p2pPort := config.P2PPort
	apiPort := config.APIPort
	SetFarmingDebugMode(true)
	// Initialize plot manager if plot directories are configured
	if len(config.Dirs) > 0 {
		// Use the first directory for plots (can be enhanced to support multiple)
		plotDir := config.Dirs[0]
		if err := InitializePlotManager(plotDir); err != nil {
			return fmt.Errorf("failed to initialize plot manager: %w", err)
		}
	} else {
		// Try default ./plots directory
		if err := InitializePlotManager("./plots"); err != nil {
			fmt.Printf("⚠️  No plots found in ./plots: %v\n", err)
			fmt.Printf("⚠️  Node will run without farming capability\n")
		}
	}

	// Create the P2P blockchain node
	node, err := NewP2PBlockchainNode(p2pPort, apiPort, config)
	if err != nil {
		return fmt.Errorf("failed to create blockchain node: %w", err)
	}

	fmt.Printf("🌑 Shadowy Node Started\n")
	fmt.Printf("  P2P Port: %d\n", p2pPort)
	fmt.Printf("  API Port: %d\n", apiPort)
	fmt.Printf("\nPress Ctrl+C to stop...\n")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down node...")
	return node.Close()
}
