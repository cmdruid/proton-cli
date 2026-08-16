# Drive

Your Drive as paths. Files are encrypted before they leave your machine and decrypted after they arrive, per block, with your keys.

## Items

### List and inspect

```bash
proton drive items list                       # root
proton drive items list /Documents
proton drive items get /Documents/report.pdf  # type, size, checksum, sharing state
```

### Upload

```bash
proton drive items upload ./report.pdf /Documents
proton drive items upload ./report.pdf            # to the root
proton drive items upload --recursive ./project /Backup
pg_dump mydb | proton drive items upload - /Backups/db.sql
```

Uploads show progress on stderr and print `✓ Uploaded <name>` when done.

A name already taken is refused, so nothing is overwritten by accident. `--if-exists` answers the question instead:

```bash
proton drive items upload --if-exists replace ./report.pdf /Documents  # a new revision of it
proton drive items upload --if-exists rename ./report.pdf /Documents   # keeps both, as "report (1).pdf"
proton drive items upload --if-exists skip ./report.pdf /Documents     # leaves what is there alone
```

With `--recursive` the answer is about the folder the tree lands in, since a tree is one thing with one name:

```bash
proton drive items upload --recursive --if-exists replace ./project /Backup  # into the folder already there, file by file
proton drive items upload --recursive --if-exists rename ./project /Backup   # the whole tree beside it, as "project (1)"
proton drive items upload --recursive --if-exists skip ./project /Backup     # writes none of it
```

A file standing where a folder must go, or a folder where a file must go, is refused before anything is written: neither can take the other's place, and an upload does not remove things.

### Download

```bash
proton drive items download /Documents/report.pdf --output ./report.pdf
proton drive items download /Documents/report.pdf --output-dir ./downloads/  # keep the name
proton drive items download /Documents/report.pdf --output - | less
proton drive items download /Documents/report.pdf --output ./report.pdf --force
```

### Move, rename, copy

```bash
proton drive items update /Documents/old.txt --name new.txt
proton drive items move /Documents/report.pdf --into /Archive
proton drive items copy /Documents/report.pdf --into /Archive
```

### Trash and delete

```bash
proton drive items trash /Documents/old.pdf     # reversible
proton drive items delete /Documents/old.pdf    # permanent
```

Both accept filters instead of paths:

```bash
proton drive items trash --pattern "*.tmp" --scope /Build --recursive
proton drive items trash --older-than 90d --scope /Logs --recursive
proton drive items delete --larger-than 100MB --scope /Downloads --recursive
proton drive items delete --scope /Temp --recursive --all
```

Filters: `--pattern` (glob), `--larger-than`, `--smaller-than`, `--older-than`, `--newer-than`, `--scope`, `--recursive`, `--all`. `move` and `copy` take them too. Always try them with `--dry-run` first.

### Revisions

Uploading over a file with `--if-exists replace` keeps what was there as a revision.

```bash
proton drive items revisions list /Documents/report.pdf
proton drive items revisions download /Documents/report.pdf 8f3a1c22 --output ./earlier.pdf
proton drive items revisions restore /Documents/report.pdf 8f3a1c22
proton drive items revisions delete /Documents/report.pdf 8f3a1c22     # permanent
```

`download` leaves the file as it is: it reads an old version out, where `restore` puts one back in place. It takes `--output`, `--output-dir` and `--force` like every other download, and `--output -` streams the old version into a pipe.

Proton carries a restore out in the background, so the file goes back to the earlier contents a moment after the command returns. The version it succeeds stays in the history, so a restore can itself be undone.

The version the file is at now is the one thing neither command touches: restoring it would do nothing, and deleting it would be deleting the file.

## Folders

```bash
proton drive folders create /Documents/Invoices
```

## Sharing

### Public links

```bash
proton drive share get /Documents/report.pdf   # who has access, plus the link
proton drive share link /Documents/report.pdf  # create or show the link
proton drive share link /Documents/report.pdf --expires 7d --password hunter2
proton drive share link /Documents/project --edit
proton drive share unlink /Documents/report.pdf
```

### Sharing with people

```bash
proton drive share add /Documents/report.pdf bob@proton.me
proton drive share add /Documents/project bob@proton.me --edit --message "Draft for review"
proton drive share remove /Documents/report.pdf bob@proton.me
```

### Invitations sent to you

```bash
proton drive invitations list
proton drive invitations accept INVITATION_ID
proton drive invitations decline INVITATION_ID
```

## Trash

```bash
proton drive trash list
proton drive trash restore LINK_ID...
proton drive trash empty        # permanent, across all volumes
```

`empty` lists what it would destroy and asks first. See [When it asks first](../language.md#when-it-asks-first).

## Photos

```bash
proton drive photos list
proton drive photos list --tag favorites
proton drive photos upload ./IMG_0001.jpg
proton drive photos download LINK_ID --output-dir ./photos/
proton drive photos trash LINK_ID...       # reversible
proton drive photos delete LINK_ID...      # permanent
proton drive photos favorite LINK_ID...
proton drive photos unfavorite LINK_ID...
```

Tags: `favorites`, `screenshots`, `videos`, `live-photos`, `motion-photos`, `selfies`, `portraits`, `bursts`, `panoramas`, `raw`.

### Albums

```bash
proton drive photos albums list
proton drive photos albums create --name Holiday
proton drive photos list --album ALBUM_LINK_ID
proton drive photos albums add ALBUM_LINK_ID PHOTO_LINK_ID...
proton drive photos albums remove ALBUM_LINK_ID PHOTO_LINK_ID...
proton drive photos albums delete Holiday                    # by name, or by link ID
proton drive photos albums delete Holiday --delete-photos
```

## Settings

```bash
proton drive settings                          # how long previous versions are kept
proton drive settings set version-history 30d
```

| Key | Values |
| --- | --- |
| `version-history` | `off`, `7d`, `30d`, `180d`, `1y`, `10y` |

Keeping more than the default is a paid feature.
