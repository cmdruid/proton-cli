# Limitations

proton aims for parity with what the Proton web clients let you do. Some things it will never do, because Proton's platform doesn't allow it; others aren't built yet.

## Platform constraints

### Colors come from a fixed palette

Labels, folders, calendars, and contact groups accept only Proton's 20 accent colors. `--color` prints the allowed values when given anything else.

### Search and list are eventually consistent

`search` and `list` read Proton's server-side index, which lags a few seconds behind a change. A message you just sent might not appear yet; one you just deleted or unscheduled might still show up. The web client behaves the same way.

To confirm a change, run `get` on the ID the command printed instead of searching for the subject again.

### Some operations need your password again

Proton asks for your password again before some operations, even though you are already signed in. `calendar settings calendars delete` and `mail settings autoreply set` are the ones the CLI reaches today. It asks in a terminal, or takes `--password-file` / `--password-stdin`. The key password sealed into the session cannot stand in: it is a one-way derivation, and Proton re-authenticates against the password itself.

### Signing in needs an authenticator app, not a security key

FIDO2 sign-in speaks WebAuthn, which needs a browser. An account whose only second factor is a security key cannot be used from proton; adding a TOTP authenticator makes it work.

### CAPTCHAs need a display

Proton may ask for human verification at login. Release binaries embed a webview helper for it, but a headless machine has nowhere to draw it, and `go install` builds don't include the helper at all. See [Human verification](human-verification.md) for the workaround.

### A message cannot be put back into a mailbox

Proton exposes no endpoint, to any client, that ingests a message file into an existing mailbox. `mail messages export` therefore has no exact inverse: [`--eml`](commands/mail.md#import) reads a file back into a draft or a send, which is as close as the platform allows.

### An answer to an invitation covers the whole series

Proton lets you answer one occurrence of a recurring invitation differently from the rest, by storing a personal copy of that occurrence. proton does not: `calendar events respond` refuses a reference that names an occurrence and tells you to answer the series.

### A recurring event needs a zone that can be named

An event is anchored to an IANA time zone so that a series keeps its wall-clock time when the clocks change. proton reads that zone from `TZ`, from `/etc/localtime` or from `/etc/timezone`, and falls back to the zone your Proton calendar settings are drawn in. On a host where none of those answers - a Windows machine with no `TZ` set and no calendar setting - a new event is stored as a plain UTC instant instead, and a recurring one will drift by an hour across a daylight-saving change. Pass `--zone Europe/Vienna` to be explicit.

### An update is not signed by a key you can verify offline

`self update` checks the downloaded binary against the release's `checksums.txt`, which proves the bytes were not corrupted in transit but not that the release itself is the maintainer's: both come from the same origin. Closing that needs the checksums signed with a key whose public half is built into the tool. That is not in place yet.

### An exported message is plaintext

Export decrypts, so the files it writes are readable by anything. The original `DKIM-Signature` and `ARC-*` headers no longer verify, since the body they signed was the encrypted one. The web client's own export behaves the same way.

## Out of scope

proton mirrors the Proton web clients for Mail, Drive, Calendar, Pass, and Contacts. Other Proton products (VPN, Wallet, Docs, Meet, Lumo, Authenticator) are not covered, and neither are endpoints that exist in the API but have no equivalent action in a web client.

For anything the commands don't reach, [`proton api`](commands/api.md) sends raw authenticated requests to any endpoint.
