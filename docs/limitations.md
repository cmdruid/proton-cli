# What proton-cli can't do

proton-cli aims for parity with what the Proton web clients let you do. Some things it will never do, because Proton's platform doesn't allow it; others aren't built yet. This page is the honest list of both.

## Platform constraints

| What you can't do | Why | Instead |
| --- | --- | --- |
| Sign in with a passkey held in a phone | The handoff runs over Bluetooth from a browser | Use a key you can plug in, or a TOTP code |
| Sign in on a headless machine | Proton may ask for a CAPTCHA, which needs a display | [Copy a session across](troubleshooting.md#signing-in-on-a-headless-machine) |
| Pick any colour for a label | Proton allows only its 20 accent colours | `--color` prints the palette when refused |
| Trust `list` immediately after a change | Proton's index is eventually consistent | `get` the ID the command printed ([why](design-notes.md#why-search-lags)) |
| Put a message file back into a mailbox | Proton exposes no such endpoint, to any client | `--eml` reads one into a draft or a send |
| Answer one occurrence of a recurring invitation | Needs a personal copy of that occurrence | Answer the whole series |
| Verify an update offline | Checksums and binaries share an origin, and are unsigned | Not closed yet; needs a built-in public key |

**Three commands ask for your password again**, because Proton guards their endpoints behind an elevated session:

- `calendar settings calendars delete`
- `mail messages expire`
- `mail settings autoreply set`

They prompt, or take `--password-file` / `--password-stdin`. The key password sealed into your session cannot stand in: it is a one-way derivation, and Proton re-authenticates against the password itself.

Proton answers some unrelated refusals with the same code. So one of these may ask for your password and then be refused anyway, for a reason no password would have fixed.

**A recurring event needs a zone that can be named.** proton reads it from `TZ`, `/etc/localtime` or `/etc/timezone`, then falls back to your Proton calendar settings. Where none of those answers, a new event is stored as a plain UTC instant - and a recurring one then drifts by an hour when the clocks change. Pass `--zone Europe/Vienna` to be sure.

**Exported mail is plaintext.** Export decrypts, so the files are readable by anything. The original `DKIM-Signature` and `ARC-*` headers no longer verify, since the body they signed was the encrypted one. The web client's export behaves the same way.

## Not built yet

Each has an equivalent in a web client, and each is unbuilt for a stated reason rather than an oversight.

| What's missing | Why |
| --- | --- |
| Drive computers and shared bookmarks | Neither exists until you use a desktop client or open somebody's link, so neither can be tested |
| Mail forwarding itself to another address | Needs an OpenPGP forwarding primitive the Go libraries don't implement |
| Mail forwarding to a non-Proton address | Proton emails that address a link its owner must follow |
| Moving a Pass item between vaults | Re-encrypting to another share loses access on failure rather than failing loudly |

## Out of scope

proton-cli mirrors the Proton web clients for Mail, Drive, Calendar, Pass and Contacts. Other Proton products - VPN, Wallet, Docs, Meet, Lumo, Authenticator - are not covered, and neither are endpoints that exist in the API but have no equivalent action in a web client.

Some things a web client does are out of reach from a terminal and stay that way: creating a passkey needs WebAuthn, adding a Zoom or Google Meet link needs that provider's OAuth, and importing from Gmail or IMAP needs the same. Drive's search builds its index in the browser rather than on the server, so `drive items list --pattern --recursive` walks the tree instead.

For anything the commands don't reach, [`proton api`](apps/api.md) sends raw authenticated requests to any endpoint.
