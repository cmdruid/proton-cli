//go:build embed_hv && windows && arm64

package hv

import _ "embed"

//go:embed assets/proton-hv-windows-arm64.exe
var helperBinary []byte
