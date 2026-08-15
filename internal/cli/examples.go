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
	"proton-cli account get": {
		"proton-cli account get",
		"proton-cli account get --output json",
	},
	"proton-cli account login": {
		"proton-cli account login",
		"proton-cli account login --profile work",
		"proton-cli account login --user me@proton.me --password-file /run/secrets/proton",
		"proton-cli account login --user me@proton.me --password-stdin --totp 123456",
	},
	"proton-cli account logout": {
		"proton-cli account logout",
		"proton-cli account logout --revoke",
		"proton-cli account logout --all",
	},
	"proton-cli account profiles list":   {"proton-cli account profiles list"},
	"proton-cli account profiles delete": {"proton-cli account profiles delete work"},
	"proton-cli account sessions list":   {"proton-cli account sessions list"},
	"proton-cli account sessions revoke": {
		"proton-cli account sessions revoke 5bH2mQxK",
		"proton-cli account sessions revoke --others",
	},
	"proton-cli account settings get":  {"proton-cli account settings get"},
	"proton-cli account settings list": {"proton-cli account settings list"},
	"proton-cli account settings set": {
		"proton-cli account settings set locale de_AT",
		"proton-cli account settings set news off",
	},
	"proton-cli api": {
		"proton-cli api GET /core/v4/users",
		"proton-cli api GET /mail/v4/messages --query 'PageSize=5'",
		"proton-cli api POST /mail/v4/labels --body '{\"Name\":\"Work\",\"Color\":\"#8080FF\",\"Type\":1}'",
	},

	// ── calendar ──
	"proton-cli calendar events create": {
		"proton-cli calendar events create --title Dentist --start 2026-04-16T14:00 --duration 1h",
		"proton-cli calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule 'FREQ=WEEKLY;COUNT=10' --remind 15m",
		"proton-cli calendar events create --title Holiday --start 2026-07-01 --all-day --calendar Personal",
		"proton-cli calendar events create --title 'Design review' --start 2026-04-20T10:00 --duration 45m --attendee jane@example.com --location 'Room 3'",
	},
	"proton-cli calendar events list": {
		"proton-cli calendar events list",
		"proton-cli calendar events list --start 2026-04-15 --end 2026-04-30",
		"proton-cli calendar events list --calendar Work",
	},
	"proton-cli calendar events get": {
		"proton-cli calendar events get Dentist",
		"proton-cli calendar events get 4f2a1b9c@2026-04-22T09:00",
	},
	"proton-cli calendar events update": {
		"proton-cli calendar events update Dentist --start 2026-04-16T15:30",
		"proton-cli calendar events update 4f2a1b9c@2026-04-22T09:00 --location 'Room 3'",
		"proton-cli calendar events update 4f2a1b9c@2026-04-22T09:00 --title Standup --future",
	},
	"proton-cli calendar events delete": {
		"proton-cli calendar events delete Dentist",
		"proton-cli calendar events delete 4f2a1b9c@2026-05-04T09:00 --future",
	},
	"proton-cli calendar events respond": {
		"proton-cli calendar events respond 'Team sync' --status accept",
		"proton-cli calendar events respond 'Team sync' --status decline",
	},
	"proton-cli calendar settings calendars list": {"proton-cli calendar settings calendars list"},
	"proton-cli calendar settings calendars create": {
		"proton-cli calendar settings calendars create --name Work",
		"proton-cli calendar settings calendars create --name Personal --color pacific",
	},
	"proton-cli calendar settings calendars update": {
		"proton-cli calendar settings calendars update Work --name Office",
		"proton-cli calendar settings calendars update Work --color enzian",
	},
	"proton-cli calendar settings calendars delete": {"proton-cli calendar settings calendars delete Work"},
	"proton-cli calendar settings get":              {"proton-cli calendar settings get"},
	"proton-cli calendar settings list":             {"proton-cli calendar settings list"},
	"proton-cli calendar settings set": {
		"proton-cli calendar settings set week-start monday",
		"proton-cli calendar settings set default-duration 30",
	},

	// ── contacts ──
	"proton-cli contacts list": {
		"proton-cli contacts list",
		"proton-cli contacts list --output json",
	},
	"proton-cli contacts get": {
		"proton-cli contacts get jane@example.com",
		"proton-cli contacts get 'Jane Roe'",
	},
	"proton-cli contacts create": {
		"proton-cli contacts create --name 'Jane Roe' --email jane@example.com",
		"proton-cli contacts create --name 'Jane Roe' --email jane@example.com --phone '+43 660 1234567' --organization Acme",
	},
	"proton-cli contacts update": {
		"proton-cli contacts update jane --job-title 'Head of Design'",
		"proton-cli contacts update jane --email jane.roe@work.example --birthday 1990-04-16",
	},
	"proton-cli contacts delete":      {"proton-cli contacts delete jane"},
	"proton-cli contacts groups list": {"proton-cli contacts groups list"},
	"proton-cli contacts groups create": {
		"proton-cli contacts groups create --name Team",
		"proton-cli contacts groups create --name Family --color strawberry",
	},
	"proton-cli contacts groups update": {
		"proton-cli contacts groups update Team --name Engineering",
		"proton-cli contacts groups update Team --color reef",
	},
	"proton-cli contacts groups delete": {"proton-cli contacts groups delete Team"},
	"proton-cli contacts groups add":    {"proton-cli contacts groups add Team jane"},
	"proton-cli contacts groups remove": {"proton-cli contacts groups remove Team jane"},
	"proton-cli contacts keys list":     {"proton-cli contacts keys list jane"},
	"proton-cli contacts keys pin": {
		"proton-cli contacts keys pin jane --key jane-pubkey.asc",
		"proton-cli contacts keys pin jane --email jane@example.com --key - --no-encrypt",
	},
	"proton-cli contacts keys unpin": {
		"proton-cli contacts keys unpin jane",
		"proton-cli contacts keys unpin jane --email jane@example.com",
	},

	// ── drive ──
	"proton-cli drive items list": {
		"proton-cli drive items list",
		"proton-cli drive items list /Documents",
	},
	"proton-cli drive items get": {"proton-cli drive items get /Documents/report.pdf"},
	"proton-cli drive items upload": {
		"proton-cli drive items upload ./report.pdf /Documents",
		"proton-cli drive items upload --recursive ./project /Backup",
		"proton-cli drive items upload --if-exists replace ./report.pdf /Documents",
		"pg_dump mydb | gzip | proton-cli drive items upload - /Backups/db.sql.gz",
	},
	"proton-cli drive items download": {
		"proton-cli drive items download /Documents/report.pdf --output-dir .",
		"proton-cli drive items download /Documents/report.pdf --output - > report.pdf",
	},
	"proton-cli drive items update": {"proton-cli drive items update /Documents/report.pdf --name summary.pdf"},
	"proton-cli drive items move": {
		"proton-cli drive items move /Documents/report.pdf --into /Archive",
		"proton-cli drive items move --pattern '*.log' --scope /Build --recursive --into /Archive",
	},
	"proton-cli drive items copy": {
		"proton-cli drive items copy /Documents/report.pdf --into /Archive",
		"proton-cli drive items copy --pattern '*.pdf' --scope /Documents --into /Backup",
	},
	"proton-cli drive items trash": {
		"proton-cli drive items trash /Documents/report.pdf",
		"proton-cli drive items trash --pattern '*.tmp' --scope /Build --recursive",
		"proton-cli drive items trash --older-than 1y --scope /Downloads --dry-run",
	},
	"proton-cli drive items delete": {
		"proton-cli drive items delete /Documents/report.pdf",
		"proton-cli drive items delete --pattern '*.tmp' --scope /Build --recursive --yes",
	},
	"proton-cli drive items revisions list":     {"proton-cli drive items revisions list /Documents/report.pdf"},
	"proton-cli drive items revisions restore":  {"proton-cli drive items revisions restore /Documents/report.pdf 5bH2mQxK"},
	"proton-cli drive items revisions download": {"proton-cli drive items revisions download /Documents/report.pdf 5bH2mQxK --output-dir ."},
	"proton-cli drive items revisions delete":   {"proton-cli drive items revisions delete /Documents/report.pdf 5bH2mQxK"},
	"proton-cli drive folders create": {
		"proton-cli drive folders create /Documents/2026",
	},
	"proton-cli drive share get": {"proton-cli drive share get /Documents/report.pdf"},
	"proton-cli drive share link": {
		"proton-cli drive share link /Documents/report.pdf",
		"proton-cli drive share link /Documents/report.pdf --expires 7d --password hunter2",
		"proton-cli drive share link /Documents --edit",
	},
	"proton-cli drive share unlink": {"proton-cli drive share unlink /Documents/report.pdf"},
	"proton-cli drive share add": {
		"proton-cli drive share add /Documents jane@example.com",
		"proton-cli drive share add /Documents jane@example.com --edit --message 'Have a look'",
	},
	"proton-cli drive share remove":     {"proton-cli drive share remove /Documents jane@example.com"},
	"proton-cli drive invitations list": {"proton-cli drive invitations list"},
	"proton-cli drive invitations accept": {
		"proton-cli drive invitations accept 5bH2mQxK",
	},
	"proton-cli drive invitations decline": {"proton-cli drive invitations decline 5bH2mQxK"},
	"proton-cli drive trash list":          {"proton-cli drive trash list"},
	"proton-cli drive trash restore":       {"proton-cli drive trash restore 5bH2mQxK"},
	"proton-cli drive trash empty":         {"proton-cli drive trash empty"},
	"proton-cli drive photos list": {
		"proton-cli drive photos list",
		"proton-cli drive photos list --album Holidays",
		"proton-cli drive photos list --tag favorites",
	},
	"proton-cli drive photos upload":      {"proton-cli drive photos upload ./IMG_2291.jpg"},
	"proton-cli drive photos download":    {"proton-cli drive photos download 5bH2mQxK --output-dir ."},
	"proton-cli drive photos favorite":    {"proton-cli drive photos favorite 5bH2mQxK"},
	"proton-cli drive photos unfavorite":  {"proton-cli drive photos unfavorite 5bH2mQxK"},
	"proton-cli drive photos trash":       {"proton-cli drive photos trash 5bH2mQxK"},
	"proton-cli drive photos delete":      {"proton-cli drive photos delete 5bH2mQxK"},
	"proton-cli drive photos albums list": {"proton-cli drive photos albums list"},
	"proton-cli drive photos albums create": {
		"proton-cli drive photos albums create --name Holidays",
	},
	"proton-cli drive photos albums add":    {"proton-cli drive photos albums add Holidays 5bH2mQxK"},
	"proton-cli drive photos albums remove": {"proton-cli drive photos albums remove Holidays 5bH2mQxK"},
	"proton-cli drive photos albums delete": {
		"proton-cli drive photos albums delete Holidays",
		"proton-cli drive photos albums delete Holidays --delete-photos",
	},
	"proton-cli drive settings get":  {"proton-cli drive settings get"},
	"proton-cli drive settings list": {"proton-cli drive settings list"},
	"proton-cli drive settings set":  {"proton-cli drive settings set revision-retention 30"},

	// ── mail: messages ──
	"proton-cli mail messages list": {
		"proton-cli mail messages list",
		"proton-cli mail messages list --unread",
		"proton-cli mail messages list --folder archive --page-size 50",
		"proton-cli mail messages list --starred --output json",
	},
	"proton-cli mail messages get": {
		"proton-cli mail messages get 'Invoice #2291'",
		"proton-cli mail messages get 5bH2mQxK --format html",
		"proton-cli mail messages get 5bH2mQxK --body-only --strip-quotes",
	},
	"proton-cli mail messages search": {
		"proton-cli mail messages search --from billing@example.com",
		"proton-cli mail messages search --keyword invoice --after 2026-01-01",
		"proton-cli mail messages search --subject Report --folder archive --limit 20",
	},
	"proton-cli mail messages send": {
		"proton-cli mail messages send --to jane@example.com --subject Report --body 'See attached.' --attach ./report.pdf",
		"proton-cli mail messages send --to team@example.com --subject Standup --body -",
		"proton-cli mail messages send --to jane@example.com --subject Reminder --send-at 2026-04-16T09:00",
		"proton-cli mail messages send --eml ./draft.eml",
	},
	"proton-cli mail messages reply": {
		"proton-cli mail messages reply 'Invoice #2291' --body 'Thanks, paid today.'",
		"proton-cli mail messages reply 'Invoice #2291' --all --body 'Noted.'",
		"proton-cli mail messages reply 'Invoice #2291' --body 'Draft first.' --draft",
	},
	"proton-cli mail messages forward": {
		"proton-cli mail messages forward 'Invoice #2291' --to jane@example.com",
		"proton-cli mail messages forward 'Invoice #2291' --to jane@example.com --no-attachments",
	},
	"proton-cli mail messages move": {
		"proton-cli mail messages move 'Invoice #2291' --into archive",
		"proton-cli mail messages move --from newsletter@example.com --older-than 90d --into archive",
	},
	"proton-cli mail messages trash": {
		"proton-cli mail messages trash 'Invoice #2291'",
		"proton-cli mail messages trash --unread --older-than 30d",
		"proton-cli mail messages trash --from newsletter@example.com --older-than 90d --dry-run",
	},
	"proton-cli mail messages delete": {
		"proton-cli mail messages delete 5bH2mQxK",
		"proton-cli mail messages delete --folder spam --all --yes",
	},
	"proton-cli mail messages label": {
		"proton-cli mail messages label 'Invoice #2291' --label Accounting",
		"proton-cli mail messages label --from billing@example.com --label Accounting",
	},
	"proton-cli mail messages unlabel": {"proton-cli mail messages unlabel 'Invoice #2291' --label Accounting"},
	"proton-cli mail messages star":    {"proton-cli mail messages star 'Invoice #2291'"},
	"proton-cli mail messages unstar":  {"proton-cli mail messages unstar 'Invoice #2291'"},
	"proton-cli mail messages mark read": {
		"proton-cli mail messages mark read 'Invoice #2291'",
		"proton-cli mail messages mark read --folder inbox --all",
	},
	"proton-cli mail messages mark unread": {"proton-cli mail messages mark unread 'Invoice #2291'"},
	"proton-cli mail messages unschedule": {
		"proton-cli mail messages unschedule 5bH2mQxK",
		"proton-cli mail messages unschedule --all",
	},
	"proton-cli mail messages export": {
		"proton-cli mail messages export 'Invoice #2291' --output-dir ./backup",
		"proton-cli mail messages export --folder archive --all --output-dir ./mail-backup",
		"proton-cli mail messages export --folder archive --older-than 1y --format mbox --output archive.mbox",
	},
	"proton-cli mail messages attachments list": {
		"proton-cli mail messages attachments list 'Invoice #2291'",
		"proton-cli mail messages attachments list 5bH2mQxK --include-inline",
	},
	"proton-cli mail messages attachments download": {
		"proton-cli mail messages attachments download 'Invoice #2291' --output-dir .",
		"proton-cli mail messages attachments download 5bH2mQxK kQ81mDx4 --output invoice.pdf",
	},

	// ── mail: conversations ──
	"proton-cli mail conversations list": {
		"proton-cli mail conversations list",
		"proton-cli mail conversations list --unread --folder inbox",
	},
	"proton-cli mail conversations get": {
		"proton-cli mail conversations get 'Quarterly numbers'",
		"proton-cli mail conversations get 5bH2mQxK --summary",
	},
	"proton-cli mail conversations search": {
		"proton-cli mail conversations search --from jane@example.com",
		"proton-cli mail conversations search --keyword budget --limit 20",
	},
	"proton-cli mail conversations reply": {
		"proton-cli mail conversations reply 'Quarterly numbers' --body 'Looks right to me.'",
		"proton-cli mail conversations reply 'Quarterly numbers' --all --body Agreed.",
	},
	"proton-cli mail conversations forward": {
		"proton-cli mail conversations forward 'Quarterly numbers' --to jane@example.com",
	},
	"proton-cli mail conversations move": {
		"proton-cli mail conversations move 'Quarterly numbers' --into archive",
		"proton-cli mail conversations move --older-than 90d --folder inbox --into archive",
	},
	"proton-cli mail conversations trash": {
		"proton-cli mail conversations trash 'Quarterly numbers'",
		"proton-cli mail conversations trash --from newsletter@example.com --older-than 90d",
	},
	"proton-cli mail conversations delete": {"proton-cli mail conversations delete 5bH2mQxK"},
	"proton-cli mail conversations label": {
		"proton-cli mail conversations label 'Quarterly numbers' --label Accounting",
	},
	"proton-cli mail conversations unlabel": {
		"proton-cli mail conversations unlabel 'Quarterly numbers' --label Accounting",
	},
	"proton-cli mail conversations star":   {"proton-cli mail conversations star 'Quarterly numbers'"},
	"proton-cli mail conversations unstar": {"proton-cli mail conversations unstar 'Quarterly numbers'"},
	"proton-cli mail conversations mark read": {
		"proton-cli mail conversations mark read 'Quarterly numbers'",
		"proton-cli mail conversations mark read --folder inbox --all",
	},
	"proton-cli mail conversations mark unread": {"proton-cli mail conversations mark unread 'Quarterly numbers'"},
	"proton-cli mail conversations export": {
		"proton-cli mail conversations export 'Quarterly numbers' --output-dir ./backup",
	},
	"proton-cli mail conversations attachments list": {
		"proton-cli mail conversations attachments list 'Quarterly numbers'",
	},
	"proton-cli mail conversations attachments download": {
		"proton-cli mail conversations attachments download 'Quarterly numbers' --output-dir .",
	},

	// ── mail: drafts ──
	"proton-cli mail drafts list": {"proton-cli mail drafts list"},
	"proton-cli mail drafts create": {
		"proton-cli mail drafts create --to team@example.com --subject Standup --body 'Notes to follow.'",
		"proton-cli mail drafts create --to jane@example.com --subject Report --attach ./report.pdf",
	},
	"proton-cli mail drafts update": {
		"proton-cli mail drafts update 5bH2mQxK --body 'Notes attached.'",
		"proton-cli mail drafts update 5bH2mQxK --detach report.pdf",
	},
	"proton-cli mail drafts send": {
		"proton-cli mail drafts send 5bH2mQxK",
		"proton-cli mail drafts send 5bH2mQxK --send-at 2026-04-16T09:00",
	},
	"proton-cli mail drafts delete": {"proton-cli mail drafts delete 5bH2mQxK"},

	// ── mail: settings ──
	"proton-cli mail settings get":  {"proton-cli mail settings get"},
	"proton-cli mail settings list": {"proton-cli mail settings list"},
	"proton-cli mail settings set": {
		"proton-cli mail settings set signature off",
		"proton-cli mail settings set view-mode conversation",
	},
	"proton-cli mail settings labels list": {"proton-cli mail settings labels list"},
	"proton-cli mail settings labels create": {
		"proton-cli mail settings labels create --name Work",
		"proton-cli mail settings labels create --name Accounting --color pacific",
	},
	"proton-cli mail settings labels update": {
		"proton-cli mail settings labels update Work --name Office",
		"proton-cli mail settings labels update Work --color enzian",
	},
	"proton-cli mail settings labels delete": {"proton-cli mail settings labels delete Work"},
	"proton-cli mail settings folders list":  {"proton-cli mail settings folders list"},
	"proton-cli mail settings folders create": {
		"proton-cli mail settings folders create --name Receipts",
		"proton-cli mail settings folders create --name 2026 --parent Receipts --color olive",
	},
	"proton-cli mail settings folders update": {
		"proton-cli mail settings folders update Receipts --name Invoices",
	},
	"proton-cli mail settings folders delete": {"proton-cli mail settings folders delete Receipts"},
	"proton-cli mail settings addresses list": {"proton-cli mail settings addresses list"},
	"proton-cli mail settings addresses get":  {"proton-cli mail settings addresses get me@proton.me"},
	"proton-cli mail settings addresses update": {
		"proton-cli mail settings addresses update me@proton.me --display-name 'Roman'",
		"proton-cli mail settings addresses update me@proton.me --signature - --html",
		"proton-cli mail settings addresses update me@proton.me --clear-signature",
	},
	"proton-cli mail settings filters list": {"proton-cli mail settings filters list"},
	"proton-cli mail settings filters create": {
		"proton-cli mail settings filters create --name Receipts --sieve ./receipts.sieve",
	},
	"proton-cli mail settings filters update":  {"proton-cli mail settings filters update Receipts --sieve ./receipts.sieve"},
	"proton-cli mail settings filters enable":  {"proton-cli mail settings filters enable Receipts"},
	"proton-cli mail settings filters disable": {"proton-cli mail settings filters disable Receipts"},
	"proton-cli mail settings filters delete":  {"proton-cli mail settings filters delete Receipts"},
	"proton-cli mail settings autoreply get":   {"proton-cli mail settings autoreply get"},
	"proton-cli mail settings autoreply set": {
		"proton-cli mail settings autoreply set --message 'Away until Monday.'",
		"proton-cli mail settings autoreply set --message 'On holiday.' --start 2026-07-01 --end 2026-07-14",
	},
	"proton-cli mail settings autoreply enable":  {"proton-cli mail settings autoreply enable"},
	"proton-cli mail settings autoreply disable": {"proton-cli mail settings autoreply disable"},

	// ── pass ──
	"proton-cli pass items list": {
		"proton-cli pass items list",
		"proton-cli pass items list --vault Work",
		"proton-cli pass items list --type login",
	},
	"proton-cli pass items get": {
		"proton-cli pass items get github.com",
		"proton-cli pass items get GitHub --output json",
	},
	"proton-cli pass items create": {
		"proton-cli pass items create --name GitHub --username roman --password hunter2 --url github.com",
		"proton-cli pass items create --type note --name 'Door codes' --note 'Front: 1234'",
		"proton-cli pass items create --type credit-card --name 'Travel card' --holder 'Roman' --number 4111111111111111 --expiry 2030-04",
	},
	"proton-cli pass items update": {
		"proton-cli pass items update GitHub --password hunter3",
		"proton-cli pass items update GitHub --username roman-16 --url github.com",
	},
	"proton-cli pass items trash": {
		"proton-cli pass items trash GitHub",
		"proton-cli pass items trash --vault Work --older-than 1y",
	},
	"proton-cli pass items delete": {
		"proton-cli pass items delete GitHub",
		"proton-cli pass items delete --vault Work --all --yes",
	},
	"proton-cli pass vaults list": {"proton-cli pass vaults list"},
	"proton-cli pass vaults create": {
		"proton-cli pass vaults create --name Work",
	},
	"proton-cli pass vaults update":   {"proton-cli pass vaults update Work --name Office"},
	"proton-cli pass vaults delete":   {"proton-cli pass vaults delete Work"},
	"proton-cli pass aliases list":    {"proton-cli pass aliases list", "proton-cli pass aliases list --vault Work"},
	"proton-cli pass aliases options": {"proton-cli pass aliases options"},
	"proton-cli pass aliases create": {
		"proton-cli pass aliases create --prefix shop --mailbox me@proton.me",
		"proton-cli pass aliases create --prefix news --mailbox me@proton.me --vault Work --name 'Newsletter alias'",
	},
	"proton-cli pass aliases enable":  {"proton-cli pass aliases enable shop"},
	"proton-cli pass aliases disable": {"proton-cli pass aliases disable shop"},
	"proton-cli pass trash list":      {"proton-cli pass trash list"},
	"proton-cli pass trash restore": {
		"proton-cli pass trash restore GitHub",
		"proton-cli pass trash restore --all",
	},
	"proton-cli pass trash empty": {"proton-cli pass trash empty"},

	// ── proton-cli itself ──
	"proton-cli version": {
		"proton-cli version",
		"proton-cli version --output json",
	},
	"proton-cli update": {
		"proton-cli update",
		"proton-cli update --check",
		"proton-cli update 1.9.11",
		"proton-cli update --reinstall",
	},
	"proton-cli uninstall": {
		"proton-cli uninstall --dry-run",
		"proton-cli uninstall --yes",
		"proton-cli uninstall --yes --purge",
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
