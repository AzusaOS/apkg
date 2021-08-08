package apkgdb

func (d *DB) SubGet(sub ArchOS) (*DB, error) {
	if sub.OS == d.osV && sub.Arch == d.archV {
		// ok, this is us!
		return d, nil
	}

	d.subLk.RLock()
	db, ok := d.sub[sub]
	d.subLk.RUnlock()

	if ok {
		return db, nil
	}

	d.subLk.Lock()
	defer d.subLk.Unlock()

	// double check, just in case
	db, ok = d.sub[sub]
	if ok {
		return db, nil
	}

	db, err := NewOsArch(d.prefix, d.name, d.path, sub.OS.String(), sub.Arch.String())
	if err != nil {
		return nil, err
	}
	db.parent = d

	d.sub[sub] = db
	return db, nil
}

func (d *DB) ListSubs() []ArchOS {
	d.subLk.RLock()
	defer d.subLk.RUnlock()

	res := make([]ArchOS, 0, len(d.sub))
	for k := range d.sub {
		res = append(res, k)
	}
	return res
}
