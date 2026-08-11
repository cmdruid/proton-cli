# Contributing

Thanks for helping out. Issues, ideas, and pull requests are all welcome.

## Scope

proton-cli mirrors what the Proton **web clients** let a user do. If an action isn't possible in the official web UI, it doesn't belong here, even when an API endpoint for it exists. Web-client parity beats API completeness.

## Getting set up

The repository uses [devbox](https://www.jetify.com/devbox) and [direnv](https://direnv.net/) to pin the toolchain:

```bash
git clone https://github.com/roman-16/proton-cli.git
cd proton-cli
direnv allow      # or: devbox shell
```

Without devbox you need Go 1.26 or newer, plus `actionlint`, `charm-freeze`, `golangci-lint`, `goreleaser`, `just`, `nixfmt`, `protoc`, `protoc-gen-go` and `shellcheck` for the tasks below.

## Everyday commands

`just --list` is the full set. The ones you'll reach for:

```bash
go build .        # quick build (no CAPTCHA helper)
just build        # release-shaped binary, embeds the webview helper
just run -- mail messages list
just lint         # format, regenerate, and check everything; run before every commit
just test-fast    # unit, golden and conformance tests
just flake        # build the nix package, after a dependency bump
just snapshot     # every release artifact, without publishing
just demo         # regenerate the README demo images
just update       # move every dependency and tool to the latest version
```

`just lint` has to pass with no findings, and has to leave the tree clean. It formats Go and Nix, regenerates the command reference, and checks the workflows, the release configuration, the shell scripts and the Go, so a stale generated file fails the same way a lint finding does. CI runs the same recipe.

## README demo images

The terminal panel in the README is a recording of a real session, rendered with [freeze](https://github.com/charmbracelet/freeze). It records as `primary`, the same account the integration tests use, so it needs no credentials of its own:

```bash
just demo
```

[`scripts/terminal-demo/README.md`](scripts/terminal-demo/README.md) explains the pieces and the rules that keep the recording honest.

## Tests

`just test-fast` runs the unit, golden and conformance tests. No credentials, no network, seconds to finish, safe to run any time:

```bash
just test-fast
```

The suite under `tests/` is different: those are **integration tests against the live Proton API**. They run on the primary and secondary accounts, create and delete real data in them, and take several minutes.

```bash
export PROTON_CLI_TEST_PRIMARY_USER=primary@proton.me      # never your own account
export PROTON_CLI_TEST_PRIMARY_PASSWORD=...
export PROTON_CLI_TEST_SECONDARY_USER=secondary@proton.me
export PROTON_CLI_TEST_SECONDARY_PASSWORD=...

just test-one TestMailSendAndRead    # a single integration test
just test                            # everything
```

`just login` and `just seed` sign the accounts in and fill them, for working with them by hand.

Never point any of this at an account you care about. Credentials can go in a local `.env` file (see `.env.example`), which the devbox shell loads automatically.

Unit test files are named after the file they test (`size.go` → `size_test.go`). The integration tests are grouped by feature area instead.

## Project layout

| Path | Contents |
| --- | --- |
| `main.go` | Entry point |
| `internal/cli/` | Cobra command tree, flags, exit codes |
| `internal/service/` | Per-product logic (mail, drive, calendar, contacts, pass) |
| `internal/proton/` | API client, request plumbing, error types |
| `internal/crypto/`, `internal/account/` | Key handling, SRP login, sessions |
| `internal/render/`, `internal/view/` | Output formatting: tables, JSON, YAML, progress |
| `internal/hv/` | Human-verification webview helper |
| `cmd/proton-cli-hv/` | The webview helper binary |
| `tests/` | Live-API integration tests |
| `scripts/` | OpenAPI generator, installers, release helpers, README demo |
| `assets/` | Logo and the generated README demo images |
| `docs/` | User documentation |

## Working with Proton's API

Proton's web client is the reference for endpoints, payload shapes, and crypto flows:

```bash
cd /tmp && git clone --depth 1 https://github.com/ProtonMail/WebClients.git
```

`openapi.yaml` in the repository root is generated from that source and covers roughly 740 endpoints. Regenerate it with:

```bash
just openapi
```

A weekly workflow does the same thing and commits when upstream changes. See [`scripts/README.md`](scripts/README.md).

## Pull requests

- Keep the change focused, and match the surrounding style.
- Run `just lint` and `just test-fast`.
- Add or adjust integration tests when you touch behaviour that they cover.
- Update `docs/` and, when it's user-facing, the README.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `build:`, …); the release notes are generated from them.

## Releases

Running the **Release** workflow with a version runs the same checks a pull request faces against the commit being released, builds the CAPTCHA helper on a native runner per platform, and only then tags. The tag is pushed last on purpose: it is fetched by users, resolved by `go install`, and the version GoReleaser derives, so nothing that outlives a failed run happens until everything that can fail has passed. GoReleaser then builds every target, publishes the GitHub release, and updates the APT repository, AUR, Homebrew tap, winget, and npm.

Re-running the workflow with the same version and `skip-tag-check` resumes from the existing tag, and re-checks that tag's own commit rather than whatever `main` has become since.

`just snapshot` runs the same GoReleaser pipeline locally without publishing, so a packaging mistake surfaces before the tag rather than after it. It builds the helper for your platform and stands in placeholders for the other four, since those need native runners: the artifacts in `dist/` prove the packaging, not the helper bytes, and the foreign binaries in there are not runnable.

## Security

Please don't file security issues publicly. [`SECURITY.md`](SECURITY.md) has the private reporting channels.
