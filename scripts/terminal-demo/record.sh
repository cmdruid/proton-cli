#!/usr/bin/env bash
# Records the README demo by running proton-cli for real and writing the
# session, ANSI and all, to stdout. `just demo` pipes that into freeze.
#
#   just demo
#
# Run `just demo-seed` first so the account has something tidy to show. The
# only thing this script changes is the file it uploads, which it deletes
# again afterwards - the cleanup panel is a dry run, so it removes nothing.
set -euo pipefail

cd "$(dirname "$0")/../.."
# shellcheck source=scripts/terminal-demo/profile.sh
. scripts/terminal-demo/profile.sh

# A fixed window keeps the rendered image identical across machines.
stty columns 84 rows 40 2>/dev/null || true

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
printf 'The north trail is open again.\n' >"$work/trail-map.txt"

# Warm the session up before recording, so the transcript opens on a command
# rather than on an authentication notice, and no panel pauses halfway through to
# unlock the keys. `pass vaults list` is the cheapest command that does both: it
# reuses a saved session and needs the key hierarchy.
#
# Deliberately not `account login`, which would sign in afresh on every run and
# leave a trail of sessions on the demo account.
"$bin" pass vaults list >/dev/null 2>&1

# Only the prompt marker is colored; the rest is whatever proton-cli prints.
prompt() { printf '\033[38;2;138;110;255m$\033[0m %s\n' "$*"; }

prompt "proton-cli mail messages list --unread --page-size 3"
"$bin" mail messages list --unread --page-size 3 || true
printf '\n'

prompt "proton-cli drive items upload trail-map.txt /Documents"
"$bin" drive items upload "$work/trail-map.txt" /Documents || true
printf '\n'

# A dry run is the most useful thing to show: it names the rows it would touch
# rather than counting them, which is the difference between approving a bulk
# delete and hoping.
prompt "proton-cli drive items trash --scope /Documents --pattern '*.txt' --dry-run"
"$bin" drive items trash --scope /Documents --pattern '*.txt' --dry-run || true
printf '\n'

prompt "proton-cli pass items list --vault Personal"
"$bin" pass items list --vault Personal || true

"$bin" drive items delete /Documents/trail-map.txt >/dev/null 2>&1 || true
