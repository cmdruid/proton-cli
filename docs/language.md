# The language

proton-cli has one grammar. Learn the grammar and you can guess the rest.

```
proton-cli <app> <collection> <verb> [TARGET…] [--flags]
```

```bash
proton-cli mail messages list
proton-cli mail settings labels create --name Work
proton-cli drive items move /report.pdf --into /Archive
proton-cli pass vaults update SHARE_ID --name Personal
```

A **group** never does anything itself - it only holds other commands. So `proton-cli mail settings` prints help, and `proton-cli mail settings get` shows your settings. Every command that acts is named by a verb.

## Verbs

One word per idea, everywhere it appears.

| Verb | Means |
| --- | --- |
| `list` | enumerate a collection |
| `get` | show one thing in full |
| `search` | ask Proton's index (mail only) |
| `create` | make a new thing |
| `update` | change fields of an existing thing |
| `delete` | remove permanently |
| `trash` | remove reversibly |
| `restore` | undo a removal |
| `empty` | clear out a trash |
| `move --into` | put into another container |
| `copy --into` | duplicate into another container |
| `upload` / `download` | move bytes to or from your disk |
| `export` | write documents to disk |
| `send` / `reply` / `forward` | mail going out |
| `label` / `unlabel` | attach or detach a label |
| `star` / `unstar` | add to or remove from Starred |
| `mark read` / `mark unread` | whether something counts as read |
| `enable` / `disable` | turn a thing on or off |
| `link` / `unlink` | a public share link |
| `add` / `remove` | put a member into a container, or take one out |
| `accept` / `decline` | answer an invitation |
| `set` | write one setting |
| `login` / `logout` | your session |

To rename something, use `update --name`:

```bash
proton-cli drive items update /old.txt --name new.txt
proton-cli pass vaults update SHARE_ID --name Personal
```

## Targets

Wherever a command shows **`REF`**, four things work:

```bash
proton-cli mail messages get 5bH2mQxKT9wLpN4v…    # the full ID
proton-cli mail messages get 5bH2mQxK             # the short ID a list printed
proton-cli mail messages get "Invoice #2291"      # the subject
proton-cli contacts get jane                      # a name or an address
```

A Pass item and a calendar event each need two IDs, written as **one** slash-separated token. Lists print them in this form, and you paste them back the same way:

```bash
proton-cli pass items get SHARE_ID/ITEM_ID
proton-cli calendar events get CALENDAR_ID/EVENT_ID
```

A **recurring** event is stored once and happens many times, so one more thing is needed to name a single occurrence: its own start, after an `@`. `calendar events list` prints exactly what you paste back.

```bash
proton-cli calendar events get 4f2a1b9c@2026-04-16T09:00   # one occurrence
proton-cli calendar events get 4f2a1b9c                    # the whole series
```

So the reference decides how far a change reaches: keep the `@` part and you act on that occurrence, drop it and you act on the series. `--future` widens one occurrence to it and every later one.

**Drive is different.** Files and folders are named by their `PATH`. Trashed items, photos and albums have no path, so they are named by `REF`, the ID their list showed.

```bash
proton-cli drive items get /Documents/report.pdf
proton-cli drive trash restore 7Kd91mQx
```

If nothing matches a reference the command exits `3`. If several things match, it lists them and exits `4`.

## Saying which ones

Every command that can act on many things takes the same two ways of saying which: name them, or describe them. Both at once is a union.

```bash
proton-cli mail messages trash 5bH2mQxK 9xL4pQrT
proton-cli mail messages trash --from newsletter@example.com --older-than 90d
proton-cli mail messages trash 5bH2mQxK --unread --folder spam
```

| Filter | Means |
| --- | --- |
| `--folder` · `--scope` · `--vault` | where to look |
| `--pattern` | match the name against a glob (Drive) |
| `--older-than` · `--newer-than` | by age |
| `--larger-than` · `--smaller-than` | by size (Drive) |
| `--type` | by kind |
| `--unread` · `--starred` | by state (Mail) |
| `--recursive` | descend into subfolders (Drive) |
| `--limit` | cap how many |
| `--all` | yes, really everything in scope |

A command needs at least one reference or filter:

```console
$ proton-cli mail messages trash
Error: Nothing selected.
Try:   pass a REF, or a filter such as --unread, --starred, --from or --older-than.
       Use --all to target a whole folder.
```

## When it asks first

proton-cli asks before it removes something it cannot put back, and before it removes things you did not name. Nothing else ever stops to ask.

Those are the two ways a removal surprises you: the wrong verb, and the wrong filter.

| | you named it | a filter found it |
| --- | --- | --- |
| `delete` · `empty` · `uninstall` | asks | asks |
| `trash` | just does it | asks |
| everything else | just does it | just does it |

The question shows the things themselves, never a count:

```console
$ proton-cli mail messages delete --from newsletter@example.com --older-than 90d
Would delete 3 messages:

ID        FROM          SUBJECT              DATE
────────  ────────────  ───────────────────  ────────────────
hR8sT2vW  Example News  January round-up     2026-01-08 06:00
kM4nP9qL  Example News  December round-up    2025-12-08 06:00
zC7bX1yE  Example News  November round-up    2025-11-08 06:00

This cannot be undone. Continue? [y/N]
```

Only a permanent removal says *This cannot be undone*, because only a permanent removal cannot be. Trashing is recoverable, so it asks the shorter question and `restore` puts things back.

Anything but a plain `y` means no, including pressing enter.

### In a script

A script has nobody to ask, so the question becomes an error and nothing is removed:

```console
$ proton-cli mail messages delete --folder spam --older-than 30d
Error: Would delete 112 messages. This cannot be undone.
Try:   --yes to confirm, or --dry-run to see what it would touch.
```

`--yes` is the answer given in advance:

```bash
proton-cli mail messages delete --folder spam --older-than 30d --yes
```

## Dry runs

Every command that changes something takes `--dry-run`. It resolves references, applies filters, and shows you **the things themselves** - not a count:

```console
$ proton-cli mail messages trash --from newsletter@example.com --older-than 90d --dry-run
Dry run - would move 3 messages to trash:

ID        FROM          SUBJECT              DATE
────────  ────────────  ───────────────────  ────────────────
hR8sT2vW  Example News  January round-up     2026-01-08 06:00
kM4nP9qL  Example News  December round-up    2025-12-08 06:00
zC7bX1yE  Example News  November round-up    2025-11-08 06:00
```

Dry-run output goes to stderr, so it still appears if you redirect stdout.

## Flags mean one thing

Each flag name means the same thing everywhere. `--to` is always an email recipient, and `--into` is always a destination container:

```bash
proton-cli mail messages send --to alice@proton.me       # a recipient
proton-cli mail messages search --to alice@proton.me     # matching a recipient
proton-cli mail messages move REF --into archive         # a destination
```

`--force` only ever means "overwrite a local file". `--all` only ever means "everything in scope".

## Folders and labels are not the same

A message lives in exactly one **folder** and carries any number of **labels**.

```bash
proton-cli mail messages move REF --into archive     # it leaves where it was
proton-cli mail messages label REF --label Work      # it stays where it is
```

Passing a label to `move` is an error, not a silent relabel:

```console
$ proton-cli mail messages move REF --into Work
Error: "Work" is a label, not a folder - moving needs a folder.
Try:   to attach the label instead, use `label --label Work`.
       To see the folders, run `proton-cli mail settings folders list`.
```

## Streaming with `-`

A single `-` means stdin for an input and stdout for an output:

```bash
echo "Deployed." | proton-cli mail messages send --to me@proton.me --subject Deploy --body -
pg_dump mydb | proton-cli drive items upload - /Backups/db.sql
proton-cli drive items download /Backups/db.sql --output - | psql mydb
```

## Getting help

Every command documents itself, and shell completion knows the whole tree - including which values each constrained flag accepts.

```bash
proton-cli --help
proton-cli mail messages send --help
proton-cli completion zsh > "${fpath[1]}/_proton-cli"
```
