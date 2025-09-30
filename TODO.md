# Shadowy Blockchain TODO List

## 🚨 **CRITICAL ISSUES**

### 1. Fix plotlib ML-DSA87 crash in mining
**Status**: Blocking real proof-of-space mining
**Error**: `runtime error: invalid memory address or nil pointer dereference` in `plotlib/pkg/storageproof.NewSolution()`
**ROOT CAUSE**: Plot files contain nil/invalid ML-DSA87 private keys. Plotlib tries to sign with plot file keys, not miner keys.
**Stack trace**:
```
github.com/cloudflare/circl/sign/mldsa/mldsa87.(*PrivateKey).Sign(0xc000dbe000, ...)
github.com/lpreimesberger/plotlib/pkg/storageproof.NewSolution({0xc000d8ff28, 0x20, 0x20}, 0x65, 0xc000dbe000)
github.com/lpreimesberger/plotlib/pkg/storageproof.(*PlotCollection).LookUp(0x1748e20?, {0xc000152aa0, 0x20, 0x28?})
shadowy/lib.GenerateProofOfSpace({0x8, 0xcf, 0x7e, 0x36, ...}, ...)
```

**Root cause**: ML-DSA87 private key is nil when trying to sign in plotlib
**Current workaround**: Mining disabled in `lib/tendermint.go:454-468`
**Impact**: Coinbase rewards work but no real proof-of-space mining

## 🧪 **TESTING PRIORITIES**

### 2. Test complete transaction flow end-to-end
**Status**: Ready for testing
**Steps needed**:
- [ ] Start node and verify balance accumulation (should gain ~50 SHADOW per block)
- [ ] Test `/api/balance` endpoint returns real UTXO data
- [ ] Test `/api/utxos` endpoint shows coinbase UTXOs
- [ ] Create and send transaction between addresses
- [ ] Verify UTXO spend/create cycle works
- [ ] Test transaction validation against UTXO set

### 3. Multi-node P2P transaction testing
**Status**: Nodes talking, need transaction propagation test
**Steps needed**:
- [ ] Start 2+ nodes with seed connections
- [ ] Verify mempool transaction gossip between nodes
- [ ] Test that transactions broadcast across network
- [ ] Verify consistent UTXO state across nodes

## 🔧 **ENHANCEMENTS**

### 4. Improve API error handling and responses
**Status**: Basic implementation complete
**Improvements needed**:
- [ ] Better error messages for failed UTXO lookups
- [ ] Add transaction status/receipt endpoints
- [ ] Add mempool query endpoints
- [ ] Improve address validation error messages

### 5. Add transaction creation helpers
**Status**: Basic send transaction implemented
**Enhancements**:
- [ ] UTXO selection algorithms (coin selection)
- [ ] Automatic fee calculation
- [ ] Multi-output transaction support via API
- [ ] Transaction templates for common operations

### 6. Performance optimizations
**Status**: Working but not optimized
**Areas for improvement**:
- [ ] UTXO store indexing optimizations
- [ ] Cache frequently accessed UTXOs
- [ ] Batch UTXO operations for better performance
- [ ] Database connection pooling

## 🛡️ **SECURITY & VALIDATION**

### 7. Enhanced transaction validation
**Status**: Basic validation implemented
**Additional checks needed**:
- [ ] Signature verification in transaction validation
- [ ] Double-spend protection enforcement
- [ ] Transaction size and complexity limits
- [ ] Fee validation and minimum fee enforcement

### 8. Network security improvements
**Status**: Basic P2P working
**Hardening needed**:
- [ ] Rate limiting for API endpoints
- [ ] Transaction flood protection
- [ ] Peer reputation system
- [ ] Invalid transaction penalty system

## 📚 **DOCUMENTATION**

### 9. Complete API documentation
**Status**: Basic docs in API.md
**Missing sections**:
- [ ] Transaction format specifications
- [ ] Error code reference
- [ ] Rate limiting documentation
- [ ] Examples for all endpoints

### 10. Developer documentation
**Status**: Code comments exist
**Needed**:
- [ ] Architecture overview document
- [ ] UTXO state management explanation
- [ ] Mining/consensus process documentation
- [ ] Database schema documentation

## 🚀 **DEPLOYMENT & OPERATIONS**

### 11. Production deployment preparation
**Status**: Development ready
**Production needs**:
- [ ] Docker containerization
- [ ] Configuration management
- [ ] Logging and monitoring integration
- [ ] Health check endpoints
- [ ] Graceful shutdown procedures

### 12. Network bootstrapping
**Status**: Single node working
**Network setup**:
- [ ] Genesis block with initial UTXOs for testing
- [ ] Seed node configuration
- [ ] Network upgrade procedures
- [ ] Checkpoint system for fast sync

---

## 📊 **CURRENT ARCHITECTURE STATUS**

### ✅ **WORKING COMPONENTS**
- Block production (Tendermint consensus)
- UTXO persistence (LevelDB)
- Transaction validation pipeline
- HTTP API with real data
- Mempool integration
- Coinbase transaction creation
- Address-based UTXO lookups
- Balance calculations

### ⚠️ **DISABLED COMPONENTS**
- Real proof-of-space mining (plotlib crash)
- ML-DSA87 signature generation in farming

### 🎯 **IMMEDIATE PRIORITIES**
1. **Fix plotlib crash** - Critical for real mining
2. **Test transaction flow** - Verify the complete system works
3. **Multi-node testing** - Ensure P2P works correctly

---

## 🐛 **KNOWN BUGS & WORKAROUNDS**

### plotlib ML-DSA87 Crash
**Workaround**: Mining disabled, coinbase still awards tokens for testing
**Location**: `lib/tendermint.go` lines 454-468
**Fix needed**: Debug why ML-DSA87 private key is nil in plotlib

### UTXO API Performance
**Issue**: No pagination for large UTXO sets
**Workaround**: Acceptable for testing, needs pagination for production

### Transaction Fee Handling
**Issue**: Fee calculation is basic
**Workaround**: Fixed fees work for testing

---

*Last updated: Based on conversation state before restart*
*Next session: Focus on plotlib crash investigation and transaction flow testing*