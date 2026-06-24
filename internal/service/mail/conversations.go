package mail

import (
	"context"
	"fmt"
	"net/url"
	"sort"

	"github.com/roman-16/proton-cli/internal/crypto/keys"
	"github.com/roman-16/proton-cli/internal/proton"
)

type rawConversation struct {
	ID                                     string
	Subject                                string
	NumMessages, NumUnread, NumAttachments int
	Time                                   int64
	Senders                                []map[string]any
	Recipients                             []map[string]any
	Labels                                 []struct{ ID string }
}

func toConversation(c rawConversation) Conversation {
	labels := make([]string, 0, len(c.Labels))
	for _, l := range c.Labels {
		labels = append(labels, l.ID)
	}
	return Conversation{
		ID: c.ID, Subject: c.Subject,
		NumMessages: c.NumMessages, NumUnread: c.NumUnread, NumAttachments: c.NumAttachments,
		Time: c.Time, Senders: c.Senders, Recipients: c.Recipients, Labels: labels,
	}
}

func (s *Service) ConversationsList(ctx context.Context, opts ListOptions) ([]Conversation, int, error) {
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
		Total         int
		Conversations []rawConversation
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/conversations", Query: q}, &r); err != nil {
		return nil, 0, err
	}
	out := make([]Conversation, 0, len(r.Conversations))
	for _, c := range r.Conversations {
		out = append(out, toConversation(c))
	}
	return out, r.Total, nil
}

func (s *Service) ConversationsSearch(ctx context.Context, opts SearchOptions) ([]Conversation, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	q := searchQuery(opts, true)
	if err := validateDates(opts, q); err != nil {
		return nil, 0, err
	}
	var r struct {
		Total         int
		Conversations []rawConversation
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/conversations", Query: q}, &r); err != nil {
		return nil, 0, err
	}
	out := make([]Conversation, 0, len(r.Conversations))
	for _, c := range r.Conversations {
		out = append(out, toConversation(c))
	}
	return out, r.Total, nil
}

func (s *Service) ConversationRead(ctx context.Context, u *keys.Unlocked, id string) (*ConversationFull, error) {
	var r struct {
		Conversation rawConversation
		Messages     []rawMessage
	}
	if err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/conversations/" + id}, &r); err != nil {
		return nil, s.crossTableProbe(ctx, id, err, "conversations")
	}
	sort.SliceStable(r.Messages, func(i, j int) bool { return r.Messages[i].Time < r.Messages[j].Time })
	msgs := make([]Full, 0, len(r.Messages))
	for _, m := range r.Messages {
		// Proton returns the full Body only for the most recent message; older
		// ones come back as metadata. Lazy-load each older body so the whole
		// thread decrypts.
		if m.Body == "" {
			if full, err := s.fetchMessageRaw(ctx, m.ID); err == nil {
				m = *full
			}
		}
		msgs = append(msgs, s.decryptMessage(ctx, u, m))
	}
	return &ConversationFull{Conversation: toConversation(r.Conversation), Messages: msgs}, nil
}

func (s *Service) AssertConversationKind(ctx context.Context, id string) error {
	err := s.C.Decode(ctx, proton.Request{Method: "GET", Path: "/mail/v4/conversations/" + id}, nil)
	if err == nil {
		return nil
	}
	return s.crossTableProbe(ctx, id, err, "conversations")
}

func (s *Service) ConversationsTrash(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/label",
		Body: map[string]any{"LabelID": labelTrash, "IDs": ids},
	}, nil)
}

func (s *Service) ConversationsDelete(ctx context.Context, ids []string, folder string) error {
	labelID := labelAllMail
	if folder != "" {
		labelID = ResolveFolder(folder)
	}
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/delete",
		Body: map[string]any{"IDs": ids, "LabelID": labelID},
	}, nil)
}

func (s *Service) ConversationsMove(ctx context.Context, ids []string, folder string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/label",
		Body: map[string]any{"LabelID": ResolveFolder(folder), "IDs": ids},
	}, nil)
}

func (s *Service) ConversationsMark(ctx context.Context, ids []string, read, unread, starred, unstar bool, folder string) error {
	if read {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/conversations/read", Body: map[string]any{"IDs": ids}}, nil); err != nil {
			return err
		}
	}
	if unread {
		labelID := labelAllMail
		if folder != "" {
			labelID = ResolveFolder(folder)
		}
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/conversations/unread", Body: map[string]any{"IDs": ids, "LabelID": labelID}}, nil); err != nil {
			return err
		}
	}
	if starred {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/conversations/label", Body: map[string]any{"LabelID": labelStarred, "IDs": ids}}, nil); err != nil {
			return err
		}
	}
	if unstar {
		if err := s.C.Decode(ctx, proton.Request{Method: "PUT", Path: "/mail/v4/conversations/unlabel", Body: map[string]any{"LabelID": labelStarred, "IDs": ids}}, nil); err != nil {
			return err
		}
	}
	return nil
}
