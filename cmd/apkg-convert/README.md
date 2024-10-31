# apkg process

In order to generate apkg files you need a valid signature key.

To generate with YubiHSM2:

	generate asymmetric 0 0 pkg_sign_ed25519 1 sign-eddsa ed25519

Install:

	go install github.com/AzusaOS/apkg/cmd/apkg-convert@latest
