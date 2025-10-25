package lib

import (
	"crypto/sha256"
	"encoding/ascii85"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

	// Plot proof data (proves we have the plot file) - stored as base85 strings
	PlotHash      string `json:"plot_hash"`       // Hash found in plot file (base85)
	PlotPublicKey string `json:"plot_public_key"` // Public key from plot file (base85)
	PlotSignature string `json:"plot_signature"`  // Signature with plot private key (base85)
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

	encodedLen := ascii85.MaxEncodedLen(len(challengeHash))
	dst := make([]byte, encodedLen)
	ascii85.Encode(dst, challengeHash[:])

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
	ok, err := solution.Verify()
	if !ok {
		fmt.Printf("solution self check failed? " + err.Error())
	}
	x, _ := json.Marshal(solution)
	os.WriteFile("solution0.json", x, 0600)

	if farmingDebugMode {
		fmt.Printf("🎯 Found solution: distance=%d\n", solution.Distance)
		fmt.Printf("🎯 Plot public key length: %d\n", len(solution.PublicKey))
		fmt.Printf("🎯 Plot signature length: %d\n", len(solution.Signature))
	}

	// Keep plot data in base85 format (no decode needed!)
	// Create plot proof data to be signed by miner using the base85 string
	plotProofData := createPlotProofData(string(dst), solution.PublicKey, solution.Distance)

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
		fmt.Printf("   Challenge: %x\n", challengeHash[:16])
		fmt.Printf("   PlotPubKey length (base85): %d\n", len(solution.PublicKey))
		fmt.Printf("   PlotSignature length (base85): %d\n", len(solution.Signature))
		fmt.Printf("   PlotHash (base85): %s\n", solution.Hash[:32])
	}

	newPOS := ProofOfSpace{
		ChallengeHash:  challengeHash,
		PlotHash:       solution.Hash,
		PlotPublicKey:  solution.PublicKey,
		PlotSignature:  solution.Signature,
		Distance:       uint64(solution.Distance),
		MinerPublicKey: minerPublicKeyBytes,
		MinerSignature: minerSignature,
	}
	out, _ := json.Marshal(newPOS)
	os.WriteFile("proof.json", out, 0600)

	if !ValidateProofOfSpace(&newPOS) {
		fmt.Printf("Selfcheck of latest proof FAILED?")
	}
	return &newPOS, nil
}

// ValidateProofOfSpace validates a proof of space
func ValidateProofOfSpace(proof *ProofOfSpace) bool {
	if farmingDebugMode {
		fmt.Printf("✅ Validating proof: challenge=%x, distance=%d\n",
			proof.ChallengeHash, proof.Distance)
	}

	// Use plotlib to verify the plot signature directly (it expects base85)
	theSolution := storageproof.Solution{
		Hash:      proof.PlotHash,
		PublicKey: proof.PlotPublicKey,
		Signature: proof.PlotSignature,
		Distance:  int(proof.Distance),
	}
	encodedLen := ascii85.MaxEncodedLen(len(proof.ChallengeHash))
	dst := make([]byte, encodedLen)
	ascii85.Encode(dst, proof.ChallengeHash[:])

	ok, err := theSolution.Verify()
	if !ok {
		if farmingDebugMode {
			fmt.Printf("❌ Plot signature verification failed: %v\n", err)
			//x, _ := json.Marshal(theSolution)
			//			os.WriteFile("solution1.json", x, 0600)
		}
		return false
	}

	// Verify the miner signature over the plot proof data
	// Use base85 string for consistency with generation
	plotProofData := createPlotProofData(string(dst), proof.PlotPublicKey, int(proof.Distance))
	//	fmt.Printf("validating cha: %s\n", hex.EncodeToString(plotProofData))
	//	fmt.Printf("validating pub: %s\n", hex.EncodeToString(proof.MinerPublicKey))
	minerPubKey := &mldsa87.PublicKey{}
	if err := minerPubKey.UnmarshalBinary(proof.MinerPublicKey); err != nil {
		if farmingDebugMode {
			fmt.Printf("❌ Failed to unmarshal miner public key: %v\n", err)
		}
		return false
	}

	if !mldsa87.Verify(minerPubKey, plotProofData, nil, proof.MinerSignature) {
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

// GeneratePlot creates a new plot file using plotlib
// This is a wrapper around storageproof.Plot() for CLI integration
func GeneratePlot(destDir string, kValue uint32, verbose bool) error {
	return storageproof.Plot(destDir, kValue, verbose)
}

// createPlotProofData creates the data that the miner will sign
// Uses base85 string directly to avoid any encoding/decoding issues
func createPlotProofData(challengeHash string, plotPublicKeyBase85 string, distance int) []byte {
	// Create a deterministic representation of the plot proof
	// Hash the base85 string directly - no byte order issues!
	hash := sha256.New()
	// omg - no more []bytes please!
	// tbs is 'to be signed' in x509 docs
	tbs := fmt.Sprintf("%s/%s/%d", string(challengeHash), plotPublicKeyBase85, distance)
	hash.Write([]byte(tbs))
	return hash.Sum(nil)
}
