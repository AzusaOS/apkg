package apkgdb

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/petar/GoLLRB/llrb"
	bolt "go.etcd.io/bbolt"
)

func newTestDB(t *testing.T) (*DB, func()) {
	t.Helper()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "test.linux.amd64.db")

	bdb, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	d := &DB{
		dbptr: bdb,
		path:  dir,
		name:  "test",
		os:    "linux",
		arch:  "amd64",
		ino:   llrb.New(),
		pkgI:  make(map[[32]byte]uint64),
		nextI: 1000,
	}
	return d, func() { bdb.Close() }
}

func TestPinSetAndGet(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	err := d.SetPin("stable", "sys-libs.glibc", "2.41")
	if err != nil {
		t.Fatal(err)
	}

	v := d.GetPin("stable", "sys-libs.glibc")
	if v != "2.41" {
		t.Errorf("expected 2.41, got %q", v)
	}

	// Different channel should not match
	v = d.GetPin("testing", "sys-libs.glibc")
	if v != "" {
		t.Errorf("expected empty for different channel, got %q", v)
	}

	// Different prefix should not match
	v = d.GetPin("stable", "sys-libs.musl")
	if v != "" {
		t.Errorf("expected empty for different prefix, got %q", v)
	}
}

func TestPinDelete(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.41")

	err := d.DeletePin("stable", "sys-libs.glibc")
	if err != nil {
		t.Fatal(err)
	}

	v := d.GetPin("stable", "sys-libs.glibc")
	if v != "" {
		t.Errorf("expected empty after delete, got %q", v)
	}
}

func TestPinOverwrite(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.40")
	d.SetPin("stable", "sys-libs.glibc", "2.41")

	v := d.GetPin("stable", "sys-libs.glibc")
	if v != "2.41" {
		t.Errorf("expected 2.41 after overwrite, got %q", v)
	}
}

func TestListPins(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.41")
	d.SetPin("stable", "dev-lang.python", "3.11")
	d.SetPin("testing", "sys-libs.glibc", "2.42")

	pins := d.ListPins("stable")
	if len(pins) != 2 {
		t.Fatalf("expected 2 pins for stable, got %d", len(pins))
	}
	if pins["sys-libs.glibc"] != "2.41" {
		t.Errorf("expected glibc 2.41, got %q", pins["sys-libs.glibc"])
	}
	if pins["dev-lang.python"] != "3.11" {
		t.Errorf("expected python 3.11, got %q", pins["dev-lang.python"])
	}

	pins = d.ListPins("testing")
	if len(pins) != 1 {
		t.Fatalf("expected 1 pin for testing, got %d", len(pins))
	}

	pins = d.ListPins("nonexistent")
	if len(pins) != 0 {
		t.Errorf("expected 0 pins for nonexistent channel, got %d", len(pins))
	}
}

func TestLookupPinTxLatestChannel(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.41")
	d.channel = "latest"

	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	var result string
	d.dbptr.View(func(tx *bolt.Tx) error {
		result = d.lookupPinTx(tx, "sys-libs.glibc")
		return nil
	})
	if result != "" {
		t.Errorf("latest channel should skip pins, got %q", result)
	}
}

func TestLookupPinTxWithChannel(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.41")
	d.channel = "stable"

	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	var result string
	d.dbptr.View(func(tx *bolt.Tx) error {
		result = d.lookupPinTx(tx, "sys-libs.glibc")
		return nil
	})
	if result != "2.41" {
		t.Errorf("expected 2.41, got %q", result)
	}
}

func TestLookupPinTxNoPins(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.channel = "stable"

	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	var result string
	d.dbptr.View(func(tx *bolt.Tx) error {
		result = d.lookupPinTx(tx, "sys-libs.glibc")
		return nil
	})
	if result != "" {
		t.Errorf("expected empty with no pins, got %q", result)
	}
}

// TestPinnedLookup tests that internalLookup constrains the cursor seek when a pin is set.
func TestPinnedLookup(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	// Populate p2p bucket with two versions of a package
	err := d.dbptr.Update(func(tx *bolt.Tx) error {
		p2p, err := tx.CreateBucketIfNotExists([]byte("p2p"))
		if err != nil {
			return err
		}
		pkg, err := tx.CreateBucketIfNotExists([]byte("pkg"))
		if err != nil {
			return err
		}
		for _, b := range []string{"path", "header", "sig", "meta"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}

		// Add two versions: 1.0 and 2.0
		for _, ver := range []struct {
			name string
			hash byte
		}{
			{"test.pkg.core.1.0.linux.amd64", 0x01},
			{"test.pkg.core.2.0.linux.amd64", 0x02},
		} {
			hash := make([]byte, 32)
			hash[0] = ver.hash
			inoCount := make([]byte, 8)
			binary.BigEndian.PutUint64(inoCount, 10)
			nameB := []byte(ver.name)

			nameC := collatedVersion(ver.name)
			p2pVal := append(append(append([]byte(nil), hash...), inoCount...), nameB...)
			if err := p2p.Put(nameC, p2pVal); err != nil {
				return err
			}

			sizeB := make([]byte, 8)
			binary.BigEndian.PutUint64(sizeB, 1000)
			inoBin := make([]byte, 8)
			pkgVal := append(append(append(append([]byte{0}, sizeB...), inoBin...), inoCount...), nameB...)
			if err := pkg.Put(hash, pkgVal); err != nil {
				return err
			}

			// Create empty entries in required buckets
			for _, b := range []string{"path", "header", "sig", "meta"} {
				bucket := tx.Bucket([]byte(b))
				if err := bucket.Put(hash, []byte{}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Without pin (latest channel): should resolve to version 2.0
	d.channel = "latest"
	n, err := d.internalLookup("test.pkg.core")
	if err != nil {
		t.Fatalf("unpinned lookup failed: %v", err)
	}
	unpinnedIno := n

	// Set pin to version 1.0
	d.SetPin("stable", "test.pkg.core", "1.0")
	d.channel = "stable"

	n, err = d.internalLookup("test.pkg.core")
	if err != nil {
		t.Fatalf("pinned lookup failed: %v", err)
	}
	pinnedIno := n

	// The pinned inode should be different from unpinned (different package)
	if pinnedIno == unpinnedIno {
		t.Error("pinned and unpinned lookup returned same inode — pin was not applied")
	}
}

func TestPinnedLookupMissingVersionFallback(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	// Populate p2p bucket with only version 2.0
	err := d.dbptr.Update(func(tx *bolt.Tx) error {
		p2p, err := tx.CreateBucketIfNotExists([]byte("p2p"))
		if err != nil {
			return err
		}
		pkg, err := tx.CreateBucketIfNotExists([]byte("pkg"))
		if err != nil {
			return err
		}
		for _, b := range []string{"path", "header", "sig", "meta"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}

		hash := make([]byte, 32)
		hash[0] = 0x02
		inoCount := make([]byte, 8)
		binary.BigEndian.PutUint64(inoCount, 10)
		nameB := []byte("test.pkg.core.2.0.linux.amd64")

		nameC := collatedVersion("test.pkg.core.2.0.linux.amd64")
		p2pVal := append(append(append([]byte(nil), hash...), inoCount...), nameB...)
		if err := p2p.Put(nameC, p2pVal); err != nil {
			return err
		}

		sizeB := make([]byte, 8)
		binary.BigEndian.PutUint64(sizeB, 1000)
		inoBin := make([]byte, 8)
		pkgVal := append(append(append(append([]byte{0}, sizeB...), inoBin...), inoCount...), nameB...)
		if err := pkg.Put(hash, pkgVal); err != nil {
			return err
		}

		for _, b := range []string{"path", "header", "sig", "meta"} {
			bucket := tx.Bucket([]byte(b))
			if err := bucket.Put(hash, []byte{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Pin to nonexistent version 1.0 — should fall back to 2.0
	d.SetPin("stable", "test.pkg.core", "1.0")
	d.channel = "stable"

	n, err := d.internalLookup("test.pkg.core")
	if err != nil {
		t.Fatalf("lookup with missing pinned version should fall back, got error: %v", err)
	}
	if n == 0 {
		t.Error("expected non-zero inode from fallback")
	}
}

func TestPinKeyFormat(t *testing.T) {
	k := pinKey("stable", "sys-libs.glibc")
	expected := "stable\x00sys-libs.glibc"
	if string(k) != expected {
		t.Errorf("pinKey = %q, want %q", k, expected)
	}
}

func TestEmptyChannelBehavesLikeLatest(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	d.SetPin("stable", "sys-libs.glibc", "2.41")
	d.channel = "" // empty channel

	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	var result string
	d.dbptr.View(func(tx *bolt.Tx) error {
		result = d.lookupPinTx(tx, "sys-libs.glibc")
		return nil
	})
	if result != "" {
		t.Errorf("empty channel should skip pins, got %q", result)
	}
}

func TestGetPinClosedDB(t *testing.T) {
	d := &DB{}
	v := d.GetPin("stable", "sys-libs.glibc")
	if v != "" {
		t.Errorf("expected empty for nil dbptr, got %q", v)
	}
}

func TestListPinsClosedDB(t *testing.T) {
	d := &DB{}
	pins := d.ListPins("stable")
	if pins != nil {
		t.Errorf("expected nil for nil dbptr, got %v", pins)
	}
}

func TestDeleteNonexistentPin(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	// Should not error when deleting a pin that doesn't exist
	err := d.DeletePin("stable", "nonexistent")
	if err != nil {
		t.Errorf("unexpected error deleting nonexistent pin: %v", err)
	}
}

func TestSetChannelPropagatesToSubs(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	// Simulate a sub-database
	sub := &DB{channel: ""}
	d.sub = map[ArchOS]*DB{
		{OS: Linux, Arch: AMD64}: sub,
	}

	d.SetChannel("testing")

	if d.channel != "testing" {
		t.Errorf("parent channel = %q, want %q", d.channel, "testing")
	}
	if sub.channel != "testing" {
		t.Errorf("sub channel = %q, want %q", sub.channel, "testing")
	}
}

func TestSetChannelOnClosedDB(t *testing.T) {
	// Should not panic
	d, cleanup := newTestDB(t)
	defer cleanup()
	d.SetChannel("stable")
	if d.channel != "stable" {
		t.Errorf("channel = %q, want stable", d.channel)
	}
}

func TestDeletePinNoBucket(t *testing.T) {
	d, cleanup := newTestDB(t)
	defer cleanup()

	// pins bucket doesn't exist yet — delete should succeed
	err := d.DeletePin("stable", "foo")
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("unexpected error: %v", err)
	}
}
