package mail

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	srp "github.com/ProtonMail/go-srp"
	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/ProtonMail/gopenpgp/v2/helper"
	"github.com/roman-16/proton-cli/internal/proton"
)

// eoAddress builds an encrypted-for-outside sub-package: the body session key
// and attachment keys are wrapped with the password, and a random token plus an
// SRP verifier let the recipient authenticate to Proton's EO viewer.
func eoAddress(sessionKey *pgp.SessionKey, password, hint string, atts []*draftAttachment, modulus, modulusID string) (map[string]any, error) {
	bodyKP, err := pgp.EncryptSessionKeyWithPassword(sessionKey, []byte(password))
	if err != nil {
		return nil, err
	}
	tokenSK, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	token := base64.StdEncoding.EncodeToString(tokenSK.Key)
	encToken, err := helper.EncryptMessageWithPassword([]byte(password), token)
	if err != nil {
		return nil, err
	}
	auth, err := eoAuth(password, modulus, modulusID)
	if err != nil {
		return nil, err
	}
	addr := map[string]any{
		"Type":          pkgEO,
		"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		"Token":         token,
		"EncToken":      encToken,
		"Auth":          auth,
		"Signature":     0,
	}
	if hint != "" {
		addr["PasswordHint"] = hint
	}
	akp, err := attachmentPasswordKeyPackets(atts, password)
	if err != nil {
		return nil, err
	}
	if akp != nil {
		addr["AttachmentKeyPackets"] = akp
	}
	return addr, nil
}

// eoAuth builds the SRP verifier the EO recipient uses to prove knowledge of the
// password, reusing the shared modulus with a fresh per-recipient salt.
func eoAuth(password, modulus, modulusID string) (map[string]any, error) {
	salt := make([]byte, 10)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	auth, err := srp.NewAuthForVerifier([]byte(password), modulus, salt)
	if err != nil {
		return nil, err
	}
	verifier, err := auth.GenerateVerifier(2048)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Version":   srpAuthVersion,
		"ModulusID": modulusID,
		"Salt":      base64.StdEncoding.EncodeToString(salt),
		"Verifier":  base64.StdEncoding.EncodeToString(verifier),
	}, nil
}

func (s *Service) fetchModulus(ctx context.Context) (modulus, modulusID string, err error) {
	var m struct{ Modulus, ModulusID string }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/auth/modulus"}, &m); err != nil {
		return "", "", err
	}
	return m.Modulus, m.ModulusID, nil
}
