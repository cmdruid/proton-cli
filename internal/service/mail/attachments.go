package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"

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

// ConversationAttachmentsList returns the union of attachments across all
// messages in the conversation, ordered by message Time ascending.
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

// AttachmentsList returns the attachment metadata for a message. Inline
// attachments are filtered out unless includeInline is true.
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

// AttachmentDownload returns the decrypted bytes of a single attachment.
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
