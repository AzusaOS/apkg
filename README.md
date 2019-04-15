# TardigradeOS - tpkg

Work in progress

# Structure

* /pkg/... is a non-listable directory but lookups will return a symlink to a given package's full name, or a directory of the actual package contents

For example:

	$ readlink ~/pkg/libs.zlib
	libs.zlib.1.2.11.linux.amd64
	$ ls ~/pkg/libs.zlib/ -la
	total 4
	drwxr-xr-x 3 root root    100 Apr 14 01:44 .
	dr--r--r-- 1 root root   4096 Jan  1  1970 ..
	-rw-r--r-- 0 root root 146362 Apr 14 01:44 libz.a
	lrwxrwxrwx 1 root root     14 Apr 14 01:44 libz.so -> libz.so.1.2.11
	lrwxrwxrwx 1 root root     14 Apr 14 01:44 libz.so.1 -> libz.so.1.2.11
	-rwxr-xr-x 0 root root 117632 Apr 14 01:44 libz.so.1.2.11
	drwxr-xr-x 2 root root     30 Apr 14 01:44 pkgconfig

# Package names

Each package name is made of sections separated by dots. Typically it would be vendor.package.major.minor.revision.patch

Additional sections can be added, and less-specific names will always point to the most recent package with a given prefix.

For example, package foobar v1.2.3 released as part of the core will be called core.foobar.1.2.3, however core.foobar will also work.

# Database

Encoding is big endian unless specified.

## Master DB

Master DB: one file + signature.

* Header
* ID & basic pkg info for each package (inode range, etc)
* Signature

Header:

* Magic "TPDB"
* File Format Version (0x00000001)
* Flags int64 (beta, etc)
* Creation date/time
* OS (linux, darwin, windows, etc)
* Architecture (amd64, i386, etc)
* Download URL prefix (should contain architecture?)
* Location in file of indices, length, all int32 (file should never reach 4GB)
* ... more?

Basic pkg info:

* 0x00 (uint8)
* ID (16 bytes)
* File header signature (file header contains checksum of rest of file)
* Size

## Data File

Each data file contains a header, JSON-encoded metadata, a hash data descriptor, and data blocks

* 0 Magic "TPKG"
* 4 File Format Version (0x00000001)
* 8 Flags int64
* 16 Creation date/time int64 + int64 (unix + nano)
* 32 MetaData offset int32
* 36 MetaData length int32
* 40 MetaData hash (sha256)
* 72 Hash descriptor offset int32
* 76 Hash descriptor length int32
* 80 Hash descriptor hash (sha256)
* 112 Signature offset uint32
* 116 Data offset uint32
* 120 end of header
