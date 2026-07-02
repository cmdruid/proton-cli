package app

import (
	"os"
	"strings"
)

// profileEnvSegment normalizes a profile name into the segment used in
// profile-scoped environment variables: upper-cased, with every rune outside
// [A-Z0-9] replaced by '_'. "work" -> "WORK", "my-work" -> "MY_WORK".
func profileEnvSegment(profile string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(profile) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// envForProfile resolves a PROTON_ setting for the active profile, preferring
// the profile-scoped PROTON_<SEGMENT>_<base> variable and falling back to the
// unscoped PROTON_<base>. A variable set to the empty string counts as unset,
// so an empty scoped value still falls through to the unscoped one (matching
// firstNonEmpty's semantics at the call site).
func envForProfile(profile, base string) string {
	if seg := profileEnvSegment(profile); seg != "" {
		if v := os.Getenv("PROTON_" + seg + "_" + base); v != "" {
			return v
		}
	}
	return os.Getenv("PROTON_" + base)
}
