# Agent brief — local `proton-cli` tool

This checkout is our fork of a third-party tool, not a project we originally wrote. Fork: [cmdruid/proton-cli](https://github.com/cmdruid/proton-cli). Upstream: [roman-16/proton-cli](https://github.com/roman-16/proton-cli) (MIT, unofficial, not Proton AG). Remotes: `origin` = fork, `upstream` = roman-16.

Clone path: `~/Repos/tools/proton-cli`.
Binary: `proton` (alias `proton-cli`) in `~/.local/bin`.
How to build, install, configure, and run: [INSTALL.md](INSTALL.md).
How to cut a GitHub Release (do not, until asked): [RELEASE.md](RELEASE.md).
Local security review: [SECURITY-AUDIT.md](SECURITY-AUDIT.md).
Upstream contributor notes (do not follow for our use): `git show upstream/main:AGENTS.md` and [CONTRIBUTING.md](CONTRIBUTING.md).

## What this is

`proton` talks to Proton’s HTTPS API the way the web apps do. It decrypts mail locally with Proton’s `go-srp` and `gopenpgp`. It does **not** use Proton Mail Bridge.

Grammar: `proton <app> <collection> <verb> [TARGET…]`. Apps: `mail`, `drive`, `calendar`, `pass`, `contacts`, `account`.

## Do

- Rebuild from this clone after a reviewed `git pull origin`. Follow [INSTALL.md](INSTALL.md). Fetch `upstream` only to review roman-16's changes before merging.
- Prefer `--output json` (or config `output: json`) when parsing.
- Use `--dry-run` before any mutation. Honor confirmations; never pass `--yes` unless the owner asked for that exact command.
- Treat session files under `~/.config/proton-cli/sessions/` as secrets (`0600`).
- If a command needs a password and there is no session, stop and tell the owner to run `proton account login`. Do not invent credentials or put a password on the command line.

## Do not

- Cut a GitHub Release or run `proton update` until the owner asks and [RELEASE.md](RELEASE.md) has been followed. Do not install roman-16's release binaries or `curl | sh` from upstream.
- `git push` to `origin`, force-push, or open PRs unless the owner asked.
- Run `just test` / `just test-paid` (live API against real accounts). `just test-fast` is offline and fine.
- Point `PROTON_API_URL` at anything other than `https://mail.proton.me/api`.
- Run as root.
- Poll in a tight loop (`mail messages watch` is the intended wait). Aggressive API use can look like abuse.
- Export mail/contacts to a world-readable path. Exports are plaintext.

## Mail commands agents actually use

```bash
proton account get --output json
proton mail messages list --unread --output json
proton mail messages list --folder all --keyword invoice --output json
proton mail messages get REF --output json
proton mail conversations get REF --output json
proton mail messages send --to ADDR --subject S --body B --dry-run
```

`REF` may be a full ID, a short ID, or a unique subject. IDs are scoped to the current folder.

Sending, trashing, moving, and emptying folders change the account. Preview with `--dry-run` first.

## Rebuild

```bash
cd ~/Repos/tools/proton-cli
export PATH="$HOME/.local/go/bin:$PATH"
go build -trimpath -o "$HOME/.local/bin/proton" ./cmd/proton
ln -sfn "$HOME/.local/bin/proton" "$HOME/.local/bin/proton-cli"
```

Full steps, Go bootstrap, and config: [INSTALL.md](INSTALL.md).
