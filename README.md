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
* ID + basic pkg info (inode range, etc)
* ID→info offset index
* Name→ID index

Header:

* File Format Version (0x00000001)
* Flags int64 (beta, etc)
* Creation date/time
* Architecture (amd64, i386, etc)
* Download URL prefix (should contain architecture)
* Location in file of indices, length, all int32 (file should never reach 4GB)
* ... more?

Basic pkg info:

* ID (16 bytes)
* File header signature (file header contains checksum of rest of file)
* Size
* Timestamp

## Data File

Each data file contains a header, JSON-encoded metadata, a hash data descriptor, and data blocks

* File Format Version (0x00000001)
* Flags int64
* Creation date/time int64 + int64 (unix + nano)
* MetaData offset int32
* MetaData length int32
* MetaData hash (sha256)
* Hash descriptor offset int32
* Hash descriptor length int32
* Hash descriptor hash (sha256)
* Data offset int32

