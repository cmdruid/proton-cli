# Mail

Read, send, search, and organize mail. Bodies are decrypted locally and outgoing mail is encrypted and signed with your address key, exactly like the web client.

Anywhere a command takes `REF`, a subject or sender works as well as an ID. See [Concepts](../concepts.md).

## Messages

### List

```bash
proton-cli mail messages list
proton-cli mail messages list --folder archive
proton-cli mail messages list --unread
proton-cli mail messages list --page 1 --page-size 50
proton-cli mail messages list --folder scheduled     # queued scheduled sends
```

Folders: `inbox`, `sent`, `drafts`, `trash`, `spam`, `archive`, `starred`, `scheduled`, `all`, or any label ID.

### Search

```bash
proton-cli mail messages search --keyword invoice
proton-cli mail messages search --from billing@example.com --after 2026-01-01
proton-cli mail messages search --subject "Q1 report" --folder archive
proton-cli mail messages search --to alice@proton.me --before 2026-04-01 --limit 100
```

`--from` and `--to` match addresses; use `--keyword` to match display names and body text too.

### Read

```bash
proton-cli mail messages read REF                       # headers, body, attachment list
proton-cli mail messages read --format html REF         # original HTML
proton-cli mail messages read --format raw REF          # untouched body
proton-cli mail messages read --body-only REF > body.txt
proton-cli mail messages read --strip-quotes REF        # drop quoted reply blocks
proton-cli mail messages read --include-inline REF      # list inline images too
```

Text output includes a `Sig:` line with the verdict of the signature check on the sender's key.

### Send

```bash
proton-cli mail messages send --to alice@proton.me --subject Hi --body "Hello there"
proton-cli mail messages send --to a@example.com --cc b@example.com --bcc c@example.com \
  --subject Hi --body Hello
proton-cli mail messages send --to alice@proton.me --subject Hi --body "<b>Hi</b>" --html
proton-cli mail messages send --to alice@proton.me --subject Report --body "See attached." \
  --attach ./report.pdf --attach ./annex.xlsx
proton-cli mail messages send --to alice@proton.me --subject Hi --body "<img src=cid:logo.png>" \
  --html --attach-inline ./logo.png
echo "Deployed." | proton-cli mail messages send --to me@proton.me --subject Deploy --body -
```

Scheduling, expiry, and password-protected mail for recipients outside Proton:

```bash
proton-cli mail messages send --to alice@proton.me --subject Standup --body Hi \
  --send-at 2026-05-01T09:00                 # local time; confirms the resolved time
proton-cli mail messages send --to alice@proton.me --subject Secret --body Hi --expires 7d
proton-cli mail messages send --to bob@gmail.com --subject Secret --body "..." \
  --eo-password hunter2 --eo-password-hint "our usual"
```

`send` prints the new message ID on stdout, so `ID=$(proton-cli mail messages send ...)` works.

### Unschedule

```bash
proton-cli mail messages list --folder scheduled
proton-cli mail messages unschedule REF     # back to Drafts
proton-cli mail messages unschedule --all
```

### Organize

```bash
proton-cli mail messages trash REF...
proton-cli mail messages delete REF...              # permanent
proton-cli mail messages move --dest archive REF...
proton-cli mail messages mark read REF...
proton-cli mail messages mark unread REF...
proton-cli mail messages star REF...
proton-cli mail messages unstar REF...
```

Each of those also takes filters instead of references, and acts on everything that matches:

```bash
proton-cli mail messages trash --unread --older-than 30d
proton-cli mail messages move --dest archive --from newsletter@example.com --older-than 7d
proton-cli mail messages delete --folder spam --all
proton-cli mail messages mark read --folder inbox --all
```

Filters: `--folder`, `--from`, `--to`, `--subject`, `--keyword`, `--unread`, `--older-than`, `--newer-than`, `--limit` (default 150, Proton's per-page cap), `--all`. Add `--dry-run` to see the list first.

## Conversations

Conversations are whole threads, with the same verbs as messages.

```bash
proton-cli mail conversations list --folder inbox --unread
proton-cli mail conversations search --keyword invoice
proton-cli mail conversations read REF                 # every message, chronological
proton-cli mail conversations read --summary REF        # one line per message
proton-cli mail conversations read --strip-quotes REF
proton-cli mail conversations trash REF...
proton-cli mail conversations move --dest archive REF...
proton-cli mail conversations mark read REF...
proton-cli mail conversations star REF...
```

## Attachments

```bash
proton-cli mail attachments list MESSAGE_ID
proton-cli mail attachments list --include-inline MESSAGE_ID
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID --output ./file.pdf
proton-cli mail attachments download MESSAGE_ID --all --output-dir ./attachments/
proton-cli mail attachments download MESSAGE_ID ATTACHMENT_ID --output - | less
```

Existing files are never overwritten silently: names collide into `file (2).pdf`, or pass `--force`.

The same commands work across a whole thread:

```bash
proton-cli mail conversations attachments list CONVERSATION_ID
proton-cli mail conversations attachments download CONVERSATION_ID --all --output-dir ./thread/
```

## Labels and folders

```bash
proton-cli mail labels list
proton-cli mail labels create --name Important --color "#8080FF"
proton-cli mail labels create --name Projects --folder
proton-cli mail labels create --name Clients --folder --parent PARENT_FOLDER_ID
proton-cli mail labels update LABEL_ID --name Renamed --color "#DB60D6"
proton-cli mail labels delete LABEL_ID...
```

Colors have to be one of Proton's accent colors; an invalid value prints the allowed list.

## Filters

Server-side [Sieve](https://en.wikipedia.org/wiki/Sieve_(mail_filtering_language)) filters, the same ones the web client creates.

```bash
proton-cli mail filters list
proton-cli mail filters create --name "Archive invoices" \
  --sieve 'require ["fileinto"]; if header :contains "Subject" "invoice" { fileinto "Archive"; }'
proton-cli mail filters update FILTER_ID --name "New name"
proton-cli mail filters disable FILTER_ID
proton-cli mail filters enable FILTER_ID
proton-cli mail filters delete FILTER_ID
```

## Addresses

```bash
proton-cli mail addresses list
```
