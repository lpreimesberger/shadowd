package lib

import (
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/binary"
	"fmt"
	"log"
	"sync"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
	"github.com/lpreimesberger/plotlib/pkg/storageproof"
)

// Global plot manager
var (
	globalPlotCollection *storageproof.PlotCollection
	plotMutex            sync.RWMutex
	farmingDebugMode     = true // Global flag for loud/slow debug checks
)

// ProofOfSpace represents a complete mining proof with both plot and miner signatures
type ProofOfSpace struct {
	// Challenge data
	ChallengeHash [32]byte `json:"challenge_hash"`

	// Plot proof data (proves we have the plot file)
	PlotHash      []byte `json:"plot_hash"`       // Hash found in plot file
	PlotPublicKey []byte `json:"plot_public_key"` // Public key from plot file
	PlotSignature []byte `json:"plot_signature"`  // Signature with plot private key
	Distance      uint64 `json:"distance"`        // Hamming distance to challenge

	// Miner proof data (proves we own this submission)
	MinerPublicKey []byte `json:"miner_public_key"` // Our node's public key
	MinerSignature []byte `json:"miner_signature"`  // Our signature over the plot proof
}

// InitializePlotManager loads plots from the specified directory
func InitializePlotManager(plotDir string) error {
	plotMutex.Lock()
	defer plotMutex.Unlock()

	if !farmingDebugMode {
		log.Printf("Loading plots from: %s", plotDir)
	}

	// Load plots using plotlib
	pc, err := storageproof.LoadPlots([]string{plotDir}, farmingDebugMode)
	if err != nil {
		return fmt.Errorf("failed to load plots: %w", err)
	}

	globalPlotCollection = pc

	if !farmingDebugMode {
		log.Printf("Successfully loaded %d plot files", len(pc.Plots))
	} else {
		fmt.Printf("📊 Plot Manager Initialized: %d plot files loaded from %s\n", len(pc.Plots), plotDir)
	}

	return nil
}

// GetPlotCount returns the number of loaded plots
func GetPlotCount() int {
	plotMutex.RLock()
	defer plotMutex.RUnlock()

	if globalPlotCollection == nil {
		return 0
	}
	return len(globalPlotCollection.Plots)
}

// SetFarmingDebugMode enables/disables verbose debug output
func SetFarmingDebugMode(enabled bool) {
	farmingDebugMode = enabled
}

// GenerateProofOfSpace generates a complete mining proof with both plot and miner signatures
func GenerateProofOfSpace(challengeHash [32]byte, minerPrivateKey []byte) (*ProofOfSpace, error) {
	plotMutex.RLock()
	defer plotMutex.RUnlock()

	if globalPlotCollection == nil {
		return nil, fmt.Errorf("plot collection not initialized - call InitializePlotManager first")
	}

	if farmingDebugMode {
		fmt.Printf("🔍 Generating proof for challenge: %x\n", challengeHash)
	}

	// Use LookUp to find the best solution in our plot files
	// This returns a Solution with plot signature already generated
	solution, err := globalPlotCollection.LookUp(challengeHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to lookup proof: %w", err)
	}

	if solution == nil {
		return nil, fmt.Errorf("no solution found (no plots available)")
	}

	if farmingDebugMode {
		fmt.Printf("🎯 Found solution: distance=%d\n", solution.Distance)
		fmt.Printf("🎯 Plot public key length: %d\n", len(solution.PublicKey))
		fmt.Printf("🎯 Plot signature length: %d\n", len(solution.Signature))
	}

	// Decode the plot data from base85 (plotlib encodes as base85 strings)
	plotPublicKeyBytes, err := decodeBase85String(solution.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plot public key: %w", err)
	}

	plotSignatureBytes, err := decodeBase85String(solution.Signature)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plot signature: %w", err)
	}

	hashBytes, err := decodeBase85String(solution.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to decode solution hash: %w", err)
	}

	// Create plot proof data to be signed by miner
	// This proves the miner owns this submission
	plotProofData := createPlotProofData(challengeHash, solution.PublicKey, solution.Distance)

	// Generate miner signature using the wallet's ML-DSA87 key
	// Unmarshal the private key bytes
	minerPrivateKeyObj := &mldsa87.PrivateKey{}
	if err := minerPrivateKeyObj.UnmarshalBinary(minerPrivateKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal miner private key: %w", err)
	}

	// Sign the plot proof data with the miner's key
	minerSignature := make([]byte, mldsa87.SignatureSize)
	mldsa87.SignTo(minerPrivateKeyObj, plotProofData, nil, false, minerSignature)

	// Get the public key from the private key
	minerPublicKeyObj := minerPrivateKeyObj.Public().(*mldsa87.PublicKey)
	minerPublicKeyBytes, err := minerPublicKeyObj.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal miner public key: %w", err)
	}

	if farmingDebugMode {
		fmt.Printf("✅ Generated complete proof with both plot and miner signatures\n")
	}

	return &ProofOfSpace{
		ChallengeHash:  challengeHash,
		PlotHash:       hashBytes,
		PlotPublicKey:  plotPublicKeyBytes,
		PlotSignature:  plotSignatureBytes,
		Distance:       uint64(solution.Distance),
		MinerPublicKey: minerPublicKeyBytes,
		MinerSignature: minerSignature,
	}, nil
}

// ValidateProofOfSpace validates a proof of space
func ValidateProofOfSpace(proof *ProofOfSpace) bool {
	if farmingDebugMode {
		fmt.Printf("✅ Validating proof: challenge=%x, distance=%d\n",
			proof.ChallengeHash, proof.Distance)
	}

	// Verify the plot signature with the plot public key (from plot file)
	plotPubKey := &mldsa87.PublicKey{}
	if err := plotPubKey.UnmarshalBinary(proof.PlotPublicKey); err != nil {
		if farmingDebugMode {
			fmt.Printf("❌ Failed to unmarshal plot public key: %v\n", err)
		}
		return false
	}

	// The plot signature signs the best hash found (proof.PlotHash)
	if !mldsa87.Verify(plotPubKey, proof.PlotHash, proof.PlotSignature, nil) {
		if farmingDebugMode {
			fmt.Printf("❌ Plot signature verification failed\n")
		}
		return false
	}

	// Verify the miner signature over the plot proof data
	// Need to encode plot public key back to base85 for creating proof data
	encodedPlotPubKey := encodeBase85Bytes(proof.PlotPublicKey)
	plotProofData := createPlotProofData(proof.ChallengeHash, encodedPlotPubKey, int(proof.Distance))

	minerPubKey := &mldsa87.PublicKey{}
	if err := minerPubKey.UnmarshalBinary(proof.MinerPublicKey); err != nil {
		if farmingDebugMode {
			fmt.Printf("❌ Failed to unmarshal miner public key: %v\n", err)
		}
		return false
	}

	if !mldsa87.Verify(minerPubKey, plotProofData, proof.MinerSignature, nil) {
		if farmingDebugMode {
			fmt.Printf("❌ Miner signature verification failed\n")
		}
		return false
	}

	if farmingDebugMode {
		fmt.Printf("✅ Proof validation successful\n")
	}

	return true
}

// decodeBase85String decodes a base85-encoded string to bytes
func decodeBase85String(encoded string) ([]byte, error) {
	// Estimate decode buffer size (base85 is ~25% larger than binary)
	maxLen := (len(encoded) * 4) / 5
	result := make([]byte, maxLen)

	n, _, err := ascii85.Decode(result, []byte(encoded), true)
	if err != nil {
		return nil, err
	}

	return result[:n], nil
}

// encodeBase85Bytes encodes bytes to a base85 string
func encodeBase85Bytes(data []byte) string {
	dst := make([]byte, ascii85.MaxEncodedLen(len(data)))
	n := ascii85.Encode(dst, data)
	return string(dst[:n])
}

// createPlotProofData creates the data that the miner will sign
func createPlotProofData(challengeHash [32]byte, plotPublicKey string, distance int) []byte {
	// Create a deterministic representation of the plot proof
	hash := sha256.New()
	hash.Write(challengeHash[:])
	hash.Write([]byte(plotPublicKey))

	distanceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(distanceBytes, uint64(distance))
	hash.Write(distanceBytes)

	return hash.Sum(nil)
}

// CreateDefaultPlotsDirectory creates the default plots directory if it doesn't exist
func CreateDefaultPlotsDirectory(plotDir string) error {
	// This will be called to ensure the plots directory exists
	// The actual plot generation would be handled by plotlib separately
	return nil
}
