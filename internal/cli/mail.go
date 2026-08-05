package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/idcache"
	mailsvc "github.com/roman-16/proton-cli/internal/service/mail"
	"github.com/roman-16/proton-cli/internal/units"
	"github.com/spf13/cobra"
)

func newMailCmd() *cobra.Command {
	c := &cobra.Command{Use: "mail", Short: "Mail operations"}
	c.AddCommand(messagesCmd(), draftsCmd(), conversationsCmd(), attachmentsCmd(), mailSettingsCmd())
	return c
}

// handleWrongTable converts a *mailsvc.WrongTableError into an exit-3 redirect
// hint to the other command tree. Other errors pass through unchanged.
func handleWrongTable(err error, otherVerb string) error {
	if err == nil {
		return nil
	}
	var wte *mailsvc.WrongTableError
	if !errors.As(err, &wte) {
		return err
	}
	var otherTree string
	switch wte.Kind {
	case "conversation":
		otherTree = "mail conversations"
	case "message":
		otherTree = "mail messages"
	default:
		return err
	}
	return errs.WithExit(3, fmt.Errorf(
		"that ID is a %s, not a %s. Run: proton-cli %s %s %s",
		wte.Kind, mailsvc.OppositeKind(wte.Kind), otherTree, otherVerb, wte.ID))
}

// hintFromTo emits a stderr hint suggesting --keyword when a --from/--to search
// returned zero results without --keyword.
func hintFromTo(c *Invocation, noun string, opts mailsvc.SearchOptions) {
	if opts.Keyword != "" || (opts.From == "" && opts.To == "") {
		return
	}
	flag := "--from"
	val := opts.From
	if val == "" {
		flag = "--to"
		val = opts.To
	}
	c.R().Info(fmt.Sprintf(
		"Hint: %s matches the email address only. To also search display\n"+
			"      names and message content, try:\n"+
			"        proton-cli mail %s search --keyword %s",
		flag, noun, val))
}

func registerSearchFlags(c *cobra.Command, opts *mailsvc.SearchOptions, defaultFolder string) {
	c.Flags().StringVar(&opts.Keyword, "keyword", "", "Search keyword")
	c.Flags().StringVar(&opts.From, "from", "", "Filter by sender email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.To, "to", "", "Filter by recipient email address (use --keyword to also match display names)")
	c.Flags().StringVar(&opts.Subject, "subject", "", "Filter by subject")
	c.Flags().StringVar(&opts.After, "after", "", "After date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Before, "before", "", "Before date (YYYY-MM-DD)")
	c.Flags().StringVar(&opts.Folder, "folder", defaultFolder, "Folder to search in")
	c.Flags().IntVar(&opts.Limit, "limit", 25, "Max results")
}

// msgFilter collects batch-filter flags shared by trash/delete/move/mark on
// both the messages and conversations trees.
type msgFilter struct {
	unread                             bool
	from, to, subject, keyword, folder string
	olderThan, newerThan               string
	all                                bool
	limit                              int
}

func (f *msgFilter) register(c *cobra.Command) {
	c.Flags().BoolVar(&f.unread, "unread", false, "Match unread messages")
	c.Flags().StringVar(&f.from, "from", "", "Match sender")
	c.Flags().StringVar(&f.to, "to", "", "Match recipient")
	c.Flags().StringVar(&f.subject, "subject", "", "Match subject")
	c.Flags().StringVar(&f.keyword, "keyword", "", "Match keyword")
	c.Flags().StringVar(&f.folder, "folder", "", "Scope to a folder")
	c.Flags().StringVar(&f.olderThan, "older-than", "", "Match messages older than DURATION (e.g. 30d, 2w, 1h)")
	c.Flags().StringVar(&f.newerThan, "newer-than", "", "Match messages newer than DURATION")
	c.Flags().BoolVar(&f.all, "all", false, "Confirm matching every message in the scope (required when no other filter is set)")
	c.Flags().IntVar(&f.limit, "limit", 150, "Maximum messages to affect when using filters (Proton caps at 150 per page)")
}

func (f *msgFilter) set() bool {
	return f.unread || f.from != "" || f.to != "" || f.subject != "" || f.keyword != "" ||
		f.folder != "" || f.olderThan != "" || f.newerThan != "" || f.all
}

func (f *msgFilter) onlyAll() bool {
	return f.all && !f.unread && f.from == "" && f.to == "" && f.subject == "" &&
		f.keyword == "" && f.folder == "" && f.olderThan == "" && f.newerThan == ""
}

func (f *msgFilter) toSearch() (mailsvc.SearchOptions, error) {
	opts := mailsvc.SearchOptions{
		Keyword: f.keyword, From: f.from, To: f.to, Subject: f.subject,
		Folder: f.folder, Limit: f.limit, Unread: f.unread,
	}
	if opts.Folder == "" {
		opts.Folder = "all"
	}
	if f.olderThan != "" {
		d, err := units.ParseDuration(f.olderThan)
		if err != nil {
			return opts, fmt.Errorf("invalid --older-than: %w", err)
		}
		opts.Before = time.Now().Add(-d).Format("2006-01-02")
	}
	if f.newerThan != "" {
		d, err := units.ParseDuration(f.newerThan)
		if err != nil {
			return opts, fmt.Errorf("invalid --newer-than: %w", err)
		}
		opts.After = time.Now().Add(-d).Format("2006-01-02")
	}
	return opts, nil
}

// mailIDCollector supplies the message-vs-conversation specifics that
// collectMailIDs needs; the messages and conversations trees build one each.
type mailIDCollector struct {
	noun, plural string // "message"/"messages", "conversation"/"conversations"
	assertKind   func(context.Context, string) error
	resolve      func(context.Context, string) (string, error)
	searchIDs    func(context.Context, mailsvc.SearchOptions) ([]string, error)
}

// collectMailIDs unions explicit REFs (resolved + kind-checked) with the IDs of
// entities matched by the batch filters. It is the shared core of the messages
// and conversations selection logic.
func collectMailIDs(c *Invocation, args []string, f *msgFilter, col mailIDCollector) ([]string, error) {
	refs, err := resolvePrefixes(c.App, args)
	if err != nil {
		return nil, err
	}
	if len(refs) == 1 && idcache.IsFullID(refs[0]) {
		if err := col.assertKind(c.Ctx, refs[0]); err != nil {
			var wte *mailsvc.WrongTableError
			if errors.As(err, &wte) {
				return nil, err
			}
		}
	}
	var ids []string
	for _, ref := range refs {
		id, err := col.resolve(c.Ctx, ref)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if f.set() {
		if f.onlyAll() {
			c.R().Info(fmt.Sprintf("--all with no other filter will affect every %s in the account. Add --folder to scope it.", col.noun))
		}
		search, err := f.toSearch()
		if err != nil {
			return nil, err
		}
		matched, err := col.searchIDs(c.Ctx, search)
		if err != nil {
			return nil, err
		}
		ids = append(ids, matched...)
	}
	if len(args) == 0 && !f.set() {
		return nil, fmt.Errorf("no %s selected: pass REF(s) or a filter (e.g. --unread, --from, --older-than); use --all to target an entire folder", col.plural)
	}
	return dedupe(ids), nil
}

// bulkMailAction is the shared RunE for the messages/conversations bulk verbs
// (trash/delete/star/unstar). collect selects the IDs; noun fills the dry-run
// line; do performs the mutation.
func bulkMailAction(collect func(*Invocation, []string, *msgFilter) ([]string, error), noun string, f *msgFilter, successFmt, otherVerb string, do func(c *Invocation, ids []string) error) func(*cobra.Command, []string) error {
	return run([]Step{stepAuth}, func(c *Invocation) error {
		ids, err := collect(c, c.Args, f)
		if err != nil {
			return handleWrongTable(err, otherVerb)
		}
		if c.App.DryRun {
			c.R().Info(fmt.Sprintf("dry-run: would affect %d %s(s)", len(ids), noun))
			for _, id := range ids {
				_, _ = fmt.Fprintln(c.R().Stderr, "  "+id)
			}
			return nil
		}
		if err := do(c, ids); err != nil {
			return err
		}
		c.R().Success(fmt.Sprintf(successFmt, len(ids)))
		return nil
	})
}
