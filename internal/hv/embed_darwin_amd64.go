//go:build embed_hv && darwin && amd64

package hv

import _ "embed"

//go:embed assets/proton-hv-darwin-amd64
var helperBinary []byte
