# configuration to build dist pkg

DIST_ARCHS=linux_amd64 linux_arm64
GO_TAGS=fuse

#GOLDFLAGS=-linkmode external -extldflags -static
export CGO_ENABLED=0
