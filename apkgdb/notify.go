package apkgdb

type NotifyTarget interface {
	NotifyInode(ino uint64, offt int64, data []byte) error
}

func (db *DB) notifyInode(ino uint64, offt int64, data []byte) error {
	for {
		tgt := db.ntgt
		if tgt != nil {
			return tgt.NotifyInode(ino, offt, data)
		}
		db = db.parent
		if db == nil {
			// no target to notify
			return nil
		}
	}
}

func (db *DB) SetNotifyTarget(tgt NotifyTarget) {
	db.ntgt = tgt
}
