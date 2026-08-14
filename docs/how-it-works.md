# How it works

proton-cli talks to the same API as the Proton web apps, with the same authentication and the same encryption. Nothing proxies your data, and no server other than Proton's sees it.

## Logging in

1. **Session** - an unauthenticated session is created via `POST /auth/v4/sessions`.
2. **SRP** - login runs [Secure Remote Password](https://en.wikipedia.org/wiki/Secure_Remote_Password_protocol) through Proton's own [go-srp](https://github.com/ProtonMail/go-srp), so your password is never sent to the server, not even hashed. Two-factor codes are handled in the same exchange.
3. **Key password** - the salted password derived during login is what unlocks your PGP keys. It stays on your machine.
4. **Session file** - tokens plus the key password (encrypted with a random client key held server-side) are written to `~/.config/proton-cli/sessions/<profile>.json` with mode `0600`, so later commands don't re-authenticate. [Security](../SECURITY.md) documents this storage model in full.
5. **Refresh** - expired access tokens are refreshed automatically.

Proton may occasionally require human verification during step 2. See [Human verification](human-verification.md).

## The key hierarchy

Proton's encryption is a tree, and proton-cli walks the same one as the web client, using [gopenpgp](https://github.com/ProtonMail/gopenpgp):

```
key password
  └── user key
        ├── address keys        (mail, signing)
        ├── calendar keys       (events)
        ├── drive node keys     (files and folders)
        ├── contact encryption  (contact cards)
        └── pass vault keys     (vault and item keys)
```

Unlocking happens lazily, and alongside everything else: a command that only lists metadata never touches your keys, and one that decrypts asks for them at the same time as the content they will open, so the keys cost no round trip of their own. Below the user key, only the branch a command actually reads is unwrapped - a calendar's keys, a vault's, a file's.

## What is encrypted with what

| Content | Encrypted with | Signed with |
| --- | --- | --- |
| Mail bodies and attachments | Session key per message | Address key |
| Calendar events | Calendar key | Address key |
| Drive file contents | Node key, per block | Address key |
| Drive file and folder names | Parent node key | Address key |
| Contact cards | User key | User key |
| Pass items | AES-256-GCM item key | symmetric, no signature |
| Pass vaults | AES-256-GCM vault key | symmetric, no signature |

Reading works the other way around: content arrives encrypted, gets decrypted locally, and signatures are verified against the sender's key. `mail messages get` reports the verdict on a `Sig:` line.

## What leaves your machine

- API requests to `https://mail.proton.me/api` over HTTPS, authenticated with your session tokens.
- Encrypted payloads you asked to create: an encrypted message, an encrypted file block, an encrypted event.
- The SRP proof during login, which does not reveal your password.

What never leaves: your password, your key password, and your private keys.

## Where the API definitions come from

The endpoint shapes are generated from Proton's own open-source [web client](https://github.com/ProtonMail/WebClients) into [`openapi.yaml`](../openapi.yaml), covering roughly 740 endpoints. A weekly workflow regenerates it, so the CLI tracks upstream changes rather than guessing.

## Elevated operations

Proton guards its most destructive endpoints behind an elevated session scope. A request that needs one and hasn't got it comes back refused, and the client is expected to prove a human is present: re-run SRP against a scope-granting endpoint, retry the request, then drop the scope again.

proton-cli handles that in the transport layer, the way the web clients do, rather than in each command. So no command has to know which operations are guarded - it runs, the server asks, your password is requested once, the request is retried, and the elevation is dropped immediately afterwards.

Both halves of SRP are verified, on the initial sign-in and on every elevation: the server has to prove it knows your verifier just as you prove you know your password.

## Two-factor authentication

A TOTP code is asked for only when the account actually has one enabled, so a code is never wasted on a guess.

Security keys (FIDO2/WebAuthn) need a browser, so proton-cli cannot sign in with one.
