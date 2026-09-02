package kit

import (
	"github.com/cmdruid/proton-cli/internal/crypto/pgp"
	"github.com/cmdruid/proton-cli/internal/ui"
)

// SignatureField reports whether what was decrypted is also provably from who it
// claims to be.
//
// It is the one field in the CLI that carries a verdict rather than a value, and
// the only one where not noticing is a security problem rather than an
// inconvenience: end-to-end encryption is what this tool is for, and "invalid"
// means tampered or spoofed. Printed in the same grey as the date above it, that
// word is invisible; a reader scanning a message header has no reason to stop on
// it.
//
// So the verdict is painted. The word is unchanged and carries the meaning on
// its own, which is what keeps a pipe, --no-color and a colour-blind reader
// equally well served - the colour only decides whether the eye stops there.
func SignatureField(verdict string) ui.Field {
	return ui.Field{
		Label:  "Signature",
		Value:  verdict,
		Always: true,
		Role:   signatureRole(verdict),
	}
}

// signatureRole maps gopenpgp's four verdicts onto the three the reader needs.
//
// Only Invalid is bad. Unverified is the common and benign case - the signer's
// key is simply not available here - so treating it as an alarm would teach
// people to ignore the one verdict that is one. Unsigned is a fact about the
// sender, not about this message, and is left plain.
func signatureRole(verdict string) ui.Role {
	switch pgp.VerifyResult(verdict) {
	case pgp.Verified:
		return ui.Success
	case pgp.Unverified:
		return ui.Caution
	case pgp.Invalid:
		return ui.Danger
	}
	return ui.Plain
}
