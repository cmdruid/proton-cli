# Naming things

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

## Short IDs

On a terminal, lists shorten Proton's IDs to eight characters and remember what they showed you, so you can paste one straight back. They carry no ellipsis and never start with a dash, so they copy cleanly out of a table and a shell reads them as what they are.

Pipes, redirects and `--output json` always emit **full** IDs, so no script ever sees a truncated value. `--full-ids` switches shortening off interactively too.

A short ID only resolves on the machine that printed it - the lookup table lives in `~/.config/proton-cli/idcache/<profile>.json`. Copied one from elsewhere? Run the matching `list` here first, or use the full ID.

## Two IDs in one

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

## Drive is addressed by path

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

## Full IDs that start with a dash

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
