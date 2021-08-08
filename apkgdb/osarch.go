package apkgdb

import "strings"

type OS uint32
type Arch uint32

type ArchOS struct {
	OS   OS
	Arch Arch
}

const (
	AnyOS OS = iota
	Linux
	Darwin
	Windows
	BadOS OS = 0xffffffff
)

const (
	AnyArch Arch = iota
	X86
	AMD64
	ARM
	ARM64
	BadArch Arch = 0xffffffff
)

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
	default:
		return BadArch // unknown
	}
}

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
