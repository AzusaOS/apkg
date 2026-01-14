package apkgdb

import "strings"

// OS represents an operating system type.
type OS uint32

// Arch represents a CPU architecture type.
type Arch uint32

// ArchOS combines an OS and architecture into a single type,
// used as a key for sub-database lookups.
type ArchOS struct {
	OS   OS
	Arch Arch
}

// OS constants for supported operating systems.
const (
	AnyOS OS = iota
	Linux
	Darwin
	Windows
	BadOS OS = 0xffffffff
)

// Arch constants for supported CPU architectures.
const (
	AnyArch Arch = iota
	X86
	AMD64
	ARM
	ARM64
	RiscV64
	BadArch Arch = 0xffffffff
)

// ParseOS parses an OS string (e.g., "linux", "darwin") and returns the corresponding OS constant.
func ParseOS(os string) OS {
	switch os {
	case "any":
		return AnyOS
	case "linux":
		return Linux
	case "darwin":
		return Darwin
	case "windows":
		return Windows
	default:
		return BadOS // unknown
	}
}

// ParseArch parses an architecture string (e.g., "amd64", "arm64") and returns the corresponding Arch constant.
func ParseArch(arch string) Arch {
	switch arch {
	case "any":
		return AnyArch
	case "386":
		return X86
	case "amd64":
		return AMD64
	case "arm":
		return ARM
	case "arm64":
		return ARM64
	case "riscv64":
		return RiscV64
	default:
		return BadArch // unknown
	}
}

// ParseArchOS parses a combined OS.Arch string (e.g., "linux.amd64") and returns the corresponding ArchOS.
func ParseArchOS(archos string) ArchOS {
	// for example "linux.amd64"
	pos := strings.IndexByte(archos, '.')
	if pos <= 0 {
		return ArchOS{BadOS, BadArch}
	}
	os := ParseOS(archos[:pos])
	arch := ParseArch(archos[pos+1:])
	if os == BadOS || arch == BadArch {
		return ArchOS{BadOS, BadArch}
	}
	return ArchOS{os, arch}
}

func (os OS) String() string {
	switch os {
	case AnyOS:
		return "any"
	case Linux:
		return "linux"
	case Darwin:
		return "darwin"
	case Windows:
		return "windows"
	default:
		return "unknown"
	}
}

func (arch Arch) String() string {
	switch arch {
	case AnyArch:
		return "any"
	case X86:
		return "386"
	case AMD64:
		return "amd64"
	case ARM:
		return "arm"
	case ARM64:
		return "arm64"
	case RiscV64:
		return "riscv64"
	default:
		return "unknown"
	}
}

func (archos ArchOS) String() string {
	return archos.OS.String() + "." + archos.Arch.String()
}

func (archos ArchOS) IsValid() bool {
	return archos.OS != BadOS && archos.Arch != BadArch
}
