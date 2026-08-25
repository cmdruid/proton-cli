# Limitations

proton aims for parity with what the Proton web clients let you do. Some things it will never do, because Proton's platform doesn't allow it; others aren't built yet.

## Platform constraints

### Colors come from a fixed palette

Labels, folders, calendars, and contact groups accept only Proton's 20 accent colors. `--color` prints the allowed values when given anything else.

### Search and list are eventually consistent

`list` reads Proton's server-side index, which lags a few seconds behind a change. A message you just sent might not appear yet; one you just deleted or unscheduled might still show up. The web client behaves the same way.

To confirm a change, run `get` on the ID the command printed instead of searching for the subject again.

### Some operations need your password again

Proton asks for your password again before some operations, even though you are already signed in. `calendar settings calendars delete`, `mail settings autoreply set` and `mail messages expire` are the ones the CLI reaches today. It asks in a terminal, or takes `--password-file` / `--password-stdin`. The key password sealed into the session cannot stand in: it is a one-way derivation, and Proton re-authenticates against the password itself.

Proton answers "your session is not elevated" and some of its other refusals with the same code, so a command may ask for your password and then be refused for a reason a password was never going to fix. The refusal it prints is Proton's own.

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

`proton update` checks the downloaded binary against the release's `checksums.txt`, which proves the bytes were not corrupted in transit but not that the release itself is the maintainer's: both come from the same origin. Closing that needs the checksums signed with a key whose public half is built into the tool. That is not in place yet.

### An exported message is plaintext

Export decrypts, so the files it writes are readable by anything. The original `DKIM-Signature` and `ARC-*` headers no longer verify, since the body they signed was the encrypted one. The web client's own export behaves the same way.

## Not built yet

These have an equivalent in a web client, so they belong here eventually. Each is unbuilt for a stated reason rather than an oversight.

### Drive computers and shared bookmarks are not listed

Proton Drive's desktop clients register as "computers", and a public link you have opened is kept as a bookmark. A computer only exists once you install Proton Drive's desktop client, and a bookmark only once you open somebody's link in the web client, so neither can be exercised from here - and building against a shape nothing can ever return is how a command that never worked ships looking like it does.

### Mail cannot be set to forward itself

Forwarding to another Proton address is end-to-end encrypted, which Proton does by handing the forwardee a re-encryption key rather than your own: the request carries a forwardee private key, an activation token and a set of proxy instances. Those come from an OpenPGP forwarding primitive that the Go libraries this is built on do not implement, so there is nothing here to generate them with.

Forwarding to an address outside Proton needs no keys, but Proton emails that address a link the owner has to follow before anything is forwarded - a flow that ends somewhere this tool cannot reach.

### A Pass item cannot move between vaults

Moving one re-encrypts its keys to the destination share. Getting that wrong loses access to the item rather than failing loudly, so it waits on being able to verify a round trip.

## Out of scope

proton mirrors the Proton web clients for Mail, Drive, Calendar, Pass, and Contacts. Other Proton products (VPN, Wallet, Docs, Meet, Lumo, Authenticator) are not covered, and neither are endpoints that exist in the API but have no equivalent action in a web client.

Some things a web client does are out of reach from a terminal, and stay that way: creating a passkey and signing in with a security key need WebAuthn, which needs a browser; adding a Zoom or Google Meet link to an event needs that provider's OAuth; and importing from Gmail or an IMAP host needs the same. Drive's search builds an encrypted index in the browser rather than on the server, so `proton drive items list --pattern --recursive` walks the tree instead.

For anything the commands don't reach, [`proton api`](commands/api.md) sends raw authenticated requests to any endpoint.
