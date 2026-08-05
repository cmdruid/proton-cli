# Contacts

Contacts, their pinned encryption keys, and groups. Contact cards are encrypted and signed with your user key.

`REF` is a contact ID, a name, or an email address.

## Contacts

```bash
proton-cli contacts list
proton-cli contacts get jane
proton-cli contacts create --name "Jane Roe" --email jane@example.com --phone "+43 1 234567"
proton-cli contacts create --name "John Doe" \
  --email john@example.com --email john@work.example \
  --title CTO --org "Example GmbH" --birthday 1990-01-31 \
  --address "Stephansplatz 1, 1010 Vienna" --url https://example.com --note "Met at conference"
proton-cli contacts update jane --email jane@newdomain.com
proton-cli contacts delete jane
```

`--email` and `--phone` are repeatable. On `update` they replace the existing values rather than adding to them.

## Pinned keys

Pinning a public key to a contact means mail to that address is encrypted to the key *you* trust, not just whatever the server hands back.

```bash
proton-cli contacts pin-key jane --key jane-pubkey.asc
proton-cli contacts pin-key jane --email jane@example.com --key -    # armored key on stdin
proton-cli contacts pin-key jane --key jane.asc --no-encrypt         # pin for verification only
proton-cli contacts pin-key jane --key jane.asc --scheme pgp-inline  # default: pgp-mime
proton-cli contacts unpin-key jane
proton-cli contacts unpin-key jane --email jane@example.com
```

`--email` picks which of the contact's addresses the key applies to when there are several.

## Groups

```bash
proton-cli contacts groups list
proton-cli contacts groups create --name Team --color "#8080FF"
proton-cli contacts groups add GROUP_ID jane john
proton-cli contacts groups remove GROUP_ID jane
proton-cli contacts groups delete GROUP_ID
```

Group colors have to be Proton accent colors; an invalid value prints the allowed list.
