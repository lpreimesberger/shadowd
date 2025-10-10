package lib

import (
	"encoding/binary"
	"time"

	"golang.org/x/crypto/sha3"
)

// GenerateChallenge creates a deterministic challenge for a given block height
// Uses SHAKE-256 (SHA-3 XOF) for ASIC resistance
func GenerateChallenge(prevBlockHash string, blockHeight uint64, timestamp int64) [32]byte {
	// Floor timestamp to 10-second blocks to prevent grinding
	floorTimestamp := (timestamp / 10) * 10

	// Create SHAKE-256 instance
	shake := sha3.NewShake256()

	// Write inputs: prevBlockHash || blockHeight || floorTimestamp
	shake.Write([]byte(prevBlockHash))

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, blockHeight)
	shake.Write(heightBytes)

	timestampBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(timestampBytes, uint64(floorTimestamp))
	shake.Write(timestampBytes)

	// Read 32 bytes (256 bits) of output
	var challenge [32]byte
	shake.Read(challenge[:])

	return challenge
}

// GetCurrentChallenge computes the challenge for the current block height
func (bc *Blockchain) GetCurrentChallenge() [32]byte {
	bc.chainLock.RLock()
	defer bc.chainLock.RUnlock()

	prevHash := ""
	height := uint64(0)

	if len(bc.blocks) > 0 {
		lastBlock := bc.blocks[len(bc.blocks)-1]
		prevHash = lastBlock.Hash
		height = lastBlock.Index + 1
	}

	return GenerateChallenge(prevHash, height, time.Now().Unix())
}
