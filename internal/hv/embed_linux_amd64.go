//go:build embed_hv && linux && amd64

package hv

import _ "embed"

//go:embed assets/proton-cli-hv-linux-amd64
var helperBinary []byte
