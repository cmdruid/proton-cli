//go:build !embed_hv

// Default build (no `-tags embed_hv`): the helper is NOT embedded.
// extractHelper() returns ErrHelperMissing, the cli layer surfaces
// that as "this build of proton-cli has no captcha-HV support; please
// log in via Proton web/mobile to clear HV, or rebuild with
// -tags embed_hv after producing helper assets via goreleaser".
//
// Goreleaser overrides this stub with the real per-platform embed by
// building with `-tags embed_hv`.
package hv

// helperBinary is empty in default builds. extractHelper checks
// len() == 0 and returns ErrHelperMissing.
var helperBinary []byte
