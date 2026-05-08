//go:build embed_hv && windows && amd64

package hv

import _ "embed"

//go:embed assets/proton-cli-hv-windows-amd64.exe
var helperBinary []byte
