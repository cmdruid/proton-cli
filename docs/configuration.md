# Environment variables, files and global flags

There is no config file to maintain. Everything proton-cli reads comes from a flag you pass, an environment variable, or the session it saved when you signed in - and this page is the complete list of all three.

For signing in, profiles and sessions, see [Your Proton account](apps/account.md).

## Environment variables

Eight, and none of them can name an account.

| Variable | Description |
| --- | --- |
| `PROTON_PROFILE` | Active profile (default: `default`) |
| `PROTON_API_URL` | API base URL (default: `https://mail.proton.me/api`) |
| `NO_COLOR` | Set to any value, even empty, to turn colored output off ([no-color.org](https://no-color.org)) |
| `COLORTERM` | `truecolor` or `24bit` if your terminal takes 24-bit color and does not advertise it; only affects how exactly a color swatch is drawn ([why](design-notes.md#why-colour-is-asked-for-by-name)) |
| `PROTON_NO_INPUT` | Set to any value, even empty, to never prompt; a missing credential becomes an error |
| `PROTON_LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `PROTON_NO_UPDATE_CHECK` | Set to any value, even empty, to never look for a new release ([what that is](installation.md#updating)) |
| `PROTON_VERIFIED` | A human verification already solved, as the refusal printed it ([when you need it](troubleshooting.md#nothing-here-can-be-asked)) |

## Files on disk

| Path | Contents |
| --- | --- |
| `~/.config/proton-cli/sessions/<profile>.json` | Session tokens and the encrypted key password (mode `0600`) |
| `~/.config/proton-cli/idcache/<profile>.json` | Short-ID lookup table (see [Short IDs](language.md#short-ids)) |
| `~/.config/proton-cli/update-check.json` | When proton last looked for a new release ([why](installation.md#updating)) |

Those paths are the Linux ones. macOS uses `~/Library/Application Support/proton-cli/` and Windows uses `%APPDATA%\proton-cli\`. Nothing else is written.

## Global flags

| Flag | Effect |
| --- | --- |
| `-p`, `--profile NAME` | Which profile to act as |
| `-o`, `--output text\|json\|yaml` | Output format (default `text`) |
| `-n`, `--dry-run` | Preview a mutation without applying it |
| `-y`, `--yes` | Answer confirmation prompts with yes; needed by a script that removes things ([why](language.md#when-it-asks-first)) |
| `-q`, `--quiet` | Suppress the non-essential stderr output |
| `--full-ids` | Don't shorten IDs in interactive output |
| `--no-color` | Turn colored output off (env: `NO_COLOR`) |
| `--log-level debug\|info\|warn\|error` | Logging verbosity (env: `PROTON_LOG_LEVEL`) |
| `--no-input` | Never prompt; a missing credential becomes an error (env: `PROTON_NO_INPUT`) |
| `--verified TOKEN` | A human verification already solved, as the refusal printed it (env: `PROTON_VERIFIED`) |

The five you type most have a single-letter form, and they cluster - so `-qn` is a quiet dry run ([why only five](design-notes.md#why-one-flag-name-means-one-thing)).

`--api-url URL` points the CLI at a different API host. It works but is hidden from `--help`, because it is for developing proton rather than for using it.
