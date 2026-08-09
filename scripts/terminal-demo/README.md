# Terminal demo

The panel in the README is a recording of a real session, not a mock-up. `record.sh` runs proton-cli against the primary account inside a pty, and [freeze](https://github.com/charmbracelet/freeze) renders the captured ANSI into `assets/demo-dark.svg` and `assets/demo-light.svg`.

It records as `primary`, the same account the integration suite runs on, so it needs no credentials of its own:

```bash
just demo        # seed, record, render
```

## What's here

| File | Role |
| --- | --- |
| `record.sh` | Runs the session and writes it, ANSI and all, to stdout |
| `dark.json`, `light.json` | freeze window styling per theme |

## How it stays honest and stable

- **Real output.** Every line below a prompt comes from the binary. Edit `record.sh` to change what is shown, never the captured text.
- **The data is the suite's.** `scripts/seed` fills the account, and its names read like somebody's account for this reason. Change what the panel shows by changing the fixture there.
- **The inbox is staged, not owned.** An account receives share notifications and Proton's own marketing, and neither can be kept out. `just demo` marks the inbox read and re-sends the three the panel shows, so they are the only unread mail whatever else has arrived.
- **One layout everywhere.** The tables, the confirmation, the progress bar and the dry-run preview are all drawn by `internal/ui`, so the panel shows the same shapes the rest of the CLI uses. `internal/ui`'s golden tests pin those bytes, which means a change visible in the SVG shows up in a failing test first.
- **The primary account, named here.** The session sends a message and uploads a file, and what it shows ends up in the README, so `just demo` seeds and stages `primary` before recording. The account's Drive needs to have been opened once, since a brand-new account has no volume yet.
- **The profile comes from the environment**, not a `--profile` flag, so the recorded commands stay free of demo plumbing.
- **Fixed window.** `record.sh` sets the pty to 84 columns, so column widths don't depend on the machine that records.
- **Login happens before recording.** `account login` runs first, so the transcript opens on a command rather than an authentication notice. It also unlocks the key hierarchy, so no recorded panel pauses to do that halfway through.
- **Colors come from the CLI.** proton-cli colors interactive output on its own; only the `$` prompt marker is added by the script.
- **Each panel states its own default text color.** Text that carries no ANSI color takes its color from freeze's syntax theme, and a theme that defines none leaves an invalid `fill` in the SVG that every renderer resolves differently. `just demo` therefore rewrites that one attribute to Proton's text token per panel, rather than depending on a theme (which would also override `background`).
- **Nothing destructive is recorded.** The session lists, uploads, and *previews* a cleanup with `--dry-run`, which changes nothing. The one thing it does create - the uploaded file - is deleted afterwards; the seeded data stays, so re-recording is cheap.

Dates and IDs change with every recording, so expect the SVGs to differ on each run.
