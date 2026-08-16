# Mail

Read, write, send, search, and organize mail. Bodies are decrypted locally and outgoing mail is encrypted and signed with your address key, exactly like the web client.

`proton mail` is the mailbox. Everything you configure lives under [`mail settings`](#settings), one subcommand per page of Proton's own mail settings.

Anywhere a command takes `REF`, a subject or sender works as well as an ID. See [The language](../language.md).

## Messages

### List

```bash
proton mail messages list
proton mail messages list --folder archive
proton mail messages list --unread
proton mail messages list --page 1 --page-size 50
proton mail messages list --folder scheduled     # queued scheduled sends
```

Folders: `inbox`, `sent`, `drafts`, `trash`, `spam`, `archive`, `starred`, `scheduled`, `all`, or any label ID.

### Search

```bash
proton mail messages search --keyword invoice
proton mail messages search --from billing@example.com --after 2026-01-01
proton mail messages search --subject "Q1 report" --folder archive
proton mail messages search --to alice@proton.me --before 2026-04-01 --limit 100
```

`--from` and `--to` match addresses; use `--keyword` to match display names and body text too.

### Read

```bash
proton mail messages get REF                       # headers, body, attachment list
proton mail messages get --format html REF         # original HTML
proton mail messages get --format raw REF          # untouched body
proton mail messages get --body-only REF > body.txt
proton mail messages get --strip-quotes REF        # drop quoted reply blocks
proton mail messages get --include-inline REF      # list inline images too
```

Text output includes a `Sig:` line with the verdict of the signature check on the sender's key.

### Send

```bash
proton mail messages send --to alice@proton.me --subject Hi --body "Hello there"
proton mail messages send --to "Alice <alice@proton.me>" --cc b@example.com --bcc c@example.com --subject Hi --body Hello
proton mail messages send --to alice@proton.me --subject Hi --body "<b>Hi</b>" --html
proton mail messages send --to alice@proton.me --subject Report --body "See attached." --attach ./report.pdf --attach ./annex.xlsx
proton mail messages send --to alice@proton.me --subject Hi --body "<img src=cid:logo.png>" --html --attach-inline ./logo.png
echo "Deployed." | proton mail messages send --to me@proton.me --subject Deploy --body -
```

On an account with several addresses, `--from` chooses which one it leaves from:

```bash
proton mail messages send --from work@example.com --to alice@proton.me --subject Hi --body Hello
proton mail messages send --from me+shop@proton.me ...     # plus aliases work too
```

Scheduling, expiry, and password-protected mail for recipients outside Proton:

```bash
proton mail messages send --to alice@proton.me --subject Standup --body Hi --send-at 2026-05-01T09:00                 # local time; confirms the resolved time
proton mail messages send --to alice@proton.me --subject Secret --body Hi --expires 7d
proton mail messages send --to bob@gmail.com --subject Secret --body "..." --eo-password hunter2 --eo-password-hint "our usual"
```

`send` prints the new message ID on stdout, so `ID=$(proton mail messages send ...)` works.

### Reply and forward

```bash
proton mail messages reply REF --body "Thanks, paid today."
proton mail messages reply REF --all --body "Looping in the team."
proton mail messages forward REF --to alice@proton.me --body "FYI"
proton mail conversations reply REF --body "Works for me."   # newest message in a thread
```

The original is quoted below your text, the subject gains `Re:` or `Fw:` (never twice), and the thread stays a thread. A reply leaves from the address the original arrived on; a forward carries the original's attachments without re-uploading them.

```bash
proton mail messages reply REF --body Hi --no-quote          # your text only
proton mail messages forward REF --to a@b.c --no-attachments # leave them behind
proton mail messages reply REF --body Hi --draft             # stop before sending
```

`--draft` prints the new draft's ID so you can pick it up with `mail drafts update`, the same as clicking Reply in the web client and leaving the composer open. Reply and forward also take `--to/--cc/--bcc`, `--attach`, `--attach-inline`, `--html`, `--from`, `--no-signature`, `--send-at`, `--expires` and the `--eo-password` pair.

### Export

```bash
proton mail messages export REF --output invoice.eml
proton mail messages export REF --output -                  # to stdout
proton mail messages export --folder archive --older-than 1y --output-dir ./backup
proton mail messages export --folder inbox --format mbox --output inbox.mbox
proton mail conversations export REF --output thread.mbox
```

Standalone RFC 822 documents you can open in any mail client, grep, or hand to other tools. Export takes the same filters as `trash` and `move`, so archiving a whole folder is one command. `--format eml` writes one file per message named `<date> <subject>.eml`; `--format mbox` concatenates everything into one file or stream. `--no-attachments` skips attachment downloads, which is much faster for a large archive.

**Exported files are not encrypted**, and their original DKIM signatures no longer verify. The web client's export behaves the same way.

### Import

```bash
proton mail messages send --eml ./message.eml
proton mail drafts create --eml ./message.eml
proton mail messages send --eml ./message.eml --to someone-else@proton.me
```

`--eml` reads an RFC 822 file - recipients, subject, body, and attachments - and any flag you also pass overrides what the file says. Because the file is already a finished message, no signature is appended to it.

There is no way to place an old message into your archive: Proton exposes no endpoint that ingests one, for any client. Migrating a mailbox from another provider is Easy Switch, which is [not covered](../limitations.md).

### Unschedule

```bash
proton mail messages list --folder scheduled
proton mail messages unschedule REF     # back to Drafts
proton mail messages unschedule --all
```

### Organize

```bash
proton mail messages trash REF...
proton mail messages delete REF...               # permanent
proton mail messages move REF... --into archive  # it leaves where it was
proton mail messages label REF... --label Work   # it stays where it is
proton mail messages unlabel REF... --label Work
proton mail messages mark read REF...
proton mail messages mark unread REF...
proton mail messages star REF...
proton mail messages unstar REF...
```

Each of those also takes filters instead of references, and acts on everything that matches:

```bash
proton mail messages trash --unread --older-than 30d
proton mail messages move --into archive --from newsletter@example.com --older-than 7d
proton mail messages delete --folder spam --all
proton mail messages mark read --folder inbox --all
```

Filters: `--folder`, `--from`, `--to`, `--subject`, `--keyword`, `--unread`, `--starred`, `--older-than`, `--newer-than`, `--limit` (default 150, Proton's per-page cap), `--all`. Add `--dry-run` to see the list first.

## Drafts

A draft is a message, so `mail messages get`, `move` and the rest already work on one. This tree holds what only makes sense before a message goes out.

```bash
proton mail drafts create --to alice@proton.me --subject Report --body "Draft one."
proton mail drafts list
proton mail drafts update REF --body "Draft two."
proton mail drafts update REF --subject "Q1 report" --to alice@proton.me --cc bob@proton.me
proton mail drafts update REF --attach ./report.pdf --detach old-annex.xlsx
proton mail drafts send REF
proton mail drafts send REF --send-at 2026-05-01T09:00
proton mail drafts delete REF
```

`edit` replaces only what you pass. `--to`, `--cc` and `--bcc` replace the whole list; `--attach` adds a file and `--detach` removes one by name or ID. `REF` resolves within Drafts only, so editing "Report" can never reach a message you already sent.

Sending a draft delivers it exactly as stored, including whatever signature it was created with.

## Conversations

Conversations are whole threads, with the same verbs as messages.

```bash
proton mail conversations list --folder inbox --unread
proton mail conversations search --keyword invoice
proton mail conversations get REF            # every message, chronological
proton mail conversations get --summary REF  # one line per message
proton mail conversations get --strip-quotes REF
proton mail conversations reply REF --body "Works for me."
proton mail conversations export REF --output thread.mbox
proton mail conversations trash REF...
proton mail conversations move --into archive REF...
proton mail conversations mark read REF...
proton mail conversations star REF...
```

## Attachments

```bash
proton mail messages attachments list MESSAGE_ID
proton mail messages attachments list --include-inline MESSAGE_ID
proton mail messages attachments download MESSAGE_ID ATTACHMENT_ID --output ./file.pdf
proton mail messages attachments download MESSAGE_ID --all --output-dir ./attachments/
proton mail messages attachments download MESSAGE_ID ATTACHMENT_ID --output - | less
```

Existing files are never overwritten silently: names collide into `file (2).pdf`, or pass `--force`.

The same commands work across a whole thread:

```bash
proton mail conversations attachments list CONVERSATION_ID
proton mail conversations attachments download CONVERSATION_ID --all --output-dir ./thread/
```

## Settings

One subcommand per page of Proton's mail settings.

```bash
proton mail settings              # everything, at a glance
proton mail settings set          # the writable keys, grouped by page
```

### Preferences

```bash
proton mail settings set view-mode conversations
proton mail settings set page-size 100
proton mail settings set draft-type text/html
proton mail settings set hide-remote-images on
proton mail settings set delay-send 10
proton mail settings set pm-signature off
```

Every key has a fixed set of values, checked before anything is sent. Values can be given by name or by Proton's own number:

```console
$ proton mail settings set view-mode threads
Error: view-mode accepts: conversations, messages
$ proton mail settings set delay-send 999
Error: delay-send accepts 0-20 (seconds)
```

### Identity and addresses

```bash
proton mail settings addresses list
proton mail settings addresses get me@proton.me
proton mail settings addresses update me@proton.me --display-name "Roman L."
proton mail settings addresses update me@proton.me --signature "Roman | Vienna"
proton mail settings addresses update me@proton.me --signature - < signature.html --html
proton mail settings addresses update me@proton.me --clear-signature
```

**Your signature is applied to outgoing mail automatically**, as in the web client: the address's own signature, plus Proton's *"Sent with Proton Mail secure email."* footer when your account has it enabled. Free accounts have that footer forced on and cannot switch it off.

Pass `--no-signature` on any sending command to leave both out, or turn the footer off account-wide with `proton mail settings set pm-signature off` (paid plans only). `--eml` and `mail drafts send` never append anything, since in both cases the body is already final.

Proton stores signatures as HTML. Plain text is escaped and its newlines become line breaks; `--html` passes markup through untouched.

### Folders and labels

```bash
proton mail settings labels list
proton mail settings labels create --name Important --color "#8080FF"
proton mail settings folders create --name Projects
proton mail settings folders create --name Clients --parent PARENT_FOLDER_ID
proton mail settings labels update LABEL_ID --name Renamed --color "#DB60D6"
proton mail settings labels delete Important        # by name, or by label ID
```

Deleting a folder or label names it and asks first; the messages it held are not deleted. See [When it asks first](../language.md#when-it-asks-first).

Colors have to be one of Proton's accent colors; an invalid value prints the whole palette, and is refused before anything is sent.

A message lives in exactly one folder and carries any number of labels. `move --into` takes what `folders list` shows; `label --label` takes what `labels list` shows.

### Filters

Server-side [Sieve](https://en.wikipedia.org/wiki/Sieve_(mail_filtering_language)) filters, the same ones the web client creates.

```bash
proton mail settings filters list
proton mail settings filters create --name "Archive invoices" --sieve 'require ["fileinto"]; if header :contains "Subject" "invoice" { fileinto "Archive"; }'
proton mail settings filters create --name Big --sieve - < filter.sieve
proton mail settings filters update FILTER_ID --name "New name"
proton mail settings filters disable "Archive invoices"   # by name, or by filter ID
proton mail settings filters enable "Archive invoices"
proton mail settings filters delete "Archive invoices"
```

A Sieve script is not recoverable once deleted, so `delete` names the filter and asks first.

### Auto-reply

```bash
proton mail settings autoreply get                    # current schedule and message
proton mail settings autoreply set --repeat fixed --start 2026-07-01T09:00 --end 2026-07-14T18:00 --message "I'm away until the 14th."
proton mail settings autoreply disable
proton mail settings autoreply enable
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

Saving a schedule turns the auto-reply on; `disable` switches it off and keeps the schedule for later. Proton sends every auto-reply with the subject `Auto` and offers no way to change it, so neither does proton. Auto-reply is a paid feature.
