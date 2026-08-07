package mail

import (
	"context"

	"github.com/roman-16/proton-cli/internal/proton"
)

// Every organising action Proton offers on mail reduces to four primitives:
// attach a label, detach a label, mark read, mark unread. Moving to a folder is
// attaching a folder label. Starring is attaching the Starred label. Trashing is
// attaching Trash.
//
// Naming the primitives and building the verbs from them means the CLI's words
// and Proton's model line up, instead of a single `Mark(read, unread, starred,
// unstar bool)` doing four unrelated things depending on which flag is set.

// Label attaches a label to messages.
func (s *Service) Label(ctx context.Context, ids []string, labelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/messages/label",
		Body: map[string]any{"LabelID": labelID, "IDs": ids},
	}, nil)
}

// Unlabel detaches a label from messages.
func (s *Service) Unlabel(ctx context.Context, ids []string, labelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/messages/unlabel",
		Body: map[string]any{"LabelID": labelID, "IDs": ids},
	}, nil)
}

// Trash moves messages to the Trash folder, from where they can be restored.
func (s *Service) Trash(ctx context.Context, ids []string) error {
	return s.Label(ctx, ids, labelTrash)
}

// Delete removes messages irrecoverably.
func (s *Service) Delete(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/messages/delete",
		Body: map[string]any{"IDs": ids},
	}, nil)
}

// MarkRead clears the unread flag on messages.
func (s *Service) MarkRead(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/messages/read",
		Body: map[string]any{"IDs": ids},
	}, nil)
}

// MarkUnread sets the unread flag on messages.
func (s *Service) MarkUnread(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/messages/unread",
		Body: map[string]any{"IDs": ids},
	}, nil)
}

// Unschedule cancels a scheduled send, pulling the message out of the Scheduled
// queue and back into Drafts - the web client's "Edit and reschedule". The
// message keeps its ID.
func (s *Service) Unschedule(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := s.C.Decode(ctx, proton.Request{
			Method: "POST", Path: "/mail/v4/messages/" + id + "/cancel_send",
		}, nil); err != nil {
			return err
		}
	}
	return nil
}

// ── conversations ──
//
// The same four primitives, with one asymmetry Proton imposes: marking a thread
// unread, and deleting one, apply within a mailbox rather than globally, because
// a thread can have messages in several. An empty scope means All Mail.

func (s *Service) ConversationsLabel(ctx context.Context, ids []string, labelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/label",
		Body: map[string]any{"LabelID": labelID, "IDs": ids},
	}, nil)
}

func (s *Service) ConversationsUnlabel(ctx context.Context, ids []string, labelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/unlabel",
		Body: map[string]any{"LabelID": labelID, "IDs": ids},
	}, nil)
}

func (s *Service) ConversationsTrash(ctx context.Context, ids []string) error {
	return s.ConversationsLabel(ctx, ids, labelTrash)
}

func (s *Service) ConversationsDelete(ctx context.Context, ids []string, scopeLabelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/delete",
		Body: map[string]any{"IDs": ids, "LabelID": orAllMail(scopeLabelID)},
	}, nil)
}

func (s *Service) ConversationsMarkRead(ctx context.Context, ids []string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/read",
		Body: map[string]any{"IDs": ids},
	}, nil)
}

func (s *Service) ConversationsMarkUnread(ctx context.Context, ids []string, scopeLabelID string) error {
	return s.C.Decode(ctx, proton.Request{
		Method: "PUT", Path: "/mail/v4/conversations/unread",
		Body: map[string]any{"IDs": ids, "LabelID": orAllMail(scopeLabelID)},
	}, nil)
}

func orAllMail(labelID string) string {
	if labelID == "" {
		return labelAllMail
	}
	return labelID
}
