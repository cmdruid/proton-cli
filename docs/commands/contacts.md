# proton contacts

Contacts, their groups and their pinned keys.

Every command under `proton contacts`, with the arguments and flags it takes. For these commands in use, see [the guide](../apps/contacts.md).

Holds `create`, `delete`, `export`, `get`, `groups`, `import`, `keys`, `list`, `merge` and `update`.

## `create`

Create a contact.

```
proton contacts create
```

```bash
proton contacts create --name 'Jane Roe' --email jane@example.com
proton contacts create --name 'Jane Roe' --email work:jane@acme.com --phone cell:+43123456 --anniversary 2015-06-20
proton contacts create --name 'Jane Roe' --email jane@example.com --phone '+43 660 1234567' --organization Acme
```

| Flag | Description |
| --- | --- |
| `--address stringArray` | Set a postal address, as ADDRESS or KIND:ADDRESS (repeatable) |
| `--anniversary string` | Set the anniversary (e.g. 2015-06-20) |
| `--birthday string` | Set the birthday (e.g. 1990-01-31) |
| `--email stringArray` | Set an email address, as ADDRESS or KIND:ADDRESS (repeatable) |
| `--first-name string` | Set the given name |
| `--gender string` | Set the gender |
| `--job-title string` | Set the job title |
| `--language string` | Set the preferred language (e.g. de-AT) |
| `--last-name string` | Set the family name |
| `--name string` | Set the name shown in listings |
| `--nickname string` | Set the nickname |
| `--note string` | Set the note |
| `--organization string` | Set the organization |
| `--phone stringArray` | Set a phone number, as NUMBER or KIND:NUMBER (repeatable) |
| `--role string` | Set the role played in the organization |
| `--timezone string` | Set the time zone (e.g. Europe/Vienna) |
| `--website stringArray` | Set a website, as URL or KIND:URL (repeatable) |

## `delete`

Delete contacts.

```
proton contacts delete REF...
```

```bash
proton contacts delete jane
```

## `export`

Write contacts out as .vcf files, or as one stream with --output -.

Named contacts are written; with none named, the whole address book is, narrowed by --keyword. Properties this tool has no flag for travel too, since the stored card goes out whole.

```
proton contacts export [REF...]
```

```bash
proton contacts export --output-dir ./address-book
proton contacts export jane --output jane.vcf
proton contacts export --output - > contacts.vcf
```

| Flag | Description |
| --- | --- |
| `--force` | Overwrite a file that already exists |
| `--keyword string` | Match text in the name or the address |
| `--output string` | Write to this path, or - for stdout |
| `--output-dir string` | Write into this directory, keeping each item's own name |

## `get`

Show one contact in full.

```
proton contacts get REF
```

```bash
proton contacts get jane@example.com
proton contacts get 'Jane Roe'
```

## `groups`

Contact groups.

Holds `add`, `create`, `delete`, `get`, `list`, `remove` and `update`.

### `groups add`

Add contacts to a group.

Proton groups addresses rather than people, so a colleague's work address can be in a group while their personal one is not. Naming a contact means all of their addresses; --email narrows it to the ones you name, and then exactly one contact may be named.

```
proton contacts groups add REF CONTACT_REF...
```

```bash
proton contacts groups add Team jane
```

| Flag | Description |
| --- | --- |
| `--email stringArray` | Act on this address only, rather than all of the contact's (repeatable) |

### `groups create`

Create a contact group.

```
proton contacts groups create
```

```bash
proton contacts groups create --name Team
proton contacts groups create --name Family --color strawberry
```

| Flag | Description |
| --- | --- |
| `--color string` | Accent color, by name (purple) or hex (#8080FF) (default `#8080FF`) |
| `--name string` | Group name |

### `groups delete`

Delete contact groups.

```
proton contacts groups delete REF...
```

```bash
proton contacts groups delete Team
```

### `groups get`

Show one group and the addresses in it.

```
proton contacts groups get REF
```

```bash
proton contacts groups get Team
```

### `groups list`

List contact groups.

```
proton contacts groups list
```

```bash
proton contacts groups list
```

### `groups remove`

Remove contacts from a group.

Proton groups addresses rather than people, so a colleague's work address can be in a group while their personal one is not. Naming a contact means all of their addresses; --email narrows it to the ones you name, and then exactly one contact may be named.

```
proton contacts groups remove REF CONTACT_REF...
```

```bash
proton contacts groups remove Team jane
```

| Flag | Description |
| --- | --- |
| `--email stringArray` | Act on this address only, rather than all of the contact's (repeatable) |

### `groups update`

Rename or recolor a contact group.

```
proton contacts groups update REF
```

```bash
proton contacts groups update Team --name Engineering
proton contacts groups update Team --color reef
```

| Flag | Description |
| --- | --- |
| `--color string` | New accent color, as a hex value |
| `--name string` | New group name |

## `import`

Read contacts in from a .vcf file, or from stdin with -.

Each card goes in whole, so a property this tool has no flag for survives the trip. A card with no name and no address is skipped and named, since there would be nothing to file it under.

Nothing is merged: importing the same file twice makes duplicates, because nothing here can tell a re-import from a file somebody edited.

```
proton contacts import PATH
```

```bash
proton contacts import contacts.vcf
proton contacts import - < exported.vcf
```

## `keys`

Public keys pinned to a contact.

Pinning a key means mail to that address is encrypted to the key you trust, rather than to whatever the server hands back.

Holds `list`, `pin` and `unpin`.

### `keys list`

List the keys pinned to a contact.

```
proton contacts keys list REF
```

```bash
proton contacts keys list jane
```

### `keys pin`

Pin a public key so mail to a contact is encrypted to it.

```
proton contacts keys pin REF
```

```bash
proton contacts keys pin jane --key jane-pubkey.asc
proton contacts keys pin jane --email jane@example.com --key - --no-encrypt
```

| Flag | Description |
| --- | --- |
| `--email string` | Which of the contact's addresses the key applies to |
| `--key string` | Armoured public key file (- for stdin) |
| `--no-encrypt` | Store the key for verification only, leaving encryption off |
| `--scheme string` | PGP scheme for recipients outside Proton: pgp-mime, pgp-inline |

### `keys unpin`

Remove the keys pinned to a contact.

```
proton contacts keys unpin REF
```

```bash
proton contacts keys unpin jane
proton contacts keys unpin jane --email jane@example.com
```

| Flag | Description |
| --- | --- |
| `--email string` | Which of the contact's addresses to unpin |

## `list`

List contacts.

```
proton contacts list
```

```bash
proton contacts list
proton contacts list --output json
```

| Flag | Description |
| --- | --- |
| `--desc` | Reverse the order |
| `--keyword string` | Match text in the name or the address |
| `--page int` | Which page of results, counting from zero |
| `--page-size int` | How many contacts per page (default `50`) |
| `--sort string` | Order by: name, email (default `name`) |

## `merge`

Fold duplicate contacts into one.

A shared address decides, compared case-insensitively; two entries merely sharing a name are not duplicates.

The oldest of each set is kept, so a group or a pinned key referring to it still does. Everything the others had is added, and nothing is overwritten.

```
proton contacts merge
```

```bash
proton contacts merge --dry-run
proton contacts merge
```

## `update`

Change a contact's details.

Only what you pass is replaced. --email and --phone replace the whole list rather than adding to it, so pass every address you want the contact to keep.

```
proton contacts update REF
```

```bash
proton contacts update jane --job-title 'Head of Design'
proton contacts update jane --email jane.roe@work.example --birthday 1990-04-16
```

| Flag | Description |
| --- | --- |
| `--address stringArray` | Replace a postal address, as ADDRESS or KIND:ADDRESS (repeatable) |
| `--anniversary string` | Replace the anniversary (e.g. 2015-06-20) |
| `--birthday string` | Replace the birthday (e.g. 1990-01-31) |
| `--email stringArray` | Replace an email address, as ADDRESS or KIND:ADDRESS (repeatable) |
| `--first-name string` | Replace the given name |
| `--gender string` | Replace the gender |
| `--job-title string` | Replace the job title |
| `--language string` | Replace the preferred language (e.g. de-AT) |
| `--last-name string` | Replace the family name |
| `--name string` | Replace the name shown in listings |
| `--nickname string` | Replace the nickname |
| `--note string` | Replace the note |
| `--organization string` | Replace the organization |
| `--phone stringArray` | Replace a phone number, as NUMBER or KIND:NUMBER (repeatable) |
| `--role string` | Replace the role played in the organization |
| `--timezone string` | Replace the time zone (e.g. Europe/Vienna) |
| `--website stringArray` | Replace a website, as URL or KIND:URL (repeatable) |

---

Every command also takes the [flags that work everywhere](README.md#flags-that-work-on-every-command).
