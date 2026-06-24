package pgp

import (
	"errors"

	"github.com/ProtonMail/gopenpgp/v2/constants"
	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
)

// VerifyResult is the outcome of checking a PGP signature. It mirrors the
// granularity gopenpgp exposes (and is a superset of Proton's web-client
// VERIFICATION_STATUS): a missing/invalid signature is a fact to surface, not a
// control-flow error.
type VerifyResult string

const (
	// Verified: a signature is present and valid.
	Verified VerifyResult = "verified"
	// Unsigned: no signature was present.
	Unsigned VerifyResult = "unsigned"
	// Unverified: a signature is present but could not be checked (the signer's
	// key is unavailable). Benign and common - not "forged".
	Unverified VerifyResult = "unverified"
	// Invalid: a signature is present and cryptographically wrong - the only
	// alarming verdict (tampering or spoofing).
	Invalid VerifyResult = "invalid"
)

// Classify maps a gopenpgp Decrypt / VerifyDetached error to a VerifyResult.
// A nil error means the signature verified. A non-signature error (a genuine
// decryption failure) maps to Unverified; callers that care about the
// distinction should inspect the error themselves before calling Classify.
func Classify(err error) VerifyResult {
	if err == nil {
		return Verified
	}
	var sigErr pgp.SignatureVerificationError
	if errors.As(err, &sigErr) {
		switch sigErr.Status {
		case constants.SIGNATURE_OK:
			return Verified
		case constants.SIGNATURE_NOT_SIGNED:
			return Unsigned
		case constants.SIGNATURE_NO_VERIFIER:
			return Unverified
		default: // SIGNATURE_FAILED, SIGNATURE_BAD_CONTEXT
			return Invalid
		}
	}
	return Unverified
}

// VerifyDetachedStatus verifies an armored detached signature over msg with kr.
//
// Detached verification cannot distinguish a cryptographically bad signature
// from one made by a key the verifier does not hold (rotated, inactive, or
// another party): gopenpgp reports both as a failure. To avoid crying "invalid"
// over what may simply be a key we lack, this never returns Invalid - both map
// to Unverified. Reserve Invalid for embedded-signature decryption (via
// Classify), where gopenpgp does distinguish "no verifier" from "bad signature".
func VerifyDetachedStatus(kr *pgp.KeyRing, msg *pgp.PlainMessage, sigArmored string) VerifyResult {
	if sigArmored == "" {
		return Unsigned
	}
	if kr == nil {
		return Unverified
	}
	sig, err := pgp.NewPGPSignatureFromArmored(sigArmored)
	if err != nil {
		return Unverified
	}
	if r := Classify(kr.VerifyDetached(msg, sig, pgp.GetUnixTime())); r != Invalid {
		return r
	}
	return Unverified
}

// Aggregate combines per-part verdicts into one, mirroring Proton's
// getAggregatedEventVerificationStatus: any Invalid wins, then any Unverified,
// then any Verified, else Unsigned. The empty set is Unsigned.
func Aggregate(rs ...VerifyResult) VerifyResult {
	var sawUnverified, sawVerified bool
	for _, r := range rs {
		switch r {
		case Invalid:
			return Invalid
		case Unverified:
			sawUnverified = true
		case Verified:
			sawVerified = true
		}
	}
	switch {
	case sawUnverified:
		return Unverified
	case sawVerified:
		return Verified
	default:
		return Unsigned
	}
}
