package mail

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

type rawListMessage struct {
	ID             string
	Subject        string
	Unread         int
	Time           int64
	Sender         struct{ Name, Address string }
	NumAttachments int
}

func toMessage(m rawListMessage) Message {
	return Message{
		ID: m.ID, Subject: m.Subject, Unread: m.Unread, Time: m.Time,
		FromName: m.Sender.Name, FromAddress: m.Sender.Address,
		NumAttachments: m.NumAttachments,
	}
}

// List returns a page of messages.
func (s *Service) List(ctx context.Context, opts ListOptions) ([]Message, int, error) {
	if opts.PageSize <= 0 {
		opts.PageSize = 25
	}
	q := url.Values{}
	q.Set("LabelID", ResolveFolder(opts.Folder))
	q.Set("Page", fmt.Sprintf("%d", opts.Page))
	q.Set("PageSize", fmt.Sprintf("%d", opts.PageSize))
	q.Set("Sort", "Time")
	q.Set("Desc", "1")
	if opts.Unread {
		q.Set("Unread", "1")
	}
	var r struct {
		Total    int
		Messages []rawListMessage
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages", Query: q}, &r); err != nil {
		return nil, 0, err
	}
	out := make([]Message, 0, len(r.Messages))
	for _, m := range r.Messages {
		out = append(out, toMessage(m))
	}
	return out, r.Total, nil
}

// Search returns messages matching the given filters.
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]Message, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	q := searchQuery(opts, false)
	if err := validateDates(opts, q); err != nil {
		return nil, 0, err
	}
	var r struct {
		Total    int
		Messages []rawListMessage
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages", Query: q}, &r); err != nil {
		return nil, 0, err
	}
	out := make([]Message, 0, len(r.Messages))
	for _, m := range r.Messages {
		out = append(out, toMessage(m))
	}
	return out, r.Total, nil
}

// searchQuery builds the shared messages/conversations search query. recipients
// switches the "To" field name to "Recipients" (conversations endpoint).
func searchQuery(opts SearchOptions, recipients bool) url.Values {
	folder := opts.Folder
	if folder == "" {
		folder = "all"
	}
	q := url.Values{}
	q.Set("LabelID", ResolveFolder(folder))
	q.Set("Sort", "Time")
	q.Set("Desc", "1")
	q.Set("PageSize", fmt.Sprintf("%d", opts.Limit))
	if opts.Unread {
		q.Set("Unread", "1")
	}
	if opts.Keyword != "" {
		q.Set("Keyword", opts.Keyword)
	}
	if opts.From != "" {
		q.Set("From", opts.From)
	}
	if opts.To != "" {
		if recipients {
			q.Set("Recipients", opts.To)
		} else {
			q.Set("To", opts.To)
		}
	}
	if opts.Subject != "" {
		q.Set("Subject", opts.Subject)
	}
	return q
}

func validateDates(opts SearchOptions, q url.Values) error {
	if opts.After != "" {
		t, err := time.Parse("2006-01-02", opts.After)
		if err != nil {
			return fmt.Errorf("invalid --after: %w", err)
		}
		q.Set("Begin", fmt.Sprintf("%d", t.Unix()))
	}
	if opts.Before != "" {
		t, err := time.Parse("2006-01-02", opts.Before)
		if err != nil {
			return fmt.Errorf("invalid --before: %w", err)
		}
		q.Set("End", fmt.Sprintf("%d", t.Unix()))
	}
	return nil
}

// rawMessage is the API shape for a single message envelope.
type rawMessage struct {
	ID          string
	Subject     string
	Sender      map[string]any
	ToList      []map[string]any
	Time        int64
	Body        string
	MIMEType    string
	AddressID   string
	Attachments []struct {
		ID, Name, MIMEType, KeyPackets, Disposition string
		Size                                        int64
	}
}

func (s *Service) decryptMessage(u *keys.Unlocked, m rawMessage) Full {
	addrKR, ok := u.AddrKR(m.AddressID)
	if !ok {
		if kr, _, _, err := u.FirstAddrKR(); err == nil {
			addrKR = kr
		}
	}
	var body string
	if addrKR == nil {
		body = "(decryption failed: no address key available)"
	} else if b, err := decryptBody(m.Body, addrKR); err != nil {
		body = "(decryption failed: " + err.Error() + ")"
	} else {
		body = b
	}
	atts := make([]Attachment, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		atts = append(atts, Attachment{
			ID: a.ID, Name: a.Name, Size: a.Size, MIMEType: a.MIMEType,
			Disposition: a.Disposition, KeyPackets: a.KeyPackets,
		})
	}
	return Full{
		ID: m.ID, Subject: m.Subject, Sender: m.Sender, ToList: m.ToList,
		Time: m.Time, Body: body, MIMEType: m.MIMEType, AddressID: m.AddressID,
		Attachments: atts,
	}
}

// Read returns a single message with decrypted body.
func (s *Service) Read(ctx context.Context, u *keys.Unlocked, id string) (*Full, error) {
	raw, err := s.fetchMessageRaw(ctx, id)
	if err != nil {
		return nil, s.crossTableProbe(ctx, id, err, "messages")
	}
	full := s.decryptMessage(u, *raw)
	return &full, nil
}

func (s *Service) fetchMessageRaw(ctx context.Context, id string) (*rawMessage, error) {
	var r struct{ Message rawMessage }
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages/" + id}, &r); err != nil {
		return nil, err
	}
	return &r.Message, nil
}

// AssertMessageKind probes the messages endpoint for an ID-shaped string and
// returns *WrongTableError when the ID belongs to the conversations table.
func (s *Service) AssertMessageKind(ctx context.Context, id string) error {
	err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/messages/" + id}, nil)
	if err == nil {
		return nil
	}
	return s.crossTableProbe(ctx, id, err, "messages")
}

// Trash moves messages to trash.
func (s *Service) Trash(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/label", Body: map[string]any{"LabelID": "3", "IDs": ids}}, nil)
}

// Delete permanently deletes messages.
func (s *Service) Delete(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/delete", Body: map[string]any{"IDs": ids}}, nil)
}

// Move moves messages to the given folder.
func (s *Service) Move(ctx context.Context, ids []string, folder string) error {
	return s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/label", Body: map[string]any{"LabelID": ResolveFolder(folder), "IDs": ids}}, nil)
}

// Mark sets read/unread/starred flags on messages.
func (s *Service) Mark(ctx context.Context, ids []string, read, unread, starred, unstar bool) error {
	body := map[string]any{"IDs": ids}
	if read {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/read", Body: body}, nil); err != nil {
			return err
		}
	}
	if unread {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/unread", Body: body}, nil); err != nil {
			return err
		}
	}
	if starred {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/label", Body: map[string]any{"LabelID": "10", "IDs": ids}}, nil); err != nil {
			return err
		}
	}
	if unstar {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/messages/unlabel", Body: map[string]any{"LabelID": "10", "IDs": ids}}, nil); err != nil {
			return err
		}
	}
	return nil
}
