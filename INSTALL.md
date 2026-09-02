# Install `proton` from this clone

Local, audited install of our fork [cmdruid/proton-cli](https://github.com/cmdruid/proton-cli) (upstream [roman-16/proton-cli](https://github.com/roman-16/proton-cli)). `proton update` fetches GitHub Releases from **this fork**, not upstream. Do not use roman-16's `curl | sh` installer. Do not run `proton update` until this fork has a release we built and published.

Machine layout:

| What | Where |
|---|---|
| Clone | `~/Repos/tools/proton-cli` |
| Go toolchain | `~/.local/go` (official tarball, not Ubuntu’s 1.22) |
| Binary | `~/.local/bin/proton` |
| Alias | `~/.local/bin/proton-cli` → `proton` |
| Config | `~/.config/proton-cli/config.yaml` |
| Sessions | `~/.config/proton-cli/sessions/*.json` (mode `0600`) |

`~/.local/bin` is already on `PATH` on this workstation.

## 1. Prerequisites

- Linux amd64
- Network once, to fetch Go and Go modules
- Go **1.26.5 or newer**. This clone’s `go.mod` requires `1.26.5`. Ubuntu/Pop!_OS apt only has 1.22 — do not use that.

### Bootstrap Go 1.26.8 (if `go version` is missing or too old)

Pinned to the latest Go 1.26 patch from [go.dev/dl](https://go.dev/dl/). SHA-256 is from that page.

```bash
set -euo pipefail
GO_VER=1.26.8
GO_TAR="go${GO_VER}.linux-amd64.tar.gz"
GO_SHA=d0f743b33e8d8945e6b1f432edd15785c70507121d6e2a723b21285eddf8b57b
WORKDIR="${TMPDIR:-/tmp}/go-bootstrap"
mkdir -p "$WORKDIR"
curl -fsSL "https://go.dev/dl/${GO_TAR}" -o "$WORKDIR/$GO_TAR"
echo "${GO_SHA}  $WORKDIR/$GO_TAR" | sha256sum -c
rm -rf "$HOME/.local/go"
mkdir -p "$HOME/.local"
tar -C "$HOME/.local" -xzf "$WORKDIR/$GO_TAR"
export PATH="$HOME/.local/go/bin:$PATH"
go version   # expect go1.26.8 linux/amd64
```

Keep `$HOME/.local/go/bin` on `PATH` for later rebuilds (shell profile or the export above).

## 2. Compile

From the clone, with the toolchain on `PATH`:

```bash
cd ~/Repos/tools/proton-cli
export PATH="$HOME/.local/go/bin:$PATH"
go build -trimpath -o "$HOME/.local/bin/proton" ./cmd/proton
ln -sfn "$HOME/.local/bin/proton" "$HOME/.local/bin/proton-cli"
chmod 755 "$HOME/.local/bin/proton"
```

The binary reports version `dev` unless you inject ldflags. Leave it as `dev`: `proton update` refuses to overwrite a development build unless you name a release, which is the behavior we want.

Check:

```bash
proton version
proton --help
```

## 3. Configure

Create `~/.config/proton-cli/config.yaml` if it does not exist:

```yaml
output: json
log-level: warn
no-update-check: true
no-input: true
confirm:
  ask:
    "*": mutations
```

- `output: json` — agents parse stdout.
- `no-update-check: true` — no daily GitHub “new release” probe. Same as `PROTON_NO_UPDATE_CHECK=1`.
- `no-input: true` — never block a scheduled/agent run on a password prompt; missing credentials become an error. For an interactive login, pass `--no-input=false`.
- `confirm.ask` — mutations still ask when a human is on a TTY.

Do **not** put `--api-url`, passwords, or `--yes` in this file.

## 4. Sign in (once, interactive)

The owner must be present. 2FA and CAPTCHA cannot be automated safely.

```bash
proton --no-input=false account login
```

This writes `~/.config/proton-cli/sessions/default.json` (mode `0600`): session tokens plus the key password wrapped with a server-held AES-256-GCM client key. Revoking the session from any Proton app makes that file useless for decryption.

Unattended later:

```bash
proton account get
proton mail messages list --unread
```

If `account get` says you are not signed in, stop and ask the owner to log in again. Do not pass `--password-file` unless the owner named a secret file.

## 5. Run

```bash
proton account get
proton mail messages list --unread
proton mail messages get REF
proton mail messages send --to you@example.com --subject Hi --body Hello --dry-run
```

`--output json` is redundant if the config already sets `output: json`.

## 6. Update this tool

```bash
cd ~/Repos/tools/proton-cli
git fetch origin
git log HEAD..origin/main --oneline   # review, then merge/rebase if wanted
# re-run section 2 (compile)
```

`proton update` is retargeted at `cmdruid/proton-cli`. It still only verifies SHA-256 from the same GitHub account, so only run it after we have published a release from this clone (see [RELEASE.md](RELEASE.md)). Until then it will fail (no releases yet). Rebuild from this clone instead of `proton update`.

Never install from upstream:

```bash
curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh | sh
```

## 7. Uninstall

```bash
rm -f "$HOME/.local/bin/proton" "$HOME/.local/bin/proton-cli"
# optional: sessions and config
# rm -rf "$HOME/.config/proton-cli"
```

Leave `~/Repos/tools/proton-cli` unless you also want the clone gone. Leave `~/.local/go` if other Go tools use it.
