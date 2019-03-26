#/bin/sh

ZLIB_VER=1.2.11

# testing with only zlib
if [ ! -f zlib-${ZLIB_VER}.tar.gz ]; then
	wget http://zlib.net/zlib-${ZLIB_VER}.tar.gz
fi

if [ ! -d zlib-${ZLIB_VER} ]; then
	tar xvf zlib-${ZLIB_VER}.tar.gz
fi

cd zlib-${ZLIB_VER}
make distclean
rm -fr dist

# configure & build
./configure --prefix=/pkg/by-name/core.zlib.${ZLIB_VER} --libdir=/pkg/by-name/libs.zlib.${ZLIB_VER}
make
mkdir dest
make install DESTDIR=dest

