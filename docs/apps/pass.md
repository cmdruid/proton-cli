# Proton Pass from the command line

Logins, notes, cards, SSH keys, identities, aliases and two-factor codes in Proton Pass. Items are decrypted on your machine with the vault and item keys.

Every command and flag is in the [`proton pass` reference](../commands/pass.md). This page is the things people actually do.

An item takes two IDs to address, written as one token: `SHARE_ID/ITEM_ID`. A name or URL works instead.

## Find and read

```bash
proton pass items list --vault Work
proton pass items get github.com                # by name or URL
proton pass items totp github.com               # the current two-factor code
proton pass generate --length 32                # a new password, made locally
```

`get` prints the item's fields, including the password and the TOTP **secret**, to stdout. Pass stores the secret rather than the code, so `totp` is what works the current code out, and it reports how long that code has left.

`generate` reaches no account and needs no session. Its alphabet is Proton's own, which leaves out `i`, `o`, `l` and their capitals unless letters are all the password has. Every kind you ask for is guaranteed to appear, and a length too short to hold one of each is refused.

## Create and edit

Every type takes `--name`, and optionally `--vault`, `--note`, `--field NAME=VALUE` and `--hidden NAME=VALUE`.

```bash
proton pass items create --name GitHub --username roman --url github.com \
  --password "$(openssl rand -base64 24)" --totp-uri "otpauth://totp/GitHub?secret=..."

proton pass items create --type note --name "Door codes" --note "Front: 1234"
proton pass items create --type credit-card --name Visa --holder "Roman L" --number 4111111111111111 --expiry 2028-12 --cvv 123
proton pass items create --type wifi --name Home --ssid MyNetwork --password pw --security WPA2
proton pass items create --type ssh-key --name laptop --public-key "$(cat ~/.ssh/id_ed25519.pub)"
proton pass items create --type identity --name Me --full-name "Jane Roe" --email jane@example.com --city Vienna
proton pass items create --type custom --name "Staging server" --field "Host=10.0.0.5" --hidden "Root password=secret"

proton pass items update github.com --password "new-secret"
```

Types are `login` (the default), `note`, `credit-card`, `wifi`, `ssh-key`, `identity`, `alias` and `custom`. Identity stores thirty-one fields; the reference lists them. `update` takes the same flags as `create` and leaves anything you don't pass alone.

**Sections.** A field can name the heading it sits under, in the same token:

```bash
proton pass items create --type custom --name Router \
  --field "Network/SSID=home" --hidden "Network/Key=hunter2" \
  --field "Admin/URL=http://192.168.0.1" --hidden "Admin/Password=secret"
```

A field is identified by its section and name together, so `Network/Password` and `Admin/Password` are two fields. Only the types whose Pass editor offers headings can carry them: `custom`, `ssh-key`, `wifi` and `identity`.

A custom field can hold a two-factor secret too - the flag is `--totp-field`, not `--totp`, which is the code a re-authentication asks for:

```bash
proton pass items update GitHub --totp-field "Backup=otpauth://totp/GitHub?secret=JBSWY3DPEHPK3PXP"
```

## Trash and delete

```bash
proton pass items trash github.com
proton pass trash restore github.com
proton pass items delete github.com          # permanent
proton pass items trash --older-than 1y --type login --dry-run
```

`delete` and `trash empty` are permanent, so they show what would go and ask. So does a filtered `trash`, since the filter chose them rather than you. See [When it asks first](../language.md#when-it-asks-first).

## Vaults

```bash
proton pass vaults create --name Work
proton pass vaults update Work --description "Shared team logins" --icon 7 --color 3
proton pass vaults delete Work               # by name, or by share ID
```

Pass shows its icons and colours as a grid with no names, so the numbers are what there is. Deleting a vault takes everything in it, so it names the vault and asks first.

`proton pass items pin github.com` keeps an item at the top of the list.

## Aliases

Hide-my-email addresses that forward to your own mailboxes.

```bash
proton pass aliases options                   # available suffixes and mailboxes
proton pass aliases create --prefix shop --mailbox me@proton.me
```

Proton makes the address from your prefix, a word of its own, and the suffix. It picks a new word every time and only settles when the alias is made, so creating one tells you what it made:

```
✓ Created alias "shop" as shop.jasmine329@passinbox.com.
```

An alias is an item, so it is read and edited like one:

```bash
proton pass items get shop
proton pass items update shop --mailbox work@proton.me    # where its mail arrives
proton pass items update shop --display-name "Jane R"     # what recipients see
```

When an address starts attracting spam, **switch it off rather than delete it**. A disabled alias keeps its address and stops receiving; deleting it burns the address for good.

```bash
proton pass aliases disable shop
```

### Replying as an alias

An alias forwards mail to you, but a reply would leave from your real address and give it away. A contact is the answer. Proton mints a second address standing for one correspondent, and mail you send there reaches them as though the alias had written it.

```bash
proton pass aliases contacts create shopping seller@example.com --name "The seller"
proton pass aliases contacts list shopping                       # WRITE TO shows the address
proton pass aliases contacts block shopping seller@example.com
```

### Where aliases arrive

```bash
proton pass settings mailboxes create me@example.com
proton pass settings mailboxes verify me@example.com --code 123456
proton pass settings mailboxes delete me@example.com --transfer-to other@example.com
```

A new mailbox receives nothing until it answers: Proton emails it a code, and `verify` is where that code goes back. `resend` sends another and retires the one before it.

Deleting a mailbox needs somewhere for its aliases to go, which is what `--transfer-to` names. Without it, a mailbox that still has aliases is refused rather than quietly leaving them receiving nothing.

## Sharing a vault

```bash
proton pass vaults share add Work jane@proton.me --access editor
proton pass vaults share list Work
proton pass vaults share remove Work jane@proton.me
```

A vault is opened by its share key, and every item is sealed under that key. So sharing means handing over the key itself - **every rotation of it**, since an item made before the last rotation is still sealed under an older one.

The key goes out encrypted to their key and signed with yours. That is why this only works with another Proton account: an address Proton holds no keys for has nothing to encrypt to.

`--access` is `viewer`, `editor` or `manager`.

For a vault somebody gave you:

```bash
proton pass invitations list
proton pass invitations accept Work
```

The vault's name and item count are readable before you take it - the invitation carries the key that opens that much. What is *in* it is not, until you accept.

## Secure links

```bash
proton pass links create github.com --expires 7d
proton pass links create github.com --expires 24h --views 1
proton pass links revoke 5bH2mQxK
```

A link that shows one item to somebody with no Proton account. The item stays encrypted, and a key made for the link is what opens it.

**That key travels in the URL after the `#`**, which a browser never sends to the server. So the URL is the secret: anyone holding the whole of it can read the item until the link expires or is revoked.

`--expires` is required. A link nobody remembered to revoke is how one of these goes wrong, and there is no sensible default for how long a secret should outlive its reason.

`create` writes the URL to stdout and the warning to stderr, so `LINK=$(proton pass links create … --expires 7d)` captures the link alone. `list` shows whole URLs, key and all, so a link you mislaid is read back rather than revoked and made again.

## Backups

```bash
proton pass export --output pass-backup.zip --passphrase-file ~/.backup-passphrase
proton pass import pass-backup.zip --passphrase-file ~/.backup-passphrase
```

The archive is the one **Proton Pass itself writes**, so the app opens what this writes and this opens what the app wrote.

**Without a passphrase the archive holds every password in plain text**, and the command says so as it writes. With one, the document is encrypted to it and stored as `data.pgp`, which is what Proton's own importer looks for first. The passphrase comes from a file, from stdin with `--passphrase-stdin`, or from a prompt - never from a flag value ([why](../design-notes.md#why-a-password-is-never-a-flag-value)).

Importing **adds** items. Nothing in an export says which existing item it was, so importing the same file twice puts the items in twice. Items land in the vault the file names, and a vault that isn't there yet is made. `--dry-run` lists what would land, and where.

Aliases are the exception. An alias address belongs to the account Proton gave it to, so each one is named and skipped while everything else lands.

## History and breaches

```bash
proton pass items revisions list github.com    # every edit, newest first
proton pass breaches list                      # worst first
proton pass breaches get jane@proton.me
```

Pass keeps every edit, so a password changed by mistake can be read back. A revision written under a key this account no longer holds is still listed by its number.

`breaches` is Pass Monitor: which of your addresses have turned up in somebody else's data breach, when, and what was exposed. If a password leaked in the clear it shows the last few characters, which is what tells you which one to change. Nothing here writes.
