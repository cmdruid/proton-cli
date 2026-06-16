package mail

import (
	"context"
	"encoding/base64"
	"fmt"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

// Send sends a new mail. Handles both internal (PGP) and external recipients.
func (s *Service) Send(ctx context.Context, u *keys.Unlocked, to, subject, body string) error {
	addrKR, _, senderEmail, err := u.FirstAddrKR()
	if err != nil {
		return err
	}

	plainMsg := pgp.NewPlainMessageFromString(body)
	encDraft, err := addrKR.Encrypt(plainMsg, addrKR)
	if err != nil {
		return fmt.Errorf("encrypt draft: %w", err)
	}
	armoredDraft, err := encDraft.GetArmored()
	if err != nil {
		return err
	}
	var draft struct {
		Code    int
		Message struct{ ID string }
	}
	draftBody := map[string]any{
		"Message": map[string]any{
			"ToList":   []map[string]string{{"Address": to, "Name": to}},
			"CCList":   []any{},
			"BCCList":  []any{},
			"Subject":  subject,
			"Sender":   map[string]string{"Address": senderEmail, "Name": ""},
			"Body":     armoredDraft,
			"MIMEType": "text/plain",
		},
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/mail/v4/messages", Body: draftBody}, &draft); err != nil {
		return err
	}
	messageID := draft.Message.ID
	cleanup := func() {
		_, _ = s.C.Do(ctx, proton.Request{Method: "DELETE", Path: "/mail/v4/messages/delete", Body: map[string]any{"IDs": []string{messageID}}})
	}

	var keysRes struct {
		Address struct {
			Keys []struct{ PublicKey string }
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/keys/all", Query: keys.Query("Email", to)}, &keysRes); err != nil {
		cleanup()
		return err
	}
	internal := len(keysRes.Address.Keys) > 0

	sessionKey, err := pgp.GenerateSessionKey()
	if err != nil {
		cleanup()
		return err
	}
	encBody, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(body), addrKR)
	if err != nil {
		cleanup()
		return err
	}

	var packages []map[string]any
	if internal {
		bodyKP, err := addrKR.EncryptSessionKey(sessionKey)
		if err != nil {
			cleanup()
			return err
		}
		recKey, err := pgp.NewKeyFromArmored(keysRes.Address.Keys[0].PublicKey)
		if err != nil {
			cleanup()
			return fmt.Errorf("parse recipient key: %w", err)
		}
		recKR, err := pgp.NewKeyRing(recKey)
		if err != nil {
			cleanup()
			return err
		}
		recKP, err := recKR.EncryptSessionKey(sessionKey)
		if err != nil {
			cleanup()
			return err
		}
		packages = []map[string]any{{
			"Addresses": map[string]any{
				to: map[string]any{"Type": 1, "BodyKeyPacket": base64.StdEncoding.EncodeToString(recKP), "Signature": 0},
			},
			"MIMEType":      "text/plain",
			"Type":          1,
			"Body":          base64.StdEncoding.EncodeToString(encBody),
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		}}
	} else {
		packages = []map[string]any{{
			"Addresses": map[string]any{to: map[string]any{"Type": 4, "Signature": 0}},
			"MIMEType":  "text/plain",
			"Type":      4,
			"Body":      base64.StdEncoding.EncodeToString(encBody),
			"BodyKey":   map[string]any{"Key": base64.StdEncoding.EncodeToString(sessionKey.Key), "Algorithm": sessionKey.Algo},
		}}
	}

	sendReq := map[string]any{"ExpirationTime": nil, "AutoSaveContacts": 0, "Packages": packages}
	resp, err := s.C.Do(ctx, proton.Request{Method: "POST", Path: "/mail/v4/messages/" + messageID, Body: sendReq})
	if err != nil {
		cleanup()
		return err
	}
	if resp.Status >= 400 {
		cleanup()
		return fmt.Errorf("send failed: %s", string(resp.Body))
	}
	return nil
}
