# proton-cli documentation

Start with the [README](../README.md) for the tour. These pages are the details.

## Getting set up

| Page | What's in it |
| --- | --- |
| [Installation](installation.md) | Every install method per platform, checksums, updating, uninstalling, building from source |
| [Configuration](configuration.md) | Credentials, environment variables, profiles for multiple accounts, where files live |
| [Human verification](human-verification.md) | What happens when Proton asks for a CAPTCHA |

## Using it

| Page | What's in it |
| --- | --- |
| [Concepts](concepts.md) | References, short IDs, output formats, exit codes, streaming, dry runs |
| [Scripting](scripting.md) | Pipelines, JSON with `jq`, cron and systemd recipes |
| [Limitations](limitations.md) | What Proton's platform doesn't allow, and what isn't built yet |

## Command reference

| Page | Commands |
| --- | --- |
| [Mail](commands/mail.md) | `messages`, `conversations`, `attachments`, `labels`, `filters`, `addresses` |
| [Drive](commands/drive.md) | `items`, `folders`, `share`, `invitations`, `trash`, `photos` |
| [Calendar](commands/calendar.md) | `calendars`, `events` |
| [Pass](commands/pass.md) | `items`, `vaults`, `alias` |
| [Contacts](commands/contacts.md) | contacts, pinned keys, groups |
| [Settings](commands/settings.md) | account and mail settings |
| [Raw API](commands/api.md) | `api` for anything not covered by a command |

## Under the hood

| Page | What's in it |
| --- | --- |
| [How it works](how-it-works.md) | Login, the key hierarchy, what gets encrypted with which key |
| [Security](../SECURITY.md) | Session storage model, hardening, reporting a vulnerability |
| [Contributing](../CONTRIBUTING.md) | Building, linting, testing, releasing |
