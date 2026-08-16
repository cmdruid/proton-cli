# Contacts

Contacts, their pinned encryption keys, and groups. Contact cards are encrypted and signed with your user key.

`REF` is a contact ID, a name, or an email address.

## Contacts

```bash
proton contacts list
proton contacts get jane
proton contacts create --name "Jane Roe" --email jane@example.com --phone "+43 1 234567"
proton contacts create --name "John Doe" --email john@example.com --email john@work.example --job-title CTO --organization "Example GmbH" --birthday 1990-01-31 --address "Stephansplatz 1, 1010 Vienna" --website https://example.com --note "Met at conference"
proton contacts update jane --email jane@newdomain.com
proton contacts delete jane
```

`--email` and `--phone` are repeatable. On `update` they replace the existing values rather than adding to them.

## Pinned keys

Pinning a public key to a contact means mail to that address is encrypted to the key *you* trust, not just whatever the server hands back.

```bash
proton contacts keys pin jane --key jane-pubkey.asc
proton contacts keys pin jane --email jane@example.com --key -    # armored key on stdin
proton contacts keys pin jane --key jane.asc --no-encrypt         # pin for verification only
proton contacts keys pin jane --key jane.asc --scheme pgp-inline  # default: pgp-mime
proton contacts keys unpin jane
proton contacts keys unpin jane --email jane@example.com
```

`--email` picks which of the contact's addresses the key applies to when there are several.

## Groups

```bash
proton contacts groups list
proton contacts groups create --name Team --color "#8080FF"
proton contacts groups add GROUP_ID jane john
proton contacts groups remove GROUP_ID jane
proton contacts groups delete Team                # by name, or by group ID
```

Group colors have to be Proton accent colors; an invalid value prints the allowed list.
