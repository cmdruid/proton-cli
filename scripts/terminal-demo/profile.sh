# Resolves the demo account for record.sh and seed.sh. Sourced, not executed.
#
# The demo runs against the "alt" profile, so its credentials come from
# PROTON_ALT_USER and PROTON_ALT_PASSWORD like any other profile's do. The
# profile is exported rather than passed as a flag, so the recorded commands
# stay free of demo plumbing.

export PROTON_PROFILE=alt

address=${PROTON_ALT_USER:-}

# Profile-scoped variables fall back to the unscoped PROTON_USER / PROTON_PASSWORD,
# which would point the recording at a personal account and then publish its
# contents. Require the alt pair explicitly instead.
if [[ -z $address || -z ${PROTON_ALT_PASSWORD:-} ]]; then
	echo "set PROTON_ALT_USER and PROTON_ALT_PASSWORD to a throwaway account" >&2
	echo "(the demo sends mail and uploads files, and what it shows ends up in the README)" >&2
	exit 1
fi

bin=${PROTON_CLI:-./proton-cli}
if [[ ! -x $bin ]]; then
	echo "$bin is missing - run: just build" >&2
	exit 1
fi
