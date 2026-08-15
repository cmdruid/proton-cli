[doc("Build the release-shaped binary, with the CAPTCHA webview helper embedded")]
build:
    bash scripts/build-hv-helpers.sh
    go build -tags=embed_hv -o proton-cli .

[doc("Remove everything generated: the binary, the helpers, the completions, the release output")]
clean:
    rm --recursive --force completions dist internal/hv/assets proton-cli

[doc("Re-record the README panels by running a real session, then render them")]
demo: build
    #!/usr/bin/env bash
    set -euo pipefail
    ansi=/tmp/proton-cli-demo.ansi
    go run ./scripts/seed --profile primary --stage
    script --quiet --command "bash scripts/terminal-demo/record.sh" --return /dev/null \
        | sed --expression 's/\r$//' --expression 's/.*\r//' > "$ansi"
    render() {
        freeze "$ansi" --config "scripts/terminal-demo/$1.json" --output "assets/demo-$1.svg" < /dev/null
        sed --in-place "s|\(<g font-family=[^>]*\)fill=\"[^\"]*\"|\1fill=\"$2\"|" "assets/demo-$1.svg"
    }
    render dark "#FFFFFF"
    render light "#0C0C14"

[doc("Regenerate the command reference from the command tree")]
docs:
    go run ./scripts/gendocs

[doc("Build the nix package from the working tree, which a flake only sees once tracked")]
flake:
    #!/usr/bin/env bash
    set -euo pipefail
    work=$(just worktree)
    trap 'rm --recursive --force "$work"' EXIT
    cd "$work"
    nix build --print-build-logs

[doc("Update the golden files that pin every response's exact bytes")]
golden:
    go test ./internal/ui/ -update -count=1

[doc("Fix and format everything fixable, then lint with no findings allowed")]
lint:
    gofmt -w .
    nixfmt flake.nix
    just docs
    actionlint
    goreleaser check
    shellcheck scripts/*.sh scripts/terminal-demo/*.sh
    golangci-lint run ./...

[doc("Sign the two test accounts in, for working with them by hand")]
login: build
    go run ./scripts/seed --login

[doc("Print the version and the release notes the current CHANGELOG.md would publish")]
notes:
    go run ./scripts/changelog
    go run ./scripts/changelog --notes

[doc("Regenerate openapi.yaml from the WebClients TypeScript source")]
openapi:
    cd scripts && bun install --frozen-lockfile && bun run generate-openapi

[doc("Regenerate the Pass protobuf bindings")]
proto:
    protoc --proto_path=internal/service/pass/proto/protos \
        --go_out=internal/service/pass/proto --go_opt=paths=source_relative \
        internal/service/pass/proto/protos/item-v1.proto \
        internal/service/pass/proto/protos/vault-v1.proto

[doc("Build and run")]
run *args:
    go run . {{ args }}

[doc("Fill the test accounts with the data the suite expects (the suite runs this too)")]
seed *args: build
    go run ./scripts/seed {{ args }}

[doc("Build every release artifact without publishing, the way a tag would")]
snapshot: build
    #!/usr/bin/env bash
    set -euo pipefail
    export AUR_KEY="${AUR_KEY:-unused}"
    export TAP_WINGET_TOKEN="${TAP_WINGET_TOKEN:-unused}"
    placeholders=()
    trap 'rm --force ${placeholders[@]+"${placeholders[@]}"}' EXIT
    for helper in proton-cli-hv-linux-amd64 proton-cli-hv-linux-arm64 \
        proton-cli-hv-darwin-amd64 proton-cli-hv-darwin-arm64 \
        proton-cli-hv-windows-amd64.exe; do
        if [ ! -s "internal/hv/assets/$helper" ]; then
            printf 'placeholder\n' > "internal/hv/assets/$helper"
            placeholders+=("internal/hv/assets/$helper")
        fi
    done
    goreleaser release --snapshot --clean --skip=publish

# How many live tests run at once.
#
# Four, measured: it puts the suite within a minute of what eight does, and these
# are real accounts. Raise it one step at a time and only after a full run shows no
# rate limiting - the suite fails on the first sign of it - and never past eight.
parallel := "4"

# The timeouts say how long a run may take before something is wrong, not how long
# it takes: the suite runs in about three minutes and one test in seconds. A
# timeout of half an hour would let a hang look like a slow day.
[doc("Everything, including the live-API suite against the two test accounts")]
test: test-fast
    go test ./tests/ -v -count=1 -timeout 10m -parallel {{ parallel }} -shuffle=on

[doc("The live suite one test at a time, for a run that looks flaky or an account that has been throttled")]
test-serial: test-fast
    go test ./tests/ -v -count=1 -timeout 20m -parallel 1

[doc("Unit, golden, conformance and offline tests: no API, no credentials, seconds not minutes")]
test-fast:
    go test ./cmd/... ./internal/... ./scripts/... ./tests/offline/ -count=1

[doc("Run a single test (or a `|`-separated regex of test names)")]
test-one pattern:
    go test ./tests/ -v -count=1 -run '{{ pattern }}' -timeout 5m

[doc("Report what the live suite spent its time on, and how deep each command's request graph was")]
test-report *pattern=".":
    #!/usr/bin/env bash
    set -euo pipefail
    trace="${PROTON_CLI_TEST_TRACE:-/tmp/proton-cli-trace.jsonl}"
    PROTON_CLI_TEST_TRACE="$trace" PROTON_CLI_TEST_TRACE_REQUESTS=1 \
        go test ./tests/ -count=1 -run '{{ pattern }}' -timeout 20m -parallel {{ parallel }} || true
    go run ./scripts/testreport "$trace"

[doc("Record which of Proton's API the live suite reaches, for the check that no change quietly narrows it")]
coverage:
    #!/usr/bin/env bash
    set -euo pipefail
    trace="${PROTON_CLI_TEST_TRACE:-/tmp/proton-cli-trace.jsonl}"
    PROTON_CLI_TEST_TRACE="$trace" PROTON_CLI_TEST_TRACE_REQUESTS=1 \
        go test ./tests/ -count=1 -timeout 10m -parallel {{ parallel }}
    go run ./scripts/testreport --coverage "$trace" > tests/api-coverage.golden
    git --no-pager diff --stat tests/api-coverage.golden || true

[doc("Move every dependency and tool to the latest version")]
update:
    go get -u ./...
    go mod tidy
    just vendor-hash
    cd scripts && bun update
    devbox update
    just lint
    just test-fast
    just flake

[doc("Recompute the vendorHash in flake.nix, which every change to go.mod invalidates")]
vendor-hash:
    #!/usr/bin/env bash
    set -euo pipefail
    work=$(just worktree)
    trap 'rm --recursive --force "$work"' EXIT
    sed --in-place 's|vendorHash = "[^"]*"|vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="|' "$work/flake.nix"
    git -C "$work" add --all
    log=$(cd "$work" && nix build --print-build-logs 2>&1 || true)
    hash=$(printf '%s\n' "$log" | sed --quiet 's/^ *got: *//p' | tail -1)
    if [ -z "$hash" ]; then
        printf '%s\n' "$log" >&2
        exit 1
    fi
    sed --in-place "s|vendorHash = \"[^\"]*\"|vendorHash = \"$hash\"|" flake.nix
    printf 'vendorHash = %s\n' "$hash"

[private]
worktree:
    #!/usr/bin/env bash
    set -euo pipefail
    work=$(mktemp --directory)
    git ls-files --cached --others --exclude-standard -z \
        | tar --create --null --files-from=- \
        | tar --extract --directory="$work"
    git -C "$work" init --quiet
    git -C "$work" add --all
    printf '%s\n' "$work"
