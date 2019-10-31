package apkgdb

type OS uint32
type Arch uint32

const (
	Linux OS = iota
	Darwin
	Windows
)

const (
	X86 Arch = iota
	AMD64
	ARM
	ARM64
)

func ParseOS(os string) OS {
	switch os {
	case "linux":
		return Linux
	case "darwin":
		return Darwin
	case "windows":
		return Windows
	default:
		return OS(0xffffffff) // unknown
	}
}

func ParseArch(arch string) Arch {
	switch arch {
	case "386":
		return X86
	case "amd64":
		return AMD64
	case "arm":
		return ARM
	case "arm64":
		return ARM64
	default:
		return Arch(0xffffffff) // unknown
	}
}

func (os OS) String() string {
	switch os {
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
