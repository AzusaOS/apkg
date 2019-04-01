#/bin/sh
set -e

ZLIB_VER=1.2.11
ARCH=`uname -m`
OS=`uname -s | tr A-Z a-z`

# testing with only zlib
if [ ! -f zlib-${ZLIB_VER}.tar.gz ]; then
	wget http://zlib.net/zlib-${ZLIB_VER}.tar.gz
fi

if [ ! -d zlib-${ZLIB_VER} ]; then
	echo "Extracting zlib-${ZLIB_VER} ..."
	tar xf zlib-${ZLIB_VER}.tar.gz
fi

echo "Compiling zlib-${ZLIB_VER} ..."
cd zlib-${ZLIB_VER}
make distclean >make_distclean.log 2>&1

# configure & build
./configure >configure.log 2>&1 --prefix=/pkg/by-name/core.zlib.${ZLIB_VER} --libdir=/pkg/by-name/libs.zlib.${ZLIB_VER}
make >make.log 2>&1
mkdir -p ../dist
make >make_install.log 2>&1 install DESTDIR=../dist

cd ..

echo "Building squashfs..."

# build squashfs files
# dist/pkg/by-name/core.zlib.${ZLIB_VER}
# dist/pkg/by-name/libs.zlib.${ZLIB_VER}

mksquashfs "dist/pkg/by-name/core.zlib.${ZLIB_VER}" "dist/core.zlib.${ZLIB_VER}.${OS}.${ARCH}.squashfs" -no-exports -all-root -b 4096
mksquashfs "dist/pkg/by-name/libs.zlib.${ZLIB_VER}" "dist/libs.zlib.${ZLIB_VER}.${OS}.${ARCH}.squashfs" -no-exports -all-root -b 4096

for foo in dist/*.squashfs; do 
	php convert.php "$foo"
done
