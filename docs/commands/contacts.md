# Contacts

Contacts, their pinned encryption keys, and groups. Contact cards are encrypted and signed with your user key.

`REF` is a contact ID, a name, or an email address.

## Contacts

```bash
proton contacts list
proton contacts list --keyword jane --sort email
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
proton contacts groups get Team                   # and who is in it
proton contacts groups create --name Team --color "#8080FF"
proton contacts groups add Team jane john
proton contacts groups remove Team jane
proton contacts groups delete Team                # by name, or by group ID
```

Group colors have to be Proton accent colors; an invalid value prints the allowed list.

A listing of groups cannot say who is in one: Proton keeps membership on the address rather than on the group, so `get` is what asks the addresses.

## Merge duplicates

```bash
proton contacts merge --dry-run    # what it would fold, and into what
proton contacts merge
```

Two entries reachable at the **same address** are one person; two merely sharing a name are not, since people are routinely called the same thing. Addresses are compared case-insensitively.

The oldest of each set is kept, so a group or a pinned key that refers to it still does afterwards. Everything the others had that it did not is added, and nothing it already had is overwritten.

## Export and import

```bash
proton contacts export --output-dir ./address-book     # one .vcf per contact
proton contacts export --output - > contacts.vcf       # one file, all of them
proton contacts export jane --output jane.vcf
proton contacts import contacts.vcf
proton contacts import - < exported.vcf
```

A contact is stored as several cards - a signed one carrying the identity, an encrypted one carrying everything else - and a file has to be one card with all of them, so export merges them. Import splits them the same way on the way back in.

**A property this tool has no flag for still travels.** The stored card goes out and in whole, so an anniversary, a photo or a second postal address survives a round trip even though no flag sets one.

**An import is addressed by UID.** A card carries the UID of the contact it is, so reading a file back changes that contact rather than making a second one: export, edit the file, import, and the address book says what the file says. A card with no name and no address is skipped and named; the rest still land.

## Fields

Every field takes the same flag on `create` and `update`:

```bash
proton contacts create --name "Jane Roe" \
  --first-name Jane --last-name Roe --nickname Janey \
  --email work:jane@acme.com --email home:jane@example.com \
  --phone cell:+43123456 --address home:"1 Example St, Vienna" \
  --website work:https://acme.com \
  --organization Acme --job-title Engineer --role "Team lead" \
  --birthday 1990-01-31 --anniversary 2015-06-20 \
  --gender female --language de-AT --timezone Europe/Vienna \
  --note "Likes tea"
```

`--email`, `--phone`, `--address` and `--website` are repeatable and may say what kind they are, the way Proton's own editor offers one on each. A bare value states no kind, which vCard distinguishes from `other`.

| Field | Kinds |
| --- | --- |
| `--email` | `home`, `work`, `other` |
| `--phone` | `home`, `work`, `other`, `cell`, `main`, `fax`, `pager` |
| `--address` | `home`, `work`, `other` |
| `--website` | `home`, `work`, `other` |

A word before the colon that is not one of these is part of the value, so `--website https://example.com` keeps its scheme.

## Groups act on addresses

Proton groups **addresses**, not people, so a colleague's work address can be in the team group while their personal one is not.

```bash
proton contacts groups add Team jane                          # all of Jane's addresses
proton contacts groups add Team jane --email jane@acme.com    # only that one
```

With `--email`, exactly one contact may be named - which address belongs to whom is a question only one contact can answer.

Naming a contact really does mean all of their addresses. Proton has an endpoint that takes contacts rather than addresses, but it labels one address per contact, so this resolves them and groups every one.
