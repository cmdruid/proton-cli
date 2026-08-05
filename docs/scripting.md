# Scripting

proton-cli is designed to be a good citizen in a pipeline: data on stdout, chatter on stderr, machine-readable output on request, and exit codes that mean something. See [Concepts](concepts.md) for the rules.

## Capturing new IDs

Creating commands print the new ID to stdout and the confirmation to stderr, so a plain assignment works:

```bash
LABEL=$(proton-cli mail labels create --name Work --color "#8080FF")
VAULT=$(proton-cli pass vaults create --name Automation)
MSG=$(proton-cli mail messages send --to me@proton.me --subject Deploy --body "Done.")
```

## JSON and `jq`

```bash
# every unread subject
proton-cli mail messages list --unread --output json | jq -r '.messages[].subject'

# senders of everything older than a week, deduplicated
proton-cli mail messages search --before 2026-04-08 --limit 200 --output json \
  | jq -r '.messages[].from_address' | sort -u

# total size of a Drive folder
proton-cli drive items list /Backup --output json | jq '[.[].size] | add'

# every vault name
proton-cli pass vaults list --output json | jq -r '.[].name'
```

JSON keys are `snake_case` and IDs are always complete, never shortened.

## Exit codes as control flow

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
0 3 * * * PROTON_USER=me@proton.me PROTON_PASSWORD="$(cat ~/.proton-pw)" \
  /usr/bin/proton-cli drive items upload --recursive /var/backups /Backups >/dev/null
```

### Keep the inbox tidy

```bash
#!/usr/bin/env bash
set -euo pipefail

# archive read newsletters older than a week
proton-cli mail messages move --dest archive --from newsletter@example.com --older-than 7d

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

### Alias-per-signup

```bash
alias() {
  proton-cli pass alias create --prefix "$1" --mailbox me@proton.me
}
alias newsletter-xyz
```

## Automation notes

- **Credentials**: keep them out of your shell history. Read them from a password manager (`PROTON_PASSWORD=$(pass show proton/password)`) or a file only your user can read.
- **2FA**: `PROTON_TOTP` is only consulted during a fresh login. For unattended jobs, log in once interactively so the session file exists, then let the job reuse it.
- **CAPTCHA**: a login on a headless machine can hit human verification, which needs a desktop. Log in on a desktop first and copy the session, or run the job somewhere with a display. See [Human verification](human-verification.md).
- **`--quiet`** silences the `✓` lines and progress bars, which is what you usually want in cron.
- **Rate limits**: bulk commands page through Proton's API and respect its caps (150 messages per page). Long-running loops should sleep between iterations.
- **Search lag**: Proton's index is eventually consistent, so a just-sent message may not appear in `search` for a few seconds. Act on the ID that the command printed instead of searching again.
