package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// Every command, shown being used.
//
// The grammar is this CLI's whole premise: one shape, learned once, then guessed
// correctly everywhere else. `--help` is where that learning happens, and a
// screen that lists twenty flags without once showing the sentence they belong
// to leaves the reader to assemble it from parts.
//
// They live here rather than beside each command for the same reason the verbs
// and the placeholders do: the examples are the language being spoken, and a
// language is easier to keep consistent when you can read it in one sitting. The
// conformance test parses every line against the real tree, so an example cannot
// name a command that does not exist, use a flag that was renamed, or illustrate
// a different command from the one it is filed under.
//
// The values are deliberately the same cast throughout - Jane Roe, invoice 2291,
// /Documents, a Work label - so the examples read as one account being used
// rather than as a hundred unrelated fragments.
var examples = map[string][]string{
	// ── account ──
	"proton account get": {
		"proton account get",
		"proton account get --output json",
	},
	"proton account login": {
		"proton account login",
		"proton account login --profile work",
		"proton account login --user me@proton.me --password-file /run/secrets/proton",
		"proton account login --user me@proton.me --password-stdin --totp 123456",
	},
	"proton account logout": {
		"proton account logout",
		"proton account logout --revoke",
		"proton account logout --all",
	},
	"proton account profiles list":   {"proton account profiles list"},
	"proton account profiles delete": {"proton account profiles delete work"},
	"proton account sessions list":   {"proton account sessions list"},
	"proton account sessions revoke": {
		"proton account sessions revoke 5bH2mQxK",
		"proton account sessions revoke --others",
	},
	"proton account settings get":  {"proton account settings get"},
	"proton account settings list": {"proton account settings list"},
	"proton account settings set": {
		"proton account settings set locale de_AT",
		"proton account settings set news off",
	},
	"proton api": {
		"proton api GET /core/v4/users",
		"proton api GET /mail/v4/messages --query 'PageSize=5'",
		"proton api POST /mail/v4/labels --body '{\"Name\":\"Work\",\"Color\":\"#8080FF\",\"Type\":1}'",
	},

	// ── calendar ──
	"proton calendar events create": {
		"proton calendar events create --title Dentist --start 2026-04-16T14:00 --duration 1h",
		"proton calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule 'FREQ=WEEKLY;COUNT=10' --remind 15m",
		"proton calendar events create --title Holiday --start 2026-07-01 --all-day --calendar Personal",
		"proton calendar events create --title 'Design review' --start 2026-04-20T10:00 --duration 45m --attendee jane@example.com --location 'Room 3'",
	},
	"proton calendar events list": {
		"proton calendar events list",
		"proton calendar events list --start 2026-04-15 --end 2026-04-30",
		"proton calendar events list --calendar Work",
	},
	"proton calendar events get": {
		"proton calendar events get Dentist",
		"proton calendar events get 4f2a1b9c@2026-04-22T09:00",
	},
	"proton calendar events update": {
		"proton calendar events update Dentist --start 2026-04-16T15:30",
		"proton calendar events update 4f2a1b9c@2026-04-22T09:00 --location 'Room 3'",
		"proton calendar events update 4f2a1b9c@2026-04-22T09:00 --title Standup --future",
	},
	"proton calendar events delete": {
		"proton calendar events delete Dentist",
		"proton calendar events delete 4f2a1b9c@2026-05-04T09:00 --future",
	},
	"proton calendar events respond": {
		"proton calendar events respond 'Team sync' --status accept",
		"proton calendar events respond 'Team sync' --status decline",
	},
	"proton calendar settings calendars list": {"proton calendar settings calendars list"},
	"proton calendar settings calendars create": {
		"proton calendar settings calendars create --name Work",
		"proton calendar settings calendars create --name Personal --color pacific",
	},
	"proton calendar settings calendars update": {
		"proton calendar settings calendars update Work --name Office",
		"proton calendar settings calendars update Work --color enzian",
	},
	"proton calendar settings calendars delete": {"proton calendar settings calendars delete Work"},
	"proton calendar settings get":              {"proton calendar settings get"},
	"proton calendar settings list":             {"proton calendar settings list"},
	"proton calendar settings set": {
		"proton calendar settings set week-start monday",
		"proton calendar settings set default-duration 30",
	},

	// ── contacts ──
	"proton contacts list": {
		"proton contacts list",
		"proton contacts list --output json",
	},
	"proton contacts get": {
		"proton contacts get jane@example.com",
		"proton contacts get 'Jane Roe'",
	},
	"proton contacts create": {
		"proton contacts create --name 'Jane Roe' --email jane@example.com",
		"proton contacts create --name 'Jane Roe' --email jane@example.com --phone '+43 660 1234567' --organization Acme",
	},
	"proton contacts update": {
		"proton contacts update jane --job-title 'Head of Design'",
		"proton contacts update jane --email jane.roe@work.example --birthday 1990-04-16",
	},
	"proton contacts delete":      {"proton contacts delete jane"},
	"proton contacts groups list": {"proton contacts groups list"},
	"proton contacts groups create": {
		"proton contacts groups create --name Team",
		"proton contacts groups create --name Family --color strawberry",
	},
	"proton contacts groups update": {
		"proton contacts groups update Team --name Engineering",
		"proton contacts groups update Team --color reef",
	},
	"proton contacts groups delete": {"proton contacts groups delete Team"},
	"proton contacts groups add":    {"proton contacts groups add Team jane"},
	"proton contacts groups remove": {"proton contacts groups remove Team jane"},
	"proton contacts keys list":     {"proton contacts keys list jane"},
	"proton contacts keys pin": {
		"proton contacts keys pin jane --key jane-pubkey.asc",
		"proton contacts keys pin jane --email jane@example.com --key - --no-encrypt",
	},
	"proton contacts keys unpin": {
		"proton contacts keys unpin jane",
		"proton contacts keys unpin jane --email jane@example.com",
	},

	// ── drive ──
	"proton drive items list": {
		"proton drive items list",
		"proton drive items list /Documents",
	},
	"proton drive items get": {"proton drive items get /Documents/report.pdf"},
	"proton drive items upload": {
		"proton drive items upload ./report.pdf /Documents",
		"proton drive items upload --recursive ./project /Backup",
		"proton drive items upload --if-exists replace ./report.pdf /Documents",
		"pg_dump mydb | gzip | proton drive items upload - /Backups/db.sql.gz",
	},
	"proton drive items download": {
		"proton drive items download /Documents/report.pdf --output-dir .",
		"proton drive items download /Documents/report.pdf --output - > report.pdf",
	},
	"proton drive items update": {"proton drive items update /Documents/report.pdf --name summary.pdf"},
	"proton drive items move": {
		"proton drive items move /Documents/report.pdf --into /Archive",
		"proton drive items move --pattern '*.log' --scope /Build --recursive --into /Archive",
	},
	"proton drive items copy": {
		"proton drive items copy /Documents/report.pdf --into /Archive",
		"proton drive items copy --pattern '*.pdf' --scope /Documents --into /Backup",
	},
	"proton drive items trash": {
		"proton drive items trash /Documents/report.pdf",
		"proton drive items trash --pattern '*.tmp' --scope /Build --recursive",
		"proton drive items trash --older-than 1y --scope /Downloads --dry-run",
	},
	"proton drive items delete": {
		"proton drive items delete /Documents/report.pdf",
		"proton drive items delete --pattern '*.tmp' --scope /Build --recursive --yes",
	},
	"proton drive items revisions list":     {"proton drive items revisions list /Documents/report.pdf"},
	"proton drive items revisions restore":  {"proton drive items revisions restore /Documents/report.pdf 5bH2mQxK"},
	"proton drive items revisions download": {"proton drive items revisions download /Documents/report.pdf 5bH2mQxK --output-dir ."},
	"proton drive items revisions delete":   {"proton drive items revisions delete /Documents/report.pdf 5bH2mQxK"},
	"proton drive folders create": {
		"proton drive folders create /Documents/2026",
	},
	"proton drive share get": {"proton drive share get /Documents/report.pdf"},
	"proton drive share link": {
		"proton drive share link /Documents/report.pdf",
		"proton drive share link /Documents/report.pdf --expires 7d --password hunter2",
		"proton drive share link /Documents --edit",
	},
	"proton drive share unlink": {"proton drive share unlink /Documents/report.pdf"},
	"proton drive share add": {
		"proton drive share add /Documents jane@example.com",
		"proton drive share add /Documents jane@example.com --edit --message 'Have a look'",
	},
	"proton drive share remove":     {"proton drive share remove /Documents jane@example.com"},
	"proton drive invitations list": {"proton drive invitations list"},
	"proton drive invitations accept": {
		"proton drive invitations accept 5bH2mQxK",
	},
	"proton drive invitations decline": {"proton drive invitations decline 5bH2mQxK"},
	"proton drive trash list":          {"proton drive trash list"},
	"proton drive trash restore":       {"proton drive trash restore 5bH2mQxK"},
	"proton drive trash empty":         {"proton drive trash empty"},
	"proton drive photos list": {
		"proton drive photos list",
		"proton drive photos list --album Holidays",
		"proton drive photos list --tag favorites",
	},
	"proton drive photos upload":      {"proton drive photos upload ./IMG_2291.jpg"},
	"proton drive photos download":    {"proton drive photos download 5bH2mQxK --output-dir ."},
	"proton drive photos favorite":    {"proton drive photos favorite 5bH2mQxK"},
	"proton drive photos unfavorite":  {"proton drive photos unfavorite 5bH2mQxK"},
	"proton drive photos trash":       {"proton drive photos trash 5bH2mQxK"},
	"proton drive photos delete":      {"proton drive photos delete 5bH2mQxK"},
	"proton drive photos albums list": {"proton drive photos albums list"},
	"proton drive photos albums create": {
		"proton drive photos albums create --name Holidays",
	},
	"proton drive photos albums add":    {"proton drive photos albums add Holidays 5bH2mQxK"},
	"proton drive photos albums remove": {"proton drive photos albums remove Holidays 5bH2mQxK"},
	"proton drive photos albums delete": {
		"proton drive photos albums delete Holidays",
		"proton drive photos albums delete Holidays --delete-photos",
	},
	"proton drive settings get":  {"proton drive settings get"},
	"proton drive settings list": {"proton drive settings list"},
	"proton drive settings set":  {"proton drive settings set revision-retention 30"},

	// ── mail: messages ──
	"proton mail messages list": {
		"proton mail messages list",
		"proton mail messages list --unread",
		"proton mail messages list --folder archive --page-size 50",
		"proton mail messages list --starred --output json",
	},
	"proton mail messages get": {
		"proton mail messages get 'Invoice #2291'",
		"proton mail messages get 5bH2mQxK --format html",
		"proton mail messages get 5bH2mQxK --body-only --strip-quotes",
	},
	"proton mail messages search": {
		"proton mail messages search --from billing@example.com",
		"proton mail messages search --keyword invoice --after 2026-01-01",
		"proton mail messages search --subject Report --folder archive --limit 20",
	},
	"proton mail messages send": {
		"proton mail messages send --to jane@example.com --subject Report --body 'See attached.' --attach ./report.pdf",
		"proton mail messages send --to team@example.com --subject Standup --body -",
		"proton mail messages send --to jane@example.com --subject Reminder --send-at 2026-04-16T09:00",
		"proton mail messages send --eml ./draft.eml",
	},
	"proton mail messages reply": {
		"proton mail messages reply 'Invoice #2291' --body 'Thanks, paid today.'",
		"proton mail messages reply 'Invoice #2291' --all --body 'Noted.'",
		"proton mail messages reply 'Invoice #2291' --body 'Draft first.' --draft",
	},
	"proton mail messages forward": {
		"proton mail messages forward 'Invoice #2291' --to jane@example.com",
		"proton mail messages forward 'Invoice #2291' --to jane@example.com --no-attachments",
	},
	"proton mail messages move": {
		"proton mail messages move 'Invoice #2291' --into archive",
		"proton mail messages move --from newsletter@example.com --older-than 90d --into archive",
	},
	"proton mail messages trash": {
		"proton mail messages trash 'Invoice #2291'",
		"proton mail messages trash --unread --older-than 30d",
		"proton mail messages trash --from newsletter@example.com --older-than 90d --dry-run",
	},
	"proton mail messages delete": {
		"proton mail messages delete 5bH2mQxK",
		"proton mail messages delete --folder spam --all --yes",
	},
	"proton mail messages label": {
		"proton mail messages label 'Invoice #2291' --label Accounting",
		"proton mail messages label --from billing@example.com --label Accounting",
	},
	"proton mail messages unlabel": {"proton mail messages unlabel 'Invoice #2291' --label Accounting"},
	"proton mail messages star":    {"proton mail messages star 'Invoice #2291'"},
	"proton mail messages unstar":  {"proton mail messages unstar 'Invoice #2291'"},
	"proton mail messages mark read": {
		"proton mail messages mark read 'Invoice #2291'",
		"proton mail messages mark read --folder inbox --all",
	},
	"proton mail messages mark unread": {"proton mail messages mark unread 'Invoice #2291'"},
	"proton mail messages unschedule": {
		"proton mail messages unschedule 5bH2mQxK",
		"proton mail messages unschedule --all",
	},
	"proton mail messages export": {
		"proton mail messages export 'Invoice #2291' --output-dir ./backup",
		"proton mail messages export --folder archive --all --output-dir ./mail-backup",
		"proton mail messages export --folder archive --older-than 1y --format mbox --output archive.mbox",
	},
	"proton mail messages attachments list": {
		"proton mail messages attachments list 'Invoice #2291'",
		"proton mail messages attachments list 5bH2mQxK --include-inline",
	},
	"proton mail messages attachments download": {
		"proton mail messages attachments download 'Invoice #2291' --output-dir .",
		"proton mail messages attachments download 5bH2mQxK kQ81mDx4 --output invoice.pdf",
	},

	// ── mail: conversations ──
	"proton mail conversations list": {
		"proton mail conversations list",
		"proton mail conversations list --unread --folder inbox",
	},
	"proton mail conversations get": {
		"proton mail conversations get 'Quarterly numbers'",
		"proton mail conversations get 5bH2mQxK --summary",
	},
	"proton mail conversations search": {
		"proton mail conversations search --from jane@example.com",
		"proton mail conversations search --keyword budget --limit 20",
	},
	"proton mail conversations reply": {
		"proton mail conversations reply 'Quarterly numbers' --body 'Looks right to me.'",
		"proton mail conversations reply 'Quarterly numbers' --all --body Agreed.",
	},
	"proton mail conversations forward": {
		"proton mail conversations forward 'Quarterly numbers' --to jane@example.com",
	},
	"proton mail conversations move": {
		"proton mail conversations move 'Quarterly numbers' --into archive",
		"proton mail conversations move --older-than 90d --folder inbox --into archive",
	},
	"proton mail conversations trash": {
		"proton mail conversations trash 'Quarterly numbers'",
		"proton mail conversations trash --from newsletter@example.com --older-than 90d",
	},
	"proton mail conversations delete": {"proton mail conversations delete 5bH2mQxK"},
	"proton mail conversations label": {
		"proton mail conversations label 'Quarterly numbers' --label Accounting",
	},
	"proton mail conversations unlabel": {
		"proton mail conversations unlabel 'Quarterly numbers' --label Accounting",
	},
	"proton mail conversations star":   {"proton mail conversations star 'Quarterly numbers'"},
	"proton mail conversations unstar": {"proton mail conversations unstar 'Quarterly numbers'"},
	"proton mail conversations mark read": {
		"proton mail conversations mark read 'Quarterly numbers'",
		"proton mail conversations mark read --folder inbox --all",
	},
	"proton mail conversations mark unread": {"proton mail conversations mark unread 'Quarterly numbers'"},
	"proton mail conversations export": {
		"proton mail conversations export 'Quarterly numbers' --output-dir ./backup",
	},
	"proton mail conversations attachments list": {
		"proton mail conversations attachments list 'Quarterly numbers'",
	},
	"proton mail conversations attachments download": {
		"proton mail conversations attachments download 'Quarterly numbers' --output-dir .",
	},

	// ── mail: drafts ──
	"proton mail drafts list": {"proton mail drafts list"},
	"proton mail drafts create": {
		"proton mail drafts create --to team@example.com --subject Standup --body 'Notes to follow.'",
		"proton mail drafts create --to jane@example.com --subject Report --attach ./report.pdf",
	},
	"proton mail drafts update": {
		"proton mail drafts update 5bH2mQxK --body 'Notes attached.'",
		"proton mail drafts update 5bH2mQxK --detach report.pdf",
	},
	"proton mail drafts send": {
		"proton mail drafts send 5bH2mQxK",
		"proton mail drafts send 5bH2mQxK --send-at 2026-04-16T09:00",
	},
	"proton mail drafts delete": {"proton mail drafts delete 5bH2mQxK"},

	// ── mail: settings ──
	"proton mail settings get":  {"proton mail settings get"},
	"proton mail settings list": {"proton mail settings list"},
	"proton mail settings set": {
		"proton mail settings set signature off",
		"proton mail settings set view-mode conversation",
	},
	"proton mail settings labels list": {"proton mail settings labels list"},
	"proton mail settings labels create": {
		"proton mail settings labels create --name Work",
		"proton mail settings labels create --name Accounting --color pacific",
	},
	"proton mail settings labels update": {
		"proton mail settings labels update Work --name Office",
		"proton mail settings labels update Work --color enzian",
	},
	"proton mail settings labels delete": {"proton mail settings labels delete Work"},
	"proton mail settings folders list":  {"proton mail settings folders list"},
	"proton mail settings folders create": {
		"proton mail settings folders create --name Receipts",
		"proton mail settings folders create --name 2026 --parent Receipts --color olive",
	},
	"proton mail settings folders update": {
		"proton mail settings folders update Receipts --name Invoices",
	},
	"proton mail settings folders delete": {"proton mail settings folders delete Receipts"},
	"proton mail settings addresses list": {"proton mail settings addresses list"},
	"proton mail settings addresses get":  {"proton mail settings addresses get me@proton.me"},
	"proton mail settings addresses update": {
		"proton mail settings addresses update me@proton.me --display-name 'Roman'",
		"proton mail settings addresses update me@proton.me --signature - --html",
		"proton mail settings addresses update me@proton.me --clear-signature",
	},
	"proton mail settings filters list": {"proton mail settings filters list"},
	"proton mail settings filters create": {
		"proton mail settings filters create --name Receipts --sieve ./receipts.sieve",
	},
	"proton mail settings filters update":  {"proton mail settings filters update Receipts --sieve ./receipts.sieve"},
	"proton mail settings filters enable":  {"proton mail settings filters enable Receipts"},
	"proton mail settings filters disable": {"proton mail settings filters disable Receipts"},
	"proton mail settings filters delete":  {"proton mail settings filters delete Receipts"},
	"proton mail settings autoreply get":   {"proton mail settings autoreply get"},
	"proton mail settings autoreply set": {
		"proton mail settings autoreply set --message 'Away until Monday.'",
		"proton mail settings autoreply set --message 'On holiday.' --start 2026-07-01 --end 2026-07-14",
	},
	"proton mail settings autoreply enable":  {"proton mail settings autoreply enable"},
	"proton mail settings autoreply disable": {"proton mail settings autoreply disable"},

	// ── pass ──
	"proton pass items list": {
		"proton pass items list",
		"proton pass items list --vault Work",
		"proton pass items list --type login",
	},
	"proton pass items get": {
		"proton pass items get github.com",
		"proton pass items get GitHub --output json",
	},
	"proton pass items create": {
		"proton pass items create --name GitHub --username roman --password hunter2 --url github.com",
		"proton pass items create --type note --name 'Door codes' --note 'Front: 1234'",
		"proton pass items create --type credit-card --name 'Travel card' --holder 'Roman' --number 4111111111111111 --expiry 2030-04",
	},
	"proton pass items update": {
		"proton pass items update GitHub --password hunter3",
		"proton pass items update GitHub --username roman-16 --url github.com",
	},
	"proton pass items trash": {
		"proton pass items trash GitHub",
		"proton pass items trash --vault Work --older-than 1y",
	},
	"proton pass items delete": {
		"proton pass items delete GitHub",
		"proton pass items delete --vault Work --all --yes",
	},
	"proton pass vaults list": {"proton pass vaults list"},
	"proton pass vaults create": {
		"proton pass vaults create --name Work",
	},
	"proton pass vaults update":   {"proton pass vaults update Work --name Office"},
	"proton pass vaults delete":   {"proton pass vaults delete Work"},
	"proton pass aliases list":    {"proton pass aliases list", "proton pass aliases list --vault Work"},
	"proton pass aliases options": {"proton pass aliases options"},
	"proton pass aliases create": {
		"proton pass aliases create --prefix shop --mailbox me@proton.me",
		"proton pass aliases create --prefix news --mailbox me@proton.me --vault Work --name 'Newsletter alias'",
	},
	"proton pass aliases enable":  {"proton pass aliases enable shop"},
	"proton pass aliases disable": {"proton pass aliases disable shop"},
	"proton pass trash list":      {"proton pass trash list"},
	"proton pass trash restore": {
		"proton pass trash restore GitHub",
		"proton pass trash restore --all",
	},
	"proton pass trash empty": {"proton pass trash empty"},

	// ── proton itself ──
	"proton version": {
		"proton version",
		"proton version --output json",
	},
	"proton update": {
		"proton update",
		"proton update --check",
		"proton update 1.9.11",
		"proton update --reinstall",
	},
	"proton uninstall": {
		"proton uninstall --dry-run",
		"proton uninstall --yes",
		"proton uninstall --yes --purge",
	},
}

// attachExamples gives every leaf the examples filed under its path.
//
// Cobra indents an Example block itself only in some templates, so the lines are
// indented here, once, and every command's help reads the same.
func attachExamples(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if lines, ok := examples[c.CommandPath()]; ok {
			indented := make([]string, len(lines))
			for i, l := range lines {
				indented[i] = "  " + l
			}
			c.Example = strings.Join(indented, "\n")
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
