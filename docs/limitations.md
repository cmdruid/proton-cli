# Limitations

proton-cli aims for broad parity with the Proton web clients. The items below
fall into two groups: constraints that are **inherent** to Proton's design or
platform, and a short list of features **not yet implemented**.

## Inherent constraints

### Colors are restricted to Proton's accent palette

Labels, folders, calendars and contact groups accept only Proton's 20 fixed
accent colors. The CLI validates `--color` and prints the allowed hex values on
error; arbitrary colors are rejected by the API.

### Calendar deletion requires your password

`calendar calendars delete` performs a password-scoped operation, so it needs
`PROTON_PASSWORD` to be set even when you authenticated with a stored session.

### CAPTCHA cannot be solved headlessly

Proton may require human verification (CAPTCHA) at login. Release binaries embed
a small webview helper to solve it, but:

- a headless environment (server, container, no GUI) can't display it, and
- `go install` builds don't include the helper at all.

See [Human Verification](../README.md#human-verification-captcha). Run the
command on a desktop machine, or install a release binary, to get past a CAPTCHA.

## Not yet implemented

- **Adding custom tags to photos** — the Favorites tag can be toggled with
  `drive photos favorite` / `unfavorite`, and any tag removed with
  `drive photos tags remove`, but the other classification tags (screenshots,
  videos, selfies, …) are assigned only by Proton's automatic classification
  and can't be added manually.
- **Encrypting mail to a contact-pinned key.** `mail messages send` reaches
  external recipients via `--eo-password` (Encrypted Outside) or a public key
  Proton discovers automatically (WKD/keyserver); pinning a specific public key
  in Contacts and encrypting to it isn't wired up yet.
