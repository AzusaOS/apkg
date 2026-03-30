package apkgdb

import (
	"errors"

	bolt "go.etcd.io/bbolt"
)

// pinKey builds the BoltDB key for a pin: "channel\x00prefix".
func pinKey(channel, prefix string) []byte {
	k := make([]byte, len(channel)+1+len(prefix))
	copy(k, channel)
	k[len(channel)] = 0x00
	copy(k[len(channel)+1:], prefix)
	return k
}

// SetPin adds or updates a version pin for the given channel and package prefix.
func (d *DB) SetPin(channel, prefix, version string) error {
	if err := d.writeStart(); err != nil {
		return err
	}
	defer d.writeEnd()

	return d.dbptr.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("pins"))
		if err != nil {
			return err
		}
		return b.Put(pinKey(channel, prefix), []byte(version))
	})
}

// DeletePin removes a version pin for the given channel and package prefix.
func (d *DB) DeletePin(channel, prefix string) error {
	if err := d.writeStart(); err != nil {
		return err
	}
	defer d.writeEnd()

	return d.dbptr.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("pins"))
		if b == nil {
			return nil
		}
		return b.Delete(pinKey(channel, prefix))
	})
}

// GetPin returns the pinned version for the given channel and package prefix,
// or an empty string if no pin exists.
func (d *DB) GetPin(channel, prefix string) string {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	if d.dbptr == nil {
		return ""
	}

	var v string
	_ = d.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("pins"))
		if b == nil {
			return nil
		}
		if val := b.Get(pinKey(channel, prefix)); val != nil {
			v = string(val)
		}
		return nil
	})
	return v
}

// ListPins returns all pins for the given channel as a map of prefix → version.
func (d *DB) ListPins(channel string) map[string]string {
	d.dbrw.RLock()
	defer d.dbrw.RUnlock()

	if d.dbptr == nil {
		return nil
	}

	result := make(map[string]string)
	_ = d.dbptr.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("pins"))
		if b == nil {
			return nil
		}
		pfx := []byte(channel + "\x00")
		c := b.Cursor()
		for k, v := c.Seek(pfx); k != nil; k, v = c.Next() {
			if len(k) < len(pfx) || string(k[:len(pfx)]) != string(pfx) {
				break
			}
			result[string(k[len(pfx):])] = string(v)
		}
		return nil
	})
	return result
}

// lookupPin checks the active channel's pins for a version pin matching name.
// Returns the version prefix to constrain lookup, or "" if no pin applies.
// Must be called within a bolt View transaction with the read lock held.
func (d *DB) lookupPinTx(tx *bolt.Tx, name string) string {
	ch := d.channel
	if ch == "" || ch == "latest" {
		return ""
	}

	b := tx.Bucket([]byte("pins"))
	if b == nil {
		return ""
	}

	v := b.Get(pinKey(ch, name))
	if v == nil {
		return ""
	}
	return string(v)
}

// ErrPinnedVersionNotFound is returned internally when a pinned version doesn't exist.
var ErrPinnedVersionNotFound = errors.New("pinned version not found")
