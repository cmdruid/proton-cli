# Cut a release (this fork)

Do **not** cut a release until the owner asks. This file is the checklist for when they do.

A release on [cmdruid/proton-cli](https://github.com/cmdruid/proton-cli) is a **GitHub Release** whose assets `proton update` can fetch:

- `checksums.txt` (SHA-256)
- `proton-cli_linux_amd64` (this workstation)
- the other OS/arch binaries GoReleaser builds

Until that exists, install by rebuilding from this clone ([INSTALL.md](INSTALL.md) §2). Do not run `proton update`.

This fork publishes **only** to GitHub Releases. APT, AUR, Homebrew, winget, and npm are not part of the pipeline (removed from `.goreleaser.yaml` and `.github/workflows/release.yml`).

## Automation is still off

`.github/workflows/release.yml` will, when enabled, treat a new `CHANGELOG.md` version heading on `main` as a release request: tag, GoReleaser, GitHub Release. It is **disabled** (`gh workflow disable Release`) because the newest heading is still **2.10.0**, unpublished here. Enabling it now would ship `v2.10.0` as our first release.

Enable it only after `CHANGELOG.md` names **2.10.1** (or newer) and that commit is what you intend to ship:

```bash
gh workflow enable Release --repo cmdruid/proton-cli
```

Until then, cut by hand as below, or do not cut at all.

## Versioning

Follow [Semantic Versioning](https://semver.org/) and [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The last upstream version is **2.10.0**. Our first fork release should be **2.10.1** (or 2.11.0 / 3.0.0 if the change is larger). Do not reuse `2.10.0`.

There is no `[Unreleased]` section. Write the new heading only in the commit that cuts the release.

## Prerequisites

- Push access to `origin` (`cmdruid/proton-cli`)
- Go toolchain from [INSTALL.md](INSTALL.md) (`~/.local/go`)
- [GoReleaser](https://goreleaser.com/) v2 (`goreleaser version`)
- `gh` authenticated as `cmdruid` with `repo` scope
- A clean git tree on `main`, matching `origin/main`

Optional local check (does not publish):

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go run ./scripts/changelog        # newest releasable version, or empty
go run ./scripts/changelog --notes
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

`just notes` / `just snapshot` are the same if `just` is installed.

## Cut the release (manual)

1. **Diff since the last tag** (or since the fork, if there is no tag on origin):

   ```bash
   git fetch origin --tags
   git log --oneline "$(git describe --tags --abbrev=0 2>/dev/null || echo origin/main^)"..HEAD
   ```

2. **Write the changelog.** Insert a new section at the top of `CHANGELOG.md`, after the preamble, before `## [2.10.0]`:

   ```markdown
   ## [2.10.1] - YYYY-MM-DD

   ### Added
   - …

   ### Changed
   - Self-update and installers fetch GitHub Releases from cmdruid/proton-cli.
   - Releases publish to GitHub only.
   ```

   Categories are the Keep a Changelog six, in that order, only those that apply. `go run ./scripts/changelog` must print `2.10.1`. Versions may only step to a patch, minor, or major of the previous heading.

3. **Commit on `main`** (changelog only, plus anything that belongs in that version). Push to `origin`. Wait for CI on that commit to pass.

4. **Tag the same commit:**

   ```bash
   git checkout main
   git pull --ff-only origin main
   git tag --annotate v2.10.1 -m "Release v2.10.1"
   git push origin v2.10.1
   ```

5. **Publish the GitHub Release:**

   ```bash
   go run ./scripts/changelog --notes > /tmp/proton-cli-notes.md
   export GITHUB_TOKEN="$(gh auth token)"
   goreleaser release --clean --release-notes /tmp/proton-cli-notes.md
   rm -f /tmp/proton-cli-notes.md
   ```

   GoReleaser must run from a **clean** tree at the tagged commit. It creates the GitHub Release for the current tag.

6. **Verify:**

   ```bash
   gh release view v2.10.1 --repo cmdruid/proton-cli
   gh release download v2.10.1 --repo cmdruid/proton-cli \
     --pattern checksums.txt --pattern proton-cli_linux_amd64 --dir /tmp/proton-rel
   sha256sum --check --ignore-missing /tmp/proton-rel/checksums.txt
   ```

   Required names: `checksums.txt`, `proton-cli_linux_amd64`, `proton-cli_linux_arm64`, `proton-cli_darwin_amd64`, `proton-cli_darwin_arm64`, `proton-cli_windows_amd64.exe`, `proton-cli_windows_arm64.exe`.

7. **Install from the release** (only after step 6):

   ```bash
   proton update 2.10.1 --reinstall
   proton version
   ```

   Or rebuild from source as in [INSTALL.md](INSTALL.md) if you want a `dev` binary instead of the tagged one.

## If it fails partway

- A tag that exists and a missing GitHub Release: fix the tree, check out that tag, re-run step 5. Do not move the tag to a newer commit.
- A GitHub Release that already exists: do not republish; add missing assets with `gh release upload` only if you know why GoReleaser skipped them.
- Never delete a published tag people may have fetched.

## Aftercare

- Rebuild notes belong in [INSTALL.md](INSTALL.md), not here.
