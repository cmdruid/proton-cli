package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// ConversationAttachment carries its parent MessageID so callers can
// disambiguate and download against the correct message.
type ConversationAttachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	MIMEType    string `json:"mime_type"`
	Disposition string `json:"disposition"`
	MessageID   string `json:"message_id"`
}

func (s *Service) ConversationAttachmentsList(ctx context.Context, convID string, includeInline bool) ([]ConversationAttachment, error) {
	var r struct{ Messages []rawMessage }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/conversations/" + convID}, &r); err != nil {
		return nil, s.crossTableProbe(ctx, convID, err, "conversations")
	}
	sort.SliceStable(r.Messages, func(i, j int) bool { return r.Messages[i].Time < r.Messages[j].Time })
	var out []ConversationAttachment
	for _, m := range r.Messages {
		for _, a := range m.Attachments {
			att := Attachment{Disposition: a.Disposition}
			if !includeInline && att.IsInline() {
				continue
			}
			out = append(out, ConversationAttachment{
				ID: a.ID, Name: a.Name, Size: a.Size,
				MIMEType: a.MIMEType, Disposition: a.Disposition, MessageID: m.ID,
			})
		}
	}
	return out, nil
}

// AttachmentsList drops inline attachments unless includeInline is set.
func (s *Service) AttachmentsList(ctx context.Context, msgID string, includeInline bool) ([]Attachment, error) {
	var r struct {
		Message struct {
			Attachments []struct {
				ID, Name, MIMEType, Disposition string
				Size                            int64
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages/" + msgID}, &r); err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(r.Message.Attachments))
	for _, a := range r.Message.Attachments {
		out = append(out, Attachment{ID: a.ID, Name: a.Name, Size: a.Size, MIMEType: a.MIMEType, Disposition: a.Disposition})
	}
	if !includeInline {
		out = FilterInline(out)
	}
	return out, nil
}

func (s *Service) AttachmentDownload(ctx context.Context, u *keys.Unlocked, msgID, attID string) ([]byte, string, error) {
	var r struct {
		Message struct {
			AddressID   string
			Attachments []struct {
				ID, Name, KeyPackets string
			}
		}
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages/" + msgID}, &r); err != nil {
		return nil, "", err
	}
	var keyPackets, name string
	for _, a := range r.Message.Attachments {
		if a.ID == attID {
			keyPackets, name = a.KeyPackets, a.Name
			break
		}
	}
	if keyPackets == "" {
		return nil, "", &errs.NotFound{Kind: "attachment", Ref: attID}
	}
	addrKR, ok := u.AddrKR(r.Message.AddressID)
	if !ok {
		kr, _, _, err := u.FirstAddrKR()
		if err != nil {
			return nil, "", err
		}
		addrKR = kr
	}
	resp, err := s.C.Do(ctx, proton.Request{Method: "GET", Path: "/mail/v4/attachments/" + attID})
	if err != nil {
		return nil, "", err
	}
	kp, err := base64.StdEncoding.DecodeString(keyPackets)
	if err != nil {
		return nil, "", fmt.Errorf("decode key packets: %w", err)
	}
	split := pgp.NewPGPSplitMessage(kp, resp.Body)
	dec, err := addrKR.Decrypt(split.GetPGPMessage(), nil, 0)
	if err != nil {
		return nil, "", fmt.Errorf("decrypt attachment: %w", err)
	}
	return dec.GetBinary(), name, nil
}

// InlineAttachment is an in-memory attachment supplied by a caller (e.g. a
// calendar ICS invitation) rather than read from disk.
type InlineAttachment struct {
	Filename string
	MIMEType string
	Data     []byte
}

// uploadedAttachment couples a server attachment ID with the session key its
// data packet was encrypted under, so the key can be re-wrapped per recipient
// at send time.
type uploadedAttachment struct {
	ID         string
	SessionKey *pgp.SessionKey
}

// preparedAttachment is an attachment's raw bytes plus metadata, gathered once
// so it can be both uploaded (for internal/EO/cleartext packages) and embedded
// verbatim in a PGP/MIME body. A non-empty ContentID marks the part inline
// (an image embedded in the HTML body via cid:), which Proton records as
// disposition "inline".
type preparedAttachment struct {
	Filename  string
	MIMEType  string
	Data      []byte
	ContentID string
}

// prepareAttachments reads local files and normalizes inline attachments into a
// single list, resolving the MIME type from the file extension when needed.
func prepareAttachments(paths []string, inline []InlineAttachment) ([]preparedAttachment, error) {
	out := make([]preparedAttachment, 0, len(paths)+len(inline))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		out = append(out, preparedAttachment{Filename: filepath.Base(path), MIMEType: mimeType, Data: data})
	}
	for _, ia := range inline {
		mimeType := ia.MIMEType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		out = append(out, preparedAttachment{Filename: ia.Filename, MIMEType: mimeType, Data: ia.Data})
	}
	return out, nil
}

// prepareInlineImages reads each inline image path, assigns it a Content-ID,
// appends an <img src="cid:..."> reference to the (HTML) body so the image
// renders in place, and returns the attachments to upload. Uploading with a
// Content-ID is what makes Proton record the part as disposition "inline".
// Inline images require an HTML body.
func prepareInlineImages(opts *SendOptions, senderEmail string) ([]preparedAttachment, error) {
	if len(opts.InlineAttach) == 0 {
		return nil, nil
	}
	if !opts.HTML {
		return nil, fmt.Errorf("--attach-inline requires --html (inline images need an HTML body)")
	}
	out := make([]preparedAttachment, 0, len(opts.InlineAttach))
	var imgs strings.Builder
	for _, path := range opts.InlineAttach {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		cid, err := newContentID(senderEmail)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&imgs, "<img src=%q alt=%q>", "cid:"+cid, filepath.Base(path))
		out = append(out, preparedAttachment{
			Filename: filepath.Base(path), MIMEType: mimeType, Data: data, ContentID: cid,
		})
	}
	opts.Body += imgs.String()
	return out, nil
}

// newContentID returns a Content-ID of the form <hex>@<sender-domain>, matching
// the shape Proton's web client generates.
func newContentID(senderEmail string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	domain := "proton.me"
	if i := strings.LastIndex(senderEmail, "@"); i >= 0 && i+1 < len(senderEmail) {
		domain = senderEmail[i+1:]
	}
	return hex.EncodeToString(b) + "@" + domain, nil
}

// uploadAttachmentData encrypts in-memory data with a fresh session key (key
// packet wrapped to the draft address key), detached-signs it, and uploads it
// as a multipart form against the draft message.
func (s *Service) uploadAttachmentData(ctx context.Context, addrKR *pgp.KeyRing, messageID, filename, mimeType, contentID string, data []byte) (*uploadedAttachment, error) {
	msg := pgp.NewPlainMessage(data)

	sk, err := pgp.GenerateSessionKey()
	if err != nil {
		return nil, err
	}
	dataPacket, err := sk.Encrypt(msg)
	if err != nil {
		return nil, fmt.Errorf("encrypt attachment data: %w", err)
	}
	keyPacket, err := addrKR.EncryptSessionKey(sk)
	if err != nil {
		return nil, fmt.Errorf("encrypt attachment key: %w", err)
	}
	sig, err := addrKR.SignDetached(msg)
	if err != nil {
		return nil, fmt.Errorf("sign attachment: %w", err)
	}

	body, contentType, err := buildAttachmentForm(map[string]string{
		"Filename":  filename,
		"MessageID": messageID,
		"ContentID": contentID,
		"MIMEType":  mimeType,
	}, map[string][]byte{
		"KeyPackets": keyPacket,
		"DataPacket": dataPacket,
		"Signature":  sig.GetBinary(),
	})
	if err != nil {
		return nil, err
	}

	var res struct {
		Attachment struct{ ID string }
	}
	if err := s.C.Decode(ctx, proton.Request{
		Method: "POST", Path: "/mail/v4/attachments", Body: body, ContentType: contentType,
	}, &res); err != nil {
		return nil, fmt.Errorf("upload attachment %s: %w", filename, err)
	}
	return &uploadedAttachment{ID: res.Attachment.ID, SessionKey: sk}, nil
}

func buildAttachmentForm(fields map[string]string, files map[string][]byte) (body []byte, contentType string, err error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	for name, data := range files {
		part, err := w.CreateFormFile(name, name)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// attachmentKeyPackets wraps each attachment session key to an internal
// recipient's key ring (Type-1 per-recipient packets).
func attachmentKeyPackets(recKR *pgp.KeyRing, atts []*uploadedAttachment) (map[string]string, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(atts))
	for _, a := range atts {
		kp, err := recKR.EncryptSessionKey(a.SessionKey)
		if err != nil {
			return nil, err
		}
		out[a.ID] = base64.StdEncoding.EncodeToString(kp)
	}
	return out, nil
}

// attachmentPasswordKeyPackets wraps each attachment session key with the EO
// password (symmetric packets), for encrypted-for-outside recipients.
func attachmentPasswordKeyPackets(atts []*uploadedAttachment, password string) (map[string]string, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(atts))
	for _, a := range atts {
		kp, err := pgp.EncryptSessionKeyWithPassword(a.SessionKey, []byte(password))
		if err != nil {
			return nil, err
		}
		out[a.ID] = base64.StdEncoding.EncodeToString(kp)
	}
	return out, nil
}

// attachmentCleartextKeys exposes raw attachment session keys for external
// (Type-4 cleartext) packages.
func attachmentCleartextKeys(atts []*uploadedAttachment) map[string]any {
	if len(atts) == 0 {
		return nil
	}
	out := make(map[string]any, len(atts))
	for _, a := range atts {
		out[a.ID] = map[string]any{
			"Key":       base64.StdEncoding.EncodeToString(a.SessionKey.Key),
			"Algorithm": a.SessionKey.Algo,
		}
	}
	return out
}
