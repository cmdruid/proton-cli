# Test Guidelines

## Overview

All tests are **integration tests** that run the real `proton-cli` binary against the live Proton API. There are no mocks - every test creates real data, verifies it, and cleans up.

Unit tests live alongside the code they test (e.g. `internal/render/html_test.go`).

## Running Tests

```bash
# All integration tests (require credentials in env)
just test

# Single test
just test-one TestDriveItemsMove
```

Tests skip automatically if `PROTON_USER` and `PROTON_PASSWORD` are not set.

## Test Alt Accounts

Two secondary addresses are available as recipients when a test needs to send to someone other than the account under test:

- **`protonalt.sessions986@proton.me`** - a Proton alt. Use it when the recipient must be a Proton address (e.g. drive sharing invitations). See the `testInvitee` constant in `drive_sharing_test.go`. Mail may also be sent to it.
- **`rl00@gmx.at`** - a non-Proton (GMX) alt. Use it when a test needs to send to an external, non-Proton mailbox.

## Layout

```
tests/
├── integration_test.go      TestMain + helpers
├── settings_test.go
├── mail_test.go             messages, attachments, labels, filters, addresses, batch filters
├── drive_test.go            items, folders, trash, streaming, recursive, batch filters
├── calendar_test.go         calendars, events, scope-unlock delete
├── contacts_test.go         CRUD, REF resolution, exit codes
├── pass_test.go             vaults, items, alias, batch filters
├── output_test.go           --output text / json / yaml
├── exit_codes_test.go       0 / 1 / 3 / 4 mapping
├── profile_test.go          --profile + config.toml multi-account
├── api_test.go              raw `api` escape hatch
├── dry_run_test.go          --dry-run does not mutate
└── stdout_id_test.go        stdout=ID convention across creates
```

## How Tests Work

1. `TestMain` in `integration_test.go` builds the binary once into a temp directory.
2. Each test calls the binary as a subprocess via `run()` / `runOK()` / `runJSON()`.
3. The binary picks up `PROTON_USER` / `PROTON_PASSWORD` from the environment.
4. Tests are sequential (no `t.Parallel()`) to avoid rate limits and shared-state conflicts.

## Writing a Test

Follow **Arrange → Act → Assert**. Every test that creates data must register cleanup **immediately after creation**, before any assertion that might fail.

```go
func TestDriveItemsFoo(t *testing.T) {
    skipIfNoCredentials(t)

    // Arrange
    folder := "/" + testID() + "-foo"
    runOK(t, "drive", "folders", "create", folder)
    cleanupRun(t, fmt.Sprintf("Delete: proton-cli drive items delete --permanent %s", folder),
        "drive", "items", "delete", "--permanent", folder)

    // Act
    stdout := runOK(t, "drive", "items", "list", folder)

    // Assert
    assertContains(t, stdout, "expected-string")
}
```

## Cleanup Rules

- **Always register cleanup**, even for tests about deletion - the test might fail before reaching the delete step.
- Use `cleanupRun()` for CLI commands, `cleanup()` for custom functions.
- `t.Cleanup()` guarantees cleanup runs even on test failure.
- Cleanup failures print a loud box with a copy-pasteable command the user can run manually:

  ```
  ╔══════════════════════════════════════════════════════════════╗
  ║  ⚠️  CLEANUP FAILED - MANUAL ACTION REQUIRED                ║
  ╠══════════════════════════════════════════════════════════════╣
  ║  Delete folder: proton-cli drive items delete --permanent /test-xxx
  ║  Error: exit 1: ...
  ╚══════════════════════════════════════════════════════════════╝
  ```

## Helpers

| Helper | Purpose |
|---|---|
| `skipIfNoCredentials(t)` | Skip test if env vars not set |
| `run(t, args...)` | Run binary, return stdout/stderr/exitCode |
| `runOK(t, args...)` | Run binary, fail test on non-zero exit, return stdout |
| `runOKStderr(t, args...)` | Same as `runOK` but also returns stderr |
| `runWithStdin(t, stdin, args...)` | Run with a custom stdin reader |
| `runJSON(t, args...)` | Adds `--output json`, parses stdout as JSON **object** |
| `runJSONArray(t, args...)` | Adds `--output json`, parses stdout as JSON **array** |
| `testID()` | Unique `proton-cli-test-{ms}-{rand}` prefix |
| `cleanupRun(t, desc, args...)` | Register cleanup that runs the CLI |
| `cleanup(t, desc, func)` | Register cleanup with a custom function |
| `assertContains(t, stdout, substr)` | Assert stdout contains substring |
| `assertNotContains(t, stdout, substr)` | Assert stdout does not contain substring |
| `assertField(t, stdout, field, expected)` | Assert `Key: Value` line matches |
| `sendTestMail(t, subject)` | Send a mail to self, register cleanup, return inbox ID |
| `selfEmail()` | Return `PROTON_USER` |
| `looksLikeID(s)` | Heuristic: Proton base64 IDs end in `==` |

## Conventions the tests rely on

These are stable CLI guarantees that tests verify:

### stdout = new ID on create

Every create command writes **just the new ID** (one line, no JSON, no trailing text) to stdout and `✓ …` to stderr:

```go
stdout := runOK(t, "mail", "labels", "create", "--name", name, "--color", "#8080FF")
id := strings.TrimSpace(stdout)
// id is a bare 88-char Proton ID; stderr carried the human message.
```

This makes shell capture work: `ID=$(proton-cli ... create ...)`.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | user error (bad flag, missing arg, invalid input) |
| 2 | auth |
| 3 | not-found (REF matched no resource) |
| 4 | conflict / ambiguous (REF matched multiple resources) |
| 5 | network / 5xx |
| 130 | cancelled via Ctrl+C |

### Output format

`--output text|json|yaml` (default `text`). JSON output uses `snake_case` keys (json tags); YAML respects the same tags via `goccy/go-yaml`'s json-tag fallback.

### REF arguments

Every command that takes an ID also accepts a substring search term. Ambiguous matches return exit 4 with candidates listed on stderr.

`drive trash restore` is the single exception - it requires explicit link IDs because trashed items have encrypted names.

## Cobra and Positional IDs

Proton IDs are Base64URL-encoded and can start with `-`. Two layers in the
binary handle this automatically:

1. **Auto-`--` injection** (`internal/cli/dashids.go` `preprocessArgs`) detects a
   leading-dash token shaped like a full Proton ID (≥60 chars, ends
   `==`, URL-safe base64) and inserts `--` before it before cobra
   parses argv.
2. **Layer-C error rewrap** (`internal/cli/dashids.go` `rewrapFlagError`) replaces
   cobra/pflag's flag-parse and "accepts N args" errors with a hint
   mentioning `--` when a leading-dash ID is detected in argv.

In practice `--` is **no longer required** in tests - leading-dash full
IDs parse cleanly via Layer 1; leading-dash *short* IDs (rare - ~1.5%
of random base64 prefixes) need explicit `--` and are caught by Layer 2.

Flags can be placed before OR after positionals on every command -
same as any normal cobra CLI.

### `cleanupRun` descriptions

The copy-pasteable commands surfaced on cleanup failure no longer need
`--`; the user can paste them as-is even if the ID starts with a dash.
Keeping `--` in those strings is harmless but unnecessary.

## Naming

- Test artifacts use the `proton-cli-test-{ms}-{rand}-{purpose}` prefix (from `testID()`).
- This makes them identifiable in the Proton UI if cleanup ever fails.
- Never use short or common names that could collide with real data.

## Known Limitations

- `calendar calendars delete` requires `PROTON_PASSWORD` for the password-scope unlock - works in tests because the env var is set.
- `drive trash empty` may not clear items from non-default volumes (e.g. Photos share).
- Proton only allows specific hex colors for labels and calendars (e.g. `#8080FF`, `#3CBB3A`) - see `ACCENT_COLORS` in the WebClients source.
- The full suite runs ~20-25 minutes due to API latency and mail-delivery waits (`just test` uses a 30m timeout).
