# Todo

What is left before `v2.0.0` can be released.

Read [`AGENTS.md`](AGENTS.md) first. It holds the quality gates, the rule that the integration suite is never run unattended, and where the Proton WebClients reference checkout lives. [`tests/AGENTS.md`](tests/AGENTS.md) covers how the integration tests are written.

Everything that can be checked without credentials already passes:

```bash
just lint        # gofmt + golangci-lint, 0 issues
just build       # release-shaped binary, including the CGO webview helper
just test-fast   # unit, golden and conformance tests
just docs        # regenerates docs/commands/README.md, must produce no diff
go vet ./tests/  # the integration suite compiles
```

The three items below need a real Proton account, so they are the user's to run. Ask before running any of them, and never run `just test` unprompted.

## 1. Run the integration suite

```bash
PROTON_USER=… PROTON_PASSWORD=… just test
```

Takes several minutes and creates real data in the account.

The suite compiles and speaks the current command vocabulary, but it has never been executed against the API in that form: roughly 148 assertion lines were rewritten in bulk. Expect failures on the first pass, and treat them as suspect assertions before suspecting the product - the usual cause is an expected string that no longer matches the wording a command prints.

Work through them one at a time rather than re-running the whole suite:

```bash
just test-one TestSomeSpecificName
```

## 2. Re-record the demo images

`assets/demo-dark.svg` and `assets/demo-light.svg` are older than the interface they show. `scripts/terminal-demo/record.sh` drives a panel per feature and now includes a `--dry-run` panel that appears in neither image.

Needs a throwaway account, because the seed script writes files, vaults and messages into it:

```bash
PROTON_ALT_USER=… PROTON_ALT_PASSWORD=… just demo-seed
PROTON_ALT_USER=… PROTON_ALT_PASSWORD=… just demo
```

Then check both images: no panel clipped on the right, no row cut off at the bottom, and the alt text in `README.md` still describes what is on screen. `scripts/terminal-demo/README.md` explains the layout.

## 3. Confirm human verification end to end

Proton only asks for a CAPTCHA occasionally, so this needs a login that actually triggers one. There is no way to force it: the API's own `tests/humanverification` endpoint is not served in production.

The window should show Proton's CAPTCHA. If it comes up empty, the colour says where to look:

- **White and blank** - the page loaded but its scripts did not run. Check the URL `proton.Client.CaptchaURL` produced; the page has to come from the API subdomain (`mail-api.proton.me`), since the web app host answers the same path with a Content-Security-Policy that carries no nonce and refuses every inline script.
- **Dark and blank** - the WebKit render process died. Capture the helper's stderr.

A dark window on a **non-NixOS** host built through `devbox shell` is an environment limitation, not a bug: nix bakes a `/nix/store` RUNPATH into the helper, so it loads nix's webkitgtk, which looks for EGL drivers under `/run/opengl-driver`. That path exists on NixOS and nowhere else. Release binaries are unaffected - CI builds each Linux helper on a native runner against apt's `libwebkit2gtk-4.1-dev`. The header of `scripts/build-hv-helpers.sh` has the detail.

## 4. Release

Only after 1-3 are done.

Releases run from a workflow rather than a pushed tag:

```bash
gh workflow run release.yml -f version=2.0.0
```

The workflow builds the webview helper on a native runner per platform, embeds it, and publishes the binaries through GoReleaser.

Write the release notes from the breaking changes in the `feat!` commit message. There is no migration guide in the repo by design, so the notes are the only place a v1 user is told what to run instead.
