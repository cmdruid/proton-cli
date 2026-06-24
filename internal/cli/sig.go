package cli

import "github.com/roman-16/proton-cli/internal/crypto/pgp"

// sigText renders a signature verdict for text output. Only the alarming
// verdict is upper-cased so a genuine failure stands out; unsigned/unverified
// stay low-key because they are common and benign (external senders, mailing
// lists, keys we simply don't hold).
func sigText(v pgp.VerifyResult) string {
	if v == pgp.Invalid {
		return "INVALID"
	}
	return string(v)
}
