# TardigradeOS - tpkg

Work in progress

# Structure

* /pkg/INFO contains basic info on the currently served data (file version, etc)
* /pkg/by-name/... is a non-listable directory but lookups will return a symlink to a given package in the form ../by-id/id
* /pkg/by-id/... for each id, allows access to contents of package

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

* 0x00 (uint32)
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
