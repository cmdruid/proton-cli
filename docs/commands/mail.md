# Mail

Read, write, send, search, and organize mail. Bodies are decrypted locally and outgoing mail is encrypted and signed with your address key, exactly like the web client.

`proton-cli mail` is the mailbox. Everything you configure lives under [`mail settings`](#settings), one subcommand per page of Proton's own mail settings.

Anywhere a command takes `REF`, a subject or sender works as well as an ID. See [The language](../language.md).

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
proton-cli mail messages get REF                       # headers, body, attachment list
proton-cli mail messages get --format html REF         # original HTML
proton-cli mail messages get --format raw REF          # untouched body
proton-cli mail messages get --body-only REF > body.txt
proton-cli mail messages get --strip-quotes REF        # drop quoted reply blocks
proton-cli mail messages get --include-inline REF      # list inline images too
```

Text output includes a `Sig:` line with the verdict of the signature check on the sender's key.

### Send

```bash
proton-cli mail messages send --to alice@proton.me --subject Hi --body "Hello there"
proton-cli mail messages send --to "Alice <alice@proton.me>" --cc b@example.com --bcc c@example.com --subject Hi --body Hello
proton-cli mail messages send --to alice@proton.me --subject Hi --body "<b>Hi</b>" --html
proton-cli mail messages send --to alice@proton.me --subject Report --body "See attached." --attach ./report.pdf --attach ./annex.xlsx
proton-cli mail messages send --to alice@proton.me --subject Hi --body "<img src=cid:logo.png>" --html --attach-inline ./logo.png
echo "Deployed." | proton-cli mail messages send --to me@proton.me --subject Deploy --body -
```

On an account with several addresses, `--from` chooses which one it leaves from:

```bash
proton-cli mail messages send --from work@example.com --to alice@proton.me --subject Hi --body Hello
proton-cli mail messages send --from me+shop@proton.me ...     # plus aliases work too
```

Scheduling, expiry, and password-protected mail for recipients outside Proton:

```bash
proton-cli mail messages send --to alice@proton.me --subject Standup --body Hi --send-at 2026-05-01T09:00                 # local time; confirms the resolved time
proton-cli mail messages send --to alice@proton.me --subject Secret --body Hi --expires 7d
proton-cli mail messages send --to bob@gmail.com --subject Secret --body "..." --eo-password hunter2 --eo-password-hint "our usual"
```

`send` prints the new message ID on stdout, so `ID=$(proton-cli mail messages send ...)` works.

### Reply and forward

```bash
proton-cli mail messages reply REF --body "Thanks, paid today."
proton-cli mail messages reply REF --all --body "Looping in the team."
proton-cli mail messages forward REF --to alice@proton.me --body "FYI"
proton-cli mail conversations reply REF --body "Works for me."   # newest message in a thread
```

The original is quoted below your text, the subject gains `Re:` or `Fw:` (never twice), and the thread stays a thread. A reply leaves from the address the original arrived on; a forward carries the original's attachments without re-uploading them.

```bash
proton-cli mail messages reply REF --body Hi --no-quote          # your text only
proton-cli mail messages forward REF --to a@b.c --no-attachments # leave them behind
proton-cli mail messages reply REF --body Hi --draft             # stop before sending
```

`--draft` prints the new draft's ID so you can pick it up with `mail drafts update`, the same as clicking Reply in the web client and leaving the composer open. Reply and forward also take `--to/--cc/--bcc`, `--attach`, `--attach-inline`, `--html`, `--from`, `--no-signature`, `--send-at`, `--expires` and the `--eo-password` pair.

### Export

```bash
proton-cli mail messages export REF --output invoice.eml
proton-cli mail messages export REF --output -                  # to stdout
proton-cli mail messages export --folder archive --older-than 1y --output-dir ./backup
proton-cli mail messages export --folder inbox --format mbox --output inbox.mbox
proton-cli mail conversations export REF --output thread.mbox
```

Standalone RFC 822 documents you can open in any mail client, grep, or hand to other tools. Export takes the same filters as `trash` and `move`, so archiving a whole folder is one command. `--format eml` writes one file per message named `<date> <subject>.eml`; `--format mbox` concatenates everything into one file or stream. `--no-attachments` skips attachment downloads, which is much faster for a large archive.

**Exported files are not encrypted**, and their original DKIM signatures no longer verify. The web client's export behaves the same way.

### Import

```bash
proton-cli mail messages send --eml ./message.eml
proton-cli mail drafts create --eml ./message.eml
proton-cli mail messages send --eml ./message.eml --to someone-else@proton.me
```

`--eml` reads an RFC 822 file - recipients, subject, body, and attachments - and any flag you also pass overrides what the file says. Because the file is already a finished message, no signature is appended to it.

There is no way to place an old message into your archive: Proton exposes no endpoint that ingests one, for any client. Migrating a mailbox from another provider is Easy Switch, which is [not covered](../limitations.md).

### Unschedule

```bash
proton-cli mail messages list --folder scheduled
proton-cli mail messages unschedule REF     # back to Drafts
proton-cli mail messages unschedule --all
```

### Organize

```bash
proton-cli mail messages trash REF...
proton-cli mail messages delete REF...               # permanent
proton-cli mail messages move REF... --into archive  # it leaves where it was
proton-cli mail messages label REF... --label Work   # it stays where it is
proton-cli mail messages unlabel REF... --label Work
proton-cli mail messages mark read REF...
proton-cli mail messages mark unread REF...
proton-cli mail messages star REF...
proton-cli mail messages unstar REF...
```

Each of those also takes filters instead of references, and acts on everything that matches:

```bash
proton-cli mail messages trash --unread --older-than 30d
proton-cli mail messages move --into archive --from newsletter@example.com --older-than 7d
proton-cli mail messages delete --folder spam --all
proton-cli mail messages mark read --folder inbox --all
```

Filters: `--folder`, `--from`, `--to`, `--subject`, `--keyword`, `--unread`, `--starred`, `--older-than`, `--newer-than`, `--limit` (default 150, Proton's per-page cap), `--all`. Add `--dry-run` to see the list first.

## Drafts

A draft is a message, so `mail messages get`, `move` and the rest already work on one. This tree holds what only makes sense before a message goes out.

```bash
proton-cli mail drafts create --to alice@proton.me --subject Report --body "Draft one."
proton-cli mail drafts list
proton-cli mail drafts update REF --body "Draft two."
proton-cli mail drafts update REF --subject "Q1 report" --to alice@proton.me --cc bob@proton.me
proton-cli mail drafts update REF --attach ./report.pdf --detach old-annex.xlsx
proton-cli mail drafts send REF
proton-cli mail drafts send REF --send-at 2026-05-01T09:00
proton-cli mail drafts delete REF
```

`edit` replaces only what you pass. `--to`, `--cc` and `--bcc` replace the whole list; `--attach` adds a file and `--detach` removes one by name or ID. `REF` resolves within Drafts only, so editing "Report" can never reach a message you already sent.

Sending a draft delivers it exactly as stored, including whatever signature it was created with.

## Conversations

Conversations are whole threads, with the same verbs as messages.

```bash
proton-cli mail conversations list --folder inbox --unread
proton-cli mail conversations search --keyword invoice
proton-cli mail conversations get REF            # every message, chronological
proton-cli mail conversations get --summary REF  # one line per message
proton-cli mail conversations get --strip-quotes REF
proton-cli mail conversations reply REF --body "Works for me."
proton-cli mail conversations export REF --output thread.mbox
proton-cli mail conversations trash REF...
proton-cli mail conversations move --into archive REF...
proton-cli mail conversations mark read REF...
proton-cli mail conversations star REF...
```

## Attachments

```bash
proton-cli mail messages attachments list MESSAGE_ID
proton-cli mail messages attachments list --include-inline MESSAGE_ID
proton-cli mail messages attachments download MESSAGE_ID ATTACHMENT_ID --output ./file.pdf
proton-cli mail messages attachments download MESSAGE_ID --all --output-dir ./attachments/
proton-cli mail messages attachments download MESSAGE_ID ATTACHMENT_ID --output - | less
```

Existing files are never overwritten silently: names collide into `file (2).pdf`, or pass `--force`.

The same commands work across a whole thread:

```bash
proton-cli mail conversations attachments list CONVERSATION_ID
proton-cli mail conversations attachments download CONVERSATION_ID --all --output-dir ./thread/
```

## Settings

One subcommand per page of Proton's mail settings.

```bash
proton-cli mail settings              # everything, at a glance
proton-cli mail settings set          # the writable keys, grouped by page
```

### Preferences

```bash
proton-cli mail settings set view-mode conversations
proton-cli mail settings set page-size 100
proton-cli mail settings set draft-type text/html
proton-cli mail settings set hide-remote-images on
proton-cli mail settings set delay-send 10
proton-cli mail settings set pm-signature off
```

Every key has a fixed set of values, checked before anything is sent. Values can be given by name or by Proton's own number:

```console
$ proton-cli mail settings set view-mode threads
Error: view-mode accepts: conversations, messages
$ proton-cli mail settings set delay-send 999
Error: delay-send accepts 0-20 (seconds)
```

### Identity and addresses

```bash
proton-cli mail settings addresses list
proton-cli mail settings addresses get me@proton.me
proton-cli mail settings addresses update me@proton.me --display-name "Roman L."
proton-cli mail settings addresses update me@proton.me --signature "Roman | Vienna"
proton-cli mail settings addresses update me@proton.me --signature - < signature.html --html
proton-cli mail settings addresses update me@proton.me --clear-signature
```

**Your signature is applied to outgoing mail automatically**, as in the web client: the address's own signature, plus Proton's *"Sent with Proton Mail secure email."* footer when your account has it enabled. Free accounts have that footer forced on and cannot switch it off.

Pass `--no-signature` on any sending command to leave both out, or turn the footer off account-wide with `proton-cli mail settings set pm-signature off` (paid plans only). `--eml` and `mail drafts send` never append anything, since in both cases the body is already final.

Proton stores signatures as HTML. Plain text is escaped and its newlines become line breaks; `--html` passes markup through untouched.

### Folders and labels

```bash
proton-cli mail settings labels list
proton-cli mail settings labels create --name Important --color "#8080FF"
proton-cli mail settings folders create --name Projects
proton-cli mail settings folders create --name Clients --parent PARENT_FOLDER_ID
proton-cli mail settings labels update LABEL_ID --name Renamed --color "#DB60D6"
proton-cli mail settings labels delete Important        # by name, or by label ID
```

Deleting a folder or label names it and asks first; the messages it held are not deleted. See [When it asks first](../language.md#when-it-asks-first).

Colors have to be one of Proton's accent colors; an invalid value prints the whole palette, and is refused before anything is sent.

A message lives in exactly one folder and carries any number of labels. `move --into` takes what `folders list` shows; `label --label` takes what `labels list` shows.

### Filters

Server-side [Sieve](https://en.wikipedia.org/wiki/Sieve_(mail_filtering_language)) filters, the same ones the web client creates.

```bash
proton-cli mail settings filters list
proton-cli mail settings filters create --name "Archive invoices" --sieve 'require ["fileinto"]; if header :contains "Subject" "invoice" { fileinto "Archive"; }'
proton-cli mail settings filters create --name Big --sieve - < filter.sieve
proton-cli mail settings filters update FILTER_ID --name "New name"
proton-cli mail settings filters disable "Archive invoices"   # by name, or by filter ID
proton-cli mail settings filters enable "Archive invoices"
proton-cli mail settings filters delete "Archive invoices"
```

A Sieve script is not recoverable once deleted, so `delete` names the filter and asks first.

### Auto-reply

```bash
proton-cli mail settings autoreply get                    # current schedule and message
proton-cli mail settings autoreply set --repeat fixed --start 2026-07-01T09:00 --end 2026-07-14T18:00 --message "I'm away until the 14th."
proton-cli mail settings autoreply disable
proton-cli mail settings autoreply enable
```

`--start` and `--end` are written in the grammar the repeat mode dictates:

| `--repeat` | `--start` / `--end` | Also takes |
| --- | --- | --- |
| `fixed` | `2026-07-01T09:00` - a date and time | `--zone` |
| `daily` | `09:00` - a time of day | `--days mon,tue,wed`, `--zone` |
| `weekly` | `mon:09:00` - a weekday and time | `--zone` |
| `monthly` | `1:09:00` - a day of the month and time | `--zone` |
| `permanent` | *not used* | |

`--zone` is any IANA name and defaults to your system's. `--message` takes `-` for stdin and is stored as HTML, so plain text is escaped with its newlines turned into line breaks unless you pass `--html`.

Saving a schedule turns the auto-reply on; `disable` switches it off and keeps the schedule for later. Proton sends every auto-reply with the subject `Auto` and offers no way to change it, so neither does proton-cli. Auto-reply is a paid feature.
