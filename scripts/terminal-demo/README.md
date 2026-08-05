# Terminal demo

The panel in the README is a recording of a real session, not a mock-up. `record.sh` runs proton-cli against a demo account inside a pty, and [freeze](https://github.com/charmbracelet/freeze) renders the captured ANSI into `assets/demo-dark.svg` and `assets/demo-light.svg`.

It runs against the `alt` profile, so its credentials come from the same profile-scoped variables every other profile uses. Point them at a throwaway account, never your own:

```bash
export PROTON_ALT_USER=throwaway@proton.me
export PROTON_ALT_PASSWORD=...

just demo-seed   # once: give the account something tidy to show
just demo        # record and render
```

## What's here

| File | Role |
| --- | --- |
| `record.sh` | Runs the session and writes it, ANSI and all, to stdout |
| `seed.sh` | Puts the messages, folder, and Pass items that the session shows into the account |
| `profile.sh` | Resolves the demo account, sourced by both scripts |
| `dark.json`, `light.json` | freeze window styling per theme |

## How it stays honest and stable

- **Real output.** Every line below a prompt comes from the binary. Edit `record.sh` to change what is shown, never the captured text.
- **A demo account, explicitly.** The session sends a message and uploads a file, and what it shows ends up in the README. Profile-scoped variables normally fall back to the unscoped `PROTON_USER` / `PROTON_PASSWORD`, so the scripts refuse to run unless `PROTON_ALT_USER` and `PROTON_ALT_PASSWORD` are both set. The account's Drive needs to have been opened once, since a brand-new account has no volume yet.
- **The profile comes from the environment**, not a `--profile` flag, so the recorded commands stay free of demo plumbing.
- **Fixed window.** `record.sh` sets the pty to 84 columns, so column widths don't depend on the machine that records.
- **Login happens before recording.** Otherwise the transcript would open on an authentication notice.
- **Colors come from the CLI.** proton-cli colors interactive output on its own; only the `$` prompt marker is added by the script.
- **Each panel states its own default text color.** Text that carries no ANSI color takes its color from freeze's syntax theme, and a theme that defines none leaves an invalid `fill` in the SVG that every renderer resolves differently. `just demo` therefore rewrites that one attribute to Proton's text token per panel, rather than depending on a theme (which would also override `background`).
- **Cleanup.** The uploaded file is deleted again; the seeded data stays so re-recording is cheap.

Dates and IDs change with every recording, so expect the SVGs to differ on each run.
