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

- **`protonalt.sessions986@proton.me`** ("Proton Alt") - a Proton alt. Use it when the recipient must be a Proton address (e.g. drive sharing invitations). See the `testInvitee` constant in `drive_sharing_test.go`. Mail may also be sent to it.
- **`rl00@gmx.at`** - a non-Proton (GMX) alt. Use it when a test needs to send to an external, non-Proton mailbox.

### The "Proton Alt" second account (`alt` profile)

"Proton Alt" is also a **full second account** the tests can act *as*, not just send *to*. Use it whenever a scenario genuinely needs two Proton users - accepting a share invitation, receiving and reading mail, or organizing a calendar invite that the primary account RSVPs to.

It's wired through the CLI's per-profile env handling: the `alt` profile reads `PROTON_ALT_USER` / `PROTON_ALT_PASSWORD` (profile-scoped `PROTON_<PROFILE>_X` beats plain `PROTON_X`), with its own session at `~/.config/proton-cli/sessions/alt.json`. Drive commands as the alt run with `--profile alt`:

```go
func TestSecondAccountFoo(t *testing.T) {
    skipIfNoAltCredentials(t)                       // skips unless PROTON_ALT_USER/PASSWORD set
    runOK(t, alt("mail", "addresses", "list")...)    // runs the CLI as the second account
    // ... primary (default profile) and alt interact ...
}
```

- `skipIfNoAltCredentials(t)` - skip unless both primary and `PROTON_ALT_*` creds are set.
- `altEmail()` - the second account's address (`PROTON_ALT_USER`).
- `alt(args...)` - prefixes `--profile alt`; combine with any runner, e.g. `runOK(t, alt(...)...)`, `runJSON(t, alt(...)...)`.

Run order matters: the *primary* invites/sends, then the *alt* accepts/receives, then verify on whichever side the state landed. Register cleanup on **both** sides (each `alt(...)` mutation needs an `alt(...)` cleanup).

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
├── profile_test.go          --profile / PROTON_PROFILE multi-account
├── api_test.go              raw `api` escape hatch
├── dry_run_test.go          --dry-run does not mutate
└── stdout_id_test.go        stdout=ID convention across creates
```

## How Tests Work

1. `TestMain` in `integration_test.go` builds the binary once into a temp directory.
2. Each test calls the binary as a subprocess via `run()` / `runOK()` / `runJSON()`.
3. The binary picks up `PROTON_USER` / `PROTON_PASSWORD` from the environment; the session is persisted per profile and reused across invocations.
4. Tests are **sequential** (no `t.Parallel()`) to avoid rate limits and shared-state conflicts.

The cost of an integration test is almost entirely **network latency** - process spawn is ~4ms, but a single authenticated call is ~150-300ms and an unlock-requiring one ~0.5-1.7s. The dominant cost is tests sending a self-mail and polling for delivery. Two mechanisms below cut that: **shared fixtures** (send once, reuse) and **check-first polling**.

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
- Use `cleanupRun()` for CLI commands, `cleanup()` for custom functions - both are `t.Cleanup`-based (per-test).
- **Shared fixtures** (see below) outlive individual tests, so they register **suite-scoped** cleanup via `registerSuiteCleanup(desc, args...)`, which `TestMain` flushes after `m.Run()`. Never use `t.Cleanup` for a shared fixture - it would delete the fixture out from under other tests.
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
| `runArgs(stdin, args...)` | `t`-free runner (stdout, stderr, code, err); used by fixtures and suite cleanup |
| `sendTestMail(t, subject)` | Send a mail to self, register per-test cleanup, return inbox ID. **Only for mutating / send-path tests** - read-only tests use a shared fixture |
| `plainMail(t)` / `quotedMail(t)` / `sharedAttachment(t)` | Shared, delivered self-mail fixtures (see Performance section) |
| `waitFor(timeout, interval, check)` | Poll `check` (checks first, then sleeps) until true or timeout |
| `messageIDInFolder(folder, subject)` | `t`-free: first message ID in a folder matching subject, or `""` |
| `registerSuiteCleanup(desc, args...)` | Queue a CLI cleanup to run once at suite teardown (for shared fixtures) |
| `selfEmail()` | Return `PROTON_USER` |
| `looksLikeID(s)` | Heuristic: Proton base64 IDs end in `==` |
| `skipIfNoAltCredentials(t)` | Skip unless the `alt` second account (`PROTON_ALT_USER`/`PROTON_ALT_PASSWORD`) is configured |
| `altEmail()` | Return `PROTON_ALT_USER` (the second account's address) |
| `alt(args...)` | Prefix `--profile alt`; run a command as the second account, e.g. `runOK(t, alt(...)...)` |

## Performance: shared fixtures & polling

Most mail tests only need *some* readable message, not a freshly sent one. Sending and polling for delivery per test is the single biggest time sink, so read-only tests share a handful of messages created once per suite.

### Shared fixtures (read-only tests)

| Fixture accessor | What it gives you | Use for |
|---|---|---|
| `plainMail(t)` | `(msgID, convID, subject)` - a delivered self-mail with a plain body (no quote markers, no attachments) | reading, formats, body-only, redirects, summaries, search-hit |
| `quotedMail(t)` | `(msgID, subject)` - body carries the canonical `On <date>, <name> <addr> wrote:` reply block | strip-quotes assertions |
| `sharedAttachment(t)` (aka `findMessageWithAttachment(t)`) | `(msgID, attID, attName)` - a delivered mail with one non-inline attachment | attachment list/download/footer tests |
| `sharedMixedAttachment(t)` (aka `findMessageWithMixedAttachments(t)`) | `msgID` - a delivered mail with one **inline** image + one regular attachment (sent via `--attach-inline`) | inline-vs-attachment disposition filter tests |

They are created lazily under `sync.Once` on first use, so the send+deliver wait happens at most once per fixture per run, and they are safe to call from parallel tests.

**Rule of thumb:**

- **Read-only** test (never mutates its message) → use a shared fixture.
- **Mutating** test (mark / star / move / trash) or a test of the **send path itself** (attachments, HTML, scheduled, expiring, EO) → send your own with `sendTestMail(t, subject)`.

```go
func TestMailMessagesReadBodyOnly(t *testing.T) {
    skipIfNoCredentials(t)
    msgID, _, subject := plainMail(t)    // shared, no send/poll
    stdout := runOK(t, "mail", "messages", "read", "--body-only", msgID)
    assertContains(t, stdout, subject)
}
```

### Polling for delivery

Use `waitFor(timeout, interval, check)` - it checks **before** the first sleep, so an already-true condition is free. Never write a `for { time.Sleep(2*time.Second); ... }` loop. `messageIDInFolder(folder, subject)` and `conversationIDOf(msgID)` are `t`-free lookups that pair well with `waitFor`.

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
- `just test` runs serially; expect ~8-12 minutes (down from ~17) after the shared-fixture and polling work (30m timeout).
- Mail-delivery latency is inherent: a self-mail's inbox copy lands a few seconds after send. Amortize it with a shared fixture rather than paying it per test.
