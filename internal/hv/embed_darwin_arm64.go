//go:build embed_hv && darwin && arm64

package hv

import _ "embed"

//go:embed assets/proton-hv-darwin-arm64
var helperBinary []byte
