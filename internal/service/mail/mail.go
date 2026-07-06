// Package mail provides Proton Mail operations.
package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	pgphelper "github.com/roman-16/proton-cli/internal/crypto/pgp"
	"github.com/roman-16/proton-cli/internal/idcache"
	"github.com/roman-16/proton-cli/internal/proton"
	"github.com/roman-16/proton-cli/internal/ref"
)

// WrongTableError signals that an ID-shaped REF was passed to the wrong
// endpoint family (a conversation ID into the messages tree, or vice versa).
// The cli layer catches this to emit a redirect hint and exit 3.
type WrongTableError struct {
	// Kind is what the ID actually is ("message" or "conversation") - the
	// OTHER table from the one the user invoked.
	Kind string
	ID   string
}

func (e *WrongTableError) Error() string {
	return fmt.Sprintf("that ID is a %s, not a %s", e.Kind, OppositeKind(e.Kind))
}
func (e *WrongTableError) ExitCode() int { return 3 }

func OppositeKind(k string) string {
	if k == "conversation" {
		return "message"
	}
	return "conversation"
}

// Built-in Proton system-label IDs. The mutation endpoints reference some of
// these directly; MailboxLabelIDs is built from the same constants so the two
// can never drift.
const (
	labelInbox     = "0"
	labelTrash     = "3"
	labelSpam      = "4"
	labelAllMail   = "5"
	labelArchive   = "6"
	labelSent      = "7"
	labelDrafts    = "8"
	labelStarred   = "10"
	labelScheduled = "12"
)

var MailboxLabelIDs = map[string]string{
	"inbox":     labelInbox,
	"drafts":    labelDrafts,
	"sent":      labelSent,
	"trash":     labelTrash,
	"spam":      labelSpam,
	"archive":   labelArchive,
	"starred":   labelStarred,
	"scheduled": labelScheduled,
	"all":       labelAllMail,
}

// ResolveFolder passes unknown names through unchanged, so a raw label ID
// works anywhere a folder alias does.
func ResolveFolder(name string) string {
	if id, ok := MailboxLabelIDs[strings.ToLower(name)]; ok {
		return id
	}
	return name
}

type Service struct {
	C proton.Doer

	// senderKeys caches fetched sender public key rings (per email) for body
	// signature verification. A nil entry means "no key available" - cached so
	// we don't refetch on every message in a conversation.
	keyMu      sync.Mutex
	senderKeys map[string]*pgp.KeyRing
}

func New(c proton.Doer) *Service { return &Service{C: c} }

type Message struct {
	ID             string `json:"id"`
	Subject        string `json:"subject"`
	FromName       string `json:"from_name,omitempty"`
	FromAddress    string `json:"from_address"`
	Time           int64  `json:"time"`
	Unread         int    `json:"unread"`
	NumAttachments int    `json:"num_attachments"`
}

// Full carries a decrypted body, unlike the raw API envelope.
type Full struct {
	ID          string                 `json:"id"`
	Subject     string                 `json:"subject"`
	Sender      map[string]any         `json:"sender"`
	ToList      []map[string]any       `json:"to_list"`
	Time        int64                  `json:"time,omitempty"`
	Body        string                 `json:"body"`
	MIMEType    string                 `json:"mime_type"`
	AddressID   string                 `json:"address_id"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Signature   pgphelper.VerifyResult `json:"signature,omitempty"`
}

type Conversation struct {
	ID             string           `json:"id"`
	Subject        string           `json:"subject"`
	NumMessages    int              `json:"num_messages"`
	NumUnread      int              `json:"num_unread"`
	NumAttachments int              `json:"num_attachments"`
	Time           int64            `json:"time"`
	Senders        []map[string]any `json:"senders,omitempty"`
	Recipients     []map[string]any `json:"recipients,omitempty"`
	Labels         []string         `json:"labels,omitempty"`
}

type ConversationFull struct {
	Conversation Conversation `json:"conversation"`
	Messages     []Full       `json:"messages"`
}

// Attachment's Disposition is "inline" for HTML-referenced parts (e.g.
// signature graphics); empty or any other value counts as a real attachment.
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	MIMEType    string `json:"mime_type"`
	Disposition string `json:"disposition"`
	KeyPackets  string `json:"-"`
}

func (a Attachment) IsInline() bool { return a.Disposition == "inline" }

func FilterInline(atts []Attachment) []Attachment {
	out := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		if !a.IsInline() {
			out = append(out, a)
		}
	}
	return out
}

type ListOptions struct {
	Folder   string
	Page     int
	PageSize int
	Unread   bool
}

type SearchOptions struct {
	Keyword, From, To, Subject, Folder, After, Before string
	Limit                                             int
	Unread                                            bool
}

// decryptBody decrypts an armored PGP body with decKR and, when verKR is
// non-nil, verifies the embedded signature against it. gopenpgp returns the
// decrypted body alongside a SignatureVerificationError, so a bad/absent
// signature never hides the body - it only changes the verdict.
func decryptBody(armored string, decKR, verKR *pgp.KeyRing) (string, pgphelper.VerifyResult, error) {
	msg, err := pgp.NewPGPMessageFromArmored(armored)
	if err != nil {
		return "", pgphelper.Unverified, fmt.Errorf("parse message: %w", err)
	}
	var verifyTime int64
	if verKR != nil {
		verifyTime = pgp.GetUnixTime()
	}
	dec, err := decKR.Decrypt(msg, verKR, verifyTime)
	if err != nil {
		var sigErr pgp.SignatureVerificationError
		if errors.As(err, &sigErr) {
			return dec.GetString(), pgphelper.Classify(err), nil
		}
		return "", pgphelper.Unverified, fmt.Errorf("decrypt message: %w", err)
	}
	if verKR == nil {
		return dec.GetString(), pgphelper.Unverified, nil
	}
	return dec.GetString(), pgphelper.Verified, nil
}

// crossTableProbe wraps an HTTP 422 from a single-resource GET with a
// best-effort probe of the other table, producing a WrongTableError on hit.
func (s *Service) crossTableProbe(ctx context.Context, id string, err error, callerKind string) error {
	var apiErr *proton.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatus != 422 {
		return err
	}
	var otherPath, otherKind string
	switch callerKind {
	case "messages":
		otherPath = "/mail/v4/conversations/" + id
		otherKind = "conversation"
	case "conversations":
		otherPath = "/mail/v4/messages/" + id
		otherKind = "message"
	default:
		return err
	}
	if probeErr := s.C.Decode(ctx, proton.Request{Method: "GET", Path: otherPath}, nil); probeErr == nil {
		return &WrongTableError{Kind: otherKind, ID: id}
	}
	return err
}

func msgID(m Message) string    { return m.ID }
func msgLabel(m Message) string { return m.FromAddress + "  " + m.Subject }

func convSenderAddr(c Conversation) string {
	if len(c.Senders) > 0 {
		if a, ok := c.Senders[0]["Address"].(string); ok {
			return a
		}
	}
	return ""
}

func (s *Service) Resolve(ctx context.Context, r string) (string, error) {
	if idcache.IsFullID(r) {
		return r, nil
	}
	msgs, _, err := s.Search(ctx, SearchOptions{Keyword: r, Folder: "all", Limit: 20})
	if err != nil {
		return "", err
	}
	m, err := ref.Pick("message", r, msgs, msgID, msgLabel)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

// ResolveScheduled mirrors Resolve but scopes the keyword search to the
// Scheduled folder, so a REF can only resolve to an unschedulable message.
func (s *Service) ResolveScheduled(ctx context.Context, r string) (string, error) {
	if idcache.IsFullID(r) {
		return r, nil
	}
	msgs, _, err := s.Search(ctx, SearchOptions{Keyword: r, Folder: "scheduled", Limit: 20})
	if err != nil {
		return "", err
	}
	m, err := ref.Pick("scheduled message", r, msgs, msgID, msgLabel)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

func (s *Service) ResolveConversation(ctx context.Context, r string) (string, error) {
	if idcache.IsFullID(r) {
		return r, nil
	}
	convs, _, err := s.ConversationsSearch(ctx, SearchOptions{Keyword: r, Folder: "all", Limit: 20})
	if err != nil {
		return "", err
	}
	c, err := ref.Pick("conversation", r, convs,
		func(c Conversation) string { return c.ID },
		func(c Conversation) string { return convSenderAddr(c) + "  " + c.Subject })
	if err != nil {
		return "", err
	}
	return c.ID, nil
}
