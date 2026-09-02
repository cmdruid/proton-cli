# Cut a release (this fork)

Do **not** cut a release until the owner asks. This file is the checklist for when they do.

A release on [cmdruid/proton-cli](https://github.com/cmdruid/proton-cli) is a **GitHub Release** whose assets `proton update` can fetch:

- `checksums.txt` (SHA-256)
- `proton-cli_linux_amd64` (this workstation)
- the other OS/arch binaries GoReleaser builds

Until that exists, install by rebuilding from this clone ([INSTALL.md](INSTALL.md) §2). Do not run `proton update`.

## What we are not doing

Upstream treats `CHANGELOG.md` as a release button: a new version heading on `main` makes `.github/workflows/release.yml` tag, run GoReleaser, and publish to GitHub **plus** APT, AUR, Homebrew, winget, and npm — all of which still name `roman-16`.

On this fork that automation is **disabled** (`gh workflow disable Release` on `cmdruid/proton-cli`). Reasons:

1. The newest changelog section is still `2.10.0`. That tag exists upstream, not here. An inherited run would try to publish `v2.10.0` as *our* first release.
2. AUR / Homebrew / winget / npm / APT in `.goreleaser.yaml` and the workflow still point at roman-16’s accounts and secrets we do not have.

Do not re-enable `release.yml` on this repository until those publishers are removed or retargeted.

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

## Cut the release (manual, GitHub only)

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

5. **Publish GitHub assets only.** Skip AUR, Homebrew, winget, and distro packages:

   ```bash
   go run ./scripts/changelog --notes > /tmp/proton-cli-notes.md
   export GITHUB_TOKEN="$(gh auth token)"
   goreleaser release --clean --release-notes /tmp/proton-cli-notes.md \
     --skip=aur,homebrew,winget,nfpm
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

- Leave the Release workflow disabled (`gh workflow disable Release`) until `.github/workflows/release.yml` is retargeted. Re-enabling the inherited file would try to publish changelog `2.10.0` as this fork’s first release.
- Rebuild notes belong in [INSTALL.md](INSTALL.md), not here.
