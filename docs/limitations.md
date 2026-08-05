# Limitations

proton-cli aims for parity with what the Proton web clients let you do. Some things it will never do, because Proton's platform doesn't allow it; others simply aren't built yet.

## Platform constraints

### Colors come from a fixed palette

Labels, folders, calendars, and contact groups accept only Proton's 20 accent colors. `--color` is validated locally and prints the allowed values on error, because the API rejects anything else.

### Search and list are eventually consistent

`search` and `list` read Proton's server-side index rather than local state, and that index lags a few seconds behind a change. A message you just sent might not appear yet; one you just deleted or unscheduled might still show up. The web client behaves the same way, so there's nothing for the CLI to cache or invalidate.

To confirm a change, act on the ID the command printed with `read`, instead of searching for the subject again.

### Deleting a calendar needs your password

`calendar settings calendars delete` is a password-scoped operation, so `PROTON_PASSWORD` has to be set even when a saved session exists.

### CAPTCHAs need a display

Proton may ask for human verification at login. Release binaries embed a webview helper for it, but a headless machine has nowhere to draw it, and `go install` builds don't include the helper at all. See [Human verification](human-verification.md) for the workaround.

### A message cannot be put back into a mailbox

Proton exposes no endpoint, to any client, that ingests a message file into an existing mailbox. `mail messages export` therefore has no exact inverse: [`--eml`](commands/mail.md#import) reads a file back into a draft or a send, which is as close as the platform allows.

### An exported message is plaintext

Export decrypts, so the files it writes are readable by anything. The original `DKIM-Signature` and `ARC-*` headers also stop verifying, because the body has been rebuilt around the decrypted content. The web client's own export behaves the same way.

## Not implemented yet

- **Mail**: no encryption and key management, no IMAP/SMTP tokens, no custom domains.
- **Calendar**: no calendar sharing, no subscribed (ICS) calendars, no import or export.
- **Contacts**: no vCard import or export.
- **Pass**: no item or vault sharing, no secure links, no password history.
- **Account**: no plan, billing, or user management. No **Easy Switch**, Proton's mailbox migration from Gmail and other IMAP providers: it is a server-side job spanning four products that needs your old account's credentials or a browser sign-in, so it belongs to the account settings rather than to Mail.

## Out of scope

proton-cli mirrors the Proton web clients for Mail, Drive, Calendar, Pass, and Contacts. Other Proton products (VPN, Wallet, Docs, Meet, Lumo, Authenticator) are not covered, and neither are endpoints that exist in the API but have no equivalent action in a web client.

For anything the commands don't reach, [`proton-cli api`](commands/api.md) sends raw authenticated requests to any endpoint.
