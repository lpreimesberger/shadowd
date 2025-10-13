package lib

import (
	"bytes"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// BoltDBAdapter wraps BoltDB to implement a simple KV interface
type BoltDBAdapter struct {
	db         *bolt.DB
	bucketName []byte
}

// NewBoltDBAdapter creates a new BoltDB adapter
func NewBoltDBAdapter(dbPath string) (*BoltDBAdapter, error) {
	fmt.Printf("[BoltDB] Opening database at %s...\n", dbPath)

	// Open BoltDB with default options
	// BoltDB uses flock on the file itself - much more reliable than lock files!
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt db: %w", err)
	}

	bucketName := []byte("default")

	// Create bucket if it doesn't exist
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	fmt.Printf("[BoltDB] Successfully opened database at %s\n", dbPath)

	return &BoltDBAdapter{
		db:         db,
		bucketName: bucketName,
	}, nil
}

// Get retrieves a value by key
func (b *BoltDBAdapter) Get(key []byte) ([]byte, error) {
	var value []byte
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.bucketName)
		if bucket == nil {
			return fmt.Errorf("bucket not found")
		}
		val := bucket.Get(key)
		if val != nil {
			// Make a copy since bolt's slice is only valid during transaction
			value = make([]byte, len(val))
			copy(value, val)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return value, nil
}

// Set stores a key-value pair
func (b *BoltDBAdapter) Set(key, value []byte) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.bucketName)
		if bucket == nil {
			return fmt.Errorf("bucket not found")
		}
		return bucket.Put(key, value)
	})
}

// Iterator creates an iterator for a given prefix
func (b *BoltDBAdapter) Iterator(start, end []byte) (Iterator, error) {
	tx, err := b.db.Begin(false)
	if err != nil {
		return nil, err
	}

	bucket := tx.Bucket(b.bucketName)
	if bucket == nil {
		tx.Rollback()
		return nil, fmt.Errorf("bucket not found")
	}

	cursor := bucket.Cursor()

	return &BoltIterator{
		tx:     tx,
		cursor: cursor,
		start:  start,
		end:    end,
		first:  true,
	}, nil
}

// Close closes the database
func (b *BoltDBAdapter) Close() error {
	return b.db.Close()
}

// BoltIterator wraps BoltDB cursor to match our Iterator interface
type BoltIterator struct {
	tx     *bolt.Tx
	cursor *bolt.Cursor
	start  []byte
	end    []byte
	key    []byte
	value  []byte
	first  bool
	valid  bool
}

// Valid returns true if the iterator is positioned at a valid key
func (bi *BoltIterator) Valid() bool {
	if bi.first {
		// Position cursor on first call
		bi.first = false
		if bi.start == nil {
			bi.key, bi.value = bi.cursor.First()
		} else {
			bi.key, bi.value = bi.cursor.Seek(bi.start)
		}
		bi.valid = (bi.key != nil)
	}

	if !bi.valid {
		return false
	}

	// Check if we've passed the end
	if bi.end != nil && bi.key != nil && string(bi.key) >= string(bi.end) {
		return false
	}

	// If no explicit end, check if key still has the start prefix (common pattern for prefix scans)
	if bi.end == nil && bi.start != nil && bi.key != nil {
		if !bytes.HasPrefix(bi.key, bi.start) {
			return false
		}
	}

	return bi.key != nil
}

// Next advances the iterator to the next key
func (bi *BoltIterator) Next() {
	bi.key, bi.value = bi.cursor.Next()
	bi.valid = (bi.key != nil)
}

// Key returns the current key
func (bi *BoltIterator) Key() []byte {
	if bi.key == nil {
		return nil
	}
	// Make a copy since bolt's slice is only valid during transaction
	keyCopy := make([]byte, len(bi.key))
	copy(keyCopy, bi.key)
	return keyCopy
}

// Value returns the current value
func (bi *BoltIterator) Value() []byte {
	if bi.value == nil {
		return nil
	}
	// Make a copy since bolt's slice is only valid during transaction
	valCopy := make([]byte, len(bi.value))
	copy(valCopy, bi.value)
	return valCopy
}

// Close closes the iterator and transaction
func (bi *BoltIterator) Close() error {
	return bi.tx.Rollback()
}
