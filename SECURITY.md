# Security Policy

> **Disclaimer:** This is an unofficial, community-built tool and is not endorsed by or affiliated with Proton AG. Use at your own risk.

## Reporting a vulnerability

**Please do not report security issues via public GitHub issues, pull requests, or discussions.**

Instead, use one of these private channels:

1. **Preferred** - Open a [private security advisory](https://github.com/roman-16/proton-cli/security/advisories/new) on GitHub. This is encrypted, scoped to the maintainer, and lets us collaborate on a fix in a private fork.
2. **Alternative** - Email <roman@lerchster.dev> with the details. Use `[proton-cli security]` in the subject line.

Please include:

- A clear description of the vulnerability and its impact.
- Steps to reproduce, ideally with a minimal example.
- The affected version(s) of `proton-cli` (output of `proton-cli --version`).
- Your operating system and Go toolchain version, if relevant.
- Whether you've disclosed this to anyone else, and any disclosure timeline you have in mind.

You can expect:

- An initial acknowledgement within **7 days**.
- A triage assessment (severity, scope, planned fix) within **14 days**.
- A patched release for confirmed critical issues within **30 days** where feasible.

If you don't get a response within 7 days, please follow up - your message may have been missed.

## Supported versions

Only the **latest released version** of `proton-cli` receives security fixes. There is no long-term-support branch.

Always upgrade to the latest release before reporting an issue you suspect is fixed in newer code.

## Scope

### In scope

- Vulnerabilities in `proton-cli`'s own code, including:
  - Authentication flow (SRP login, two-factor handling).
  - Local credential and key storage.
  - PGP key handling and message decryption logic.
  - Command-line argument parsing and shell injection risks.
  - File I/O (path traversal, symlink races, insecure temp files).
- Misuse of upstream cryptographic libraries ([`gopenpgp`](https://github.com/ProtonMail/gopenpgp), [`go-srp`](https://github.com/ProtonMail/go-srp)) inside `proton-cli`.
- Build / supply-chain issues in the release pipeline that could lead to a malicious binary being distributed under the `proton-cli` name.

### Out of scope

- Vulnerabilities in **Proton's services or infrastructure**. Report those to Proton's [Bug Bounty Programme](https://proton.me/security/bug-bounty) directly (or email <security@proton.me>).
- Vulnerabilities in **upstream Go dependencies**. Report those to the respective projects. We will track upstream advisories and update dependencies in a timely manner.
- Issues that only manifest in **modified, forked, or unofficially redistributed builds** of `proton-cli`.
- Account-safety issues caused by **violating Proton's Terms of Service** (e.g., using `proton-cli` against accounts where automated access is prohibited). Such use is at your own risk.
- Theoretical issues without a demonstrated impact.

## Disclosure policy

We follow [coordinated disclosure](https://en.wikipedia.org/wiki/Coordinated_vulnerability_disclosure):

1. The reporter and maintainer agree on a disclosure timeline (default: 90 days from initial report, or sooner if a fix is shipped).
2. A patched release is published.
3. A security advisory is published on GitHub with credit to the reporter (unless they prefer to remain anonymous).
4. If a CVE is warranted, one is requested.

## How credentials are stored at rest

`proton-cli` saves a per-profile session file at `~/.config/proton-cli/sessions/<profile>.json` (mode `0600`) so you don't re-authenticate on every command. It contains:

- the session **auth tokens** (UID, access, refresh), and
- the **salted key password** that unlocks your PGP keys, stored **encrypted** with a random 256-bit AES-GCM *client key*.

That client key is **not** stored on your machine - it is held by Proton's servers and fetched over an authenticated request (`/auth/v4/sessions/local/key`) when a command needs to unlock your keys. Consequences:

- The key password is never written to disk in cleartext, so it can't be lifted from a backup, a synced home directory, a disk image, or with `grep`.
- **Revoking the session** (from the Proton web/mobile app or its session settings) makes the on-disk blob undecryptable: without a live session the client key can't be fetched, so a leaked copy of the file can no longer be turned back into your key password.

Caveat: the file still contains the session refresh token, so it is **not** safe to share - treat it as a secret. The encryption-at-rest above limits the damage of a *leaked copy* (it can be neutralised by revoking the session); it is not a substitute for protecting the file. Session files created by older versions stored the key password in cleartext; they are migrated to the encrypted form automatically on the next unlock.

## Hardening recommendations for users

`proton-cli` is unaudited. To reduce risk while using it:

- Treat the binary as you would any third-party CLI handling secrets - run it as your normal user, not as root.
- Keep `proton-cli` and its dependencies (Go runtime, OS) up to date.
- Verify release artifact checksums against the [GitHub Releases page](https://github.com/roman-16/proton-cli/releases) before installing.
- Be aware that running `proton-cli` against a Proton account may, depending on your usage pattern, trigger Proton's automated abuse detection. This is an account-safety consideration, not a vulnerability in `proton-cli`.
