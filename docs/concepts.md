# Concepts

A handful of rules apply to every command. Learn these once and the rest of the CLI is predictable.

## References: names instead of IDs

Wherever a command's usage shows `REF`, you can pass either a Proton ID or something human: a subject, a name, a URL, a title. proton-cli searches for it and acts on the single match.

```bash
proton-cli mail messages read "Invoice #2291"
proton-cli contacts get jane
proton-cli pass items get github.com
proton-cli calendar events get "Team sync"
```

If nothing matches, the command exits `3`. If several things match, it prints the candidates to stderr and exits `4`, so you can narrow the term or pass an ID.

## Short IDs

In an interactive terminal, list commands shorten Proton IDs to their first 8 characters, and remember the ones they showed you in `~/.config/proton-cli/idcache/<profile>.json`. Paste a short ID into any command that takes an ID:

```console
$ proton-cli mail messages list
ID        FROM              SUBJECT                 DATE
────────  ────────────────  ──────────────────────  ────────────────
5bH2mQxK  Fastmail Billing  Invoice #2291 is ready  2026-04-15 14:32

$ proton-cli mail messages read 5bH2mQxK
```

The moment output stops being a terminal, IDs go back to full length. Pipes, redirects, and `--output json|yaml` always emit complete IDs, so scripts never see a truncated value:

```bash
proton-cli mail messages list --output json | jq -r '.messages[].id'
```

Pass `--full-ids` to switch shortening off interactively too.

A short ID only resolves if it's in your local cache. Copied one from another machine? Run the matching list command there first, or use the full ID. If two cached IDs share the same 8 characters, the command exits `4` and shows both.

## Output formats

```bash
proton-cli mail messages list                  # aligned text table (default)
proton-cli mail messages list --output json    # snake_case JSON
proton-cli mail messages list --output yaml    # the same data as YAML
```

Text output is for you; JSON and YAML are for programs. Keys are `snake_case` and stable.

## Color

Interactive text output is colored: table headers and footers are dimmed, IDs and status markers are highlighted, and `✓` confirmations are green. Like short IDs, it only happens for a human at a terminal.

Color is off whenever output is piped or redirected, whenever `--output json` or `yaml` is used, and when you ask for it to be off:

```bash
proton-cli mail messages list --no-color
NO_COLOR=1 proton-cli mail messages list
```

So the bytes a script receives are the same either way.

## stdout is data, stderr is chatter

Data goes to stdout. Progress bars, `✓` confirmations, table footers, and warnings go to stderr. Redirecting stdout therefore gives you clean data, and you still see what happened:

```bash
proton-cli drive items download /report.pdf --output - > report.pdf
```

Commands that create something print the new ID to stdout, which makes capturing it trivial:

```bash
LABEL=$(proton-cli mail settings labels create --name Work --color "#8080FF")
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Something you passed was wrong |
| `2` | Authentication failed |
| `3` | Not found |
| `4` | Conflict, or an ambiguous reference |
| `5` | Network or server problem |
| `130` | Cancelled with `Ctrl+C` |

```bash
if ! proton-cli contacts get jane; then
  echo "no unique match (exit $?)"
fi
```

## Streaming with `-`

A single `-` means stdin for inputs and stdout for outputs, so Proton fits into a pipeline without touching the disk:

```bash
echo "Deployed successfully." | proton-cli mail messages send --to me@proton.me --subject Deploy --body -
pg_dump mydb | proton-cli drive items upload - /Backups/db.sql
proton-cli drive items download /Backups/db.sql --output - | psql mydb
```

## Dry runs

Every mutating command accepts `--dry-run`. It resolves references, applies filters, and prints exactly what it would do without touching your account:

```bash
proton-cli mail messages trash --unread --older-than 30d --dry-run
proton-cli drive items delete --pattern "*.tmp" --scope /Build --recursive --dry-run
```

## Bulk filters

Commands that act on many things at once (trash, delete, move, mark, star) take filters instead of, or in addition to, explicit references. Filters and references combine as a union:

```bash
proton-cli mail messages move --dest archive --from newsletter@example.com --older-than 7d
proton-cli pass items trash --vault Old --type login
proton-cli drive items trash --larger-than 100MB --scope /Downloads --recursive
```

`--all` is the way to say "yes, really everything in this scope" when no other filter narrows it down. Pair any of them with `--dry-run` first.

## Cancelling

`Ctrl+C` aborts cleanly: in-flight uploads and downloads stop, and the command exits `130`.

## Getting help

Every command and subcommand documents itself:

```bash
proton-cli --help
proton-cli drive --help
proton-cli mail messages send --help
```
