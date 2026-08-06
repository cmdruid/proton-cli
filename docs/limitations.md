# Limitations

proton-cli aims for parity with what the Proton web clients let you do. Some things it will never do, because Proton's platform doesn't allow it; others aren't built yet.

## Platform constraints

### Colors come from a fixed palette

Labels, folders, calendars, and contact groups accept only Proton's 20 accent colors. `--color` prints the allowed values when given anything else.

### Search and list are eventually consistent

`search` and `list` read Proton's server-side index, which lags a few seconds behind a change. A message you just sent might not appear yet; one you just deleted or unscheduled might still show up. The web client behaves the same way.

To confirm a change, run `get` on the ID the command printed instead of searching for the subject again.

### Some operations need your password again

Proton asks for your password again before its most destructive operations, even though you are already signed in. `calendar settings calendars delete` is the one the CLI reaches today. It asks in a terminal, or reads `PROTON_PASSWORD`.

### Signing in needs an authenticator app, not a security key

FIDO2 sign-in speaks WebAuthn, which needs a browser. An account whose only second factor is a security key cannot be used from proton-cli; adding a TOTP authenticator makes it work.

### CAPTCHAs need a display

Proton may ask for human verification at login. Release binaries embed a webview helper for it, but a headless machine has nowhere to draw it, and `go install` builds don't include the helper at all. See [Human verification](human-verification.md) for the workaround.

### A message cannot be put back into a mailbox

Proton exposes no endpoint, to any client, that ingests a message file into an existing mailbox. `mail messages export` therefore has no exact inverse: [`--eml`](commands/mail.md#import) reads a file back into a draft or a send, which is as close as the platform allows.

### An exported message is plaintext

Export decrypts, so the files it writes are readable by anything. The original `DKIM-Signature` and `ARC-*` headers no longer verify, since the body they signed was the encrypted one. The web client's own export behaves the same way.

## Not implemented yet

- **Mail**: no encryption and key management, no IMAP/SMTP tokens, no custom domains, no snooze.
- **Drive**: no search, no downloading an earlier revision (only restoring one), no renaming an album.
- **Calendar**: no calendar sharing, no subscribed (ICS) calendars, no import or export.
- **Contacts**: no vCard import or export.
- **Pass**: no item or vault sharing, no secure links, no password history.
- **Account**: no plan, billing, or user management. No password, two-factor or recovery changes. No **Easy Switch**, Proton's mailbox migration from Gmail and other IMAP providers. All of these are done at [account.proton.me](https://account.proton.me).

## Out of scope

proton-cli mirrors the Proton web clients for Mail, Drive, Calendar, Pass, and Contacts. Other Proton products (VPN, Wallet, Docs, Meet, Lumo, Authenticator) are not covered, and neither are endpoints that exist in the API but have no equivalent action in a web client.

For anything the commands don't reach, [`proton-cli api`](commands/api.md) sends raw authenticated requests to any endpoint.
