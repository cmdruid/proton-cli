//go:build embed_hv && linux && amd64

package hv

import _ "embed"

//go:embed assets/proton-hv-linux-amd64
var helperBinary []byte
