# configuration to build dist pkg

DIST_ARCHS=linux_amd64 linux_386 #darwin_amd64
APKG_DB="main"
APKG_NAME="core.apkg"

#GOLDFLAGS=-linkmode external -extldflags -static
export CGO_ENABLED=0
