# Output

Everything proton-cli prints is one of four things, and each looks the same wherever it appears.

## stdout is the answer, stderr is everything else

Data goes to stdout. Progress bars, confirmations, table footers, warnings, prompts and errors go to stderr. So a redirect gives you clean data and you still see what happened:

```bash
proton-cli drive items download /report.pdf --output - > report.pdf
```

## Four kinds of response

### Collections

```console
$ proton-cli mail messages list --unread --page-size 3
ID        FROM              SUBJECT                DATE              FLAGS
────────  ────────────────  ─────────────────────  ────────────────  ─────
5bH2mQxK  Fastmail Billing  Invoice #2291 ready    2026-04-15 14:32  ●★📎
9xL4pQrT  Trailhead         Weekly digest          2026-04-15 09:02  ●
2mNp7RsV  Jane Roe          Re: Quarterly numbers  2026-04-14 17:48

3 of 47 messages. Next page: --page 1
```

The `ID` column is always first and always called `ID`. Dates use one format. The `FLAGS` column reads `●` unread, `★` starred, `📎` has attachments.

An empty collection prints **nothing** on stdout - just `No messages.` on stderr - so a redirect yields an empty file rather than a stray header.

### Records

```console
$ proton-cli drive items get /Documents/report.pdf
Name:       report.pdf
Location:   /Documents
Type:       file
MIME Type:  application/pdf
Uploaded:   2026-04-02 11:20
Signature:  verified
Size:       2.4 MB
Shared:     yes
ID:         7Kd91mQx
```

Every field is spelled out, so a record can be read without knowing the API's field names.

### Documents

Decrypted content meant to be read: a header block, a blank line, the body, and whatever trails it.

```console
$ proton-cli mail messages get 5bH2mQxK
Subject:    Invoice #2291 is ready
From:       Fastmail Billing <billing@fastmail.com>
To:         me@proton.me
Date:       2026-04-15 14:32
Signature:  verified
ID:         5bH2mQxK

Hi Roman, your invoice is attached.

Attachments
ID        NAME              SIZE
────────  ────────────────  ───────
kQ81mDx4  invoice-2291.pdf  84.2 KB
```

`--body-only` gives you exactly the body and nothing else.

### Mutations

```console
$ proton-cli mail messages trash --unread --older-than 30d
✓ Moved 12 messages to trash.
```

When something is created its ID goes to **stdout** and the confirmation to stderr, so capturing the ID is a plain assignment:

```bash
LABEL=$(proton-cli mail settings labels create --name Work)
```

## Errors

One line for the problem, an indented `Try:` block for the fix.

```console
$ proton-cli mail messages get 5bH2mQxK --format htm
Error: --format accepts: text, html, raw.

$ proton-cli contacts get jane
Error: "jane" matches 2 contacts.
Try:   narrow the term, or use one of:
         7Kd91mQx  jane@example.com
         3Ns8pT2v  jane.roe@work.example
```

Mistakes that can be spotted in your command line - a misspelled setting key, an impossible `--format`, a colour outside Proton's palette - are reported immediately, without signing in or contacting Proton.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Something you passed was wrong |
| `2` | Authentication failed |
| `3` | Not found |
| `4` | Ambiguous, or a conflict |
| `5` | Network or server problem |
| `130` | Cancelled with `Ctrl+C` |

## JSON and YAML

```bash
proton-cli mail messages list --output json
proton-cli mail messages list --output yaml
```

Three rules, and they hold for every command.

**One envelope for every collection**, keyed by the collection's plural name:

```json
{
  "messages": [ { "id": "5bH2…", "subject": "Invoice #2291 is ready", "unread": true } ],
  "count": 3,
  "total": 47,
  "page": 0,
  "page_size": 3,
  "has_more": true
}
```

`count` is always there. `total`, `page`, `page_size` and `has_more` appear when the request involved them - so a consumer can tell "page 0" from "not paginated".

**Names, not numbers.** JSON uses the same vocabulary as the text output and as `set`:

```json
{ "type": "file", "state": "active", "date_format": "yyyy-mm-dd", "unread": true }
```

Timestamps are `<verb>_time` in Unix seconds; sizes are `size` in bytes.

**`--output json` always emits JSON**, mutations included:

```console
$ proton-cli mail settings labels create --name Work --output json
{
  "action": "created",
  "count": 1,
  "dry_run": false,
  "ids": ["kQ81mDx4T9…"],
  "kind": "label",
  "name": "Work"
}
```

IDs in machine output are always complete, never shortened.

The one exception is [`proton-cli api`](commands/api.md), which passes Proton's own response through unchanged.

## Colour

Interactive text output is coloured in Proton's own accents: headers and footers dimmed, IDs and status markers highlighted, `✓` green, `Error:` red.

Colour is off whenever output is piped or redirected, whenever `--output json` or `yaml` is used, and when you say so:

```bash
proton-cli mail messages list --no-color
NO_COLOR=1 proton-cli mail messages list
```

Colour never changes the layout: with it on or off, a script receives identical bytes.

## Widths

On a terminal, a table too wide to fit gives up room from its widest flexible column - never from a date or an ID.

Piped or redirected output is **never** truncated.

## Quiet

`--quiet` silences confirmations, notes, footers and progress. It never silences the answer: a script that passes `--quiet` still gets its data.

## Prompts

Only `account login` and commands that need your password again ever ask a question, and only when standard input is a terminal. Everything else fails with a message instead of waiting, so a scheduled job never hangs.

Prompts are written to stderr, so redirecting the answer never captures the question.

```bash
proton-cli account login --no-input     # fail instead of asking
PROTON_NO_INPUT=1 proton-cli account login
```

As with `NO_COLOR`, `PROTON_NO_INPUT` counts as set whatever its value, even empty.
