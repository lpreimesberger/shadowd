package lib

import (
	"fmt"
	"math"
)

// LiquidityPool represents an AMM-style constant product liquidity pool
type LiquidityPool struct {
	PoolID        string `json:"pool_id"`         // Hash of creation transaction
	TokenA        string `json:"token_a"`         // First token ID
	TokenB        string `json:"token_b"`         // Second token ID
	ReserveA      uint64 `json:"reserve_a"`       // Current locked amount of token A
	ReserveB      uint64 `json:"reserve_b"`       // Current locked amount of token B
	LPTokenID     string `json:"lp_token_id"`     // LP token ID (minted for this pool)
	LPTokenSupply uint64 `json:"lp_token_supply"` // Total LP tokens minted
	FeePercent    uint64 `json:"fee_percent"`     // Fee in basis points (30 = 0.3%, 100 = 1%)
	K             uint64 `json:"k"`               // Constant product (reserve_a * reserve_b)
	CreatedAt     uint64 `json:"created_at"`      // Block height when created
}

// CreatePoolData represents the data stored in a TX_CREATE_POOL transaction
type CreatePoolData struct {
	TokenA       string `json:"token_a"`        // First token ID
	TokenB       string `json:"token_b"`        // Second token ID
	AmountA      uint64 `json:"amount_a"`       // Initial amount of token A
	AmountB      uint64 `json:"amount_b"`       // Initial amount of token B
	FeePercent   uint64 `json:"fee_percent"`    // Fee in basis points (10-1000 = 0.1%-10%)
	PoolName     string `json:"pool_name"`      // Optional custom pool name
	PoolAddress  Address `json:"pool_address"`  // Address that created the pool
}

// AddLiquidityData represents the data stored in a TX_ADD_LIQUIDITY transaction
type AddLiquidityData struct {
	PoolID       string `json:"pool_id"`        // Pool to add liquidity to
	AmountA      uint64 `json:"amount_a"`       // Amount of token A to add
	AmountB      uint64 `json:"amount_b"`       // Amount of token B to add
	MinLPTokens  uint64 `json:"min_lp_tokens"`  // Minimum LP tokens to receive (slippage protection)
}

// RemoveLiquidityData represents the data stored in a TX_REMOVE_LIQUIDITY transaction
type RemoveLiquidityData struct {
	PoolID       string `json:"pool_id"`        // Pool to remove liquidity from
	LPTokens     uint64 `json:"lp_tokens"`      // Amount of LP tokens to burn
	MinAmountA   uint64 `json:"min_amount_a"`   // Minimum amount of token A to receive
	MinAmountB   uint64 `json:"min_amount_b"`   // Minimum amount of token B to receive
}

// SwapData represents the data stored in a TX_SWAP transaction
type SwapData struct {
	PoolID        string `json:"pool_id"`         // Pool to swap through
	TokenIn       string `json:"token_in"`        // Token being provided
	AmountIn      uint64 `json:"amount_in"`       // Amount of token being provided
	MinAmountOut  uint64 `json:"min_amount_out"`  // Minimum amount of output token (slippage protection)
}

// CalculateLPTokens calculates LP tokens to mint using sqrt(a * b)
func CalculateLPTokens(amountA, amountB uint64) uint64 {
	// Use floating point for sqrt calculation
	a := float64(amountA)
	b := float64(amountB)
	result := math.Sqrt(a * b)
	return uint64(result)
}

// CalculateSwapOutput calculates output amount for a swap with fee
// Uses constant product formula: (x + Δx * (1 - fee)) * (y - Δy) = k
func CalculateSwapOutput(amountIn, reserveIn, reserveOut, feePercent uint64) uint64 {
	// Apply fee to input (fee in basis points: 30 = 0.3%)
	feeBasisPoints := uint64(10000) // 100% = 10000 basis points
	amountInWithFee := amountIn * (feeBasisPoints - feePercent) / feeBasisPoints

	// Constant product formula: amountOut = (amountInWithFee * reserveOut) / (reserveIn + amountInWithFee)
	numerator := amountInWithFee * reserveOut
	denominator := reserveIn + amountInWithFee

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// CalculateProportionalAmount calculates the required amount of token B given amount of token A
// to maintain the pool's current ratio
func CalculateProportionalAmount(amountA, reserveA, reserveB uint64) uint64 {
	if reserveA == 0 {
		return 0
	}
	return (amountA * reserveB) / reserveA
}

// ValidatePoolRatio checks if provided amounts match the pool's ratio within tolerance
func ValidatePoolRatio(amountA, amountB, reserveA, reserveB uint64, tolerancePercent uint64) bool {
	if reserveA == 0 || reserveB == 0 {
		return false
	}

	// Calculate expected amount B based on pool ratio
	expectedB := CalculateProportionalAmount(amountA, reserveA, reserveB)

	// Check if provided amount is within tolerance (e.g., 1% = allow 1% deviation)
	tolerance := (expectedB * tolerancePercent) / 100
	diff := int64(amountB) - int64(expectedB)
	if diff < 0 {
		diff = -diff
	}

	return uint64(diff) <= tolerance
}

// GetPoolName generates a pool name based on token tickers and pool ID
func GetPoolName(tickerA, tickerB, poolID string) string {
	// Format: "SHADOW / BOOBS - abc12345"
	return fmt.Sprintf("%s / %s - %s", tickerA, tickerB, poolID[:8])
}

// ValidateFeePercent checks if fee is within allowed range (10-1000 basis points = 0.1%-10%)
func ValidateFeePercent(feePercent uint64) error {
	if feePercent < 10 || feePercent > 1000 {
		return fmt.Errorf("fee must be between 0.1%% and 10%% (10-1000 basis points), got %d", feePercent)
	}
	return nil
}

// GetLPTokenName generates a unique name for the LP token using pool ID
func GetLPTokenName(tickerA, tickerB, poolID string) string {
	// Use only alphanumeric characters - no hyphens or spaces allowed
	// Include short hash of pool ID to ensure uniqueness across multiple pools of same pair
	if len(poolID) >= 8 {
		return fmt.Sprintf("%s%sLP%s", tickerA, tickerB, poolID[:8])
	}
	return fmt.Sprintf("%s%sLP%s", tickerA, tickerB, poolID)
}

// CalculateK calculates the constant product K
func CalculateK(reserveA, reserveB uint64) uint64 {
	// For very large numbers, this could overflow
	// In production, might want to use big.Int
	return reserveA * reserveB
}
