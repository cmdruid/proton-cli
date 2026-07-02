# proton-cli

[![Release](https://img.shields.io/github/v/release/roman-16/proton-cli?sort=semver)](https://github.com/roman-16/proton-cli/releases/latest)
[![License: MIT](https://img.shields.io/github/license/roman-16/proton-cli)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/roman-16/proton-cli)](go.mod)

> **Disclaimer:** This is an unofficial, community-built tool and is not endorsed by or affiliated with Proton AG. Use at your own risk.

An unofficial command-line tool for [Proton](https://proton.me) - Mail, Drive, Calendar, Pass, and Contacts from your terminal, with real end-to-end encryption.

proton-cli implements the same authentication and encryption as the [Proton web client](https://github.com/ProtonMail/WebClients): SRP login, the PGP key hierarchy, and full end-to-end encryption, using [go-srp](https://github.com/ProtonMail/go-srp) and [gopenpgp](https://github.com/ProtonMail/gopenpgp).

## Contents

- [Features](#features)
- [Install](#install)
- [Quick start](#quick-start)
- [Core concepts](#core-concepts)
- [Configuration](#configuration)
- [Usage](#usage)
- [Scripting](#scripting)
- [How it works](#how-it-works)
- [Human verification (CAPTCHA)](#human-verification-captcha)
- [Security](#security)
- [Limitations](#limitations)
- [Development](#development)
- [API reference](#api-reference)
- [License](#license)

## Features

- **Mail** - list, search, read, send (attachments, HTML, scheduled, self-destruct), organize, and manage labels, folders, and Sieve filters.
- **Drive** - upload/download (streaming and recursive), move, copy, revisions, public links, member sharing, trash, and photos.
- **Calendar** - calendars and events, recurrence, reminders, and attendees.
- **Pass** - vaults, items (login, note, card, wifi, ssh key, identity, custom), aliases, and TOTP.
- **Contacts** - full CRUD, pinned encryption keys, and contact groups.
- **Real E2EE** - SRP login and the full PGP key hierarchy, decrypting and signing exactly like the web client.
- **Built for scripts** - `--output json`, meaningful exit codes, streaming I/O, and `stdout = new ID` on create.

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

> **Note:** `go install` builds do **not** embed the CAPTCHA helper that release binaries include. If Proton demands human verification at login, install a release binary instead. See [Human verification](#human-verification-captcha).

### Build from source

```bash
git clone https://github.com/roman-16/proton-cli.git
cd proton-cli
go build .
```

## Quick start

### 1. Set your credentials

```bash
export PROTON_USER=alice@proton.me
export PROTON_PASSWORD=your-password
# export PROTON_TOTP=123456   # if 2FA is enabled
```

The session is saved to `~/.config/proton-cli/sessions/<profile>.json` after the first login and reused automatically.

### 2. Try it

```bash
proton-cli mail messages list
proton-cli drive items list
proton-cli --help          # every command and subcommand accepts --help
proton-cli --version
```

## Core concepts

These apply across every command.

- **REF** - anywhere you see `REF`, pass either a full Proton ID or a search term (subject / name / URL / title, depending on the command). Ambiguous matches print candidates to stderr and exit `4`.
- **Short IDs** - in an interactive terminal, list commands shorten Proton IDs to 8 characters. proton-cli caches the IDs you have seen at `~/.config/proton-cli/idcache/<profile>.json`, so you can paste an 8-char prefix into any command that takes an ID. Pipes, redirection, and `--output json|yaml` always emit full IDs. Pass `--full-ids` to disable shortening. See [Short IDs](#short-ids) below.
- **Output** - `--output text|json|yaml` (default `text`). JSON/YAML use `snake_case` keys.
- **Exit codes** - `0` success · `1` user error · `2` auth · `3` not-found · `4` conflict / ambiguous · `5` network / server · `130` cancelled.
- **Create = ID on stdout** - creating commands print the new ID to stdout and `✓ …` to stderr, so `ID=$(proton-cli ... create ...)` works.
- **Streaming I/O** - `-` means stdin (inputs) or stdout (outputs), e.g. `mail messages send --body -`, `drive items upload - /path`, `drive items download /path -`.
- **Dry run** - `--dry-run` on any mutating command previews without applying.
- **Cancellation** - `Ctrl+C` aborts in-flight operations.

### Short IDs

```
$ proton-cli mail messages list
ID        FROM            SUBJECT          DATE              ⚑
────────  ──────────────  ───────────────  ────────────────  ─
NWM5AYGx  alice@a.com     Hello            2026-04-15 14:32
```

The short prefix pastes straight back into any command:

```
$ proton-cli mail messages read NWM5AYGx
Subject: Hello
...
```

Pipes and `--output json|yaml` always emit full IDs:

```
$ proton-cli mail messages list --output json | jq -r '.messages[].id'
NWM5AYGx_FIHWT2_QbBr-whe-bIE8rbZunzr5RhXGaihvQ43z2qcxcqFgVRwi7A5C-ADmohv7TjXfYbDEIHZPQ==
```

If a prefix isn't in your local cache (e.g. copied from another machine), run the matching list command first or use the full ID. Ambiguous prefixes (two cached IDs share the first 8 chars) exit `4` with both candidates listed.

## Configuration

Credentials and connection settings resolve in this order: **a flag overrides the profile-scoped env var (`PROTON_<PROFILE>_X`), which overrides the plain env var (`PROTON_X`).** See [Profiles](#profiles-multi-account).

### Environment variables

| Variable | Description |
|---|---|
| `PROTON_USER` | Proton account email |
| `PROTON_PASSWORD` | Account password (required for encrypted operations) |
| `PROTON_TOTP` | TOTP code (if 2FA is enabled) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `PROTON_APP_VERSION` | App version header (default: `Other`) |

### Profiles (multi-account)

A profile is just a name. Each profile has its own session file (`~/.config/proton-cli/sessions/<profile>.json`) and its own set of profile-scoped environment variables. Select the active profile with the `--profile` flag or the `PROTON_PROFILE` environment variable (the flag wins; both fall back to `default`):

```bash
proton-cli --profile work mail messages list
# or make it the default for your shell session:
export PROTON_PROFILE=work
proton-cli mail messages list
```

For every setting, the active profile is consulted **scoped-first, then unscoped**: `PROTON_<PROFILE>_X` takes precedence over the plain `PROTON_X`. `<PROFILE>` is the profile name upper-cased, with any non-alphanumeric character replaced by `_` (so `work` becomes `WORK`, `my-work` becomes `MY_WORK`). This applies to every env-backed setting (`USER`, `PASSWORD`, `TOTP`, `API_URL`, `APP_VERSION`):

```bash
export PROTON_PROFILE=work
export PROTON_WORK_USER=alice@company.com
export PROTON_WORK_PASSWORD=work-password
proton-cli mail messages list          # uses PROTON_WORK_*, falling back to PROTON_*
```

Only the per-profile session file is written to disk; all other wiring lives in flags and the environment.

## Usage

The examples below are representative, not exhaustive. Every command lists its full flags with `--help`, e.g. `proton-cli mail messages send --help`.

| Area | Subcommands |
|---|---|
| `mail messages` | list, search, read, send, trash, delete, move, mark, star, unstar |
| `mail conversations` | list, search, read, trash, delete, move, mark, star, unstar, attachments |
| `mail attachments` | list, download |
| `mail labels` | list, create, update, delete |
| `mail filters` | list, create, update, enable, disable, delete |
| `mail addresses` | list |
| `drive items` | list, info, upload, download, rename, move, copy, delete, revisions |
| `drive folders` | create |
| `drive share` | status, link, unlink, add, remove |
| `drive invitations` | list, accept, reject |
| `drive trash` | list, restore, empty |
| `drive photos` | list, upload, download, delete, favorite, unfavorite, albums |
| `calendar calendars` | list, create, rename, delete |
| `calendar events` | list, get, create, update, respond, delete |
| `contacts` | list, get, create, update, delete, pin-key, unpin-key, groups |
| `pass items` | list, get, create, edit, trash, restore, delete |
| `pass vaults` | list, create, rename, delete |
| `pass alias` | options, create |
| `settings` | get, mail, set |
| `api` | any endpoint (GET / POST / PUT / DELETE) |

<details>
<summary><b>Mail</b></summary>

```bash
# Messages
proton-cli mail messages list --folder inbox --unread
proton-cli mail messages search --keyword "invoice"
proton-cli mail messages search --from "amazon" --after 2026-01-01
proton-cli mail messages read REF                       # body + attachments footer
proton-cli mail messages read --format text|html|raw REF
proton-cli mail messages read --body-only REF > body.txt
proton-cli mail messages send --to a@ex.com --cc c@ex.com --subject Hi --body Hello
proton-cli mail messages send --to to@ex.com --subject Hi --body "<b>Hi</b>" --html
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --attach ./report.pdf
proton-cli mail messages send --to to@ex.com --subject Hi --body "<b>Hi</b>" --html --attach-inline ./logo.png   # embed an image inline in the HTML body
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --send-at 2026-05-01T09:00
proton-cli mail messages send --to to@ex.com --subject Hi --body Hi --expires 7d
proton-cli mail messages send --to bob@gmail.com --subject Hi --body secret --eo-password hunter2   # password-protect for non-Proton recipients
echo "body" | proton-cli mail messages send --to foo --subject bar --body -
proton-cli mail messages trash REF...
proton-cli mail messages delete REF...                  # permanent
proton-cli mail messages move --dest archive REF...
proton-cli mail messages mark read|unread REF
proton-cli mail messages star REF
proton-cli mail messages unstar REF

# Batch filters (union with any explicit REFs)
proton-cli mail messages trash --unread --older-than 30d
proton-cli mail messages move --dest archive --from "newsletter@" --older-than 7d
proton-cli mail messages delete --folder spam --all
# Combine filters, --limit, --all: proton-cli mail messages trash --help

# Conversations (full threads; same verbs as messages)
proton-cli mail conversations list --folder sent --unread
proton-cli mail conversations read CONV_ID              # full thread, chronological
proton-cli mail conversations read --summary CONV_ID    # one line per message
proton-cli mail conversations read --strip-quotes CONV_ID
proton-cli mail conversations attachments list CONV_ID
proton-cli mail conversations attachments download CONV_ID --all --output-dir ./atts/

# Attachments
proton-cli mail attachments list MESSAGE_ID
proton-cli mail attachments download MESSAGE_ID ATT_ID ./file.pdf
proton-cli mail attachments download MESSAGE_ID --all --output-dir ./atts/
proton-cli mail attachments download MESSAGE_ID ATT_ID -   # stdout
# --force, --include-inline, auto-suffix on collision: proton-cli mail attachments download --help

# Labels and folders
proton-cli mail labels list
proton-cli mail labels create --name "Important" --color "#8080FF"
proton-cli mail labels create --name "Projects" --folder --parent PARENT_LABEL_ID
proton-cli mail labels update LABEL_ID --name "Renamed" --color "#DB60D6"
proton-cli mail labels delete LABEL_ID

# Filters (Sieve)
proton-cli mail filters list
proton-cli mail filters create --name "Archive invoices" \
  --sieve 'require ["fileinto"]; if header :contains "Subject" "invoice" { fileinto "Archive"; }'
proton-cli mail filters enable|disable FILTER_ID
proton-cli mail filters delete FILTER_ID

# Addresses
proton-cli mail addresses list
```

</details>

<details>
<summary><b>Drive</b></summary>

```bash
# Items
proton-cli drive items list /Documents
proton-cli drive items info /Documents/report.pdf       # type, size, checksum, sharing
proton-cli drive items upload ./report.pdf /Documents
proton-cli drive items upload --recursive ./folder /Backup
proton-cli drive items upload - /Notes/note.txt         # from stdin
proton-cli drive items download /Documents/report.pdf ./report.pdf
proton-cli drive items download /Photos/pic.jpg -       # to stdout
proton-cli drive items rename /Documents/old.txt new.txt
proton-cli drive items move /Documents/report.pdf /Archive
proton-cli drive items copy /Documents/report.pdf /Archive
proton-cli drive items delete /Documents/old.pdf        # to trash
proton-cli drive items delete --permanent /Documents/secret.txt
proton-cli drive items revisions list /Documents/report.pdf
proton-cli drive items revisions restore /Documents/report.pdf REVISION_ID

# Batch filters
proton-cli drive items delete --pattern "*.tmp" --scope / --recursive
proton-cli drive items delete --larger-than 100MB --scope /Backups --recursive
proton-cli drive items delete --older-than 90d --scope /Logs --recursive

# Folders
proton-cli drive folders create /Documents/NewFolder

# Sharing - public links
proton-cli drive share status /Documents/report.pdf     # who has access + public link
proton-cli drive share link /Documents/report.pdf       # create/show the public link
proton-cli drive share link /Documents/report.pdf --edit --expires 7d --password hunter2
proton-cli drive share unlink /Documents/report.pdf

# Sharing - members (invite Proton users)
proton-cli drive share add /Documents/report.pdf bob@proton.me --edit
proton-cli drive share remove /Documents/report.pdf bob@proton.me

# Incoming share invitations
proton-cli drive invitations list
proton-cli drive invitations accept|reject INVITATION_ID

# Trash
proton-cli drive trash list
proton-cli drive trash restore LINK_ID...
proton-cli drive trash empty                            # across all volumes

# Photos
proton-cli drive photos list
proton-cli drive photos list --tags favorites           # filter by tag (favorites, screenshots, videos, …)
proton-cli drive photos upload ./IMG_0001.jpg
proton-cli drive photos download PHOTO_LINK_ID --out ./pics/
proton-cli drive photos delete PHOTO_LINK_ID...         # to trash (--permanent to purge)
proton-cli drive photos favorite PHOTO_LINK_ID...       # mark as favorite (album-only photos are copied to your timeline)
proton-cli drive photos unfavorite PHOTO_LINK_ID...     # remove from favorites
proton-cli drive photos albums list
proton-cli drive photos albums create --name "Holiday"
proton-cli drive photos albums add ALBUM_LINK_ID PHOTO_LINK_ID...
proton-cli drive photos albums items ALBUM_LINK_ID
```

</details>

<details>
<summary><b>Calendar</b></summary>

```bash
# Calendars
proton-cli calendar calendars list
proton-cli calendar calendars create --name "Work" --color "#8080FF"
proton-cli calendar calendars rename CALENDAR_ID --name "Personal" --color "#DB60D6"
proton-cli calendar calendars delete CALENDAR_ID        # requires PROTON_PASSWORD

# Events
proton-cli calendar events list --calendar "Work" --start 2026-04-15 --end 2026-04-20
proton-cli calendar events get CALENDAR_ID EVENT_ID
proton-cli calendar events get "Meeting"                # search by title
proton-cli calendar events create \
  --title "Meeting" --location "Vienna" --description "Quarterly sync" \
  --start "2026-04-16T14:00" --duration 1h
proton-cli calendar events create --title "Standup" --start 2026-04-16T09:00 \
  --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m --remind 1h        # recurrence + reminders
proton-cli calendar events create --title "Review" --start 2026-04-16T14:00 \
  --attendee alice@proton.me --attendee bob@example.com         # externals get an emailed invite
proton-cli calendar events update CALENDAR_ID EVENT_ID --title "Updated"
proton-cli calendar events respond CALENDAR_ID EVENT_ID --status accept    # reply to an invitation (accept|tentative|decline)
proton-cli calendar events respond "Meeting" --status decline              # by title; emails the organizer a REPLY
proton-cli calendar events delete CALENDAR_ID EVENT_ID
```

</details>

<details>
<summary><b>Contacts</b></summary>

```bash
proton-cli contacts list
proton-cli contacts get REF                             # ID or search
proton-cli contacts create --name "John Doe" --email john@example.com --phone "+1234567890"
proton-cli contacts create --name "Jane" --email a@ex.com --email b@ex.com \
  --title "CTO" --birthday 1990-01-01 --address "Vienna" --url https://jane.example
proton-cli contacts update --email "new@example.com" REF
proton-cli contacts delete REF

# Pinned keys - encrypt mail to a specific PGP key you trust for a contact
proton-cli contacts pin-key REF --key bob-pubkey.asc            # pin & auto-encrypt to it
proton-cli contacts pin-key REF --email bob@ex.com --key -      # armored key from stdin; pick which email
proton-cli contacts unpin-key REF

# Contact groups
proton-cli contacts groups list
proton-cli contacts groups create --name "Team" --color "#8080FF"
proton-cli contacts groups add GROUP_ID REF...
proton-cli contacts groups remove GROUP_ID REF...
proton-cli contacts groups delete GROUP_ID
```

</details>

<details>
<summary><b>Pass</b></summary>

```bash
# Items
proton-cli pass items list --vault "Work"
proton-cli pass items get SHARE_ID ITEM_ID
proton-cli pass items get "github.com"                  # search
proton-cli pass items create --type login --name "GitHub" --username me --password secret \
  --url github.com --totp "otpauth://..."
proton-cli pass items create --type note --name "My Note" --note "Some text"
proton-cli pass items create --type card --name "Visa" --holder "Roman" --number "4111..." --expiry "2028-12"
proton-cli pass items create --type wifi --name "Home" --ssid MyNet --password pw --security WPA2
proton-cli pass items create --type ssh_key --name "laptop" --public-key "ssh-ed25519 ..." \
  --private-key "$(cat id_ed25519)"
proton-cli pass items create --type identity --name "Me" --full-name "Jane Roe" --email jane@ex.com
proton-cli pass items create --type custom --name "Server" --field "Host=1.2.3.4" --hidden "Root PW=secret"
proton-cli pass items create --type login --name X --field "Recovery=abc"   # custom fields on any type
proton-cli pass items edit REF --password "new-secret"
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

</details>

<details>
<summary><b>Settings</b></summary>

```bash
proton-cli settings get                     # account settings
proton-cli settings mail                    # mail settings
proton-cli settings set                     # list the writable mail-setting keys
proton-cli settings set view-mode 1         # 0=conversations, 1=messages
proton-cli settings set draft-type text/html
proton-cli settings set hide-remote-images 1
```

</details>

<details>
<summary><b>Raw API</b></summary>

For any endpoint not covered by a high-level command:

```bash
proton-cli api GET /drive/volumes
proton-cli api POST /calendar/v1 --body '{"Name":"Work",...}'
proton-cli api GET /mail/v4/messages --query Page=0 --query PageSize=10
proton-cli api GET /calendar/v1 --output json | jq '.Calendars[].ID'
```

See [API reference](#api-reference) for the full endpoint spec.

</details>

## Scripting

proton-cli is built to compose in shell pipelines.

```bash
# stdout is the new ID on create - capture it directly
LABEL=$(proton-cli mail labels create --name Work --color "#8080FF")

# JSON + jq for full IDs and fields
proton-cli mail messages list --output json | jq -r '.messages[].id'

# exit codes drive control flow (0 ok · 3 not-found · 4 ambiguous)
if ! proton-cli contacts get "jane"; then
  echo "no unique match (exit $?)"
fi

# streaming: pipe a Drive file straight into another tool
proton-cli drive items download /report.pdf - | gpg --encrypt --recipient me ...

# preview any mutation first
proton-cli mail messages trash --unread --older-than 30d --dry-run
```

## How it works

1. **Session creation** - creates an unauthenticated session via `POST /auth/v4/sessions`.
2. **SRP authentication** - Secure Remote Password login with [go-srp](https://github.com/ProtonMail/go-srp), with 2FA/TOTP support.
3. **Session persistence** - per profile, saves the auth tokens plus the salted key password **encrypted** with a random client key held server-side. The key password is never written to disk in cleartext, and revoking the session makes the saved blob undecryptable. See [Security](#security).
4. **Key hierarchy** - unlocks User key → Address keys → per-service keys (Calendar, Drive, Contacts).
5. **End-to-end encryption** - encrypts/decrypts using [gopenpgp](https://github.com/ProtonMail/gopenpgp).
6. **Auto-refresh** - refreshes expired tokens automatically.

### Encryption details

| Service | Encrypt with | Sign with |
|---|---|---|
| Calendar events | Calendar key (session key) | Address key |
| Drive files | Node key (session key per block) | Address key |
| Drive names | Parent node key | Address key |
| Contacts | User key | User key |
| Mail | Session key | Address key |
| Pass items | AES-256-GCM (item key) | N/A (symmetric) |
| Pass vaults | AES-256-GCM (vault key) | N/A (symmetric) |

## Human verification (CAPTCHA)

Proton's anti-bot may demand a CAPTCHA at login. proton-cli opens a small webview window via an embedded helper, you solve it, and the original command retries automatically. No extra install is needed - the helper is `//go:embed`-ded into release binaries.

- **Linux desktop** needs `libwebkit2gtk-4.1` and `libgtk-3` installed.
- **macOS / Windows** need nothing (system WebKit / WebView2).
- **Headless** environments (server, container, no GUI) can't display the webview, so proton-cli exits with an error - run the command on a desktop machine instead.
- **`go install` builds** don't embed the helper. Install a [release binary](#install) if you hit a CAPTCHA.

## Security

proton-cli saves a per-profile session file at `~/.config/proton-cli/sessions/<profile>.json` (mode `0600`). The salted key password that unlocks your PGP keys is stored **encrypted** with a random 256-bit client key that lives server-side, so:

- the key password is never written to disk in cleartext, and
- revoking the session (from the Proton apps) makes a leaked copy of the file undecryptable.

The file still contains the session refresh token, so treat it as a secret. proton-cli is unaudited; see [`SECURITY.md`](SECURITY.md) for the full storage model, the vulnerability-reporting process, and hardening recommendations.

## Limitations

A few constraints are inherent to Proton's design or platform:

- **Colors** - labels, folders, calendars, and contact groups accept only Proton's 20 fixed accent colors; the CLI validates `--color` and lists the allowed values on error.
- **Calendar deletion** - `calendar calendars delete` is password-scoped and needs `PROTON_PASSWORD`.
- **CAPTCHA** - can't be solved headlessly, and `go install` builds don't embed the helper (see [Human verification](#human-verification-captcha)).

See [`docs/limitations.md`](docs/limitations.md) for the full list, including features not yet implemented.

## Development

Requires [devbox](https://www.jetify.com/devbox) and [direnv](https://direnv.net/):

```bash
direnv allow
go build .        # quick build
just lint         # gofmt + golangci-lint
just build        # release-shaped binary (embeds the CAPTCHA helper)
```

Tests are integration tests that run against the live Proton API and require `PROTON_USER` / `PROTON_PASSWORD`.

## API reference

See [`openapi.yaml`](openapi.yaml) for the complete API spec covering ~740 endpoints. To regenerate from the latest Proton source:

```bash
cd scripts && npm install && npm run generate-openapi
```

See [`scripts/README.md`](scripts/README.md) for details on the generator.

## License

MIT
