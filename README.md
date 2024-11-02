# AzusaOS - apkg

Work in progress

## Getting started

In order to download & run the latest build:

    curl -s https://raw.githubusercontent.com/KarpelesLab/make-go/master/get.sh | /bin/sh -s apkg
    ./apkg

Or if you're using systemd:

    curl -s https://raw.githubusercontent.com/KarpelesLab/make-go/master/systemd.sh | /bin/sh -s apkg

If run as root, this will create /pkg/main and make packages available. Data will be stored in `/var/lib/apkg`. After running apkg you can confirm everything is working by using python for example.

    $ /pkg/main/dev-lang.python.core/bin/python --version
    Python 3.12.2

# Required minimum system

The following files or devices are required to run apkg

* /dev/null
* /dev/fuse
* /etc/resolv.conf
* /tmp

# Structure

* /pkg/ directories each containing a database (main, etc)
 * /pkg/main/... is a non-listable directory but lookups will return a symlink to a given package's full name, or a directory of the actual package contents

For example:

	$ readlink ~/pkg/main/libs.zlib
	libs.zlib.1.2.11.linux.amd64
	$ ls ~/pkg/main/libs.zlib/ -la
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

## Collation

In order for package names to be appropriately sorted, the following collation rules are applied when sorting keys are generated:

* Series of digits are prefixed with a single byte which value is how many digits are following plus 0x7f. For example "1" becomes "<80>1" and 42 becomes "<81>42", this ensures sorting for version numbers.

# Installing

	curl -s https://raw.githubusercontent.com/TrisTech/make-go/master/get.sh | /bin/sh -s apkg

Note: this is EXPERIMENTAL. A lot of stuff is missing. Do not use unless you know what you are doing.

# Database

Encoding is big endian unless specified.

## DB file

DB file: one file + signature.

Header:

* 0 Magic "APDB"
* 4 File Format Version uint32 (0x00000002)
* 8 Flags uint64 (reserved for future use)
* 16 Creation date/time
* 32 OS (linux, darwin, windows, etc)
* 36 Architecture (amd64, i386, etc)
* 40 Package count uint32
* 44 archive name (32 bytes, NUL padded)
* 76 data location (always headerlen+apkgsig.SignatureSize)
* 80 data length
* 84 data sha256 (32 bytes)

Basic pkg info (starts at data):

* 0x00 (uint8)
* Header hash (32 bytes)
* Size (uint64)
* Inode count (uint32)
* Full package name (varblob)
* File relative path (varblob)
* Raw header (varblob)
* Raw signature (varblob)
* Raw meta data (json, varblob)
* database metadata (json, varblob)

## Data File

Each data file contains a header, JSON-encoded metadata, a hash data descriptor, and data blocks

* 0 Magic "APKG"
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
* 120 Data block size
* 124 end of header

## Local bolt database

Contains the following buckets

* info → contains "version"
* p2p → package name to package hash + inode count + package name
* pkg → package hash → package info (0 + size + inode num + inode count[0] + package name)
* header → package hash → header
* sig → package hash → signature
* meta → package hash → meta data
* path → package hash → file relative path
* ldso → full target filename → ld.so.cache single entry

# Metadata

The following values are present in the meta json:

* fullname
* name
* version
* names
* os (eg. "linux")
* arch (eg. "amd64")
* category
* base_name
* subcat
* size
* hash
* blocks
* block_size
* inodes
* created
* provides (key: filename)
  * size
  * mode
  * or: symlink
* ld.so.cache (if a library)

