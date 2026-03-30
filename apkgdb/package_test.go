package apkgdb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestGetPkgTxMissingBuckets(t *testing.T) {
	// Create a temporary BoltDB with only a "pkg" bucket but missing
	// "path", "header", "sig", "meta" buckets. getPkgTx should return
	// an error rather than panicking on nil bucket dereference.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	bdb, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	hash := make([]byte, 32)
	hash[0] = 0x01

	// Populate the "pkg" bucket with a valid-length entry so getPkgTx
	// gets past the initial checks and reaches the missing bucket access.
	err = bdb.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("pkg"))
		if err != nil {
			return err
		}
		// Build a minimal valid value: 1 byte type + 8 bytes size + 8 bytes padding + 8 bytes inodes + name
		val := make([]byte, 26)
		val[0] = 0
		binary.BigEndian.PutUint64(val[1:9], 1000)   // size
		binary.BigEndian.PutUint64(val[17:25], 10)    // inodes
		val[25] = 'x'                                  // name
		return b.Put(hash, val)
	})
	if err != nil {
		t.Fatal(err)
	}

	d := &DB{dbptr: bdb}

	// Call getPkgTx within a View transaction — should get an error, not a panic
	err = bdb.View(func(tx *bolt.Tx) error {
		_, err := d.getPkgTx(tx, 1000, hash)
		return err
	})
	if err != os.ErrInvalid {
		t.Errorf("expected os.ErrInvalid, got %v", err)
	}

	bdb.Close()
}

func TestGetPkgTxMissingPkgBucket(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	bdb, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bdb.Close()

	d := &DB{dbptr: bdb}
	hash := make([]byte, 32)

	err = bdb.View(func(tx *bolt.Tx) error {
		_, err := d.getPkgTx(tx, 1000, hash)
		return err
	})
	if err != os.ErrInvalid {
		t.Errorf("expected os.ErrInvalid for missing pkg bucket, got %v", err)
	}
}
