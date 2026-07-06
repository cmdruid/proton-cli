# Limitations

proton-cli aims for broad parity with the Proton web clients. The items below
fall into two groups: constraints that are **inherent** to Proton's design or
platform, and a short list of features **not yet implemented**.

## Inherent constraints

### Colors are restricted to Proton's accent palette

Labels, folders, calendars and contact groups accept only Proton's 20 fixed
accent colors. The CLI validates `--color` and prints the allowed hex values on
error; arbitrary colors are rejected by the API.

### `search` and `list` are eventually consistent

`search` and `list` read Proton's server-side index, not local state, and that index lags a few seconds behind a mutation: a message you just sent may not appear yet, and one you just deleted or unscheduled may still show up briefly. This is a property of Proton's backend - the same lag exists in the web client - so there is nothing for the CLI to cache or invalidate.

To verify a mutation reliably, act on the message ID (which `send` and the create commands print on stdout) with `read`, rather than re-running `search` on a subject.

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
