package mail

import (
	"encoding/base64"
	"fmt"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/render"
)

// buildBodyPackages encrypts the plaintext/HTML body once under a shared session
// key and returns up to three packages (internal, EO, cleartext) that reference
// it, keyed per recipient scheme.
func (s *Service) buildBodyPackages(mimeType string, opts SendOptions, atts []*uploadedAttachment, plans []plannedRecipient, addrKR *pgp.KeyRing, eoModulus, eoModulusID string) ([]map[string]any, error) {
	sessionKey, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	encBody, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(opts.Body), addrKR)
	if err != nil {
		return nil, err
	}
	bodyB64 := base64.StdEncoding.EncodeToString(encBody)

	internalAddrs := map[string]any{}
	eoAddrs := map[string]any{}
	clearAddrs := map[string]any{}

	for _, p := range plans {
		switch p.scheme {
		case schemeInternal:
			recKR, err := keyRingFromArmored(p.armoredKey)
			if err != nil {
				return nil, fmt.Errorf("parse recipient key for %s: %w", p.email, err)
			}
			recKP, err := recKR.EncryptSessionKey(sessionKey)
			if err != nil {
				return nil, err
			}
			addr := map[string]any{
				"Type":          pkgInternal,
				"BodyKeyPacket": base64.StdEncoding.EncodeToString(recKP),
				"Signature":     0,
			}
			akp, err := attachmentKeyPackets(recKR, atts)
			if err != nil {
				return nil, err
			}
			if akp != nil {
				addr["AttachmentKeyPackets"] = akp
			}
			internalAddrs[p.email] = addr
		case schemeEO:
			addr, err := eoAddress(sessionKey, opts.EOPassword, opts.EOPasswordHint, atts, eoModulus, eoModulusID)
			if err != nil {
				return nil, err
			}
			eoAddrs[p.email] = addr
		case schemeClear:
			clearAddrs[p.email] = map[string]any{"Type": pkgClear, "Signature": 0}
		}
	}

	var packages []map[string]any
	if len(internalAddrs) > 0 {
		bodyKP, err := addrKR.EncryptSessionKey(sessionKey)
		if err != nil {
			return nil, err
		}
		packages = append(packages, map[string]any{
			"Addresses":     internalAddrs,
			"MIMEType":      mimeType,
			"Type":          pkgInternal,
			"Body":          bodyB64,
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		})
	}
	if len(eoAddrs) > 0 {
		packages = append(packages, map[string]any{
			"Addresses": eoAddrs,
			"MIMEType":  mimeType,
			"Type":      pkgEO,
			"Body":      bodyB64,
		})
	}
	if len(clearAddrs) > 0 {
		clearPkg := map[string]any{
			"Addresses": clearAddrs,
			"MIMEType":  mimeType,
			"Type":      pkgClear,
			"Body":      bodyB64,
			"BodyKey":   map[string]any{"Key": base64.StdEncoding.EncodeToString(sessionKey.Key), "Algorithm": sessionKey.Algo},
		}
		if ak := attachmentCleartextKeys(atts); ak != nil {
			clearPkg["AttachmentKeys"] = ak
		}
		packages = append(packages, clearPkg)
	}
	return packages, nil
}

// buildInlinePackage builds the SEND_PGP_INLINE package: a plaintext body
// encrypted under a fresh session key, with the session key and each
// attachment key wrapped to every inline recipient's pinned PGP key. Inline is
// plaintext-only, so an HTML message body is flattened via HTMLToText. Returns
// ok=false when no recipient uses PGP-Inline.
func (s *Service) buildInlinePackage(opts SendOptions, atts []*uploadedAttachment, plans []plannedRecipient, addrKR *pgp.KeyRing) (map[string]any, bool, error) {
	body := opts.Body
	if opts.HTML {
		body = render.HTMLToText(opts.Body)
	}
	addrs := map[string]any{}
	var sessionKey *pgp.SessionKey
	var bodyB64 string
	for _, p := range plans {
		if p.scheme != schemeExternalInline {
			continue
		}
		if sessionKey == nil {
			sk, err := pgp.GenerateSessionKey()
			if err != nil {
				return nil, false, err
			}
			enc, err := sk.EncryptAndSign(pgp.NewPlainMessageFromString(body), addrKR)
			if err != nil {
				return nil, false, err
			}
			sessionKey = sk
			bodyB64 = base64.StdEncoding.EncodeToString(enc)
		}
		recKR, err := keyRingFromArmored(p.armoredKey)
		if err != nil {
			return nil, false, fmt.Errorf("parse recipient key for %s: %w", p.email, err)
		}
		recKP, err := recKR.EncryptSessionKey(sessionKey)
		if err != nil {
			return nil, false, err
		}
		addr := map[string]any{
			"Type":          pkgPGPInline,
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(recKP),
			"Signature":     0,
		}
		akp, err := attachmentKeyPackets(recKR, atts)
		if err != nil {
			return nil, false, err
		}
		if akp != nil {
			addr["AttachmentKeyPackets"] = akp
		}
		addrs[p.email] = addr
	}
	if len(addrs) == 0 {
		return nil, false, nil
	}
	bodyKP, err := addrKR.EncryptSessionKey(sessionKey)
	if err != nil {
		return nil, false, err
	}
	return map[string]any{
		"Addresses":     addrs,
		"MIMEType":      "text/plain",
		"Type":          pkgPGPInline,
		"Body":          bodyB64,
		"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
	}, true, nil
}

// buildPGPMIMEPackage builds the multipart/mixed MIME body (with embedded
// attachments), encrypts it under a fresh session key, and wraps that key to
// each external-PGP recipient's key. Returns ok=false when no recipient uses
// PGP/MIME.
func (s *Service) buildPGPMIMEPackage(opts SendOptions, prepared []preparedAttachment, plans []plannedRecipient, addrKR *pgp.KeyRing) (map[string]any, bool, error) {
	mimeType := "text/plain"
	if opts.HTML {
		mimeType = "text/html"
	}
	addrs := map[string]any{}
	var sessionKey *pgp.SessionKey
	var bodyB64 string

	for _, p := range plans {
		if p.scheme != schemeExternalPGP {
			continue
		}
		if sessionKey == nil {
			mimeStr, err := buildMIMEMessage(opts.Body, mimeType, prepared)
			if err != nil {
				return nil, false, err
			}
			sessionKey, err = pgp.GenerateSessionKey()
			if err != nil {
				return nil, false, err
			}
			enc, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(mimeStr), addrKR)
			if err != nil {
				return nil, false, err
			}
			bodyB64 = base64.StdEncoding.EncodeToString(enc)
		}
		recKR, err := keyRingFromArmored(p.armoredKey)
		if err != nil {
			return nil, false, fmt.Errorf("parse recipient key for %s: %w", p.email, err)
		}
		kp, err := recKR.EncryptSessionKey(sessionKey)
		if err != nil {
			return nil, false, err
		}
		addrs[p.email] = map[string]any{
			"Type":          pkgPGPMIME,
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(kp),
		}
	}
	if len(addrs) == 0 {
		return nil, false, nil
	}
	return map[string]any{
		"Addresses": addrs,
		"MIMEType":  "multipart/mixed",
		"Type":      pkgPGPMIME,
		"Body":      bodyB64,
	}, true, nil
}

func keyRingFromArmored(armored string) (*pgp.KeyRing, error) {
	key, err := pgp.NewKeyFromArmored(armored)
	if err != nil {
		return nil, err
	}
	return pgp.NewKeyRing(key)
}
