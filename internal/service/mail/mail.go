// Package mail provides Proton Mail operations.
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pgp "github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

// WrongTableError signals that an ID-shaped REF was passed to the wrong
// endpoint family (a conversation ID into the messages tree, or vice versa).
// The cli layer catches this to emit a redirect hint and exit 3.
type WrongTableError struct {
	// Kind is what the ID actually is ("message" or "conversation") — the
	// OTHER table from the one the user invoked.
	Kind string
	ID   string
}

func (e *WrongTableError) Error() string {
	return fmt.Sprintf("that ID is a %s, not a %s", e.Kind, oppositeKind(e.Kind))
}
func (e *WrongTableError) ExitCode() int { return 3 }

func oppositeKind(k string) string {
	if k == "conversation" {
		return "message"
	}
	return "conversation"
}

// LooksLikeID is the package's heuristic for recognising a Proton ID.
func LooksLikeID(s string) bool { return looksLikeID(s) }

func looksLikeID(s string) bool { return len(s) > 60 && strings.HasSuffix(s, "==") }

// MailboxLabelIDs maps folder aliases to Proton built-in label IDs.
var MailboxLabelIDs = map[string]string{
	"inbox":   "0",
	"drafts":  "8",
	"sent":    "7",
	"trash":   "3",
	"spam":    "4",
	"archive": "6",
	"starred": "10",
	"all":     "5",
}

// ResolveFolder returns the Proton label ID for a folder name/alias; unknown
// strings pass through so callers can use custom-label IDs directly.
func ResolveFolder(name string) string {
	if id, ok := MailboxLabelIDs[strings.ToLower(name)]; ok {
		return id
	}
	return name
}

// Service is the Mail domain service.
type Service struct{ C proton.Doer }

// New constructs a mail service.
func New(c proton.Doer) *Service { return &Service{C: c} }

// Message is a list-view mail message.
type Message struct {
	ID             string `json:"id"`
	Subject        string `json:"subject"`
	FromName       string `json:"from_name,omitempty"`
	FromAddress    string `json:"from_address"`
	Time           int64  `json:"time"`
	Unread         int    `json:"unread"`
	NumAttachments int    `json:"num_attachments"`
}

// Full is a decrypted single-message view.
type Full struct {
	ID          string           `json:"id"`
	Subject     string           `json:"subject"`
	Sender      map[string]any   `json:"sender"`
	ToList      []map[string]any `json:"to_list"`
	Time        int64            `json:"time,omitempty"`
	Body        string           `json:"body"`
	MIMEType    string           `json:"mime_type"`
	AddressID   string           `json:"address_id"`
	Attachments []Attachment     `json:"attachments,omitempty"`
}

// Conversation is a list-view conversation.
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

// ConversationFull is a decrypted thread: envelope plus all messages sorted
// chronologically.
type ConversationFull struct {
	Conversation Conversation `json:"conversation"`
	Messages     []Full       `json:"messages"`
}

// Attachment describes a message attachment. Disposition distinguishes
// "attachment" (user-facing) from "inline" (HTML-referenced, e.g. signature
// graphics). Empty/missing dispositions count as a real attachment.
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	MIMEType    string `json:"mime_type"`
	Disposition string `json:"disposition"`
	KeyPackets  string `json:"-"`
}

func (a Attachment) IsInline() bool { return a.Disposition == "inline" }

// FilterInline returns the subset of atts that are NOT inline.
func FilterInline(atts []Attachment) []Attachment {
	out := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		if !a.IsInline() {
			out = append(out, a)
		}
	}
	return out
}

// ListOptions filters for List.
type ListOptions struct {
	Folder   string
	Page     int
	PageSize int
	Unread   bool
}

// SearchOptions filters for Search.
type SearchOptions struct {
	Keyword, From, To, Subject, Folder, After, Before string
	Limit                                             int
	Unread                                            bool
}

func decryptBody(armored string, addrKR *pgp.KeyRing) (string, error) {
	msg, err := pgp.NewPGPMessageFromArmored(armored)
	if err != nil {
		return "", fmt.Errorf("parse message: %w", err)
	}
	dec, err := addrKR.Decrypt(msg, nil, pgp.GetUnixTime())
	if err != nil {
		return "", fmt.Errorf("decrypt message: %w", err)
	}
	return dec.GetString(), nil
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

// Resolve returns a message ID for either a literal ID or a keyword search.
func (s *Service) Resolve(ctx context.Context, ref string) (string, error) {
	if looksLikeID(ref) {
		return ref, nil
	}
	msgs, _, err := s.Search(ctx, SearchOptions{Keyword: ref, Folder: "all", Limit: 20})
	if err != nil {
		return "", err
	}
	switch len(msgs) {
	case 0:
		return "", &errs.NotFound{Kind: "message", Ref: ref}
	case 1:
		return msgs[0].ID, nil
	}
	cands := make([]errs.Candidate, 0, len(msgs))
	for _, m := range msgs {
		cands = append(cands, errs.Candidate{ID: m.ID, Label: m.FromAddress + "  " + m.Subject})
	}
	return "", &errs.Ambiguous{Kind: "message", Ref: ref, Candidates: cands}
}

// ResolveConversation returns a conversation ID for a literal ID or a search.
func (s *Service) ResolveConversation(ctx context.Context, ref string) (string, error) {
	if looksLikeID(ref) {
		return ref, nil
	}
	convs, _, err := s.ConversationsSearch(ctx, SearchOptions{Keyword: ref, Folder: "all", Limit: 20})
	if err != nil {
		return "", err
	}
	switch len(convs) {
	case 0:
		return "", &errs.NotFound{Kind: "conversation", Ref: ref}
	case 1:
		return convs[0].ID, nil
	}
	cands := make([]errs.Candidate, 0, len(convs))
	for _, c := range convs {
		fromAddr := ""
		if len(c.Senders) > 0 {
			if a, ok := c.Senders[0]["Address"].(string); ok {
				fromAddr = a
			}
		}
		cands = append(cands, errs.Candidate{ID: c.ID, Label: fromAddr + "  " + c.Subject})
	}
	return "", &errs.Ambiguous{Kind: "conversation", Ref: ref, Candidates: cands}
}

// RawJSON convenience for commands that emit the server payload as-is.
func RawJSON(b []byte) (json.RawMessage, error) { return json.RawMessage(b), nil }
