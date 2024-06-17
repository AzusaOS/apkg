# configuration to build dist pkg

DIST_ARCHS=linux_amd64 linux_386 linux_arm linux_arm64 #linux_riscv64 #darwin_amd64
APKG_DB="main"
APKG_NAME="azusa.apkg.core"
GO_TAGS=fuse

#GOLDFLAGS=-linkmode external -extldflags -static
export CGO_ENABLED=0
