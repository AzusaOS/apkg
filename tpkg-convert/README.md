# tpkg process

In order to generate tpkg files you need a valid signature key.

To generate with YubiHSM2:

	generate asymmetric 0 0 pkg_sign 1 sign-eddsa ed25519

Install:

	go get github.com/tardigradeos/tpkg/tpkg-convert
