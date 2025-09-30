package lib

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
	"golang.org/x/crypto/blake2b"
)

// AddressType represents the type of blockchain address
type AddressType byte

const (
	AddressTypeWallet    AddressType = 'S' // Standard wallet addresses
	AddressTypeLiquidity AddressType = 'L' // Liquidity pool addresses
	AddressTypeExchange  AddressType = 'X' // Exchange/swap addresses
	AddressTypeNFT       AddressType = 'N' // Generic non-fungible token addresses (faucets, etc.)
)

// Address represents a blockchain address derived from a public key hash
type Address [32]byte

// String returns the checksummed string representation of an address
// Format: [BOM][HEX-WITH-EIP55-CASE][LUHN-CHAR]
func (a Address) String() string {
	return a.StringWithType(AddressTypeWallet)
}

// StringWithType returns the checksummed string representation with a specific address type prefix
func (a Address) StringWithType(addrType AddressType) string {
	// Start with BOM prefix
	result := string(addrType)

	// Get the raw hex
	rawHex := hex.EncodeToString(a[:])

	// Apply EIP-55 style checksum (mixed case based on hash bits)
	checksummedHex := applyEIP55Checksum(rawHex)
	result += checksummedHex

	// Add Luhn checksum character at the end
	// Calculate on normalized (lowercase) version
	normalized := strings.ToLower(result)
	luhnChar := calculateLuhnChecksum(normalized)
	result += string(luhnChar)

	return result
}

// DeriveAddress creates an address from a public key using BLAKE2b-256
func DeriveAddress(publicKey *mldsa87.PublicKey) Address {
	// Marshal the public key to bytes for hashing
	pkBytes, _ := publicKey.MarshalBinary()
	hash := blake2b.Sum256(pkBytes)
	return Address(hash)
}

// ParseAddress converts a string to an Address with full validation
// Validates BOM prefix, optional EIP-55 checksum (if mixed case), and mandatory Luhn checksum
func ParseAddress(addrStr string) (Address, AddressType, error) {
	var addr Address

	if len(addrStr) < 3 { // Minimum: BOM + some hex + Luhn
		return addr, 0, errors.New("address too short")
	}

	// Extract and validate BOM prefix
	bomChar := addrStr[0]
	addrType := AddressType(bomChar)
	if !isValidAddressType(addrType) {
		return addr, 0, fmt.Errorf("invalid address type prefix: %c", bomChar)
	}

	// Extract Luhn checksum character (last character)
	luhnChar := addrStr[len(addrStr)-1]

	// Extract hex portion (between BOM and Luhn)
	hexPortion := addrStr[1 : len(addrStr)-1]

	// Validate Luhn checksum on normalized string (mandatory check)
	normalized := strings.ToLower(string(bomChar) + hexPortion)
	expectedLuhn := calculateLuhnChecksum(normalized)
	if luhnChar != expectedLuhn {
		return addr, 0, fmt.Errorf("invalid Luhn checksum: expected %c, got %c", expectedLuhn, luhnChar)
	}

	// Check if mixed case is used (optional EIP-55 validation)
	if hasMixedCase(hexPortion) {
		// Validate EIP-55 checksum
		if !validateEIP55Checksum(hexPortion) {
			return addr, 0, errors.New("invalid EIP-55 checksum (mixed case incorrect)")
		}
	}

	// Decode hex to bytes
	hexLower := strings.ToLower(hexPortion)
	bytes, err := hex.DecodeString(hexLower)
	if err != nil {
		return addr, 0, fmt.Errorf("invalid hex string: %w", err)
	}

	if len(bytes) != 32 {
		return addr, 0, fmt.Errorf("address must be 32 bytes, got %d", len(bytes))
	}

	copy(addr[:], bytes)
	return addr, addrType, nil
}

// isValidAddressType checks if the address type is valid
func isValidAddressType(t AddressType) bool {
	return t == AddressTypeWallet || t == AddressTypeLiquidity ||
		t == AddressTypeExchange || t == AddressTypeNFT
}

// applyEIP55Checksum applies EIP-55 style checksum to a hex string
// Characters are uppercased if the corresponding bit in the hash is 1
func applyEIP55Checksum(hexStr string) string {
	// Hash the lowercase hex string
	hash := blake2b.Sum256([]byte(strings.ToLower(hexStr)))

	result := make([]byte, len(hexStr))
	for i := 0; i < len(hexStr); i++ {
		char := hexStr[i]

		// Only apply to hex letters (a-f)
		if char >= 'a' && char <= 'f' {
			// Check if the corresponding bit in the hash is high
			byteIdx := i / 2
			bitIdx := (i % 2) * 4

			// Get the nibble
			nibble := (hash[byteIdx] >> (4 - bitIdx)) & 0x0F

			// If the high bit of the nibble is set, uppercase the character
			if nibble >= 8 {
				char = char - 'a' + 'A'
			}
		}

		result[i] = char
	}

	return string(result)
}

// validateEIP55Checksum validates the EIP-55 checksum of a hex string
func validateEIP55Checksum(hexStr string) bool {
	expected := applyEIP55Checksum(hexStr)
	return hexStr == expected
}

// hasMixedCase checks if a string contains both upper and lowercase letters
func hasMixedCase(s string) bool {
	hasUpper := false
	hasLower := false

	for _, c := range s {
		if c >= 'A' && c <= 'F' {
			hasUpper = true
		} else if c >= 'a' && c <= 'f' {
			hasLower = true
		}

		if hasUpper && hasLower {
			return true
		}
	}

	return false
}

// calculateLuhnChecksum calculates the Luhn checksum character for a string
// Uses the Luhn algorithm (mod 10) commonly used in credit cards and other checksums
func calculateLuhnChecksum(s string) byte {
	sum := 0
	odd := true

	// Process string from right to left
	for i := len(s) - 1; i >= 0; i-- {
		char := s[i]

		// Convert character to digit value (0-9 for digits, 10-35 for a-z, 36-61 for A-Z)
		var digit int
		if char >= '0' && char <= '9' {
			digit = int(char - '0')
		} else if char >= 'a' && char <= 'z' {
			digit = int(char-'a') + 10
		} else if char >= 'A' && char <= 'Z' {
			digit = int(char-'A') + 36
		} else {
			// Skip non-alphanumeric characters
			continue
		}

		// Double every second digit
		if odd {
			digit *= 2
			// If result is > 9, subtract 9 (equivalent to adding digits)
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		odd = !odd
	}

	// Calculate checksum digit
	checksum := (10 - (sum % 10)) % 10

	// Convert to ASCII character (0-9)
	return byte('0' + checksum)
}

// ValidateAddress validates an address string and returns whether it's valid
// This is a convenience function that wraps ParseAddress
func ValidateAddress(addrStr string) bool {
	_, _, err := ParseAddress(addrStr)
	return err == nil
}

// AddressFromBytes creates an Address from a 32-byte slice
func AddressFromBytes(bytes []byte) (Address, error) {
	var addr Address
	if len(bytes) != 32 {
		return addr, fmt.Errorf("address must be 32 bytes, got %d", len(bytes))
	}
	copy(addr[:], bytes)
	return addr, nil
}
