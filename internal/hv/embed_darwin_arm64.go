//go:build embed_hv && darwin && arm64

package hv

import _ "embed"

//go:embed assets/proton-cli-hv-darwin-arm64
var helperBinary []byte
