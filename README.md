# proton-cli

> **Disclaimer:** This is an unofficial, community-built tool and is not endorsed by or affiliated with Proton AG. Use at your own risk.

An unofficial command-line tool for [Proton](https://proton.me) - Mail, Drive, Calendar, Pass, and Contacts from your terminal.

Implements the same authentication and encryption as the [Proton web client](https://github.com/ProtonMail/WebClients): SRP login, PGP key hierarchy, and full end-to-end encryption using [go-srp](https://github.com/ProtonMail/go-srp) and [gopenpgp](https://github.com/ProtonMail/gopenpgp).

## Install

### Download a binary (recommended)

Grab the latest binary for your platform from [**GitHub Releases**](https://github.com/roman-16/proton-cli/releases/latest).

| Platform | Binary |
|---|---|
| Linux (x86_64) | `proton-cli_linux_amd64` |
| Linux (ARM64) | `proton-cli_linux_arm64` |
| macOS (Apple Silicon) | `proton-cli_darwin_arm64` |
| macOS (Intel) | `proton-cli_darwin_amd64` |
| Windows (x86_64) | `proton-cli_windows_amd64.exe` |

**Linux / macOS:**

```bash
curl -LO https://github.com/roman-16/proton-cli/releases/latest/download/proton-cli_linux_amd64
chmod +x proton-cli_linux_amd64
sudo mv proton-cli_linux_amd64 /usr/local/bin/proton-cli
```

**Windows:** download the `.exe` from the [releases page](https://github.com/roman-16/proton-cli/releases/latest) and add it to your PATH.

### Install with Go

```bash
go install github.com/roman-16/proton-cli@latest
```

> **Note:** A `go install` build does **not** include the CAPTCHA helper that release binaries embed (see [Human Verification](#human-verification-captcha)). If Proton demands human verification at login, install a [release binary](#download-a-binary-recommended) instead.

### Build from source

```bash
git clone https://github.com/roman-16/proton-cli.git
cd proton-cli
go build .
```

## Quick Start

### 1. Set your credentials

```bash
export PROTON_USER=alice@proton.me
export PROTON_PASSWORD=your-password
# export PROTON_TOTP=123456   # if 2FA is enabled
```

The session is saved to `~/.config/proton-cli/sessions/<profile>.json` and reused automatically. The raw `api` command works without a password; encrypted commands require it.

### 2. Try it

```bash
proton-cli mail messages list
proton-cli drive items list
proton-cli --help
```

## Conventions

- **REF** - anywhere you see `REF` in usage, you can pass either a full Proton ID or a search term (subject/name/URL/title depending on the command). Ambiguous matches print candidates to stderr and exit with code 4.
- **Exit codes** - `0` success · `1` user error · `2` auth · `3` not-found · `4` conflict / ambiguous · `5` network / server.
- **Output** - `--output text|json|yaml` (default `text`). Mutations print `✓ …` to stderr and the new ID to stdout so `ID=$(proton-cli ...)` works.
- **Streaming I/O** - `-` means stdin (inputs) or stdout (outputs). `mail messages send --body -`, `drive items upload - /path`, `drive items download /path -`.
- **Cancellation** - `Ctrl+C` aborts in-flight operations.
- **Dry run** - `--dry-run` on every mutating command previews without applying.

## Commands

### Mail

```bash
# Messages
proton-cli mail messages list
proton-cli mail messages list --folder sent
proton-cli mail messages list --folder drafts --page 1 --page-size 10 --unread
proton-cli mail messages search --keyword "invoice"
proton-cli mail messages search --from "amazon" --after 2026-01-01
proton-cli mail messages read REF                          # body + an attachments footer (non-inline)
proton-cli mail messages read --include-inline REF         # also surface signature graphics in the footer
proton-cli mail messages read --format text REF            # text|html|raw (footer only in text mode)
proton-cli mail messages read --body-only REF > body.txt   # body only, no header, no footer
proton-cli mail messages send --to "to@ex.com" --subject "Hi" --body "Hello"
proton-cli mail messages send --to a@ex.com --to b@ex.com --cc c@ex.com --bcc d@ex.com --subject Hi --body Hello
proton-cli mail messages send --to to@ex.com --subject Hi --body "<b>Hello</b>" --html
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --attach ./report.pdf --attach ./img.png
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --send-at 2026-05-01T09:00   # schedule
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --expires 7d                # self-destruct
echo "body" | proton-cli mail messages send --to foo --subject bar --body -
proton-cli mail messages trash REF...
proton-cli mail messages delete REF...                     # permanent
proton-cli mail messages move --dest archive REF...
proton-cli mail messages mark read REF                     # ACTION: read|unread
proton-cli mail messages mark unread REF
proton-cli mail messages star REF
proton-cli mail messages unstar REF

# Batch filters (union with any explicit REFs)
proton-cli mail messages trash --unread --older-than 30d
proton-cli mail messages move --dest archive --from "newsletter@" --older-than 7d
proton-cli mail messages mark read --folder inbox --unread
proton-cli mail messages delete --folder spam --all

# Conversations (full threads)
proton-cli mail conversations list
proton-cli mail conversations list --folder sent --unread
proton-cli mail conversations search --keyword "invoice"
proton-cli mail conversations read CONV_ID            # full thread, chronological; per-message attachments footer
proton-cli mail conversations read --include-inline CONV_ID  # also tag signature graphics per message
proton-cli mail conversations read --body-only CONV_ID > thread.txt  # bodies only, separated by blank lines
proton-cli mail conversations read --strip-quotes CONV_ID    # remove quoted reply blocks from each message
proton-cli mail conversations read --summary CONV_ID         # one line per message: index/total, date, sender, preview
proton-cli mail conversations trash CONV_ID...
proton-cli mail conversations delete CONV_ID...       # permanent
proton-cli mail conversations move --dest archive CONV_ID...
proton-cli mail conversations mark read CONV_ID       # ACTION: read|unread
proton-cli mail conversations star CONV_ID
proton-cli mail conversations unstar CONV_ID

# Conversation-wide attachments (union across all messages in the thread)
proton-cli mail conversations attachments list CONV_ID                              # MESSAGE_ID column shows where each one came from
proton-cli mail conversations attachments list --include-inline CONV_ID             # also show signature graphics
proton-cli mail conversations attachments download CONV_ID ATTACHMENT_ID            # resolves to its parent message internally
proton-cli mail conversations attachments download CONV_ID --all --output-dir ./atts/

# Attachments
proton-cli mail attachments list MESSAGE_ID
proton-cli mail attachments list MESSAGE_ID --include-inline                   # include signature graphics etc.
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID                  # uses the attachment's own name; auto-suffixes name_1.pdf on collision
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID ./file.pdf       # explicit path; errors if file exists
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID --output ./f.pdf # same as above via flag
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID --output ./f.pdf --force  # overwrite
proton-cli mail attachments download MESSAGE_ID --all --output-dir ./atts/     # download every attachment
proton-cli mail attachments download MESSAGE_ID --all --include-inline --output-dir ./atts/  # also fetch inline images
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID -                # stdout

# Labels and folders
proton-cli mail labels list
proton-cli mail labels create --name "Important" --color "#8080FF"
proton-cli mail labels create --name "Projects" --folder --color "#1DA583"
proton-cli mail labels create --name "Subfolder" --folder --parent PARENT_LABEL_ID
proton-cli mail labels update LABEL_ID --name "Renamed" --color "#DB60D6"
proton-cli mail labels delete LABEL_ID

# Filters
proton-cli mail filters list
proton-cli mail filters create --name "Archive invoices" \
  --sieve 'require ["fileinto"]; if header :contains "Subject" "invoice" { fileinto "Archive"; }'
proton-cli mail filters update FILTER_ID --name "Renamed" --sieve '...'
proton-cli mail filters enable FILTER_ID
proton-cli mail filters disable FILTER_ID
proton-cli mail filters delete FILTER_ID

# Addresses
proton-cli mail addresses list
```

### Drive

```bash
# Items
proton-cli drive items list
proton-cli drive items list /Documents
proton-cli drive items upload ./photo.jpg                  # to root
proton-cli drive items upload ./report.pdf /Documents      # into a folder
proton-cli drive items upload - /Notes/note.txt            # from stdin
proton-cli drive items upload --recursive ./folder /Backup
proton-cli drive items download /Documents/report.pdf ./report.pdf
proton-cli drive items download --force /Documents/report.pdf ./report.pdf   # overwrite if local exists
proton-cli drive items download /Photos/pic.jpg            # to stdout
proton-cli drive items download /Photos/pic.jpg -          # to stdout (explicit)
proton-cli drive items rename /Documents/old.txt new.txt
proton-cli drive items move /Documents/report.pdf /Archive
proton-cli drive items copy /Documents/report.pdf /Archive  # copy into another folder
proton-cli drive items delete /Documents/old-report.pdf
proton-cli drive items delete --permanent /Documents/secret.txt
proton-cli drive items info /Documents/report.pdf          # type, size, checksum, sharing
proton-cli drive items revisions list /Documents/report.pdf
proton-cli drive items revisions restore /Documents/report.pdf REVISION_ID

# Batch filters
proton-cli drive items delete --pattern "*.tmp" --recursive --scope /
proton-cli drive items delete --larger-than 100MB --scope /Backups --recursive
proton-cli drive items delete --older-than 90d --scope /Logs --recursive
proton-cli drive items delete --scope /OldStuff --all --recursive

# Folders
proton-cli drive folders create /Documents/NewFolder

# Sharing - public links
proton-cli drive share status /Documents/report.pdf        # who has access + public link
proton-cli drive share link /Documents/report.pdf          # create/show the public link
proton-cli drive share link /Documents/report.pdf --edit --expires 7d
proton-cli drive share link /Documents/report.pdf --password hunter2
proton-cli drive share unlink /Documents/report.pdf        # remove the public link

# Sharing - members (invite Proton users)
proton-cli drive share add /Documents/report.pdf bob@proton.me
proton-cli drive share add /Documents/report.pdf bob@proton.me --edit
proton-cli drive share remove /Documents/report.pdf bob@proton.me

# Incoming share invitations
proton-cli drive invitations list
proton-cli drive invitations accept INVITATION_ID
proton-cli drive invitations reject INVITATION_ID

# Trash
proton-cli drive trash list
proton-cli drive trash restore LINK_ID LINK_ID2
proton-cli drive trash empty                               # empties trash across all volumes

# Photos
proton-cli drive photos list
proton-cli drive photos download PHOTO_LINK_ID              # into current dir, original name
proton-cli drive photos download PHOTO_LINK_ID --out ./pics/
proton-cli drive photos upload ./IMG_0001.jpg
proton-cli drive photos delete PHOTO_LINK_ID...            # move to trash
proton-cli drive photos delete --permanent PHOTO_LINK_ID...  # purge
proton-cli drive photos albums list
proton-cli drive photos albums create --name "Holiday"
proton-cli drive photos albums items ALBUM_LINK_ID
proton-cli drive photos albums add ALBUM_LINK_ID PHOTO_LINK_ID...
proton-cli drive photos albums remove ALBUM_LINK_ID PHOTO_LINK_ID...
proton-cli drive photos albums delete ALBUM_LINK_ID
proton-cli drive photos tags remove PHOTO_LINK_ID TAG...   # TAG is an integer tag id
```

### Calendar

```bash
# Calendars
proton-cli calendar calendars list
proton-cli calendar calendars create --name "Work" --color "#8080FF"
proton-cli calendar calendars rename CALENDAR_ID --name "Personal" --color "#DB60D6"
proton-cli calendar calendars delete CALENDAR_ID           # requires password

# Events
proton-cli calendar events list
proton-cli calendar events list --start 2026-04-15 --end 2026-04-20
proton-cli calendar events list --calendar "Work"
proton-cli calendar events get CALENDAR_ID EVENT_ID
proton-cli calendar events get "Meeting"                   # search by title
proton-cli calendar events create \
  --title "Meeting" --location "Vienna" --description "Quarterly sync" \
  --start "2026-04-16T14:00" --duration 1h
proton-cli calendar events create --title "Standup" --start 2026-04-16T09:00 \
  --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m --remind 1h        # recurrence + reminders
proton-cli calendar events create --title "Review" --start 2026-04-16T14:00 \
  --attendee alice@proton.me --attendee bob@example.com         # Proton users added directly; external ones emailed an invite
proton-cli calendar events update CALENDAR_ID EVENT_ID --title "Updated"
proton-cli calendar events delete CALENDAR_ID EVENT_ID
proton-cli calendar events delete "Meeting"                # search by title
```

### Contacts

```bash
proton-cli contacts list
proton-cli contacts get REF                                # ID or search
proton-cli contacts create --name "John Doe" --email john@example.com --phone "+1234567890"
proton-cli contacts create --name "Jane" --email a@ex.com --email b@ex.com \
  --phone "+123" --title "CTO" --birthday 1990-01-01 --address "Vienna" --url https://jane.example
proton-cli contacts update --email "new@example.com" REF
proton-cli contacts delete REF

# Contact groups
proton-cli contacts groups list
proton-cli contacts groups create --name "Team" --color "#8080FF"
proton-cli contacts groups add GROUP_ID REF...             # add contacts to a group
proton-cli contacts groups remove GROUP_ID REF...
proton-cli contacts groups delete GROUP_ID
```

### Pass

```bash
# Items
proton-cli pass items list
proton-cli pass items list --vault "Work"
proton-cli pass items get SHARE_ID ITEM_ID
proton-cli pass items get "github.com"                     # search
proton-cli pass items create --type login --name "GitHub" --username me --password secret --url github.com --totp "otpauth://..."
proton-cli pass items create --type note --name "My Note" --note "Some text"
proton-cli pass items create --type card --name "Visa" --holder "Roman" --number "4111..." --expiry "2028-12"
proton-cli pass items create --type wifi --name "Home" --ssid MyNet --password pw --security WPA2
proton-cli pass items create --type ssh_key --name "laptop" --public-key "ssh-ed25519 ..." --private-key "$(cat id_ed25519)"
proton-cli pass items create --type identity --name "Me" --full-name "Jane Roe" --email jane@ex.com --phone +123 --organization Acme
proton-cli pass items create --type custom --name "Server" --field "Host=1.2.3.4" --hidden "Root PW=secret"
proton-cli pass items create --type login --name X --field "Recovery=abc" --hidden "PIN=1234"  # custom fields on any type
proton-cli pass items edit REF --password "new-secret"                  # any field of the item's type
proton-cli pass items trash REF
proton-cli pass items restore REF
proton-cli pass items delete REF

# Batch filters
proton-cli pass items trash --vault "Old" --type login
proton-cli pass items trash --older-than 1y --type login
proton-cli pass items delete --vault "Temporary" --all

# Vaults
proton-cli pass vaults list
proton-cli pass vaults create --name "Work"
proton-cli pass vaults rename SHARE_ID --name "Personal"
proton-cli pass vaults delete SHARE_ID

# Aliases
proton-cli pass alias options
proton-cli pass alias create --prefix my-alias --mailbox my-mailbox@proton.me
```

### Settings

```bash
proton-cli settings get          # account settings
proton-cli settings mail         # mail settings
proton-cli settings set          # list the writable mail-setting keys
proton-cli settings set view-mode 1          # 0=conversations, 1=messages
proton-cli settings set draft-type text/html
proton-cli settings set hide-remote-images 1
```

### Raw API

For any endpoint not covered by high-level commands:

```bash
proton-cli api GET /drive/volumes
proton-cli api POST /calendar/v1 --body '{"Name":"Work",...}'
proton-cli api GET /mail/v4/messages --query Page=0 --query PageSize=10
proton-cli api GET /calendar/v1 --output json | jq '.Calendars[].ID'
```

## Short IDs

In interactive terminals, list commands shorten Proton IDs to 8 characters for readability:

```
$ proton-cli mail messages list
ID        FROM            SUBJECT          DATE              ⚑
────────  ──────────────  ───────────────  ────────────────  ─
NWM5AYGx  alice@a.com     Hello            2026-04-15 14:32
```

Pipes, file redirection, and `--output json|yaml` always emit full IDs:

```
$ proton-cli mail messages list --output json | jq -r '.messages[].id' NWM5AYGx_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A5C-ADmohv7TjXfYbDEIHZPQ==
```

The CLI keeps a per-profile cache of IDs you have seen at `~/.config/proton-cli/idcache/<profile>.json`, so a short prefix can be pasted into any command that takes an ID:

```
$ proton-cli mail messages read NWM5AYGx
Subject: Hello
...
```

If the prefix isn't in your local cache (e.g. you copied it from another machine), run the matching list command first or use the full ID. Pass `--full-ids` (a global flag) to disable shortening entirely.

Ambiguous prefixes (two cached IDs share the same first 8 chars) exit 4 with both candidates listed.

## Profiles (multi-account)

Create `~/.config/proton-cli/config.toml`:

```toml
default_profile = "default"

[profiles.default]
user = "alice@proton.me"

[profiles.work]
user = "alice@company.com"
api_url = "https://mail.proton.me/api"
```

Then:

```bash
proton-cli --profile work mail messages list
```

Each profile gets its own session file at `~/.config/proton-cli/sessions/<profile>.json`.

Profiles hold non-secret wiring only (`user`, `api_url`, `app_version`). You must provide your password and TOTP via `PROTON_PASSWORD` / `PROTON_TOTP` (or `--password` / `--totp`).

## Environment Variables

| Variable | Description |
|---|---|
| `PROTON_USER` | Proton account email |
| `PROTON_PASSWORD` | Account password (required for encrypted operations) |
| `PROTON_TOTP` | TOTP code (if 2FA is enabled) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `PROTON_APP_VERSION` | App version header (default: `Other`) |

Flags override env vars; env vars override profile values.

## How It Works

1. **Session creation** - creates an unauthenticated session via `POST /auth/v4/sessions`.
2. **SRP authentication** - Secure Remote Password login with [go-srp](https://github.com/ProtonMail/go-srp), with 2FA/TOTP support.
3. **Session persistence** - per profile, saves the auth tokens plus the salted key password **encrypted** with a random client key held server-side (fetched per session). The key password is never written to disk in cleartext, and revoking the session makes the saved blob undecryptable. See [`SECURITY.md`](SECURITY.md#how-credentials-are-stored-at-rest).
4. **Key hierarchy** - unlocks User key → Address keys → per-service keys (Calendar, Drive, Contacts).
5. **End-to-end encryption** - encrypts/decrypts using [gopenpgp](https://github.com/ProtonMail/gopenpgp).
6. **Auto-refresh** - refreshes expired tokens automatically.

### Encryption Details

| Service | Encrypt with | Sign with |
|---|---|---|
| Calendar events | Calendar key (session key) | Address key |
| Drive files | Node key (session key per block) | Address key |
| Drive names | Parent node key | Address key |
| Contacts | User key | User key |
| Mail | Session key | Address key |
| Pass items | AES-256-GCM (item key) | N/A (symmetric) |
| Pass vaults | AES-256-GCM (vault key) | N/A (symmetric) |

## Human Verification (CAPTCHA)

Proton's anti-bot may demand a CAPTCHA at login. proton-cli opens a small webview window via an embedded helper, you solve it, the original command retries automatically. No extra install - the helper is `//go:embed`-ded into the main binary.

Linux desktop needs `libwebkit2gtk-4.1` + `libgtk-3` installed: macOS / Windows: nothing to install (system WebKit / WebView2).

**Headless** (server, container, no GUI): the webview can't run. proton-cli exits with an error - there is no way to solve the CAPTCHA from this environment. Run the command on a desktop machine instead.

## Limitations

A few constraints are inherent to Proton's design or platform rather than missing features:

- **Colors** — labels, folders, calendars and contact groups accept only Proton's 20 fixed accent colors; the CLI validates `--color` and lists the allowed values on error.
- **Calendar deletion** — `calendar calendars delete` is password-scoped and needs `PROTON_PASSWORD`.
- **CAPTCHA** — can't be solved in a headless environment, and `go install` builds don't embed the helper (see [Human Verification](#human-verification-captcha)).
- **External mail encryption** — `mail messages send` encrypts to Proton recipients and sends cleartext (TLS) to external ones; external-PGP and encrypted-for-outside (password) sending aren't supported. Attachments and calendar invites to external recipients do work.

See [`docs/limitations.md`](docs/limitations.md) for the full list, including features not yet implemented.

## API Reference

See [`openapi.yaml`](openapi.yaml) for the complete API spec covering ~740 endpoints. To regenerate from the latest Proton source:

```bash
cd scripts && npm install && npm run generate-openapi
```

See [`scripts/README.md`](scripts/README.md) for details on the generator.

## Development

Requires [devbox](https://www.jetify.com/devbox) and [direnv](https://direnv.net/):

```bash
direnv allow
go build .
```

## License

MIT
