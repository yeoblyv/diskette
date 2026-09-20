package main

// version is the release this binary was built from, embedded at build
// time via -ldflags "-X main.version=X.Y.Z" (see packaging/*/build_*.sh
// and the release build). "dev" means the binary was built without that
// flag — an ordinary local `go build`/`go run` during development.
var version = "dev"
