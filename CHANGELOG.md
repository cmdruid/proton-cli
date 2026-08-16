# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Adding a version section here is what publishes a release, so this file is the one place a version is decided: see [Releases](CONTRIBUTING.md#releases). Versions that shipped before this file existed are on the [releases page](https://github.com/roman-16/proton-cli/releases).

## [2.5.0] - 2026-08-17

### Added

- An install no package manager owns says when a release lands: once a day, under the command's own output. `PROTON_NO_UPDATE_CHECK=1` ends it, and a package-managed copy, a pipe and `--quiet` never see it.
- `proton changelog` says what a release changed, including one you have not installed. `--since` and `--until` cover the ground between two.
- Windows on ARM64 has its own build. winget, npm, the PowerShell installer and `proton update` hand an ARM64 machine a native binary; npm had none for it at all.

### Changed

- `update --dry-run` and `uninstall --dry-run` no longer ask you to sign in: they change this machine, not your account.

### Fixed

- Tab completion answers again - every shell's completion script was being refused as a mistyped command.

## [2.4.1] - 2026-08-16

### Changed

- `drive items upload --recursive` sends a deep tree in fewer round trips.

### Fixed

- `drive folders create /a/b/c` makes the folders above `c` instead of failing, and says how many it made.
- `drive folders create` names the file standing in the path instead of failing with `Link has no hash key`.

## [2.4.0] - 2026-08-16

### Changed

- The command is `proton`. `proton-cli` stays on your `PATH` as a second name, so nothing already written breaks and upgrading does not sign you out.
- **Breaking.** `go install github.com/roman-16/proton-cli/cmd/proton@latest` - the path gained `/cmd/proton`. Every other way of installing is unchanged.
- Colours are asked for by name, so output follows your terminal's theme instead of overruling it. Swatches keep their exact colour.

## [2.3.0] - 2026-08-15

### Added

- `drive items revisions download PATH REVISION_REF` reads an earlier version out to disk or into a pipe, leaving the file as it is.
- `drive items revisions delete PATH REVISION_REF` removes an earlier version permanently.
- `pass aliases disable REF` and `pass aliases enable REF` stop and start an alias receiving mail without burning the address.
- `--if-exists replace|rename|skip` on `drive items upload` answers a name Drive already has: a new revision, both under a numbered name, or nothing. Without it an upload still refuses.
- `pass items get` and `pass items update` handle aliases as themselves: the address, whether it is on, where it forwards, `--mailbox` and `--display-name`.
- `pass aliases create` prints the address Proton made, since it adds a word to your prefix.
- Single-letter forms for the global flags: `-p`, `-o`, `-n`, `-q`, `-y`. They cluster, so `-qn` is a quiet dry run.
- A caveat prints as its own `!` line on stderr, rather than as commentary above the green tick.

### Changed

- **Breaking.** An unrecognised reference exits `3` (not found) rather than `1`. Update scripts that read `1` as "no such thing".
- **Breaking.** A bad command line exits `1` (user error) rather than `2` (authentication), even when signed out.
- Colour marks only what carries a verdict or a colour of its own; every verdict is still spelled out in words.
- Label, folder, calendar and group lists show a colour as its swatch and name rather than a hex code.
- The `FLAGS` column counts attachments instead of drawing `📎`, which no monospace font sizes correctly.
- Tables measure width in terminal cells, so CJK subjects and emoji filenames stay aligned.
- An empty result reads `No messages match.` after a filter, rather than `No messages.`
- Transfers show speed and time left, and number themselves within a batch (`[3/27]`).
- Commands send independent requests together and never fetch the same thing twice, so they finish in fewer round trips.
- Commands that decrypt no longer wait out the key unlock before they start.

### Removed

- `--app-version` and `PROTON_APP_VERSION`. Nothing needed them.

### Fixed

- IDs that begin with `-` are no longer read as flags, in full, short and `SHARE/ITEM` references.
- `drive items revisions restore` reports success instead of an error when Proton accepts it.
- A recursive upload meeting a file where a folder must go is refused before anything is written, not part way through.
- A command that needs a feature your plan lacks says so, instead of retrying and losing the reason.
