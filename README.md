# apkg

The package manager for [AzusaOS](https://github.com/AzusaOS). apkg is a daemon that downloads cryptographically signed package databases, fetches SquashFS packages on demand, and exposes their contents through a read-only FUSE filesystem.

Packages are available instantly with no extraction step. A lookup like `/pkg/main/dev-lang.python.core` resolves to the best matching version and presents its files as a normal directory tree.

## Quick start

Download and run:

    curl -s https://raw.githubusercontent.com/KarpelesLab/make-go/master/get.sh | /bin/sh -s apkg
    ./apkg

With systemd:

    curl -s https://raw.githubusercontent.com/KarpelesLab/make-go/master/systemd.sh | /bin/sh -s apkg

Verify it works:

    $ /pkg/main/dev-lang.python.core/bin/python --version
    Python 3.12.2

## How it works

1. apkg downloads a signed package database from `https://data.apkg.net/`
2. It mounts a FUSE filesystem (at `/pkg/main` as root, or `~/pkg/main` as a user)
3. When a package is accessed, apkg downloads its SquashFS image and serves files directly from it
4. The database is checked for updates hourly; SIGHUP forces an immediate check

### Storage paths

| Context | Database | Mount point |
|---------|----------|-------------|
| Root    | `/var/lib/apkg` | `/pkg/main` |
| User    | `~/.cache/apkg` | `~/pkg/main` |

When running as root, apkg also checks `/mnt/*/AZUSA` for an AzusaOS installation and uses that path if found.

## Package names

Package names are dot-separated: `category.name.subcat.version.os.arch`

    sys-libs.glibc.libs.2.41.linux.amd64
    │        │     │    │    │     └─ architecture
    │        │     │    │    └─ operating system
    │        │     │    └─ version (variable depth)
    │        │     └─ subcategory (core, libs, dev, doc, data, mod, ...)
    │        └─ package name
    └─ category

Subcategories can be multi-level (e.g. `data.locale`, `data.fonts.afm`).

### Version resolution

Less-specific names resolve to the most recent matching version:

    $ readlink /pkg/main/sys-libs.glibc.libs
    sys-libs.glibc.libs.2.41.linux.amd64

    $ readlink /pkg/main/sys-libs.glibc.libs.2.38
    sys-libs.glibc.libs.2.38.linux.amd64

A lookup returns either a symlink (pointing to the resolved full name) or, when accessed as a directory, the package contents directly:

    $ ls /pkg/main/libs.zlib/
    libz.a  libz.so  libz.so.1  libz.so.1.2.11  pkgconfig/

### Collation

Version numbers are collated so that `1.9` sorts before `1.10`. Runs of digits are prefixed with a length byte (`0x7f` + digit count), ensuring correct lexicographic ordering of the binary keys in the database.

## Release channels

apkg supports server-distributed **release channels** that pin packages to specific versions. Channels are named sets of version pins (e.g. `stable`, `testing`) included in the signed database.

    ./apkg -channel stable     # default: use pinned versions
    ./apkg -channel latest     # always resolve to newest version
    ./apkg -channel testing    # use testing channel pins

When a pin exists for the active channel, lookups are constrained to the pinned version prefix. For example, if `stable` pins `sys-libs.glibc` to `2.40`, then `sys-libs.glibc.libs` resolves to `2.40.x` instead of the latest `2.41.x`.

If the pinned version is not available, apkg logs a warning and falls back to the latest version. If no pins exist for the active channel, behavior is identical to `latest`.

## Command-line flags

| Flag | Default | Description |
|------|---------|-------------|
| `-channel` | `stable` | Release channel for version resolution. Use `latest` to bypass all pins. |
| `-load_unsigned` | `false` | Load unsigned SquashFS packages from disk (**dangerous**, bypasses signature verification). |

## Control interface

apkg exposes an HTTP control interface for status and debugging:

| Context | Port |
|---------|------|
| Root    | TCP 100 |
| User    | TCP 10000 |

Endpoints:

- `GET /` -- status overview
- `GET /_stack` -- goroutine stack traces
- `GET /apkgdb/main` -- database status
- `GET /apkgdb/main?action=list` -- list all packages
- `GET /apkgdb/main?sub=linux.arm64` -- query a cross-architecture sub-database

A UDP listener on the same port responds to `DISCOVER` packets with the TCP port number.

## Unsigned packages (development)

The `-load_unsigned` flag enables loading unverified SquashFS packages from disk. **Do not use in production.**

    ./apkg -load_unsigned

Place `.squashfs` files in the `unsigned/` subdirectory under the database path:

- Root: `/var/lib/apkg/unsigned/`
- User: `~/.cache/apkg/unsigned/`

Files must follow the naming convention:

    category.name.subcat.version.os.arch.squashfs

For example:

    core.foobar.libs.1.2.3.linux.amd64.squashfs

The directory is monitored with inotify -- files are picked up on creation and removed on deletion without restart.

## System requirements

- `/dev/fuse`
- `/dev/null`
- `/etc/resolv.conf` (for DNS resolution)
- `/tmp`

## Supported platforms

| OS | Architectures |
|----|---------------|
| Linux | amd64, 386, arm, arm64, riscv64 |
| macOS | amd64 |
| FreeBSD | amd64, 386, arm |
| Windows | amd64, 386 |

## Building from source

    make              # build (requires Go 1.24+, uses -tags fuse)
    make test         # run tests
    make dist         # cross-compile for all platforms

The build requires the `fuse` build tag, which is set automatically by the Makefile via `contrib/config.mak`.

## Architecture

```
apkg (main)          Daemon entry point, signals, self-updater, control interface
apkgdb/              Package database: BoltDB storage, indexing, lookup, version pins
apkgfs/              FUSE filesystem: inode management, file/dir/symlink serving
apkgsig/             Cryptographic signatures: Ed25519 verification, trust store
cmd/apkg-convert/    Tool to sign and package SquashFS files into .apkg format
cmd/apkg-index/      Tool to build and upload signed database exports
```

### Security

All packages and databases are signed with Ed25519 keys. The trust store is compiled into the binary. Package signatures are verified on download; database signatures are verified on every index operation. The `-load_unsigned` flag is the only way to bypass this.

## Wire formats

### Database file (APDB)

All integers are big-endian.

Header (196 bytes):

| Offset | Size | Field |
|--------|------|-------|
| 0 | 4 | Magic `"APDB"` |
| 4 | 4 | Version (`0x00000001`) |
| 8 | 8 | Flags (reserved) |
| 16 | 8+8 | Creation timestamp (unix seconds + nanoseconds) |
| 32 | 4 | OS enum |
| 36 | 4 | Arch enum |
| 40 | 4 | Package count |
| 44 | 32 | Database name (NUL-padded) |
| 76 | 4+4 | Data offset + length |
| 84 | 32 | Data SHA-256 |
| 116 | 4+4 | ID index offset + length (reserved) |
| 124 | 32 | ID index SHA-256 (reserved) |
| 156 | 4+4 | Name index offset + length (reserved) |
| 164 | 32 | Name index SHA-256 (reserved) |

Followed by a 128-byte Ed25519 signature, then the data section.

Data section entries:

**Package (type 0x00):**

| Field | Size |
|-------|------|
| Type | 1 byte (`0x00`) |
| Header SHA-256 | 32 bytes |
| Size | 8 bytes (uint64) |
| Inode count | 4 bytes (uint32) |
| Name | varblob |
| Path | varblob |
| Header | varblob |
| Signature | varblob |
| Metadata (JSON) | varblob |

**Version pin (type 0x01):** appended after all packages.

| Field | Size |
|-------|------|
| Type | 1 byte (`0x01`) |
| Channel name | varblob |
| Package prefix | varblob |
| Version prefix | varblob |

Varblob encoding: uvarint length prefix followed by raw bytes.

### Package file (APKG)

| Offset | Size | Field |
|--------|------|-------|
| 0 | 4 | Magic `"APKG"` |
| 4 | 4 | Version (`0x00000001`) |
| 8 | 8 | Flags |
| 16 | 8+8 | Creation timestamp (unix + nanoseconds) |
| 32 | 4+4 | Metadata offset + length |
| 40 | 32 | Metadata SHA-256 |
| 72 | 4+4 | Hash table offset + length |
| 80 | 32 | Hash table SHA-256 |
| 112 | 4 | Signature offset |
| 116 | 4 | Data offset |
| 120 | 4 | Block size |

Followed by: JSON metadata, block hash table, Ed25519 signature, SquashFS data.

### Local BoltDB buckets

| Bucket | Key | Value |
|--------|-----|-------|
| `info` | `"version"` | Database version string |
| `p2p` | Collated package name | `[32B hash][8B inode count][name]` |
| `pkg` | SHA-256 hash | `[1B type][8B size][8B ino][8B inocount][name]` |
| `header` | SHA-256 hash | Raw package header |
| `sig` | SHA-256 hash | Raw signature |
| `meta` | SHA-256 hash | JSON metadata |
| `path` | SHA-256 hash | Relative file path |
| `ldso` | Library path | JSON ld.so.cache entry |
| `pins` | `channel\x00prefix` | Version prefix string |

## Package metadata

JSON fields in package metadata:

| Field | Description |
|-------|-------------|
| `full_name` | Complete package name including version, OS, arch |
| `name` | Name up to subcat (e.g. `sys-libs.glibc.libs`) |
| `version` | Version string |
| `names` | All prefix names for lookup |
| `os`, `arch` | Target platform |
| `category`, `base_name`, `subcat` | Name components |
| `size` | Package file size |
| `hash` | Block hash table SHA-256 |
| `blocks`, `block_size` | Block count and size |
| `inodes` | Inode count in SquashFS |
| `created` | `[unix_seconds, nanoseconds]` |
| `provides` | Map of filename to `{size, mode}` or `{symlink}` |
| `ld.so.cache` | Base64-encoded ld.so.cache content (libs only) |
| `virtual` | Virtual directory mappings (optional) |

## License

See [LICENSE](LICENSE).
