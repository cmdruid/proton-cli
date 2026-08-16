//go:build !webview

// Stub for builds without the `webview` build tag, so default
// `go build ./...` and `golangci-lint run ./...` don't error on this
// directory. The real implementation lives in main.go behind the
// `//go:build webview` tag and requires CGO + libwebkit2gtk on Linux.
//
// Distribution: this stub is NEVER shipped. Goreleaser builds the
// real binary with `-tags webview` for each platform and embeds it
// into the main `proton` binary via `internal/hv/assets/`.
package main

import (
	"fmt"
	"os"

	"github.com/roman-16/proton-cli/internal/hv/hvexit"
)

func main() {
	fmt.Fprintln(os.Stderr,
		"hv: webview-unavailable: this binary was built without -tags webview")
	os.Exit(hvexit.Unavailable)
}
