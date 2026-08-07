# Test Guidelines

## Overview

All tests are **integration tests** that run the real `proton-cli` binary against the live Proton API. There are no mocks - every test creates real data, verifies it, and cleans up.

Unit tests live alongside the code they test (e.g. `internal/mailtext/html_test.go`).

Three faster suites run with **no credentials and no network**, and they are the ones to reach for first - `just test-fast` runs all three in seconds:

- **unit** tests, colocated with their source
- **golden** tests in `internal/ui`, which pin the exact bytes of every response kind. Change how something looks and run `just golden`; the diff is the review.
- the **conformance** test in `internal/cli`, which walks the whole command tree and checks the interface's rules: one verb per idea, one meaning per flag, groups that never act, nothing outside `internal/ui` touching a process stream.

An inconsistency is far more likely to be caught there than here.

## Running Tests

```bash
# All integration tests (require credentials in env)
just test

# Single test
just test-one TestDriveItemsMove
```

The suite requires all four of `PROTON_USER`, `PROTON_PASSWORD`, `PROTON_ALT_USER`, and `PROTON_ALT_PASSWORD` (the primary account plus the `alt` second account).

## Test Alt Accounts

Two secondary addresses are available as recipients when a test needs to send to someone other than the account under test:

- **`PROTON_ALT_USER`** ("Proton Alt") - a Proton alt address, read via the `altEmail()` helper. Use it when the recipient must be a Proton address (e.g. drive sharing invitations). Mail may also be sent to it.
- **`rl00@gmx.at`** - a non-Proton (GMX) alt. Use it when a test needs to send to an external, non-Proton mailbox.

### The "Proton Alt" second account (`alt` profile)

"Proton Alt" is also a **full second account** the tests can act *as*, not just send *to*. Use it whenever a scenario genuinely needs two Proton users - accepting a share invitation, receiving and reading mail, or organizing a calendar invite that the primary account RSVPs to.

It's wired through the CLI's per-profile env handling: the `alt` profile reads `PROTON_ALT_USER` / `PROTON_ALT_PASSWORD` (profile-scoped `PROTON_<PROFILE>_X` beats plain `PROTON_X`), with its own session at `~/.config/proton-cli/sessions/alt.json`. Drive commands as the alt run with `--profile alt`:

```go
func TestSecondAccountFoo(t *testing.T) {
    runOK(t, alt("mail", "settings", "addresses", "list")...)    // runs the CLI as the second account
    // ... primary (default profile) and alt interact ...
}
```

- `altEmail()` - the second account's address (`PROTON_ALT_USER`); always configured, since `TestMain` enforces the alt creds up front.
- `alt(args...)` - prefixes `--profile alt`; combine with any runner, e.g. `runOK(t, alt(...)...)`, `runJSON(t, alt(...)...)`.

Run order matters: the *primary* invites/sends, then the *alt* accepts/receives, then verify on whichever side the state landed. Register cleanup on **both** sides (each `alt(...)` mutation needs an `alt(...)` cleanup).

## Layout

```
tests/
├── integration_test.go      TestMain + helpers
├── settings_test.go         account / mail / calendar / drive settings
├── mail_test.go             messages, attachments, conversations, batch filters
├── mail_compose_test.go     drafts, reply, forward, sender selection, signatures
├── mail_export_test.go      .eml and mbox export, --eml import
├── mail_identity_test.go    addresses, display name, signature, auto-reply
├── drive_test.go            items, folders, trash, streaming, recursive, batch filters
├── calendar_test.go         calendars, events, scope-unlock delete
├── contacts_test.go         CRUD, REF resolution, exit codes
├── pass_test.go             vaults, items, alias, batch filters
├── account_test.go          account, session, profiles, account settings
├── contract_test.go         the response contract: envelopes, streams, exit
│                            codes, --dry-run, stdout=ID
├── profile_test.go          --profile / PROTON_PROFILE multi-account
├── short_ids_test.go        short-ID display and resolution
├── leading_dash_ids_test.go IDs that begin with a dash
└── api_test.go              raw `api` escape hatch
```

`contract_test.go` is one file because the contract is one thing.

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
    // Arrange
    folder := "/" + testID() + "-foo"
    runOK(t, "drive", "folders", "create", folder)
    cleanupRun(t, fmt.Sprintf("Delete: proton-cli drive items delete %s", folder),
        "drive", "items", "delete", folder)

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
  ║  ⚠️  CLEANUP FAILED - MANUAL ACTION REQUIRED                 ║
  ╠══════════════════════════════════════════════════════════════╣
  ║  Delete folder: proton-cli drive items delete /test-xxx      ║
  ║  Error: exit 1: ...                                          ║
  ╚══════════════════════════════════════════════════════════════╝
  ```

## Helpers

| Helper | Purpose |
|---|---|
| `run(t, args...)` | Run binary, return stdout/stderr/exitCode |
| `runOK(t, args...)` | Run binary, fail test on non-zero exit, return stdout |
| `runOKStderr(t, args...)` | Same as `runOK` but also returns stderr |
| `runWithStdin(t, stdin, args...)` | Run with a custom stdin reader |
| `runJSON(t, args...)` | Adds `--output json`, parses stdout as JSON **object** |
| `runJSONArray(t, args...)` | Adds `--output json`, unwraps the collection envelope, returns the rows |
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
func TestMailMessagesGetBodyOnly(t *testing.T) {
    msgID, _, subject := plainMail(t)    // shared, no send/poll
    stdout := runOK(t, "mail", "messages", "get", "--body-only", msgID)
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
stdout := runOK(t, "mail", "settings", "labels", "create", "--name", name, "--color", "#8080FF")
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

`--output text|json|yaml` (default `text`).

Every collection comes out as an envelope keyed by its plural noun, always with a `count`, so one consumer reads any list:

```json
{ "messages": [ … ], "count": 3, "total": 47, "page": 0, "page_size": 3, "has_more": true }
```

`runJSONArray` unwraps it and cross-checks the count. Keys are `snake_case`, enumerated values are names rather than Proton's numbers (`"type": "file"`, not `"type": 2`), and IDs are never shortened. A mutation emits a result object rather than a bare ID, so `--output json` always means JSON.

### REF arguments

Every command that takes an ID also accepts a substring search term. Ambiguous matches return exit 4 with candidates listed on stderr.

Two-ID references are one slash-separated token: `pass items get SHARE_ID/ITEM_ID`, `calendar events get CALENDAR_ID/EVENT_ID`. Short IDs work on both halves.

**Drive addresses items by `PATH`**, because that is what Proton resolves and what a person means. Things with no place in the tree - trashed items, photos, albums - are addressed by the `REF` their list showed.

### Local validation precedes the network

Anything judgeable from the command line alone must fail without a session: an unknown setting key, a value outside a declared domain, a colour off Proton's palette, a missing required flag. A test for one of those should not need credentials to reach the error, and should assert the whole accepted domain appears in the message.

## Cobra and Positional IDs

Proton IDs are Base64URL-encoded and can start with `-`. Two layers in the binary handle this automatically:

1. **Auto-`--` injection** (`internal/cli/dashids.go` `preprocessArgs`) detects a leading-dash token shaped like a full Proton ID (≥60 chars, ends `==`, URL-safe base64) and inserts `--` before it before cobra parses argv.
2. **Layer-C error rewrap** (`internal/cli/dashids.go` `rewrapFlagError`) replaces cobra/pflag's flag-parse and "accepts N args" errors with a hint mentioning `--` when a leading-dash ID is detected in argv.

In practice `--` is **not required** in tests - leading-dash full IDs parse cleanly via Layer 1; leading-dash *short* IDs (rare - ~1.5% of random base64 prefixes) need explicit `--` and are caught by Layer 2.

**Put flags before the positionals.** Layer 1 protects the ID by inserting `--`, and everything after a `--` is positional - so a flag written after a leading-dash ID becomes an argument and the command fails with "accepts N arg(s)". Whether that happens depends on which ID the account handed out, so writing the flags last makes a test fail roughly one run in sixty rather than never:

```go
runOK(t, "mail", "messages", "attachments", "download", "--output-dir", dir, msgID)   // always works
runOK(t, "mail", "messages", "attachments", "download", msgID, "--output-dir", dir)   // works until the ID starts with '-'
```

`runJSON` and `runJSONArray` put `--output json` in front for the same reason, so they are safe with any reference.

### `cleanupRun` descriptions

The copy-pasteable commands surfaced on cleanup failure do not need `--`; the user can paste them as-is even if the ID starts with a dash.

## Naming

- Test artifacts use the `proton-cli-test-{ms}-{rand}-{purpose}` prefix (from `testID()`).
- This makes them identifiable in the Proton UI if cleanup ever fails.
- Never use short or common names that could collide with real data.

## Known Limitations

- `calendar settings calendars delete` hits an endpoint Proton guards behind an elevated session. Nothing in the command arranges that: the client elevates when the server asks, using `PROTON_PASSWORD`, and drops the scope again. It works in tests because the variable is set.
- `drive trash empty` may not clear items from non-default volumes (e.g. Photos share).
- Proton only allows specific hex colors for labels, folders, calendars and contact groups (e.g. `#8080FF`, `#3CBB3A`) - see `ACCENT_COLORS` in the WebClients source. The CLI refuses anything else locally, before a request.
- `just test` runs serially; expect ~8-12 minutes (down from ~17) after the shared-fixture and polling work (30m timeout).
- Mail-delivery latency is inherent: a self-mail's inbox copy lands a few seconds after send. Amortize it with a shared fixture rather than paying it per test.
