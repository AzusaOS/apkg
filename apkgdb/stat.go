package apkgdb

func (d *DBData) Length() uint64 {
	return uint64(len(d.data))
}

func (d *DBData) Inodes() uint64 {
	return d.inoCount
}

func (d *DBData) PackagesSize() uint64 {
	return d.totalSize
}
