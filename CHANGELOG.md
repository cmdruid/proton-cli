# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Adding a version section here is what publishes a release, so this file is the one place a version is decided: see [Releases](CONTRIBUTING.md#releases). Versions that shipped before this file existed are on the [releases page](https://github.com/roman-16/proton-cli/releases).

## [2.3.0] - 2026-08-15

### Added

- `proton-cli drive items revisions download PATH REVISION_REF` reads an earlier version of a file out without touching the file itself, with the same `--output`, `--output-dir` and `--force` as any other download, and `--output -` into a pipe.
- `proton-cli drive items revisions delete PATH REVISION_REF` removes an earlier version permanently.
- `proton-cli pass aliases disable REF` and `proton-cli pass aliases enable REF` stop and start an alias receiving mail, which is what to reach for when an address starts attracting spam: deleting it burns the address for good.
- `--if-exists replace|rename|skip` on `drive items upload` answers what to do about a name Drive already has: write the bytes as a new revision, keep both under a numbered name, or leave what is there alone. Without it an upload still refuses rather than overwriting, and with `--recursive` the answer is about the folder the tree lands in rather than about each file inside it.
- Aliases are read and edited as the items they are: `pass items get` shows the address, whether it is enabled, where it forwards and its recent activity, `pass items update` takes `--mailbox` and `--display-name`, `pass aliases list` gained a `STATUS` column, and `--output json` carries `alias`, `alias_status`, `alias_mailboxes`, `alias_display_name` and `alias_activity`.
- `pass aliases create` names the address Proton made for it, since Proton invents a word to go with your prefix.
- Single-letter forms for the five global flags you type most: `-p`, `-o`, `-n`, `-q`, `-y`. They cluster, so `-qn` is a quiet dry run, and no subcommand may take one, so `-p` is the profile everywhere.
- A caveat worth knowing prints as its own `!` line on stderr, so a file that arrived but could not be attributed no longer reads like ordinary commentary above a green tick.

### Changed

- **Breaking.** A reference Proton does not recognise exits `3` (not found) rather than `1`, in every command that takes one. A script that reads `1` as "no such thing" needs updating.
- **Breaking.** A bad command line while signed out exits `1` (user error) rather than `2` (authentication): a command now settles what its arguments alone can settle before it needs an account.
- Colour marks only what carries a verdict or a colour of its own: an unread or starred message, a signature that is `unverified` or `invalid`, the swatch beside a label, folder, calendar or group. Every verdict is still spelled out in words, so a pipe, `--no-color` and a colour-blind reader lose nothing.
- Label, folder, calendar and group lists show a colour as its swatch and name rather than as a hex code.
- The `FLAGS` column says how many files are attached instead of `📎`. No monospace font carries that emoji, so every terminal drew it two cells wide and pushed the column after it out of line.
- Widths are measured in terminal cells rather than characters, so a table stays aligned for a subject written in Japanese or a filename with an emoji in it.
- An empty result reads `No messages match.` when a filter was applied, rather than `No messages.`, so an unmatched search no longer looks like an empty account.
- Transfers say how fast they are going and how long is left, and number themselves within a batch (`[3/27]`), dropping parts as the terminal narrows rather than wrapping.
- Commands send their independent requests together instead of one at a time, and never fetch the same thing twice: reading a single calendar event took eight round trips to learn what one could have told it.
- Commands that decrypt - reading a message, opening a Pass item - no longer wait out the key unlock before they start. Three quarters of that wait now happens inside requests the command was making anyway.

### Removed

- `--app-version` and the `PROTON_APP_VERSION` environment variable. Nothing needed them, and the only thing they could do was claim to be an official Proton client.

### Fixed

- IDs that begin with `-` are no longer read as flags. About one Proton ID in sixty-four starts with a dash, since `-` is a base64 character, so the CLI was printing IDs the next command refused. Full, shortened and compound `SHARE/ITEM` references are all protected now, including the much shorter Drive invitation IDs, which never reached the API at all.
- `drive items revisions restore` reports success when it succeeds. Proton accepts a restore and carries it out in the background, which it says with a code the CLI was reading as an error.
- A recursive upload meeting a file where a folder must go, or a folder where a file must go, is refused before anything is written. It used to be discovered part way through, after other files had landed, and reported as though the destination were not a folder.
- A command that needs something your plan does not include says so, instead of spending a refresh token to ask the same question again and losing the reason it was given.
