# Scripting

Data goes to stdout, everything else to stderr, and exit codes say what went wrong. See [Output](output.md) for the details.

## Capturing new IDs

Commands that create something print the new ID to stdout, so a plain assignment works:

```bash
LABEL=$(proton-cli mail settings labels create --name Work --color "#8080FF")
VAULT=$(proton-cli pass vaults create --name Automation)
MSG=$(proton-cli mail messages send --to me@proton.me --subject Deploy --body "Done.")
```

## JSON and `jq`

```bash
# every unread subject
proton-cli mail messages list --unread --output json | jq -r '.messages[].subject'

# senders of everything older than a week, deduplicated
proton-cli mail messages search --before 2026-04-08 --limit 200 --output json | jq -r '.messages[].from_address' | sort -u

# total size of a Drive folder
proton-cli drive items list /Backup --output json | jq '[.items[].size] | add'

# every vault name
proton-cli pass vaults list --output json | jq -r '.vaults[].name'
```

Every list is an object keyed by its plural name, always with a `count`:

```bash
proton-cli mail messages list --output json | jq '.count'
proton-cli drive items list /Backup --output json | jq -r '.items[].name'
proton-cli pass vaults list --output json | jq -r '.vaults[].name'
```

Keys are `snake_case`, IDs are always complete, and enumerated values are names rather than numbers: `"type": "file"`, not `"type": 2`.

## Archiving mail to disk

`export` writes ordinary RFC 822 `.eml` files.

```bash
# a year of archive, one .eml per message, named "<date> <subject>.eml"
proton-cli mail messages export --folder archive --older-than 1y --all --output-dir ./mail-backup

# a whole folder as a single mbox, ready for Thunderbird or mutt
proton-cli mail messages export --folder inbox --all --format mbox --output inbox.mbox

# one message straight into another tool
proton-cli mail messages export "Invoice #2291" --output - | formail -X ""

# metadata and bodies only, skipping attachment downloads - much faster
proton-cli mail messages export --folder all --all --no-attachments --output-dir ./index
```

Exported files are not encrypted, so put them somewhere you would be comfortable putting the mail itself.

The reverse direction reads a file back into a draft or a send:

```bash
proton-cli mail drafts create --eml ./message.eml
proton-cli mail messages send --eml ./message.eml --to someone-else@proton.me
```

## Answering mail from a script

```bash
# acknowledge everything unread from a sender, then archive it
proton-cli mail messages search --from alerts@example.com --unread --output json | jq -r '.messages[].id' | while read -r id; do
      proton-cli mail messages reply "$id" --body "Received, thanks." --no-signature
      proton-cli mail messages move "$id" --into archive
    done
```

## Exit codes as control flow

A mutation reports itself structurally too, which is easier to check than parsing a sentence:

```bash
proton-cli mail messages trash --older-than 1y --output json | jq '.count'
```

```bash
if proton-cli pass items get "deploy-key" >/dev/null 2>&1; then
  echo "secret exists"
fi

proton-cli contacts get jane
case $? in
  0) echo "found" ;;
  3) echo "no such contact" ;;
  4) echo "ambiguous, be more specific" ;;
esac
```

## Streaming instead of temporary files

```bash
# back up a database straight into Drive
pg_dump mydb | gzip | proton-cli drive items upload - /Backups/db.sql.gz

# restore it without landing on disk
proton-cli drive items download /Backups/db.sql.gz --output - | gunzip | psql mydb

# mail a report generated on the fly
generate-report | proton-cli mail messages send --to team@example.com --subject "Nightly report" --body -

# encrypt something else with your own tooling on the way out
proton-cli drive items download /report.pdf --output - | gpg --encrypt --recipient me > report.pdf.gpg
```

## Recipes

### Nightly backup (cron)

```cron
0 3 * * * PROTON_USER=me@proton.me PROTON_PASSWORD="$(cat ~/.proton-pw)" /usr/bin/proton-cli drive items upload --recursive /var/backups /Backups >/dev/null
```

### Keep the inbox tidy

```bash
#!/usr/bin/env bash
set -euo pipefail

# archive read newsletters older than a week
proton-cli mail messages move --into archive --from newsletter@example.com --older-than 7d

# bin anything left in spam after a month
proton-cli mail messages delete --folder spam --older-than 30d
```

Run it once with `--dry-run` appended to each command before trusting it.

### Systemd timer

```ini
# ~/.config/systemd/user/proton-backup.service
[Service]
Type=oneshot
Environment=PROTON_USER=me@proton.me
EnvironmentFile=%h/.config/proton-cli/credentials
ExecStart=/usr/bin/proton-cli drive items upload --recursive %h/Documents /Backups
```

```ini
# ~/.config/systemd/user/proton-backup.timer
[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

### Out of office

```bash
proton-cli mail settings autoreply set --repeat fixed --start "$(date -d 'next monday 09:00' +%Y-%m-%dT%H:%M)" --end "$(date -d 'next friday 18:00' +%Y-%m-%dT%H:%M)" --message "Away this week. For anything urgent, contact team@example.com."

# and when you are back
proton-cli mail settings autoreply disable
```

### Alias-per-signup

```bash
alias() {
  proton-cli pass aliases create --prefix "$1" --mailbox me@proton.me
}
alias newsletter-xyz
```

## Automation notes

- **Credentials**: keep them out of your shell history. Read them from a password manager (`PROTON_PASSWORD=$(pass show proton/password)`) or a file only your user can read.
- **2FA**: `PROTON_TOTP` is only consulted during a fresh login. For unattended jobs, log in once interactively so the session file exists, then let the job reuse it.
- **CAPTCHA**: a login on a headless machine can hit human verification, which needs a desktop. Log in on a desktop first and copy the session, or run the job somewhere with a display. See [Human verification](human-verification.md).
- **`--quiet`** silences the `✓` lines and progress bars, useful in cron.
- **Rate limits**: bulk commands page through Proton's API and respect its caps (150 messages per page). Long-running loops should sleep between iterations.
- **Search lag**: Proton's index is eventually consistent, so a just-sent message may not appear in `search` for a few seconds. Act on the ID that the command printed instead of searching again.
