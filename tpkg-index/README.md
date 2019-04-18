# tpkg db generation

In order to generate tpdb file you need a valid signature key.

To generate with YubiHSM2:

	generate asymmetric 0 0 tpdb_sign_ed25519 1 sign-eddsa ed25519

Install:

	go get github.com/tardigradeos/tpkg/tpkg-index
