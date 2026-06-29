package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

// SendOptions describes a message to send. To/CC/BCC may each carry multiple
// recipients; at least one across the three is required. HTML switches the
// body MIME type to text/html.
type SendOptions struct {
	To      []string
	CC      []string
	BCC     []string
	Subject string
	Body    string
	HTML    bool
	// Attachments are local file paths to encrypt and attach.
	Attachments []string
	// InlineAttachments are in-memory attachments (e.g. a generated ICS).
	InlineAttachments []InlineAttachment
	// DeliveryTime, when > 0, schedules the send for that absolute Unix time.
	DeliveryTime int64
	// ExpiresInSeconds, when > 0, makes the message self-destruct after that
	// many seconds.
	ExpiresInSeconds int
}

// Send packages the body per recipient: internal (Proton) recipients each get
// the body session key encrypted to their key; external recipients share a
// cleartext session key. To/CC/BCC differ only on the draft envelope — at the
// send-package level every recipient is grouped purely by encryption type.
func (s *Service) Send(ctx context.Context, u *keys.Unlocked, opts SendOptions) error {
	addrKR, _, senderEmail, err := u.FirstAddrKR()
	if err != nil {
		return err
	}

	mimeType := "text/plain"
	if opts.HTML {
		mimeType = "text/html"
	}

	plainMsg := pgp.NewPlainMessageFromString(opts.Body)
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
			"ToList":   recipientList(opts.To),
			"CCList":   recipientList(opts.CC),
			"BCCList":  recipientList(opts.BCC),
			"Subject":  opts.Subject,
			"Sender":   map[string]string{"Address": senderEmail, "Name": ""},
			"Body":     armoredDraft,
			"MIMEType": mimeType,
		},
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "POST", Path: "/mail/v4/messages", Body: draftBody}, &draft); err != nil {
		return err
	}
	messageID := draft.Message.ID
	cleanup := func() {
		_, _ = s.C.Do(ctx, proton.Request{Method: "DELETE", Path: "/mail/v4/messages/delete", Body: map[string]any{"IDs": []string{messageID}}})
	}

	var atts []*uploadedAttachment
	for _, path := range opts.Attachments {
		a, err := s.uploadAttachment(ctx, addrKR, messageID, path)
		if err != nil {
			cleanup()
			return err
		}
		atts = append(atts, a)
	}
	for _, ia := range opts.InlineAttachments {
		a, err := s.uploadAttachmentData(ctx, addrKR, messageID, ia.Filename, ia.MIMEType, ia.Data)
		if err != nil {
			cleanup()
			return err
		}
		atts = append(atts, a)
	}

	sessionKey, err := pgp.GenerateSessionKey()
	if err != nil {
		cleanup()
		return err
	}
	encBody, err := sessionKey.EncryptAndSign(pgp.NewPlainMessageFromString(opts.Body), addrKR)
	if err != nil {
		cleanup()
		return err
	}

	// Group recipients by encryption type. Internal recipients each receive
	// their own BodyKeyPacket; external recipients share one cleartext BodyKey.
	internalAddrs := map[string]any{}
	externalAddrs := map[string]any{}
	for _, email := range dedupeRecipients(opts.To, opts.CC, opts.BCC) {
		var keysRes struct {
			Address struct {
				Keys []struct{ PublicKey string }
			}
		}
		if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/core/v4/keys/all", Query: keys.Query("Email", email)}, &keysRes); err != nil {
			cleanup()
			return err
		}
		if len(keysRes.Address.Keys) > 0 {
			recKey, err := pgp.NewKeyFromArmored(keysRes.Address.Keys[0].PublicKey)
			if err != nil {
				cleanup()
				return fmt.Errorf("parse recipient key for %s: %w", email, err)
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
			addr := map[string]any{
				"Type":          1,
				"BodyKeyPacket": base64.StdEncoding.EncodeToString(recKP),
				"Signature":     0,
			}
			akp, err := attachmentKeyPackets(recKR, atts)
			if err != nil {
				cleanup()
				return err
			}
			if akp != nil {
				addr["AttachmentKeyPackets"] = akp
			}
			internalAddrs[email] = addr
		} else {
			externalAddrs[email] = map[string]any{"Type": 4, "Signature": 0}
		}
	}

	var packages []map[string]any
	if len(internalAddrs) > 0 {
		bodyKP, err := addrKR.EncryptSessionKey(sessionKey)
		if err != nil {
			cleanup()
			return err
		}
		packages = append(packages, map[string]any{
			"Addresses":     internalAddrs,
			"MIMEType":      mimeType,
			"Type":          1,
			"Body":          base64.StdEncoding.EncodeToString(encBody),
			"BodyKeyPacket": base64.StdEncoding.EncodeToString(bodyKP),
		})
	}
	if len(externalAddrs) > 0 {
		extPkg := map[string]any{
			"Addresses": externalAddrs,
			"MIMEType":  mimeType,
			"Type":      4,
			"Body":      base64.StdEncoding.EncodeToString(encBody),
			"BodyKey":   map[string]any{"Key": base64.StdEncoding.EncodeToString(sessionKey.Key), "Algorithm": sessionKey.Algo},
		}
		if ak := attachmentCleartextKeys(atts); ak != nil {
			extPkg["AttachmentKeys"] = ak
		}
		packages = append(packages, extPkg)
	}

	sendReq := map[string]any{"ExpirationTime": nil, "AutoSaveContacts": 0, "Packages": packages}
	if opts.DeliveryTime > 0 {
		sendReq["DeliveryTime"] = opts.DeliveryTime
	}
	if opts.ExpiresInSeconds > 0 {
		sendReq["ExpiresIn"] = opts.ExpiresInSeconds
	}
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

func recipientList(emails []string) []map[string]string {
	out := make([]map[string]string, 0, len(emails))
	for _, e := range emails {
		out = append(out, map[string]string{"Address": e, "Name": e})
	}
	return out
}

// dedupeRecipients flattens To/CC/BCC into one list, dropping case-insensitive
// duplicates while preserving first-seen order.
func dedupeRecipients(lists ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, list := range lists {
		for _, e := range list {
			key := strings.ToLower(e)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}
