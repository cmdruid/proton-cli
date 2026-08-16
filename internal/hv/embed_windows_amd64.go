//go:build embed_hv && windows && amd64

package hv

import _ "embed"

//go:embed assets/proton-hv-windows-amd64.exe
var helperBinary []byte
