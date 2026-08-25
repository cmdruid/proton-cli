# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Adding a version section here is what publishes a release, so this file is the one place a version is decided: see [Releases](CONTRIBUTING.md#releases). Versions that shipped before this file existed are on the [releases page](https://github.com/roman-16/proton-cli/releases).

## [2.6.0] - 2026-08-26

### Added

- Calendar sharing: `calendar settings calendars share add|list|remove` hands a calendar to another Proton account, and `calendar invitations list|accept|decline` is the other side of it.
- `calendar settings calendars create --url` subscribes to a calendar published elsewhere, and `calendar settings calendars get` shows one in full.
- `calendar events export` and `calendar events import PATH` read and write `.ics` files. An import is addressed by each event's UID, so exporting, editing and importing back is a restore rather than a second copy.
- Events take `--end`, `--status`, optional attendees, and reminders that email rather than notify.
- `contacts export` and `contacts import PATH` read and write vCard files, addressed by UID the same way.
- `contacts merge` folds duplicate contacts into one, and `contacts groups get` shows which addresses are in a group.
- Contact values can say what kind they are - `--email work:jane@acme.com` - and eleven more fields are settable, among them the organization, the title and the birthday.
- Vault sharing: `pass vaults share add|list|remove` and `pass invitations list|accept|decline`.
- `pass export` and `pass import PATH` write and read a Proton Pass archive, the same zip the web client produces. Without a passphrase the archive is not encrypted, and says so as it writes.
- `pass links create|list|revoke` shows one item to somebody without a Proton account, for a while or a number of views.
- `pass breaches list|get` reports which of your addresses have turned up in somebody else's data breach.
- `pass aliases contacts create|list|block|allow|delete` gives an alias an address per correspondent, so a reply leaves as the alias instead of the mailbox behind it.
- `pass settings mailboxes list|create|verify|resend|update|delete` manages where aliases forward, and `pass settings domains list` shows what an alias can be made on.
- `pass items revisions list`, `pass items pin|unpin`, `pass items totp`, `pass vaults get`, and `pass generate` for a password.
- Items take custom sections: `--field SECTION/NAME=VALUE`, `--hidden` for a secret one, and `--totp-field` for a second code. An identity takes all 31 of its fields, not 13.
- `pass vaults create` and `update` take `--description`, `--icon` and `--color`.
- `mail settings filters create --if "subject contains invoice" --move-to Archive` describes a filter in conditions and actions and lets Proton write the Sieve; `--sieve` still takes a script of your own. `filters get` shows an existing one in the same words, and `filters update` rewrites a rule in place, keeping its position in the order.
- `mail settings filters apply` runs filters over mail already in the mailbox, and `filters reorder` sets the order they run in.
- `mail settings senders list|block|allow|spam|forget` manages the block and allow lists.
- `mail conversations snooze|unsnooze` takes a thread out of the inbox until a time you choose, and `snoozed` is addressable wherever a folder is - as are the categories Proton sorts into: `social`, `promotions`, `newsletters`, `transactions` and `updates`.
- `mail messages expire` makes a message delete itself later, `mail messages unsubscribe` asks a mailing list to stop, and `mail messages empty --folder` empties a folder outright.
- Five more mail settings: `next-message-on-move`, `pgp-scheme`, `remove-image-metadata`, `right-to-left` and `spam-action`.
- `drive shared list` shows what other people have shared with you and `drive sharing list` what you have shared. `drive items share update|resend` changes or resends an invitation, and `drive photos albums update --cover` sets an album's cover.
- `PROTON_HV_HELPER` names a CAPTCHA helper to run instead of the built-in one, which is what lets a `go install` build verify at all.
- `--passphrase-file` and `--passphrase-stdin` hand over a passphrase without putting it in the command line.

### Changed

- **Breaking.** `mail messages search` and `mail conversations search` are gone. `mail messages list` and `mail conversations list` take the same filters; they were always one request to Proton.
- **Breaking.** `drive folders create` is now `drive items create`, and `drive share ...` is now `drive items share ...`. Update any scripts that call them.
- The CAPTCHA window works on a Nix install. It opened blank saying "TLS support is not available", because nothing supplied the TLS backend GIO loads as a module.

### Fixed

- `contacts create` reports Proton's refusal instead of exiting `0` having written nothing. Proton answers inside a successful response, and only the response was being read.
- `contacts groups add` puts every one of a contact's addresses in the group, as it always said it would, and `--email` acts on the address you named rather than one that happens to belong to another contact.
- `pass aliases options` lists the domain an alias can be made on rather than a whole suffix. Proton mints the word in front of it afresh on every request, so what was listed had already stopped working by the time it could be typed back.
- `drive photos albums create` prints the new album's ID instead of nothing.

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
