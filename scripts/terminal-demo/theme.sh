#!/usr/bin/env bash
# Resolves the colour names proton emits into the shades a Proton-themed
# terminal would draw them in, so the README panels are photographed in Proton's
# colours rather than in somebody else's.
#
#   bash record.sh | bash theme.sh carbon > dark.ansi
#
# proton names colours and leaves the shade to the terminal, which is what
# lets it sit inside whatever theme the reader already uses. A recording
# therefore carries names, not colours, and something has to play the part of
# the terminal before freeze can draw it: freeze has a palette of its own, a
# pink magenta nothing about Proton, and no faint at all, so left to itself it
# would photograph the CLI in the wrong purple with its headers undimmed.
#
# The theme is Proton's own, read from WebClients (packages/colors/themes/src):
# carbon for the dark panel, snow for the light one. That is also why the two
# panels can differ at all - the binary has no idea which one it is running in.
set -euo pipefail

case ${1:-} in
    # --text-hint  --primary  --signal-success  --signal-warning  --signal-danger
    carbon) tokens=(6D697D 8A6EFF 1EA885 FF9900 F5385A) ;;
    snow) tokens=(8F8D8A 6D4AFF 1EA885 FF9900 DC3251) ;;
    *)
        echo "usage: ${0##*/} <carbon|snow>" >&2
        exit 2
        ;;
esac

esc=$'\033'

# fg is the 24-bit sequence for a "#RRGGBB" token, which is the only colour
# freeze reads.
fg() { printf '%s[38;2;%d;%d;%dm' "$esc" "0x${1:0:2}" "0x${1:2:2}" "0x${1:4:2}"; }

# Every name the CLI can emit, in the order the tokens are listed above. Faint
# stands for the muted role, which is an intensity in a terminal and has to
# become a colour here because freeze drops it.
names=('\[2m' '\[35m' '\[32m' '\[33m' '\[31m')

script=()
for i in "${!names[@]}"; do
    script+=(--expression "s|$esc${names[$i]}|$(fg "${tokens[$i]}")|g")
done
# A span ends with 39 (default foreground) or 22 (normal intensity), each of
# which freeze passes over as though nothing happened. Only a full reset closes
# a span for it, so every close becomes one. Swatches already carry their own
# 24-bit colour and need only this.
reset="${esc}[0m"
script+=(--expression "s|$esc\[39m|$reset|g" --expression "s|$esc\[22m|$reset|g")

sed "${script[@]}"
