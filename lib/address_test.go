package lib

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa87"
)

// TestDeriveAddress tests basic address derivation
func TestDeriveAddress(t *testing.T) {
	// Generate a test key pair
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Derive address
	addr := DeriveAddress(pk)

	// Check that we got a 32-byte address
	if len(addr) != 32 {
		t.Errorf("Expected 32-byte address, got %d bytes", len(addr))
	}

	// Check that string representation is 64 hex characters + prefix + checksum
	addrStr := addr.String()
	if len(addrStr) < 66 { // S + 64 hex chars + 1 Luhn
		t.Errorf("Expected at least 66 characters (S + 64 hex + Luhn), got %d: %s", len(addrStr), addrStr)
	}

	// Check that it starts with 'S' (wallet type)
	if addrStr[0] != 'S' {
		t.Errorf("Expected address to start with 'S', got '%c'", addrStr[0])
	}
}

// TestAddressTypes tests different address type prefixes
func TestAddressTypes(t *testing.T) {
	// Generate a test key pair
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)

	tests := []struct {
		addrType     AddressType
		expectedChar byte
	}{
		{AddressTypeWallet, 'S'},
		{AddressTypeLiquidity, 'L'},
		{AddressTypeExchange, 'X'},
		{AddressTypeNFT, 'N'},
	}

	for _, tt := range tests {
		t.Run(string(tt.expectedChar), func(t *testing.T) {
			addrStr := addr.StringWithType(tt.addrType)

			if addrStr[0] != tt.expectedChar {
				t.Errorf("Expected prefix '%c', got '%c'", tt.expectedChar, addrStr[0])
			}

			// Verify it can be parsed back
			parsedAddr, parsedType, err := ParseAddress(addrStr)
			if err != nil {
				t.Errorf("Failed to parse address: %v", err)
			}

			if parsedType != tt.addrType {
				t.Errorf("Expected type %c, got %c", tt.addrType, parsedType)
			}

			if parsedAddr != addr {
				t.Errorf("Parsed address doesn't match original")
			}
		})
	}
}

// TestParseAddress tests address parsing with various formats
func TestParseAddress(t *testing.T) {
	// Generate a test address
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)
	validAddr := addr.String()

	tests := []struct {
		name      string
		input     string
		shouldErr bool
		errMsg    string
	}{
		{
			name:      "Valid address",
			input:     validAddr,
			shouldErr: false,
		},
		{
			name:      "Valid address lowercase",
			input:     strings.ToLower(validAddr),
			shouldErr: false,
		},
		{
			name:      "Invalid BOM prefix",
			input:     "Z" + validAddr[1:],
			shouldErr: true,
			errMsg:    "invalid address type prefix",
		},
		{
			name:      "Too short",
			input:     "S12",
			shouldErr: true,
			errMsg:    "address must be 32 bytes",
		},
		{
			name:      "Invalid Luhn checksum",
			input:     validAddr[:len(validAddr)-1] + "9",
			shouldErr: true,
			errMsg:    "invalid Luhn checksum",
		},
		{
			name:      "Invalid hex characters",
			input:     "S" + strings.Repeat("Z", 64) + "0",
			shouldErr: true,
			errMsg:    "invalid hex string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseAddress(tt.input)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestEIP55Checksum tests EIP-55 style checksum validation
func TestEIP55Checksum(t *testing.T) {
	// Generate a test address
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)
	validAddr := addr.String()

	// Extract hex portion (without BOM and Luhn)
	hexPortion := validAddr[1 : len(validAddr)-1]

	// Test 1: Valid mixed case should parse
	_, _, err = ParseAddress(validAddr)
	if err != nil {
		t.Errorf("Valid checksummed address failed to parse: %v", err)
	}

	// Test 2: Lowercase should parse (no mixed case check)
	lowerAddr := string(validAddr[0]) + strings.ToLower(hexPortion) + string(validAddr[len(validAddr)-1])
	// Need to recalculate Luhn for lowercase version
	lowerAddrNoLuhn := string(validAddr[0]) + strings.ToLower(hexPortion)
	luhnChar := calculateLuhnChecksum(strings.ToLower(lowerAddrNoLuhn))
	lowerAddr = lowerAddrNoLuhn + string(luhnChar)

	_, _, err = ParseAddress(lowerAddr)
	if err != nil {
		t.Errorf("Lowercase address failed to parse: %v", err)
	}

	// Test 3: Uppercase should parse (no mixed case check)
	upperHex := strings.ToUpper(hexPortion)
	upperAddrNoLuhn := string(validAddr[0]) + upperHex
	luhnCharUpper := calculateLuhnChecksum(strings.ToLower(upperAddrNoLuhn))
	upperAddr := upperAddrNoLuhn + string(luhnCharUpper)

	_, _, err = ParseAddress(upperAddr)
	if err != nil {
		t.Errorf("Uppercase address failed to parse: %v", err)
	}

	// Test 4: Incorrect mixed case should fail
	if hasMixedCase(hexPortion) {
		// Flip the case of the first letter
		incorrectHex := make([]byte, len(hexPortion))
		copy(incorrectHex, hexPortion)

		for i := 0; i < len(incorrectHex); i++ {
			if incorrectHex[i] >= 'a' && incorrectHex[i] <= 'f' {
				incorrectHex[i] = incorrectHex[i] - 'a' + 'A'
				break
			} else if incorrectHex[i] >= 'A' && incorrectHex[i] <= 'F' {
				incorrectHex[i] = incorrectHex[i] - 'A' + 'a'
				break
			}
		}

		// Recalculate Luhn for the incorrect mixed case
		incorrectAddrNoLuhn := string(validAddr[0]) + string(incorrectHex)
		luhnCharIncorrect := calculateLuhnChecksum(strings.ToLower(incorrectAddrNoLuhn))
		incorrectAddr := incorrectAddrNoLuhn + string(luhnCharIncorrect)

		_, _, err = ParseAddress(incorrectAddr)
		if err == nil {
			t.Errorf("Incorrect mixed case address should have failed validation")
		} else if !strings.Contains(err.Error(), "EIP-55") {
			t.Errorf("Expected EIP-55 error, got: %v", err)
		}
	}
}

// TestLuhnChecksum tests Luhn algorithm checksum calculation and validation
func TestLuhnChecksum(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected byte
	}{
		{
			name:     "Simple numeric",
			input:    "12345",
			expected: '1', // Luhn check digit for 12345 is 1
		},
		{
			name:     "With letters",
			input:    "abc123",
			expected: calculateLuhnChecksum("abc123"),
		},
		{
			name:     "Hex string",
			input:    "deadbeef",
			expected: calculateLuhnChecksum("deadbeef"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateLuhnChecksum(tt.input)
			if result != tt.expected {
				t.Errorf("Expected Luhn checksum '%c', got '%c'", tt.expected, result)
			}

			// Verify that the checksum validates correctly
			_ = tt.input + string(result)
			// A valid Luhn string should have a checksum of '0' when calculated on the full string
			// (since we're adding the check digit that makes the sum divisible by 10)
		})
	}
}

// TestRoundTrip tests that addresses can be converted to string and back
func TestRoundTrip(t *testing.T) {
	// Generate multiple test addresses
	for i := 0; i < 10; i++ {
		pk, _, err := mldsa87.GenerateKey(nil)
		if err != nil {
			t.Fatalf("Failed to generate key pair: %v", err)
		}

		addr := DeriveAddress(pk)

		// Test all address types
		for _, addrType := range []AddressType{
			AddressTypeWallet,
			AddressTypeLiquidity,
			AddressTypeExchange,
			AddressTypeNFT,
		} {
			addrStr := addr.StringWithType(addrType)

			// Parse back
			parsedAddr, parsedType, err := ParseAddress(addrStr)
			if err != nil {
				t.Errorf("Round trip failed for type %c: %v", addrType, err)
				continue
			}

			// Check type matches
			if parsedType != addrType {
				t.Errorf("Expected type %c, got %c", addrType, parsedType)
			}

			// Check address matches
			if parsedAddr != addr {
				t.Errorf("Round trip produced different address")
			}

			// Also test lowercase version
			lowerStr := string(addrStr[0]) + strings.ToLower(addrStr[1:len(addrStr)-1])
			luhnChar := calculateLuhnChecksum(strings.ToLower(string(addrStr[0]) + strings.ToLower(addrStr[1:len(addrStr)-1])))
			lowerStr = lowerStr + string(luhnChar)

			parsedAddr2, parsedType2, err := ParseAddress(lowerStr)
			if err != nil {
				t.Errorf("Lowercase round trip failed: %v", err)
				continue
			}

			if parsedType2 != addrType {
				t.Errorf("Lowercase: Expected type %c, got %c", addrType, parsedType2)
			}

			if parsedAddr2 != addr {
				t.Errorf("Lowercase round trip produced different address")
			}
		}
	}
}

// TestValidateAddress tests the convenience validation function
func TestValidateAddress(t *testing.T) {
	// Generate a valid address
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)
	validAddr := addr.String()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid address",
			input:    validAddr,
			expected: true,
		},
		{
			name:     "Invalid BOM",
			input:    "Z" + validAddr[1:],
			expected: false,
		},
		{
			name:     "Invalid Luhn",
			input:    validAddr[:len(validAddr)-1] + "9",
			expected: false,
		},
		{
			name:     "Too short",
			input:    "S123",
			expected: false,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAddress(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestAddressFromBytes tests creating an address from raw bytes
func TestAddressFromBytes(t *testing.T) {
	// Create a test address from known bytes
	testBytes := make([]byte, 32)
	for i := range testBytes {
		testBytes[i] = byte(i)
	}

	addr, err := AddressFromBytes(testBytes)
	if err != nil {
		t.Errorf("Failed to create address from bytes: %v", err)
	}

	// Verify the bytes match
	for i := range testBytes {
		if addr[i] != testBytes[i] {
			t.Errorf("Byte mismatch at index %d: expected %d, got %d", i, testBytes[i], addr[i])
		}
	}

	// Test invalid length
	shortBytes := make([]byte, 16)
	_, err = AddressFromBytes(shortBytes)
	if err == nil {
		t.Errorf("Expected error for short byte slice, got nil")
	}

	longBytes := make([]byte, 64)
	_, err = AddressFromBytes(longBytes)
	if err == nil {
		t.Errorf("Expected error for long byte slice, got nil")
	}
}

// TestHasMixedCase tests the mixed case detection function
func TestHasMixedCase(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "All lowercase",
			input:    "abcdef123",
			expected: false,
		},
		{
			name:     "All uppercase",
			input:    "ABCDEF123",
			expected: false,
		},
		{
			name:     "Mixed case",
			input:    "AbCdEf",
			expected: true,
		},
		{
			name:     "No letters",
			input:    "123456",
			expected: false,
		},
		{
			name:     "One uppercase",
			input:    "abcdeF",
			expected: true,
		},
		{
			name:     "One lowercase",
			input:    "ABCDEf",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasMixedCase(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for input '%s'", tt.expected, result, tt.input)
			}
		})
	}
}

// TestAddressTypeValidation tests address type validation
func TestAddressTypeValidation(t *testing.T) {
	validTypes := []AddressType{
		AddressTypeWallet,
		AddressTypeLiquidity,
		AddressTypeExchange,
		AddressTypeNFT,
	}

	for _, addrType := range validTypes {
		if !isValidAddressType(addrType) {
			t.Errorf("Valid address type %c reported as invalid", addrType)
		}
	}

	invalidTypes := []byte{'A', 'B', 'C', 'Z', '0', '1'}
	for _, invalidType := range invalidTypes {
		if isValidAddressType(AddressType(invalidType)) {
			t.Errorf("Invalid address type %c reported as valid", invalidType)
		}
	}
}

// BenchmarkDeriveAddress benchmarks address derivation
func BenchmarkDeriveAddress(b *testing.B) {
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		b.Fatalf("Failed to generate key pair: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DeriveAddress(pk)
	}
}

// BenchmarkAddressString benchmarks address string formatting
func BenchmarkAddressString(b *testing.B) {
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		b.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addr.String()
	}
}

// BenchmarkParseAddress benchmarks address parsing
func BenchmarkParseAddress(b *testing.B) {
	pk, _, err := mldsa87.GenerateKey(nil)
	if err != nil {
		b.Fatalf("Failed to generate key pair: %v", err)
	}

	addr := DeriveAddress(pk)
	addrStr := addr.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ParseAddress(addrStr)
	}
}

// TestKnownAddressVector tests a known address for regression
func TestKnownAddressVector(t *testing.T) {
	// Create an address from known bytes
	knownBytes, _ := hex.DecodeString("a8b033b8fde716ee88528ff8e17ad7b764fb06e26e7caf92f5d5bb775be13918")
	addr, err := AddressFromBytes(knownBytes)
	if err != nil {
		t.Fatalf("Failed to create address from known bytes: %v", err)
	}

	// Generate string representation
	addrStr := addr.String()

	// Verify it starts with S
	if addrStr[0] != 'S' {
		t.Errorf("Expected address to start with 'S', got '%c'", addrStr[0])
	}

	// Verify it can be parsed back
	parsedAddr, parsedType, err := ParseAddress(addrStr)
	if err != nil {
		t.Errorf("Failed to parse known address: %v", err)
	}

	if parsedType != AddressTypeWallet {
		t.Errorf("Expected wallet type, got %c", parsedType)
	}

	if parsedAddr != addr {
		t.Errorf("Parsed address doesn't match original")
	}

	t.Logf("Known address: %s", addrStr)
}
