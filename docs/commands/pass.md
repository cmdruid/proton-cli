# Pass

Vaults, logins, secrets, and aliases. Items are decrypted locally with the vault and item keys.

An item takes two IDs to address, written as one token: `SHARE_ID/ITEM_ID`. A name or URL works instead.

## Items

### Find and read

```bash
proton-cli pass items list
proton-cli pass items list --vault Work
proton-cli pass items get github.com                # search by name or URL
proton-cli pass items get SHARE_ID/ITEM_ID
```

`get` prints the item's fields, including the password and TOTP secret, to stdout.

### Create

Every type takes `--name`, optional `--vault`, `--note`, `--field NAME=VALUE`, and `--hidden NAME=VALUE`.

```bash
# Login (the default type)
proton-cli pass items create --name GitHub --username roman --password "$(openssl rand -base64 24)" --url github.com --totp-uri "otpauth://totp/GitHub?secret=..."

# Note
proton-cli pass items create --type note --name "Door codes" --note "Front: 1234"

# Credit card
proton-cli pass items create --type credit-card --name Visa --holder "Roman L" --number 4111111111111111 --expiry 2028-12 --cvv 123 --pin 4321

# Wi-Fi
proton-cli pass items create --type wifi --name Home --ssid MyNetwork --password pw --security WPA2

# SSH key
proton-cli pass items create --type ssh-key --name laptop --public-key "$(cat ~/.ssh/id_ed25519.pub)" --private-key "$(cat ~/.ssh/id_ed25519)"

# Identity
proton-cli pass items create --type identity --name Me --full-name "Jane Roe" --email jane@example.com --phone "+43 1 234567" --city Vienna --country Austria

# Custom
proton-cli pass items create --type custom --name "Staging server" --field "Host=10.0.0.5" --hidden "Root password=secret"
```

Custom fields work on any type:

```bash
proton-cli pass items create --name GitHub --field "Recovery codes=abc-def"
```

Types: `login`, `note`, `credit-card`, `wifi`, `ssh-key`, `identity`, `custom`. Wi-Fi security: `WPA`, `WPA2`, `WPA3`, `WEP`.

### Edit

```bash
proton-cli pass items update github.com --password "new-secret"
proton-cli pass items update github.com --totp-uri "otpauth://totp/..."
proton-cli pass items update "Staging server" --name "Staging server (eu-1)"
```

`edit` takes the same field flags as `create`.

### Trash, restore, delete

```bash
proton-cli pass items trash github.com
proton-cli pass items delete github.com      # permanent
proton-cli pass trash list                   # what is there to restore
proton-cli pass trash restore github.com
proton-cli pass trash empty                  # permanent, all of it
```

With filters:

```bash
proton-cli pass items trash --vault Old --type login
proton-cli pass items trash --older-than 1y --type login
proton-cli pass items delete --vault Temporary --all
```

Filters: `--vault`, `--type`, `--older-than`, `--newer-than`, `--all`. Add `--dry-run` to check first.

`delete` and `trash empty` are permanent, so they show what would go and ask. So does a filtered `trash`, since the filter chose them rather than you. See [When it asks first](../language.md#when-it-asks-first).

## Vaults

```bash
proton-cli pass vaults list
proton-cli pass vaults create --name Work
proton-cli pass vaults update SHARE_ID --name Personal
proton-cli pass vaults delete Work            # by name, or by share ID
```

Deleting a vault takes everything in it, so it names the vault and asks first.

## Aliases

Hide-my-email aliases that forward to your own mailboxes.

```bash
proton-cli pass aliases options                              # available suffixes and mailboxes
proton-cli pass aliases create --prefix shop --mailbox me@proton.me
proton-cli pass aliases create --prefix shop --suffix @passmail.net --mailbox me@proton.me --mailbox work@proton.me --name "Online shops" --vault Personal
```

Proton makes the address from the prefix you choose plus a word of its own, so creating one says which address it made:

```
✓ Created alias "shop" as shop.jasmine329@passinbox.com.
```

An alias is an item, so it is read and edited like one:

```console
$ proton-cli pass items get shop
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
proton-cli pass items update shop --mailbox work@proton.me    # where its mail arrives
proton-cli pass items update shop --display-name "Jane R"     # what recipients see it sent as
proton-cli pass items update shop --name "Online shops"       # the item's own name
```

When an address starts attracting spam, switch it off rather than delete it - a disabled alias keeps its address and stops receiving, while deleting it burns the address for good:

```bash
proton-cli pass aliases disable shop
proton-cli pass aliases enable shop
proton-cli pass aliases list                                  # STATUS says which are off
```

`--output json` carries the address as `alias` and the rest as `alias_status`, `alias_mailboxes`, `alias_display_name` and `alias_activity`.
