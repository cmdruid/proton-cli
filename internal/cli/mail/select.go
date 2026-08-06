package mail

import (
	"context"
	"time"

	"github.com/roman-16/proton-cli/internal/cli/kit"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

// The mail filters, shared by every organising verb.
//
// One flag set for trash, delete, move, label, unlabel, star, unstar, mark and
// export means learning it once. It is also why `--dry-run` can show the same
// table `list` would: the filter path already has the rows.

// filters are the ways to say "which messages" without naming them.
type filters struct {
	unread  bool
	starred bool
	from    string
	to      string
	subject string
	keyword string
	folder  string
	age     kit.Range
	all     bool
	limit   int
}

func (f *filters) register(c *cobra.Command) {
	fl := c.Flags()
	fl.BoolVar(&f.unread, "unread", false, "Match unread messages")
	fl.BoolVar(&f.starred, "starred", false, "Match starred messages")
	fl.StringVar(&f.from, "from", "", "Match the sender's address")
	fl.StringVar(&f.to, "to", "", "Match a recipient's address")
	fl.StringVar(&f.subject, "subject", "", "Match text in the subject")
	fl.StringVar(&f.keyword, "keyword", "", "Match text anywhere, including display names and bodies")
	fl.StringVar(&f.folder, "folder", "", "Look only in this folder or label")
	f.age.Register(fl, "messages")
	fl.BoolVar(&f.all, "all", false, "Confirm that no narrowing filter means everything in scope")
	fl.IntVar(&f.limit, "limit", 150, "Most messages to affect (Proton pages at 150)")
}

// set reports whether the user asked for a filtered selection at all.
func (f *filters) set() bool {
	return f.unread || f.starred || f.from != "" || f.to != "" || f.subject != "" ||
		f.keyword != "" || f.folder != "" || f.age.Set() || f.all
}

// unbounded reports whether --all was given with nothing to narrow it, which is
// worth warning about before it happens.
func (f *filters) unbounded() bool {
	return f.all && !f.unread && !f.starred && f.from == "" && f.to == "" &&
		f.subject == "" && f.keyword == "" && f.folder == "" && !f.age.Set()
}

// search converts the filters into the service's query.
func (f *filters) search() (mailsvc.SearchOptions, error) {
	opts := mailsvc.SearchOptions{
		Keyword: f.keyword, From: f.from, To: f.to, Subject: f.subject,
		Folder: f.folder, Limit: f.limit, Unread: f.unread,
	}
	if opts.Folder == "" {
		opts.Folder = "all"
	}
	if f.age.OlderThan != "" {
		d, err := units.ParseDuration(f.age.OlderThan)
		if err != nil {
			return opts, kit.Fail("--older-than: %v", err)
		}
		opts.Before = time.Now().Add(-d).Format("2006-01-02")
	}
	if f.age.NewerThan != "" {
		d, err := units.ParseDuration(f.age.NewerThan)
		if err != nil {
			return opts, kit.Fail("--newer-than: %v", err)
		}
		opts.After = time.Now().Add(-d).Format("2006-01-02")
	}
	return opts, nil
}

// filterHint names the filters this command actually has, so the error a user
// sees lists real options rather than a generic sentence.
const filterHint = "--unread, --starred, --from, --subject or --older-than"

// selectMessages resolves what an organising verb should act on.
func selectMessages(c *kit.Invocation, f *filters) (kit.Selection[mailsvc.Message], error) {
	if f.unbounded() {
		c.Note("--all with no other filter affects every message in the account. Add --folder to narrow it.")
	}
	sel := kit.Selector[mailsvc.Message]{
		Noun:       "messages",
		Columns:    messageColumns(),
		IDOf:       func(m mailsvc.Message) string { return m.ID },
		FilterHint: filterHint,
		Scope:      "a whole folder",
		ByRef: func(ctx context.Context, ref string) (mailsvc.Message, error) {
			return c.App.Mail.FindMessage(ctx, ref)
		},
	}
	if f.set() {
		sel.ByFilter = func(ctx context.Context) ([]mailsvc.Message, error) {
			opts, err := f.search()
			if err != nil {
				return nil, err
			}
			msgs, _, err := c.App.Mail.Search(ctx, opts)
			if err != nil {
				return nil, err
			}
			return applyLocalFilters(msgs, f), nil
		}
	}
	return kit.Select(c, sel)
}

// selectConversations is the same selection for whole threads.
func selectConversations(c *kit.Invocation, f *filters) (kit.Selection[mailsvc.Conversation], error) {
	if f.unbounded() {
		c.Note("--all with no other filter affects every thread in the account. Add --folder to narrow it.")
	}
	sel := kit.Selector[mailsvc.Conversation]{
		Noun:       "conversations",
		Columns:    conversationColumns(),
		IDOf:       func(cv mailsvc.Conversation) string { return cv.ID },
		FilterHint: filterHint,
		Scope:      "a whole folder",
		ByRef: func(ctx context.Context, ref string) (mailsvc.Conversation, error) {
			return c.App.Mail.FindConversation(ctx, ref)
		},
	}
	if f.set() {
		sel.ByFilter = func(ctx context.Context) ([]mailsvc.Conversation, error) {
			opts, err := f.search()
			if err != nil {
				return nil, err
			}
			convs, _, err := c.App.Mail.ConversationsSearch(ctx, opts)
			if err != nil {
				return nil, err
			}
			if f.starred {
				kept := convs[:0]
				for _, cv := range convs {
					if cv.Starred() {
						kept = append(kept, cv)
					}
				}
				convs = kept
			}
			return convs, nil
		}
	}
	return kit.Select(c, sel)
}

// applyLocalFilters narrows what the server could not. Proton's search has no
// starred predicate, so that one is applied here rather than being silently
// ignored - which is what a flag the server drops amounts to.
func applyLocalFilters(msgs []mailsvc.Message, f *filters) []mailsvc.Message {
	if !f.starred {
		return msgs
	}
	kept := make([]mailsvc.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Starred() {
			kept = append(kept, m)
		}
	}
	return kept
}
