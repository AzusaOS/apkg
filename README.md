# TardigradeOS - tpkg

Work in progress

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
* Download URL format
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
