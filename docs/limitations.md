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

### Sending encrypted mail to non-Proton recipients

`mail messages send` encrypts end-to-end to Proton recipients and sends
cleartext (server-side TLS only) to external recipients — the default Proton
behavior. It does **not** implement:

- **External PGP** — encrypting to a recipient's own PGP/WKD key, or
- **Encrypted-for-outside** — password-protected message links.

Attachments and calendar invitations (a `METHOD:REQUEST` `.ics`) to external
recipients do work.

## Not yet implemented

- **Photos `favorite`** — Proton's favorite action copies a photo into a special
  favorites album whose discovery/bootstrap isn't wired up yet. Use
  `drive photos albums add` to organize photos in the meantime.
- **Attendee RSVP replies** — you can invite attendees to an event
  (`calendar events create --attendee`), but replying to an invitation
  (accept / tentative / decline) isn't implemented.
- **Adding calendar tags / new custom tags to photos** — only tag *removal*
  (`drive photos tags remove`) is supported; tags are otherwise assigned by
  Proton's automatic classification.
