<div align="center">

<img src="assets/logo.svg" width="96" height="96" alt="">

# proton-cli

**Proton, in your terminal.**

_Unofficial, community-built, not affiliated with Proton AG._

[![Release](https://img.shields.io/github/v/release/roman-16/proton-cli?sort=semver&style=flat-square&color=6D4AFF)](https://github.com/roman-16/proton-cli/releases/latest) [![Downloads](https://img.shields.io/github/downloads/roman-16/proton-cli/total?style=flat-square&color=6D4AFF)](https://github.com/roman-16/proton-cli/releases) [![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20Windows-6D4AFF?style=flat-square)](docs/installation.md) [![License](https://img.shields.io/github/license/roman-16/proton-cli?style=flat-square&color=6D4AFF)](LICENSE)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/demo-dark.svg">
  <img src="assets/demo-light.svg" alt="Listing unread mail, uploading a file to Drive, previewing which files a cleanup would remove, and listing Pass items">
</picture>

</div>
<br />

Read your mail, move files in and out of Drive, manage calendars, passwords, and contacts, all without opening a browser. proton logs in the way the Proton apps do and does the encryption on your machine, so your keys stay yours.

- **Real end-to-end encryption.** SRP login and the full PGP key hierarchy, handled locally with Proton's own [go-srp](https://github.com/ProtonMail/go-srp) and [gopenpgp](https://github.com/ProtonMail/gopenpgp). No bridge, no proxy, no browser in the middle.
- **Five apps, one binary.** Mail, Drive, Calendar, Pass, and Contacts, in a single static executable on Linux, macOS, and Windows.
- **Built for pipes and cron.** JSON and YAML with one envelope shape for every list, streaming stdin and stdout, exit codes that mean something, and `--dry-run` on everything that changes state, showing the rows it would touch.

## Install

| Method | Command |
| --- | --- |
| **Linux, macOS** | `curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh \| sh` |
| **Windows** | `irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1 \| iex` |
| **Homebrew** | `brew install --cask roman-16/tap/proton-cli` |
| **winget** | `winget install Roman-16.ProtonCLI` |
| **Arch (AUR)** | `yay -S proton-cli-bin` |
| **Nix** | `pkgs.proton-cli` |
| **npm** | `npm install -g @roman-16/proton-cli` |

**Debian, Ubuntu, Linux Mint** - the APT repository, so it updates with the rest of your system:

```bash
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://roman-16.github.io/proton-cli/gpg.key | sudo tee /etc/apt/keyrings/proton-cli.asc >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/proton-cli.asc] https://roman-16.github.io/proton-cli stable main" | sudo tee /etc/apt/sources.list.d/proton-cli.list
sudo apt update && sudo apt install proton-cli
```

There are also `.rpm` and `.apk` packages, plain binaries with checksums, and `go install`. See [Installation](docs/installation.md).

The command is `proton`, and `proton-cli` works too - every install puts both names on your `PATH`.

Already installed? `proton update`, and `proton changelog` for what that brings. An install no package manager owns says so itself, once a day.

## Get started

```console
$ proton account login
Email:            you@proton.me
Password:
Two-factor code:  123456

✓ Signed in as you@proton.me.
```

That's the whole setup. Signing in saves the session **and** unlocks your keys, so your password is needed once on this machine and not again. Every command documents itself with `--help`:

```bash
proton mail messages list
proton mail messages send --help
```

Scripting it? `proton account login --user you@proton.me --password-file /run/secrets/proton` needs no terminal. Juggling a personal and a work account? `proton account login --profile work`. More in [Getting started](docs/getting-started.md).

## What you can do

Every command reads the same way - `proton <app> <collection> <verb>` - and anywhere one wants an ID, a subject, name, or URL works too. Lists shorten IDs to 8 characters you can paste straight back. See [The language](docs/language.md).

### Mail

```bash
proton mail messages list --unread
proton mail messages get "Invoice #2291"
proton mail messages send --to alice@proton.me --subject Report --body "See attached." --attach ./report.pdf
proton mail messages reply "Invoice #2291" --body "Thanks, paid today."
```

Threads, attachments, filters, snoozing, block and allow lists, and auto-reply. → [Mail](docs/commands/mail.md)

### Drive

```bash
proton drive items list /Documents
proton drive items upload --recursive ./project /Backup
proton drive items download /Documents/report.pdf --output-dir .
proton drive items share link /Documents/report.pdf --expires 7d
```

Revisions, sharing with people, what others shared with you, and photo albums. → [Drive](docs/commands/drive.md)

### Calendar

```bash
proton calendar events list --start 2026-04-15 --end 2026-04-30
proton calendar events create --title Standup --start 2026-04-16T09:00 --duration 15m --rrule "FREQ=WEEKLY;COUNT=10" --remind 15m
proton calendar events respond "Team sync" --answer accept
proton calendar events export --start 2026-01-01 --end 2026-12-31 --output year.ics
```

Recurring events occurrence by occurrence, your own calendars, all-day events, attendees, and .ics in and out. → [Calendar](docs/commands/calendar.md)

### Pass

```bash
proton pass items list --vault Work
proton pass items get github.com
proton pass items create --name GitHub --username roman --url github.com --password "$(proton pass generate)"
proton pass items totp github.com
```

Notes, cards, SSH keys, identities, two-factor codes, item history, and backups Proton Pass itself can read. → [Pass](docs/commands/pass.md)

### Contacts

```bash
proton contacts list
proton contacts get jane
proton contacts create --name "Jane Roe" --email jane@example.com
proton contacts export --output-dir ./address-book
```

Typed addresses and phones, the full vCard field set, duplicate merging, and vCard import/export. → [Contacts](docs/commands/contacts.md)

### Account

```bash
proton account get
proton account sessions list
proton account settings set locale de_AT
proton account logout --revoke
```

Profiles and per-app settings. → [Account](docs/commands/account.md)

[`proton api`](docs/commands/api.md) reaches any endpoint the commands don't.

## Automate it

```bash
# creating something prints its new ID to stdout
ID=$(proton mail settings labels create --name Work --color purple)

# every list is an envelope keyed by its plural name, always with a count
proton mail messages list --unread --output json | jq -r '.messages[].subject'
proton drive items list /Backup --output json | jq '[.items[].size] | add'

# stream through, no temporary files
pg_dump mydb | gzip | proton drive items upload - /Backups/db.sql.gz

# archive a folder to disk as ordinary .eml files
proton mail messages export --folder archive --all --output-dir ./mail-backup

# be told when things happen: arrivals and calendar reminders, live
proton mail messages watch --output json | jq --unbuffered -r '[.from_name,.subject]|@tsv'

# check what a bulk change would touch before it happens
proton mail messages trash --from newsletter@example.com --older-than 90d --dry-run
```

Data goes to stdout and progress to stderr, so redirects stay clean. Exit codes tell user error, auth failure, not-found, ambiguity, and network trouble apart, so scripts can react to each. → [Scripting](docs/scripting.md)

Anything that removes permanently, or that removes what a filter picked out rather than what you named, shows the rows and asks first. Off a terminal it refuses instead, so an unattended run fails safe until you add `--yes`. → [When it asks first](docs/language.md#when-it-asks-first)

## Encryption you can verify

Your password never reaches Proton: login is SRP, and the key password it derives stays local and unlocks your PGP keys in memory. Mail, files, events, contacts, and Pass items are decrypted after they arrive and encrypted before they leave, with the same key hierarchy the web clients use. Signatures on incoming mail are checked and reported.

The saved session keeps your key password encrypted with a key held server-side, so revoking the session from any Proton app makes a leaked copy of the file useless. proton is unaudited, and the whole storage model is written down in [Security](SECURITY.md). The mechanics are in [How it works](docs/how-it-works.md).

## Documentation

Everything lives in [`docs/`](docs/README.md):

| Page | What's in it |
| --- | --- |
| [Installation](docs/installation.md) | Every platform, updating, the update notice, uninstalling |
| [Configuration](docs/configuration.md) | Credentials, profiles, environment variables |
| [Getting started](docs/getting-started.md) | Signing in, completion, profiles |
| [The language](docs/language.md) | The grammar: apps, collections, verbs, filters |
| [Output](docs/output.md) | The four response kinds, JSON, colour, exit codes |
| [References](docs/references.md) | Names, short IDs, compound IDs |
| **Command reference** | [Everything](docs/commands/README.md) · [Mail](docs/commands/mail.md) · [Drive](docs/commands/drive.md) · [Calendar](docs/commands/calendar.md) · [Pass](docs/commands/pass.md) · [Contacts](docs/commands/contacts.md) · [Account](docs/commands/account.md) · [API](docs/commands/api.md) |
| [Scripting](docs/scripting.md) | Pipelines, `jq`, cron and systemd |
| [How it works](docs/how-it-works.md) | Login, keys, what's encrypted with what |
| [Limitations](docs/limitations.md) | Platform constraints and gaps |

[`CHANGELOG.md`](CHANGELOG.md) records what each version changed, and `proton changelog` prints it.

## Good to know

- **Search lags a few seconds.** Proton's index is eventually consistent, so act on the ID a command printed rather than searching for the same subject again.
- **CAPTCHAs need a desktop.** Proton occasionally asks for human verification at login, which opens a small window. On a headless machine, log in elsewhere and copy the session. See [Human verification](docs/human-verification.md).
- **Label colors are Proton's; the interface's colors are yours.** Labels, folders, calendars, and groups accept only the 20 accent colors, by name (`--color purple`) or hex (`--color "#8080FF"`); an invalid `--color` prints the whole palette, and lists show the swatch and the name rather than a hex code. Everything proton colors on its own account it asks for by name, so the shades come from your terminal theme rather than from here.
- **Folders and labels are different.** A message lives in one folder and carries any number of labels, so `move --into` and `label --label` are separate verbs.

## Contributing

Bug reports, ideas, and pull requests are all welcome. [`CONTRIBUTING.md`](CONTRIBUTING.md) covers the setup, and [`SECURITY.md`](SECURITY.md) has the private channel for security issues.

## Disclaimer

proton is an independent, community-built project. It is not endorsed by, affiliated with, or supported by Proton AG. Proton is a trademark of Proton AG. Use it at your own risk, and mind Proton's terms of service.

## License

[MIT](LICENSE)
