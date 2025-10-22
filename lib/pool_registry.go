package lib

import (
	"fmt"
	"sync"
)

// PoolRegistry manages all liquidity pools in the system
type PoolRegistry struct {
	pools map[string]*LiquidityPool // poolID -> pool
	mutex sync.RWMutex
}

// NewPoolRegistry creates a new pool registry
func NewPoolRegistry() *PoolRegistry {
	return &PoolRegistry{
		pools: make(map[string]*LiquidityPool),
	}
}

// RegisterPool registers a new liquidity pool
func (pr *PoolRegistry) RegisterPool(pool *LiquidityPool) error {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	// Check if pool already exists
	if _, exists := pr.pools[pool.PoolID]; exists {
		return fmt.Errorf("pool %s already exists", pool.PoolID)
	}

	// Validate pool
	if pool.TokenA == "" || pool.TokenB == "" {
		return fmt.Errorf("invalid pool: token IDs cannot be empty")
	}
	if pool.TokenA == pool.TokenB {
		return fmt.Errorf("invalid pool: token A and B must be different")
	}
	if pool.ReserveA == 0 || pool.ReserveB == 0 {
		return fmt.Errorf("invalid pool: reserves cannot be zero")
	}
	if err := ValidateFeePercent(pool.FeePercent); err != nil {
		return err
	}

	// Store in memory
	pr.pools[pool.PoolID] = pool

	fmt.Printf("[PoolRegistry] ✅ Registered pool %s: %s/%s (K=%d, fee=%d bps)\n",
		pool.PoolID[:16], pool.TokenA[:8], pool.TokenB[:8], pool.K, pool.FeePercent)

	return nil
}

// GetPool retrieves a pool by ID
func (pr *PoolRegistry) GetPool(poolID string) (*LiquidityPool, error) {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	pool, exists := pr.pools[poolID]
	if !exists {
		return nil, fmt.Errorf("pool %s not found", poolID)
	}

	return pool, nil
}

// UpdatePool updates an existing pool (takes pool object)
func (pr *PoolRegistry) UpdatePool(pool *LiquidityPool) error {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	if _, exists := pr.pools[pool.PoolID]; !exists {
		return fmt.Errorf("pool %s not found", pool.PoolID)
	}

	// Update the pool in registry
	pr.pools[pool.PoolID] = pool

	return nil
}

// UpdatePoolReserves updates a pool's reserves and LP token supply
func (pr *PoolRegistry) UpdatePoolReserves(poolID string, reserveA, reserveB, lpTokenSupply uint64) error {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	pool, exists := pr.pools[poolID]
	if !exists {
		return fmt.Errorf("pool %s not found", poolID)
	}

	// Update reserves and recalculate K
	pool.ReserveA = reserveA
	pool.ReserveB = reserveB
	pool.LPTokenSupply = lpTokenSupply
	pool.K = CalculateK(reserveA, reserveB)

	return nil
}

// GetAllPools returns all registered pools
func (pr *PoolRegistry) GetAllPools() []*LiquidityPool {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	pools := make([]*LiquidityPool, 0, len(pr.pools))
	for _, pool := range pr.pools {
		pools = append(pools, pool)
	}
	return pools
}

// FindPoolByTokens finds a pool by token pair (order doesn't matter)
func (pr *PoolRegistry) FindPoolByTokens(tokenA, tokenB string) (*LiquidityPool, error) {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	for _, pool := range pr.pools {
		if (pool.TokenA == tokenA && pool.TokenB == tokenB) ||
			(pool.TokenA == tokenB && pool.TokenB == tokenA) {
			return pool, nil
		}
	}

	return nil, fmt.Errorf("no pool found for token pair %s/%s", tokenA, tokenB)
}


// GetPoolCount returns the number of registered pools
func (pr *PoolRegistry) GetPoolCount() int {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	return len(pr.pools)
}
