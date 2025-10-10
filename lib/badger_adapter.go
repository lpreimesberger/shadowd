package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/dgraph-io/badger/v4"
)

// BadgerDBAdapter wraps BadgerDB to implement a simple KV interface compatible with CometBFT-DB
type BadgerDBAdapter struct {
	db *badger.DB
}

// cleanStaleLock removes the LOCK file if the process that created it is dead
func cleanStaleLock(dbPath string) error {
	lockFile := filepath.Join(dbPath, "LOCK")
	data, err := os.ReadFile(lockFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No lock file, all good
		}
		return err
	}

	// Parse PID from lock file
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		fmt.Printf("[BadgerDB] Warning: Invalid PID in LOCK file: %s\n", string(data))
		// Remove invalid lock file
		return os.Remove(lockFile)
	}

	// Check if process is still running
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist, remove stale lock
		fmt.Printf("[BadgerDB] Removing stale lock for dead process %d\n", pid)
		return os.Remove(lockFile)
	}

	// Try to signal process 0 (doesn't actually send signal, just checks if process exists)
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process is dead, remove stale lock
		fmt.Printf("[BadgerDB] Removing stale lock for dead process %d\n", pid)
		return os.Remove(lockFile)
	}

	// Process is alive, lock is valid
	fmt.Printf("[BadgerDB] Lock is held by running process %d\n", pid)
	return fmt.Errorf("database is locked by running process %d", pid)
}

// NewBadgerDBAdapter creates a new BadgerDB adapter
func NewBadgerDBAdapter(dbPath string) (*BadgerDBAdapter, error) {
	fmt.Printf("[BadgerDB] Opening database at %s...\n", dbPath)

	// Check for stale lock before opening
	if err := cleanStaleLock(dbPath); err != nil {
		return nil, err
	}

	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // Disable BadgerDB logging

	// Database lock options to prevent deadlocks
	opts.BypassLockGuard = false // Keep lock guard for safety
	opts.DetectConflicts = false  // Disable conflict detection for faster txns
	opts.NumVersionsToKeep = 1    // Keep only latest version

	// Open the database
	fmt.Printf("[BadgerDB] Calling badger.Open()...\n")
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger db: %w", err)
	}
	fmt.Printf("[BadgerDB] Successfully opened database at %s\n", dbPath)

	return &BadgerDBAdapter{db: db}, nil
}

// Get retrieves a value by key
func (b *BadgerDBAdapter) Get(key []byte) ([]byte, error) {
	var value []byte
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil // Return nil value, no error
			}
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})

	if err != nil {
		return nil, err
	}
	return value, nil
}

// Set stores a key-value pair
func (b *BadgerDBAdapter) Set(key, value []byte) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// Iterator creates an iterator for a given prefix
func (b *BadgerDBAdapter) Iterator(start, end []byte) (Iterator, error) {
	txn := b.db.NewTransaction(false)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = start

	it := txn.NewIterator(opts)
	it.Rewind()

	return &BadgerIterator{
		txn: txn,
		it:  it,
		end: end,
	}, nil
}

// Close closes the database
func (b *BadgerDBAdapter) Close() error {
	return b.db.Close()
}

// BadgerIterator wraps BadgerDB iterator to match CometBFT-DB interface
type BadgerIterator struct {
	txn *badger.Txn
	it  *badger.Iterator
	end []byte
}

// Valid returns true if the iterator is positioned at a valid key
func (bi *BadgerIterator) Valid() bool {
	return bi.it.Valid()
}

// Next advances the iterator to the next key
func (bi *BadgerIterator) Next() {
	bi.it.Next()
}

// Key returns the current key
func (bi *BadgerIterator) Key() []byte {
	if !bi.it.Valid() {
		return nil
	}
	return bi.it.Item().KeyCopy(nil)
}

// Value returns the current value
func (bi *BadgerIterator) Value() []byte {
	if !bi.it.Valid() {
		return nil
	}
	val, err := bi.it.Item().ValueCopy(nil)
	if err != nil {
		return nil
	}
	return val
}

// Close closes the iterator and transaction
func (bi *BadgerIterator) Close() error {
	bi.it.Close()
	bi.txn.Discard()
	return nil
}

// Iterator interface matches CometBFT-DB iterator interface
type Iterator interface {
	Valid() bool
	Next()
	Key() []byte
	Value() []byte
	Close() error
}
