#!/usr/bin/env bash
# Resets a throwaway demo account to the state `record.sh` shows: three recent
# messages, a Documents folder, and a small Pass vault.
#
#   just demo-seed
#
# Running it again is safe - it clears what it created last time first, so the
# recording never drifts or accumulates duplicates. Proton's index lags a few
# seconds behind a send, which is why seeding is separate from recording.
set -euo pipefail

cd "$(dirname "$0")/../.."
# shellcheck source=scripts/terminal-demo/profile.sh
. scripts/terminal-demo/profile.sh

quiet() { "$bin" "$@" >/dev/null 2>&1 || true; }

# exit code 3 is "not found" - see docs/concepts.md
missing() {
	local code=0
	"$bin" "$@" >/dev/null 2>&1 || code=$?
	[[ $code -eq 3 ]]
}

echo "Seeding $address ..."

subjects=(
	"Invoice #2291 is ready"
	"Your April security report"
	"Re: hiking weekend"
)
bodies=(
	"Your invoice for August is attached to your account."
	"No unusual sign-ins this month."
	"The north trail is open again - shall we take it?"
)
for i in "${!subjects[@]}"; do
	quiet mail messages trash --subject "${subjects[$i]}" --limit 20
	quiet mail messages send --to "$address" --subject "${subjects[$i]}" --body "${bodies[$i]}"
	echo "  mail: ${subjects[$i]}"
done

if missing drive items info /Documents; then
	quiet drive folders create /Documents
fi
echo "  drive: /Documents"

if ! "$bin" pass vaults list 2>/dev/null | grep -q "Personal"; then
	quiet pass vaults create --name Personal
fi
quiet pass items trash --vault Personal --all
quiet pass items create --vault Personal --name GitHub --username roman --url github.com \
	--password "$(head -c 18 /dev/urandom | base64)"
quiet pass items create --vault Personal --type wifi --name "Home Wi-Fi" --ssid Fritzbox \
	--security WPA2 --password "$(head -c 12 /dev/urandom | base64)"
quiet pass items create --vault Personal --type note --name "Door codes" --note "Front door: 1234"
echo "  pass: Personal vault with 3 items"

echo "Done. Give Proton's index a few seconds, then run: just demo"
