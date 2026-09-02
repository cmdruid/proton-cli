# Security audit — proton-cli (local clone)

**Date:** 2026-09-02
**Tree:** `dd7e20793478d852e953c7d77614a4ca0ac993ac` (`main` at clone time)
**Auditor:** local review of auth, crypto, HTTP, persistence, and self-update (not a third-party formal audit)
**Verdict:** **PASS with conditions** — no backdoor, no credential exfil, no TLS bypass found. Install only a binary built from this clone. Do not use GitHub release binaries or `proton update`.

## Summary

Overall risk for a **self-built** install: **low**, with accepted residual risk of an unofficial, unaudited client that holds a Proton session.

| Severity | Count |
|---|---|
| critical | 0 |
| high | 0 |
| medium | 1 (self-update supply chain, mitigated by not using it) |
| low | 2 |
| informational | 3 |

## Finding 1: `proton update` trusts GitHub SHA-256 only

- **Severity:** medium
- **Category:** supply chain
- **Location:** `internal/selfmanage/update.go` (`Download`, `Apply`); `internal/cli/self/update.go`
- **Description:** The updater downloads `checksums.txt` and the binary from the same GitHub Releases namespace (this fork: `cmdruid/proton-cli`), checks SHA-256, then overwrites the running executable via `minio/selfupdate`. There is no separate GPG/minisign verification of the checksum file. A GitHub account or release-asset compromise would replace our audited binary. A binary in `~/.local/bin` is classified as `KindStandalone` and is eligible for in-place update.
- **Impact:** Remote code execution as the user on the next `proton update`.
- **Reproduction:** Install to `~/.local/bin/proton`, run `proton update` (or `proton update VERSION` on a `dev` build).
- **Remediation (ours):** Never run `proton update`. Set `no-update-check: true`. Rebuild from this clone. Leave version as `dev` so a bare `proton update` refuses.
- **Status:** accepted (operational control)

## Finding 2: Session tokens stored in plaintext

- **Severity:** low
- **Category:** data at rest
- **Location:** `internal/account/session/session.go` (`Session` struct, `Save`)
- **Description:** `~/.config/proton-cli/sessions/<profile>.json` is written mode `0600` via temp file + rename. `access_token` and `refresh_token` are plaintext JSON. The salted key password is AES-256-GCM wrapped (`internal/account/localkey`) with a 256-bit client key stored only on Proton (`/auth/v4/sessions/local/key`), so revoking the session makes the blob undecryptable. This matches Proton web-client persisted sessions.
- **Impact:** A copied session file lets an attacker use the API until the session is revoked. They cannot unwrap the key password without that live session.
- **Remediation:** Protect the file; revoke on loss (`proton account logout --revoke` or Proton account sessions).
- **Status:** accepted (same model as the official web apps)

## Finding 3: `proton api` is a full authenticated API escape hatch

- **Severity:** low
- **Category:** authorization / agent blast radius
- **Location:** `internal/cli/api/api.go`; path is concatenated onto `https://mail.proton.me/api` in `internal/proton/client.go` `doOnce`
- **Description:** Any signed-in caller can `GET`/`POST` arbitrary Proton endpoints. Paths stay on the configured API host (not an open SSRF to the public internet). `--dry-run` still refuses mutating methods at the transport layer.
- **Impact:** A confused or malicious agent prompt can change account state the rest of the CLI does not model.
- **Remediation:** Agents should not use `proton api` unless the owner asked. Prefer `--dry-run` on mutations.
- **Status:** accepted

## Informational

1. **Unofficial / unaudited project.** Small history (~150 commits). Crypto is Proton’s `go-srp` and `gopenpgp`, not a custom cipher. Residual trust is the rest of this tree and its Go module graph.
2. **`PROTON_API_URL` can retarget the API.** Intentionally not persistable in `config.yaml`. Do not set it.
3. **CAPTCHA URLs** are built from Proton’s verify host (`internal/proton/verify.go`), not from a raw attacker string via the shell. `xdg-open` is `exec.Command(name, url)` with no shell (`internal/cli/browser.go`).

## Positive observations

- Login is SRP; the password is not sent. Server proof is checked (`srpCall` in `internal/proton/auth.go`).
- Honest `User-Agent` / `x-pm-appversion` (`Other`), per Proton’s third-party client rules.
- Default HTTP client uses system TLS roots; no `InsecureSkipVerify`.
- Default base URL is `https://mail.proton.me/api`. Debug logs record method/path/status, not bodies or tokens.
- Session writes are atomic and `0600`. AES-GCM uses `crypto/rand` (`internal/crypto/aead/aesgcm.go`).
- `--dry-run` is enforced in the HTTP client for all non-GET methods, including `api`.
- In-flight requests capped at 8; 429/5xx backoff. No telemetry. Daily GitHub version check carries no account data and is disableable.
- Passwords are never CLI flags (file/stdin/TTY only).
- FIDO rpID is constrained to Proton hosts (`internal/fido`).

## Conditions of use

1. Build from this clone (`INSTALL.md`). Do not install release binaries.
2. `no-update-check: true`. Never `proton update`.
3. Run as the owner, not root.
4. Re-review before merging upstream `main`.
