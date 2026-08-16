# Terminal demo

The panel in the README is a recording of a real session, not a mock-up. `record.sh` runs proton against the primary account inside a pty, and [freeze](https://github.com/charmbracelet/freeze) renders the captured ANSI into `assets/demo-dark.svg` and `assets/demo-light.svg`.

It records as `primary`, the same account the integration suite runs on, so it needs no credentials of its own:

```bash
just demo        # seed, record, render
```

## What's here

| File | Role |
| --- | --- |
| `record.sh` | Runs the session and writes it, ANSI and all, to stdout |
| `theme.sh` | Resolves the colour names in a recording into one of Proton's themes |
| `dark.json`, `light.json` | freeze window styling per theme |

## How it stays honest and stable

- **Real output.** Every line below a prompt comes from the binary. Edit `record.sh` to change what is shown, never the captured text.
- **The data is the suite's.** `scripts/seed` fills the account, and its names read like somebody's account for this reason. Change what the panel shows by changing the fixture there.
- **The suite's leftovers are swept first.** A test run that was killed cannot clear up after itself, and what it leaves sits in the same lists the panel photographs - most visibly in Pass and on the calendar, which have nowhere to hide a stray row. `scripts/seed` removes everything named under `proton-cli-test-` before recording.
- **The inbox is staged, not owned.** An account receives share notifications and Proton's own marketing, and neither can be kept out. `just demo` marks the inbox read and re-sends the three the panel shows, so they are the only unread mail whatever else has arrived.
- **One layout everywhere.** The tables and the progress bar are all drawn by `internal/ui`, so the panel shows the same shapes the rest of the CLI uses. `internal/ui`'s golden tests pin those bytes, which means a change visible in the SVG shows up in a failing test first.
- **The primary account, named here.** The session sends a message and uploads a file, and what it shows ends up in the README, so `just demo` seeds and stages `primary` before recording. The account's Drive needs to have been opened once, since a brand-new account has no volume yet.
- **The profile comes from the environment**, not a `--profile` flag, so the recorded commands stay free of demo plumbing.
- **Fixed window.** `record.sh` sets the pty to 84 columns, so column widths don't depend on the machine that records.
- **Login happens before recording.** `account login` runs first, so the transcript opens on a command rather than an authentication notice. It also unlocks the key hierarchy, so no recorded panel pauses to do that halfway through.
- **The panel is photographed in a Proton-themed terminal.** proton names colors and leaves the shade to the terminal, so a recording carries names rather than colors and something has to play the terminal's part before freeze can draw it - freeze has a palette of its own and no faint at all. `theme.sh` resolves those names against Proton's own tokens, carbon for the dark panel and snow for the light one, which is also the only reason the two panels can differ: the binary has no idea which one it is running in. Only the `$` prompt marker is added by the script, and it names its color the same way.
- **Only the last frame of each line survives.** A pty redraws the progress bar with carriage returns, so `just demo` strips them and keeps what stood on each line when it finished, which makes the recording read like the finished screen.
- **freeze is handed its input with stdin closed.** freeze reads stdin whenever stdin is a pipe and ignores the file it was given, so `just demo` redirects it from `/dev/null`; without that, recording from anything that pipes renders nothing and fails with `Language Unknown`.
- **Each panel states its own default text color.** Text that carries no ANSI color takes its color from freeze's syntax theme, and a theme that defines none leaves an invalid `fill` in the SVG that every renderer resolves differently. `just demo` therefore rewrites that one attribute to Proton's text token per panel, rather than depending on a theme (which would also override `background`).
- **One panel per app, and no shape twice in a row.** Mail, Drive, Calendar, Pass - a list, a transfer, one record, a list - so the panel reads as the breadth of the thing rather than four screenshots of the same table. The calendar event is asked for by its name rather than its ID, because that is how references are meant to be written.
- **No secret is recorded.** `pass items get` prints passwords and TOTP secrets in full, by design, which is why Pass is shown as a listing: the panel is published in the README and the account it records is real.
- **Nothing destructive is recorded.** The session lists, uploads, and lists again. The one thing it creates - the uploaded file - is deleted afterwards, off camera; the seeded data stays, so re-recording is cheap.

Dates and IDs change with every recording, so expect the SVGs to differ on each run.
