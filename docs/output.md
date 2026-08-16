# Output

Everything proton prints is one of four things, and each looks the same wherever it appears.

## stdout is the answer, stderr is everything else

Data goes to stdout. Progress bars, confirmations, table footers, warnings, prompts and errors go to stderr. So a redirect gives you clean data and you still see what happened:

```bash
proton drive items download /report.pdf --output - > report.pdf
```

## Four kinds of response

### Collections

```console
$ proton mail messages list --unread --page-size 3
ID        FROM              SUBJECT                DATE              FLAGS
────────  ────────────────  ─────────────────────  ────────────────  ─────
5bH2mQxK  Fastmail Billing  Invoice #2291 ready    2026-04-15 14:32  ●★2
9xL4pQrT  Trailhead         Weekly digest          2026-04-15 09:02  ●
2mNp7RsV  Jane Roe          Re: Quarterly numbers  2026-04-14 17:48

3 of 47 messages. Next page: --page 1
```

The `ID` column is always first and always called `ID`. Dates use one format. The `FLAGS` column reads `●` unread, `★` starred, and a number for how many files are attached.

An empty collection prints **nothing** on stdout - just `No messages.` on stderr - so a redirect yields an empty file rather than a stray header. When a filter was applied it reads `No messages match.` instead, so an unmatched search never looks like an empty account.

### Records

```console
$ proton drive items get /Documents/report.pdf
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
$ proton mail messages get 5bH2mQxK
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
$ proton mail messages trash --unread --older-than 30d
✓ Moved 12 messages to trash.
```

When something is created its ID goes to **stdout** and the confirmation to stderr, so capturing the ID is a plain assignment:

```bash
LABEL=$(proton mail settings labels create --name Work)
```

## Errors

One line for the problem, an indented `Try:` block for the fix.

```console
$ proton mail messages get 5bH2mQxK --format htm
Error: --format accepts: text, html, raw.

$ proton contacts get jane
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
proton mail messages list --output json
proton mail messages list --output yaml
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

**Times are rendered in your own zone.** A calendar event's `start` and `end` are RFC 3339 with your offset, so the day and the clock read off the string are the ones the text output shows, and the instant is exact either way. What the event is anchored to is its own field:

```json
{ "start": "2026-04-16T16:00:00+02:00", "end": "2026-04-16T17:00:00+02:00", "zone": "Europe/Vienna" }
```

An event with no time of day has `"all_day": true`, begins at midnight on the date it names, and ends at the midnight after its last day - so `end` is never part of the event, and `end - start` is how long it lasts.

**`--output json` always emits JSON**, mutations included:

```console
$ proton mail settings labels create --name Work --output json
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

The one exception is [`proton api`](commands/api.md), which passes Proton's own response through unchanged.

## Colour

Colour is used for one thing: making the parts that carry a verdict, or a colour of their own, worth stopping on. Everything else stays plain, so what is coloured means something.

**The shades are your terminal's, not proton's.** The CLI asks for a colour by name - the same eight names ANSI has had since 1976 - and your terminal decides what each one looks like. So Proton's purple comes out as whatever purple your theme uses, and the CLI stays legible on a light background without ever having to guess you are on one.

| What | Colour |
| --- | --- |
| Headers, footers, field labels | Dimmed |
| IDs | Magenta - Proton's purple |
| `✓` a change that succeeded | Green |
| `!` a caveat worth knowing | Yellow |
| `Error:` | Red |
| `●` unread | Magenta |
| `★` starred | Yellow, standing in for the orange Proton Mail uses |
| `Signature:` | Green verified, yellow unverified, red invalid |
| `■` beside a label, folder, calendar or group | The exact colour Proton stores for it |

The swatch is the one exception, and it is not the CLI picking a colour: the hex is the value you gave that label, folder, calendar or group, so redrawing it from your theme would misreport a field rather than respect a preference. It is the only place proton writes an exact colour, and how faithfully it lands depends on whether your terminal takes 24-bit colour - set `COLORTERM=truecolor` if it does and does not say so.

Colour is off whenever output is piped or redirected, whenever `--output json` or `yaml` is used, and when you say so:

```bash
proton mail messages list --no-color
NO_COLOR=1 proton mail messages list
```

Colour never changes the layout, and never carries meaning on its own: with it on or off, a script receives identical bytes and a reader gets the same words. Every verdict is spelled out - `invalid`, `unverified` - so nothing depends on being able to see the difference between green and red.

Widths are measured in terminal cells rather than characters, so a table stays aligned for a subject written in Japanese or a filename with an emoji in it.

## Widths

On a terminal, a table too wide to fit gives up room from its widest flexible column - never from a date or an ID.

Piped or redirected output is **never** truncated.

## Quiet

`--quiet` silences confirmations, notes, footers and progress. It never silences the answer: a script that passes `--quiet` still gets its data.

## Prompts

Only `account login` and commands that need your password again ever ask a question, and only when standard input is a terminal. Everything else fails with a message instead of waiting, so a scheduled job never hangs.

Prompts are written to stderr, so redirecting the answer never captures the question.

```bash
proton account login --no-input     # fail instead of asking
PROTON_NO_INPUT=1 proton account login
```

As with `NO_COLOR`, `PROTON_NO_INPUT` counts as set whatever its value, even empty.
