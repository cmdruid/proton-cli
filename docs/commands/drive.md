# Drive

Your Drive as paths. Files are encrypted before they leave your machine and decrypted after they arrive, per block, with your keys.

## Items

### List and inspect

```bash
proton-cli drive items list                       # root
proton-cli drive items list /Documents
proton-cli drive items info /Documents/report.pdf # type, size, checksum, sharing state
```

### Upload

```bash
proton-cli drive items upload ./report.pdf /Documents
proton-cli drive items upload ./report.pdf            # to the root
proton-cli drive items upload --recursive ./project /Backup
pg_dump mydb | proton-cli drive items upload - /Backups/db.sql
```

Uploads show progress on stderr and print `✓ Uploaded <name>` when done.

### Download

```bash
proton-cli drive items download /Documents/report.pdf --output ./report.pdf
proton-cli drive items download /Documents/report.pdf --output-dir ./downloads/  # keep the name
proton-cli drive items download /Documents/report.pdf --output - | less
proton-cli drive items download /Documents/report.pdf --output ./report.pdf --force
```

### Move, rename, copy

```bash
proton-cli drive items rename /Documents/old.txt new.txt
proton-cli drive items move /Documents/report.pdf /Archive
proton-cli drive items copy /Documents/report.pdf /Archive
```

### Trash and delete

```bash
proton-cli drive items trash /Documents/old.pdf     # reversible
proton-cli drive items delete /Documents/old.pdf    # permanent
```

Both accept filters instead of paths:

```bash
proton-cli drive items trash --pattern "*.tmp" --scope /Build --recursive
proton-cli drive items trash --older-than 90d --scope /Logs --recursive
proton-cli drive items delete --larger-than 100MB --scope /Downloads --recursive
proton-cli drive items delete --scope /Temp --recursive --all
```

Filters: `--pattern` (glob), `--larger-than`, `--older-than`, `--newer-than`, `--scope`, `--recursive`, `--all`. Always try them with `--dry-run` first.

### Revisions

```bash
proton-cli drive items revisions list /Documents/report.pdf
proton-cli drive items revisions restore /Documents/report.pdf REVISION_ID
```

## Folders

```bash
proton-cli drive folders create /Documents/Invoices
```

## Sharing

### Public links

```bash
proton-cli drive share status /Documents/report.pdf   # who has access, plus the link
proton-cli drive share link /Documents/report.pdf     # create or show the link
proton-cli drive share link /Documents/report.pdf --expires 7d --password hunter2
proton-cli drive share link /Documents/project --edit
proton-cli drive share unlink /Documents/report.pdf
```

### Sharing with people

```bash
proton-cli drive share add /Documents/report.pdf bob@proton.me
proton-cli drive share add /Documents/project bob@proton.me --edit --message "Draft for review"
proton-cli drive share remove /Documents/report.pdf bob@proton.me
```

### Invitations sent to you

```bash
proton-cli drive invitations list
proton-cli drive invitations accept INVITATION_ID
proton-cli drive invitations reject INVITATION_ID
```

## Trash

```bash
proton-cli drive trash list
proton-cli drive trash restore LINK_ID...
proton-cli drive trash empty        # permanent, across all volumes
```

## Photos

```bash
proton-cli drive photos list
proton-cli drive photos list --tags favorites
proton-cli drive photos upload ./IMG_0001.jpg
proton-cli drive photos download LINK_ID --output-dir ./photos/
proton-cli drive photos trash LINK_ID...       # reversible
proton-cli drive photos delete LINK_ID...      # permanent
proton-cli drive photos favorite LINK_ID...
proton-cli drive photos unfavorite LINK_ID...
```

Tags: `favorites`, `screenshots`, `videos`, `live-photos`, `motion-photos`, `selfies`, `portraits`, `bursts`, `panoramas`, `raw`.

### Albums

```bash
proton-cli drive photos albums list
proton-cli drive photos albums create --name Holiday
proton-cli drive photos albums items ALBUM_LINK_ID
proton-cli drive photos albums add ALBUM_LINK_ID PHOTO_LINK_ID...
proton-cli drive photos albums remove ALBUM_LINK_ID PHOTO_LINK_ID...
proton-cli drive photos albums delete ALBUM_LINK_ID
proton-cli drive photos albums delete ALBUM_LINK_ID --delete-photos
```
