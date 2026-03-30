package apkgdb

import (
	"os"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestWriteEndNoPanic(t *testing.T) {
	// Verify that writeEnd does not panic when the database file cannot be reopened.
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "test.linux.amd64.db")

	bdb, err := bolt.Open(dbFile, 0600, &bolt.Options{ReadOnly: false})
	if err != nil {
		t.Fatal(err)
	}

	d := &DB{
		dbptr: bdb,
		path:  dir,
		name:  "test",
		os:    "linux",
		arch:  "amd64",
	}

	// Acquire the write lock as writeEnd expects it held
	d.dbrw.Lock()

	// Remove the database file so reopen fails
	os.Remove(dbFile)
	os.RemoveAll(dir)

	// writeEnd should log and recover, not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("writeEnd panicked: %v", r)
			}
		}()
		d.writeEnd()
	}()

	// After writeEnd, dbptr should be nil since reopen failed
	if d.dbptr != nil {
		t.Error("expected dbptr to be nil after failed reopen")
	}
}

func TestWriteStartNoPanic(t *testing.T) {
	// Verify that writeStart does not panic when the database cannot be reopened
	// in read-write mode and read-only fallback also fails.
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "test.linux.amd64.db")

	bdb, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	bdb.Close()

	// Reopen read-only so we have a valid starting state
	bdb, err = bolt.Open(dbFile, 0600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	d := &DB{
		dbptr: bdb,
		path:  "/nonexistent/path/that/does/not/exist",
		name:  "test",
		os:    "linux",
		arch:  "amd64",
	}

	var writeErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("writeStart panicked: %v", r)
			}
		}()
		writeErr = d.writeStart()
	}()

	if writeErr == nil {
		t.Error("expected an error from writeStart")
		// Clean up the lock if writeStart succeeded unexpectedly
		d.dbrw.Unlock()
	}
}
