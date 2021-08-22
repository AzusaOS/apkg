module git.atonline.com/azusa/apkg

go 1.16

require (
	github.com/MagicalTux/hsm v0.1.3
	github.com/MagicalTux/smartremote v0.0.14
	github.com/RoaringBitmap/roaring v0.4.21 // indirect
	github.com/boltdb/bolt v1.3.1
	github.com/enceve/crypto v0.0.0-20160707101852-34d48bb93815 // indirect
	github.com/golang-jwt/jwt/v4 v4.0.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/hanwen/go-fuse v1.0.0
	github.com/hanwen/go-fuse/v2 v2.0.3-0.20200103165319-0e3c45fc4899
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/petar/GoLLRB v0.0.0-20190514000832-33fb24c13b99
	golang.org/x/crypto v0.0.0-20190923035154-9ee001bba392
	golang.org/x/net v0.0.0-20190923162816-aa69164e4478 // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/sys v0.0.0-20190922100055-0a153f010e69
	golang.org/x/text v0.3.2 // indirect
	golang.org/x/tools v0.0.0-20190923221242-6816ec868d64 // indirect
)

replace github.com/golang-jwt/jwt/v4 => github.com/MagicalTux/jwt/v4 v4.0.1-0.20210822070356-97f3b221c77a
