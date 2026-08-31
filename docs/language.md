# How commands read

proton-cli has one grammar: `proton <app> <collection> <verb>`. Learn it once and you can guess the rest of the two hundred commands, name the thing you want in whatever way you already know it, and see what a bulk change would touch before it happens.

```
proton <app> <collection> <verb> [TARGET…] [--flags]
```

```bash
proton mail messages list
proton drive items move /report.pdf --into /Archive
proton pass items get github.com
proton calendar events create --title Standup --start 2026-04-16T09:00
```

A **group** never does anything itself - `proton mail settings` prints help, `proton mail settings get` shows your settings. Every command that acts is named by a verb.

## Verbs

One word per idea, everywhere it appears.

| Verb | Means |
| --- | --- |
| `list` · `get` | show a collection, or one thing in full |
| `create` · `update` · `delete` | the usual three |
| `trash` · `restore` · `empty` | remove reversibly, put back, clear out |
| `move --into` · `copy --into` | put into another container |
| `upload` · `download` | move bytes to or from your disk |
| `export` · `import` | documents to or from disk |
| `send` · `reply` · `forward` | mail going out |
| `label` · `unlabel` · `star` · `unstar` | attach or detach |
| `enable` · `disable` | turn a thing on or off |
| `add` · `remove` | put a member into a container, or take one out |
| `accept` · `decline` | answer an invitation |
| `set` | write one setting |
| `login` · `logout` | your session |

To rename anything, use `update --name`. The [command reference](commands/README.md) has the full list.

## Naming the thing you want

Wherever a command's usage shows `REF`, four things work.

```bash
proton mail messages get 5bH2mQxKT9wLpN4v…    # the full ID
proton mail messages get 5bH2mQxK             # the short ID a list printed
proton mail messages get "Invoice #2291"      # the subject
proton contacts get jane                      # a name or an address
```

If nothing matches, the command exits `3`. If more than one thing matches, it prints the candidates and exits `4`:

```console
$ proton contacts get jane
Error: "jane" matches 2 contacts.
Try:   narrow the term, or use one of:
         7Kd91mQx  jane@example.com
         3Ns8pT2v  jane.roe@work.example
```

### Short IDs

On a terminal, lists shorten Proton's IDs to eight characters and remember what they showed you, so you can paste one straight back. They carry no ellipsis and never start with a dash, so they copy cleanly out of a table and a shell reads them as what they are.

Pipes, redirects and `--output json` always emit **full** IDs, so no script ever sees a truncated value. `--full-ids` switches shortening off interactively too.

A short ID only resolves on the machine that printed it - the lookup table lives in `~/.config/proton-cli/idcache/<profile>.json`. Copied one from elsewhere? Run the matching `list` here first, or use the full ID.

### Two IDs in one

A Pass item and a calendar event each need two IDs, written as one slash-separated token. Lists print them this way and you paste them back the same way. Short IDs work on both halves at once.

```bash
proton pass items get SHARE_ID/ITEM_ID
proton calendar events get CALENDAR_ID/EVENT_ID
```

A **recurring** event is stored once and happens many times, so naming a single occurrence takes one more part: its own start, after an `@`.

```bash
proton calendar events get 4f2a1b9c@2026-04-16T09:00   # one occurrence
proton calendar events get 4f2a1b9c                    # the whole series
```

Keep the `@` part and you act on that occurrence; drop it and you act on the series. `--future` widens one occurrence to it and every later one.

### Drive is addressed by path

Files and folders are named by their path:

```bash
proton drive items get /Documents/report.pdf
proton drive items move /Documents/report.pdf --into /Archive
```

Something with no place in the tree - a trashed item, a photo, an album - has no path, so it is named by the `REF` its list showed:

```bash
proton drive trash restore 7Kd91mQx
proton drive photos download 3Ns8pT2v --output-dir ./photos
```

### Full IDs that start with a dash

Proton's IDs are base64, `-` is one of its characters, and so about one in sixty-four begins with one. Paste them like any other reference - the dash is part of the ID and is handled for you:

```bash
proton pass items get -x76EpiVSJf2oHzHgyC2D_jF8O…==/_fb26gvMWjnM7US4_wpTNm_LqI…==
proton contacts delete -bJxDLEMvt-Z6t4Yna7V8SYQ_F…==
```

One rule comes with it: **put your flags before the ID**, because everything after such an ID is read as another argument.

```bash
proton mail messages attachments download --output-dir ./files -bJxDLEMvt-…==   # yes
proton mail messages attachments download -bJxDLEMvt-…== --output-dir ./files   # no
```

Short IDs need none of this: the eight characters begin after any leading dashes, so flags can go on either side.

## Saying which ones

Every command that can act on many things takes the same two ways of saying which: name them, or describe them. Both at once is a union.

```bash
proton mail messages trash 5bH2mQxK 9xL4pQrT
proton mail messages trash --from newsletter@example.com --older-than 90d
proton mail messages trash 5bH2mQxK --unread --folder spam
```

| Filter | Means |
| --- | --- |
| `--folder` · `--scope` · `--vault` | where to look |
| `--older-than` · `--newer-than` | by age |
| `--larger-than` · `--smaller-than` | by size |
| `--pattern` | match the name against a glob |
| `--unread` · `--starred` · `--type` | by state or kind |
| `--all` | everything in scope, rather than a subset |
| `--limit` | cap how many a bulk verb affects |

`list` takes the same filters as the verbs beside it, so you can see a selection before acting on it:

```bash
proton drive items list /Build --pattern "*.tmp" --recursive   # see what matches
proton drive items trash --scope /Build --pattern "*.tmp" --recursive
```

What `list` says with its `PATH` argument, a bulk verb says with `--scope`, because it uses its arguments to name things instead.

A command needs at least one reference or filter:

```console
$ proton mail messages trash
Error: Nothing selected.
Try:   pass a REF, or a filter such as --unread, --starred, --from or --older-than.
       Use --all to target a whole folder.
```

## When it asks first

proton asks before it removes something it cannot put back, and before it removes things you did not name. Nothing else ever stops to ask.

| | you named it | a filter found it |
| --- | --- | --- |
| `delete` · `empty` · `uninstall` | asks | asks |
| `trash` | just does it | asks |
| everything else | just does it | just does it |

The question shows the things themselves, never a count:

```console
$ proton mail messages delete --from newsletter@example.com --older-than 90d
Would delete 3 messages:

ID        FROM          SUBJECT              DATE
────────  ────────────  ───────────────────  ────────────────
hR8sT2vW  Example News  January round-up     2026-01-08 06:00
kM4nP9qL  Example News  December round-up    2025-12-08 06:00
zC7bX1yE  Example News  November round-up    2025-11-08 06:00

This cannot be undone. Continue? [y/N]
```

Anything but a plain `y` means no, including pressing enter. ([Why these two cases](design-notes.md#why-it-asks-before-some-removals-and-not-others).)

**In a script** there is nobody to ask, so the question becomes an error and nothing is removed. `--yes` is the answer given in advance:

```console
$ proton mail messages delete --folder spam --older-than 30d
Error: Would delete 112 messages. This cannot be undone.
Try:   --yes to confirm, or --dry-run to see what it would touch.
```

## Dry runs

Every command that changes something takes `--dry-run`. It resolves references, applies filters, and shows you the things themselves:

```console
$ proton mail messages trash --from newsletter@example.com --older-than 90d --dry-run
Dry run - would move 3 messages to trash:

ID        FROM          SUBJECT              DATE
────────  ────────────  ───────────────────  ────────────────
hR8sT2vW  Example News  January round-up     2026-01-08 06:00
kM4nP9qL  Example News  December round-up    2025-12-08 06:00
zC7bX1yE  Example News  November round-up    2025-11-08 06:00
```

Dry-run output goes to stderr, so it still appears if you redirect stdout.

## Getting help

Every command documents itself, and completion knows the whole tree - including which values each constrained flag accepts.

```bash
proton --help
proton mail messages send --help
```

Next: [what comes back](output.md), and the page for the app you want - [Mail](apps/mail.md), [Drive](apps/drive.md), [Calendar](apps/calendar.md), [Pass](apps/pass.md) or [Contacts](apps/contacts.md).
