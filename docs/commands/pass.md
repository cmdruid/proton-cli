# Pass

Vaults, logins, secrets, and aliases. Items are decrypted locally with the vault and item keys.

Anywhere a command shows `SEARCH`, a name or URL works instead of `SHARE_ID ITEM_ID`.

## Items

### Find and read

```bash
proton-cli pass items list
proton-cli pass items list --vault Work
proton-cli pass items get github.com                # search by name or URL
proton-cli pass items get SHARE_ID ITEM_ID
```

`get` prints the item's fields, including the password and TOTP secret, to stdout.

### Create

Every type takes `--name`, optional `--vault`, `--note`, `--field NAME=VALUE`, and `--hidden NAME=VALUE`.

```bash
# Login (the default type)
proton-cli pass items create --name GitHub --username roman --password "$(openssl rand -base64 24)" \
  --url github.com --totp "otpauth://totp/GitHub?secret=..."

# Note
proton-cli pass items create --type note --name "Door codes" --note "Front: 1234"

# Credit card
proton-cli pass items create --type credit-card --name Visa \
  --holder "Roman L" --number 4111111111111111 --expiry 2028-12 --cvv 123 --pin 4321

# Wi-Fi
proton-cli pass items create --type wifi --name Home --ssid MyNetwork --password pw --security WPA2

# SSH key
proton-cli pass items create --type ssh-key --name laptop \
  --public-key "$(cat ~/.ssh/id_ed25519.pub)" --private-key "$(cat ~/.ssh/id_ed25519)"

# Identity
proton-cli pass items create --type identity --name Me --full-name "Jane Roe" \
  --email jane@example.com --phone "+43 1 234567" --city Vienna --country Austria

# Custom
proton-cli pass items create --type custom --name "Staging server" \
  --field "Host=10.0.0.5" --hidden "Root password=secret"
```

Custom fields work on any type:

```bash
proton-cli pass items create --name GitHub --field "Recovery codes=abc-def"
```

Types: `login`, `note`, `credit-card`, `wifi`, `ssh-key`, `identity`, `custom`. Wi-Fi security: `WPA`, `WPA2`, `WPA3`, `WEP`.

### Edit

```bash
proton-cli pass items edit github.com --password "new-secret"
proton-cli pass items edit github.com --totp "otpauth://totp/..."
proton-cli pass items edit "Staging server" --name "Staging server (eu-1)"
```

`edit` takes the same field flags as `create`.

### Trash, restore, delete

```bash
proton-cli pass items trash github.com
proton-cli pass items restore github.com
proton-cli pass items delete github.com      # permanent
```

With filters:

```bash
proton-cli pass items trash --vault Old --type login
proton-cli pass items trash --older-than 1y --type login
proton-cli pass items delete --vault Temporary --all
```

Filters: `--vault`, `--type`, `--older-than`, `--newer-than`, `--all`. Add `--dry-run` to check first.

## Vaults

```bash
proton-cli pass vaults list
proton-cli pass vaults create --name Work
proton-cli pass vaults rename SHARE_ID --name Personal
proton-cli pass vaults delete SHARE_ID
```

## Aliases

Hide-my-email aliases that forward to one of your mailboxes.

```bash
proton-cli pass alias options                              # available suffixes and mailboxes
proton-cli pass alias create --prefix shop --mailbox me@proton.me
proton-cli pass alias create --prefix shop --suffix @passmail.net --mailbox me@proton.me \
  --name "Online shops" --vault Personal
```
