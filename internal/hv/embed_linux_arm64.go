//go:build embed_hv && linux && arm64

package hv

import _ "embed"

//go:embed assets/proton-hv-linux-arm64
var helperBinary []byte
