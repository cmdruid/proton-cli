# References

How you name the thing you want.

## Names instead of IDs

Wherever a command's usage shows `REF`, you can pass something human and proton-cli will find it:

```bash
proton-cli mail messages get "Invoice #2291"
proton-cli contacts get jane
proton-cli pass items get github.com
proton-cli calendar events get "Team sync"
```

If nothing matches, the command exits `3`. If more than one thing matches, it prints the candidates and exits `4`, so you can narrow the term or use an ID:

```console
$ proton-cli contacts get jane
Error: "jane" matches 2 contacts.
Try:   narrow the term, or use one of:
         7Kd91mQx  jane@example.com
         3Ns8pT2v  jane.roe@work.example
```

## Short IDs

On a terminal, lists shorten Proton's IDs to their first eight characters and remember what they showed you. Paste one straight back:

```console
$ proton-cli mail messages list
ID        FROM              SUBJECT                 DATE
────────  ────────────────  ──────────────────────  ────────────────
5bH2mQxK  Fastmail Billing  Invoice #2291 is ready  2026-04-15 14:32

$ proton-cli mail messages get 5bH2mQxK
```

Short IDs carry no ellipsis, so they can be copied straight out of a table.

The moment output stops being a terminal, IDs go back to full length - pipes, redirects and `--output json` always emit complete IDs, so no script ever sees a truncated value. `--full-ids` switches shortening off interactively too.

The cache lives in `~/.config/proton-cli/idcache/<profile>.json`. A short ID only resolves if it is in there: copied one from another machine? Run the matching list command there first, or use the full ID. If two cached IDs share the same eight characters, the command shows both and exits `4`.

## Compound references

A Pass item and a calendar event each need two IDs, written as one slash-separated token. Lists print them in this form, and you paste them back the same way:

```bash
proton-cli pass items get SHARE_ID/ITEM_ID
proton-cli calendar events get CALENDAR_ID/EVENT_ID
```

Short IDs work here too, on both halves at once: `5bH2mQxK/9xL4pQrT`.

## Drive is addressed by path

Files and folders are named by their path:

```bash
proton-cli drive items get /Documents/report.pdf
proton-cli drive items move /Documents/report.pdf --into /Archive
```

Something with no place in the tree - a trashed item, a photo, an album - has no path, so it is named by the `REF` its list showed:

```bash
proton-cli drive trash restore 7Kd91mQx
proton-cli drive photos download 3Ns8pT2v --output-dir ./photos
```

## IDs that start with a dash

Proton's IDs are base64 and `-` is one of its sixty-four characters, so about one ID in sixty-four begins with a dash and looks like a flag.

proton-cli handles it. Before parsing, it protects a leading-dash reference that the command it was given could not read as flags - and this CLI defines three shorthands in total (`-h`, `-v`, `-o`), so a reference is never mistaken for one. It works for a full ID, for a shortened one, and for the compound `SHARE/ITEM` form the listings print:

```bash
proton-cli pass items get -x76EpiV/_fb26gvM
proton-cli drive photos download -Qt-s7R_
```

An ID passed as a **flag's value** needs nothing at all, because a value is read as a value whatever it starts with:

```bash
proton-cli drive photos list --album -Qt-s7R_oGCru5u3Kv6Y8Q
```

If you ever do hit an odd parse error, you can separate arguments from flags yourself:

```bash
proton-cli mail messages get -- -bH2mQxKT9wLpN4v…
```

Put flags **before** such an ID, since everything after `--` is positional.
