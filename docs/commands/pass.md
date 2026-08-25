# Pass

Vaults, logins, secrets, and aliases. Items are decrypted locally with the vault and item keys.

An item takes two IDs to address, written as one token: `SHARE_ID/ITEM_ID`. A name or URL works instead.

## Items

### Find and read

```bash
proton pass items list
proton pass items list --vault Work
proton pass items list --type login --older-than 1y
proton pass items get github.com                # search by name or URL
proton pass items get SHARE_ID/ITEM_ID
```

`get` prints the item's fields, including the password and TOTP secret, to stdout.

### Create

Every type takes `--name`, optional `--vault`, `--note`, `--field NAME=VALUE`, and `--hidden NAME=VALUE`.

```bash
# Login (the default type)
proton pass items create --name GitHub --username roman --password "$(openssl rand -base64 24)" --url github.com --totp-uri "otpauth://totp/GitHub?secret=..."

# Note
proton pass items create --type note --name "Door codes" --note "Front: 1234"

# Credit card
proton pass items create --type credit-card --name Visa --holder "Roman L" --number 4111111111111111 --expiry 2028-12 --cvv 123 --pin 4321

# Wi-Fi
proton pass items create --type wifi --name Home --ssid MyNetwork --password pw --security WPA2

# SSH key
proton pass items create --type ssh-key --name laptop --public-key "$(cat ~/.ssh/id_ed25519.pub)" --private-key "$(cat ~/.ssh/id_ed25519)"

# Identity - Pass stores thirty-one fields; these are a few
proton pass items create --type identity --name Me --full-name "Jane Roe" --email jane@example.com --phone "+43 1 234567" --city Vienna --country Austria
proton pass items create --type identity --name Work --company Acme --work-email jane@acme.com --job-title Engineer --linkedin janeroe

# Custom
proton pass items create --type custom --name "Staging server" --field "Host=10.0.0.5" --hidden "Root password=secret"
```

Custom fields work on any type:

```bash
proton pass items create --name GitHub --field "Recovery codes=abc-def"
```

### Sections

A field can name the heading it sits under, and states it in the same token, so the flags can be given in any order and read back exactly as they were written:

```bash
proton pass items create --type custom --name Router \
  --field "Network/SSID=home" --hidden "Network/Key=hunter2" \
  --field "Admin/URL=http://192.168.0.1" --hidden "Admin/Password=secret"

proton pass items update Router --hidden "Network/Key=hunter3"
```

`update` names one field and leaves the rest alone, and a field is identified by its section and its name together - so `Network/Password` and `Admin/Password` are two fields, not one.

A custom field can hold a two-factor secret as well as text or a hidden value, and [`pass items totp`](#two-factor-codes) reads one:

```bash
proton pass items update GitHub --totp-field "Backup=otpauth://totp/GitHub?secret=JBSWY3DPEHPK3PXP"
```

The flag is `--totp-field` rather than `--totp`, which is the two-factor code a re-authentication asks for. A secret no code can come out of is refused before anything is sent.

Only the types whose Pass editor offers headings can carry them: `custom`, `ssh-key`, `wifi` and `identity`. Giving one to any other type is refused before anything is sent.

Types: `login`, `note`, `credit-card`, `wifi`, `ssh-key`, `identity`, `alias`, `custom`. Wi-Fi security: `WPA`, `WPA2`, `WPA3`, `WEP`.

An alias is made by [`pass aliases create`](#aliases), because Proton and not you decides its address; `items create --type alias` is how one is listed and edited.

### Edit

```bash
proton pass items update github.com --password "new-secret"
proton pass items update github.com --totp-uri "otpauth://totp/..."
proton pass items update "Staging server" --name "Staging server (eu-1)"
```

`update` takes the same field flags as `create`.

### Trash, restore, delete

```bash
proton pass items trash github.com
proton pass items delete github.com      # permanent
proton pass trash list                   # what is there to restore
proton pass trash restore github.com
proton pass trash empty                  # permanent, all of it
```

With filters:

```bash
proton pass items trash --vault Old --type login
proton pass items trash --older-than 1y --type login
proton pass items delete --vault Temporary --all
```

Filters: `--vault`, `--type`, `--older-than`, `--newer-than`, `--all`. Add `--dry-run` to check first.

`delete` and `trash empty` are permanent, so they show what would go and ask. So does a filtered `trash`, since the filter chose them rather than you. See [When it asks first](../language.md#when-it-asks-first).

## Vaults

```bash
proton pass vaults list
proton pass vaults create --name Work
proton pass vaults update SHARE_ID --name Personal
proton pass vaults delete Work            # by name, or by share ID
```

Deleting a vault takes everything in it, so it names the vault and asks first.

## Aliases

Hide-my-email aliases that forward to your own mailboxes.

```bash
proton pass aliases options                              # available suffixes and mailboxes
proton pass aliases create --prefix shop --mailbox me@proton.me
proton pass aliases create --prefix shop --suffix passinbox.com --mailbox me@proton.me --mailbox work@proton.me --name "Online shops" --vault Personal
```

Proton makes the address from the prefix you choose, a word of its own, and the suffix. `--suffix` takes the domain: the word in front of it is Proton's to pick, it picks a new one every time it is asked, and it only settles when the alias is made. So creating one says which address it made:

```
✓ Created alias "shop" as shop.jasmine329@passinbox.com.
```

An alias is an item, so it is read and edited like one:

```console
$ proton pass items get shop
Type            alias
Name            shop
Alias           shop.jasmine329@passinbox.com
Status          enabled
Forwards To     me@proton.me
Display Name    Jane R
Activity        12 forwarded, 0 replied, 3 blocked (last 14 days)
ID              Aq7…/9Kd…
```

```bash
proton pass items update shop --mailbox work@proton.me    # where its mail arrives
proton pass items update shop --display-name "Jane R"     # what recipients see it sent as
proton pass items update shop --name "Online shops"       # the item's own name
```

### Writing as an alias

An alias forwards mail to you, but a reply would leave from your real address and give it away. A contact is the answer: Proton mints a second address standing for one correspondent, and mail you send there reaches them as though the alias had written it.

```bash
proton pass aliases contacts create shopping seller@example.com --name "The seller"
proton pass aliases contacts list shopping
proton pass aliases contacts block shopping seller@example.com   # stop their mail reaching you
proton pass aliases contacts allow shopping seller@example.com
proton pass aliases contacts delete shopping seller@example.com
```

Creating one says which address to write to. `list` shows it under `WRITE TO`, beside how much each contact has sent.

### Where aliases arrive

```bash
proton pass settings mailboxes list
proton pass settings mailboxes create me@example.com
proton pass settings mailboxes verify me@example.com --code 123456
proton pass settings mailboxes resend me@example.com
proton pass settings mailboxes update me@example.com --default
proton pass settings mailboxes delete me@example.com --transfer-to other@example.com
proton pass settings domains list
```

A new mailbox receives nothing until it answers: Proton emails it a code, and `verify` is where that code goes back. `resend` sends another, which retires the one before it.

Deleting a mailbox needs somewhere for its aliases to go, which is what `--transfer-to` names. Without it, a mailbox that still has aliases is refused rather than quietly leaving them receiving nothing.

When an address starts attracting spam, switch it off rather than delete it - a disabled alias keeps its address and stops receiving, while deleting it burns the address for good:

```bash
proton pass aliases disable shop
proton pass aliases enable shop
proton pass aliases list                                  # STATUS says which are off
```

`--output json` carries the address as `alias` and the rest as `alias_status`, `alias_mailboxes`, `alias_display_name` and `alias_activity`.

## Vaults

```bash
proton pass vaults get Work
proton pass vaults update Work --description "Shared team logins" --icon 7 --color 3
```

Pass shows its icons and colours as a grid with no names, so the numbers are what there is: `--icon 7`, `--color 3`. Anything not mentioned is left alone, including a description written in the Pass app.

## Backups

```bash
proton pass export --output pass-backup.zip --passphrase-file ~/.backup-passphrase
proton pass import pass-backup.zip --passphrase-file ~/.backup-passphrase
```

The archive is the one **Proton Pass itself writes**, so the app can open what this writes and this can open what the app wrote. Inside is a single JSON document with every vault and every item, each item's contents in the encoding Pass stores them in.

**Without a passphrase the archive holds every password in plain text**, and the command says so as it writes. With one, the document is encrypted to it and stored as `data.pgp` - which is what Proton's own importer looks for first. The passphrase is read from a file, from stdin with `--passphrase-stdin`, or typed at a prompt; never from a flag value, since anything on the command line is visible to every user on the machine.

Reading a file back **adds** its items. Nothing in an export says which existing item it was, so importing the same file twice puts the items in twice. A vault the file names lands in the vault of that name, and one that is not there yet is made - `--dry-run` lists what would land, and where, before any of it does.

Aliases are the exception: an alias address belongs to the account Proton gave it to, so it cannot be recreated elsewhere. Each one is named and skipped, and everything else still lands.

## Sharing a vault

```bash
proton pass vaults share add Work jane@proton.me
proton pass vaults share add Work jane@proton.me --access editor
proton pass vaults share list Work
proton pass vaults share remove Work jane@proton.me
```

A vault is opened by its share key, and every item in it is sealed under that key. So sharing means handing over the key itself - **every rotation of it**, because an item made before the last rotation is still sealed under an older one, and somebody given only the newest would see a vault half of which will not open. It goes out encrypted to their key and signed with yours.

That is why it only works with another Proton account: an address Proton holds no keys for has nothing to encrypt to. `--access` is `viewer`, `editor` or `manager`.

### A vault somebody gave you

```bash
proton pass invitations list
proton pass invitations accept Work
proton pass invitations decline Work
```

The vault's name and how many items it holds are readable before you take it - the invitation carries the key that opens that much, encrypted to you. What is *in* the vault is not, until you accept.

Accepting moves the keys onto your own key, which is what makes the vault open like any other of yours afterwards.

## Secure links

```bash
proton pass links create github.com --expires 7d
proton pass links create github.com --expires 24h --views 1
proton pass links list
proton pass links revoke 5bH2mQxK
```

A link that shows one item to somebody with no Proton account. The item stays encrypted: a key made for the link is what opens it, and **that key travels in the URL after the `#`**, which a browser never sends to the server. So the URL is the secret - anyone who has the whole thing can read the item until the link expires or is revoked.

`--expires` is required. A link nobody remembered to revoke is how one of these goes wrong, and there is no sensible default for how long a secret should outlive the reason it was shared. `--views` stops it after a number of openings.

`create` writes the URL to stdout and the warning to stderr, so `LINK=$(proton pass links create … --expires 7d)` captures the link and nothing else.

`list` shows the whole URL, key and all: Proton stores that key sealed under the item's own, so a link you mislaid is read back rather than revoked and made again.

## Breaches

```bash
proton pass breaches list
proton pass breaches get jane@proton.me
```

Which of your addresses have turned up in somebody else's data breach - Proton calls it Pass Monitor. `list` puts the worst first, because the reason to run it is to find what to deal with. `get` names the breaches one address appeared in, when each happened, what it exposed, and the last few characters of the password if one leaked in the clear - which is what tells you which password to change.

Nothing here writes: it reports what somebody else already leaked.

## Two-factor codes

```bash
proton pass items totp github.com
proton pass items totp github.com --output json    # then read .code
```

Pass stores the **secret**, not the code, so every client works the code out for itself. `items get` prints the secret because that is what is stored; this prints what it currently stands for.

How long the code has left is reported beside it, because a code about to expire is one worth waiting out. A second factor kept in a custom field is found too.

## Making a password

```bash
proton pass generate
proton pass generate --length 32
proton pass generate --no-symbols --length 24
```

This reaches no account and needs no session: a password is made on this machine and may never leave it.

The alphabet is Proton's own, which leaves out `i`, `o`, `l` and their capitals - the characters people misread - unless letters are all the password has, since narrowing a 20-character password's alphabet for no reason would only weaken it.

Every kind asked for is **guaranteed** to appear, so a password that has to contain a digit does. A length too short to hold one of each is refused rather than silently dropping a kind.

## History

```bash
proton pass items revisions list github.com
proton pass items revisions list github.com --output json
```

Pass keeps every edit, so a password changed by mistake can be read back. Newest first; `--output json` carries the full contents of each revision.

A revision written under a key this account no longer holds is still listed, by its number - knowing it existed is worth more than hiding it.

## Pinning

```bash
proton pass items pin github.com
proton pass items unpin github.com
```

Pinning keeps an item at the top of the list. It carries no content, so nothing is encrypted or re-encrypted.
