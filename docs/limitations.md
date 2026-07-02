# Limitations

proton-cli aims for broad parity with the Proton web clients. The items below
fall into two groups: constraints that are **inherent** to Proton's design or
platform, and a short list of features **not yet implemented**.

## Inherent constraints

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

- **Encrypting mail to a contact-pinned key.** `mail messages send` reaches
  external recipients via `--eo-password` (Encrypted Outside) or a public key
  Proton discovers automatically (WKD/keyserver); pinning a specific public key
  in Contacts and encrypting to it isn't wired up yet.
