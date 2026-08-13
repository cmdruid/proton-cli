# Test Guidelines

## Overview

All tests are **integration tests** that run the real `proton-cli` binary against the live Proton API. There are no mocks - every test creates real data, verifies it, and cleans up.

Unit tests live alongside the code they test (e.g. `internal/mailtext/html_test.go`).

Faster suites run with **no credentials and no network**, and they are the ones to reach for first - `just test-fast` runs all in about a second:

- **unit** tests, colocated with their source
- **golden** tests in `internal/ui`, which pin the exact bytes of every response kind. Change how something looks and run `just golden`; the diff is the review.
- the **conformance** test in `internal/cli`, which walks the whole command tree and checks the interface's rules: one verb per idea, one meaning per flag, groups that never act, nothing outside `internal/ui` touching a process stream.
- the **offline** suite in `tests/offline`, which runs the real binary with no session and the API pointed at a dead port. Everything judgeable from the command line alone belongs there: a value outside a declared domain, a colour off Proton's palette, a reference shaped like nothing, an argument count. Each case costs about 5ms, against about 500ms and a set of credentials in the live suite.

An inconsistency is far more likely to be caught there than here.

## Running Tests

```bash
just test                             # all integration tests, four at a time
just test-one TestDriveItemsMove      # a single one
just test-serial                      # one at a time, for a run that looks flaky
just test-report                      # where the time went, and how deep each request graph was
```

`just login` and `just seed` sign the accounts in and fill them, for working with them by hand.

The suite requires all four of `PROTON_CLI_TEST_PRIMARY_USER`, `PROTON_CLI_TEST_PRIMARY_PASSWORD`, `PROTON_CLI_TEST_SECONDARY_USER` and `PROTON_CLI_TEST_SECONDARY_PASSWORD`.

## The two accounts

The suite creates, mutates and deletes real data, so it runs on two accounts kept for that and nothing else. Most tests act as **`primary`**; the handful that genuinely need two Proton users bring in **`secondary`**.

These are the harness's own variables, not the CLI's: proton-cli takes an account from a signed-in profile, which `TestMain` establishes. The `PROTON_CLI_TEST_` prefix keeps them clear of anything the binary reads.

`TestMain` signs both profiles in before any test runs, over stdin, and writes each password to a `0600` file for the rest of the run. The file is needed because a session cannot carry elevation: Proton re-authenticates over SRP for its guarded operations, `calendar settings calendars delete` among them, and that needs the password itself - the key blob sealed at login is a one-way derivation of it. `account login` is idempotent, so a run that reuses an existing session pays nothing.

`runAs` builds the child environment from an **allowlist** rather than inheriting one. It is the single place a target account is chosen, so it is the single place the choice can be enforced: whatever you happen to have exported, the binary under test sees a stated environment and can act only as the profile named there.

### Acting as the second account

Use it whenever a scenario genuinely needs two Proton users - accepting a share invitation, receiving and reading mail, or organizing a calendar invite the primary RSVPs to.

```go
func TestSecondAccountFoo(t *testing.T) {
    runOKSecondary(t, "mail", "settings", "addresses", "list")   // runs as the second account
    // ... primary and secondary interact ...
}
```

Run order matters: the *primary* invites or sends, then the *secondary* accepts or receives, then verify on whichever side the state landed. Register cleanup on **both** sides - a mutation made as the secondary needs `cleanupRunSecondary`.

An **external, non-Proton** recipient comes from `PROTON_CLI_TEST_EXTERNAL_RECIPIENT`. Tests that need one skip when it is unset. Sending to a fake `@example.com` address instead bounces (nullMX) and litters the inbox with MAILER-DAEMON returns.

## Layout

```
tests/
├── integration_test.go      TestMain + helpers
├── lease_test.go            what two tests cannot both have, and the guards
├── trace_test.go            the per-invocation trace `just test-report` reads
├── fixture/                  what the seed puts on the account for the suite to read
├── offline/                  the real binary, no session, no network
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
└── api_test.go              raw `api` escape hatch
```

`contract_test.go` is one file because the contract is one thing.

## How Tests Work

1. `TestMain` in `integration_test.go` builds the binary once into a temp directory.
2. Each test calls the binary as a subprocess via `run()` / `runOK()` / `runJSON()`.
3. `TestMain` signs both profiles in and runs `scripts/seed/seed.sh`; each checks before it acts, so an account already in shape costs a read.
4. Each invocation names its profile through `PROTON_PROFILE` in a scrubbed child environment, and the session is reused across invocations.
5. Tests run **four at a time**, and every one of them calls `t.Parallel()`. What two tests cannot both have at once is declared and leased - see below.

## Running four at a time

The suite is bound by waiting for Proton, not by doing anything, so tests overlap. Proton absorbs it easily: sixteen concurrent invocations were measured to cost the same wall time as one. The reason the setting is **four** rather than sixteen is that these are real accounts, and four already lands within a minute of what eight does.

The safeguards, in the order they bite:

- **A single 429 fails the run.** The client backs off and would very likely succeed, so the suite would otherwise pass and teach nobody anything. Proton's first sign of displeasure stops the run and says to lower the concurrency.
- **Sends never overlap.** `runAs` takes the `sending` lease for any command that puts a message on the wire, so no test has to remember. It is held for the send and not for the wait after it.
- **The client holds at most 8 requests in flight**, and refreshes a session once per process however many requests discover it expired together.
- **Raise `parallel` in the justfile one step at a time**, only after a full run shows no rate limiting, and never past eight.

### Leases: what two tests cannot both have

Almost nothing needs a lease. Each test makes its own labels, folders, events and items under its own `testID()` and asserts on those. Two things do:

- an account has exactly **one** of some things - its settings, an address's signature, the auto-reply;
- the free plan allows only a few **calendars, vaults, labels, mail folders and filters**, and the fixture already holds one of each, so a test that makes one takes the spare slot;
- a few tests identify their own work by **comparing a listing before and after**, which another test's work would appear in (photos have no name in a listing, so there is no other way).

Those are named in `lease_test.go` and taken with `lease(t, ...)`, which holds them for the test and releases them after. This is what `t.Parallel()` alone cannot say: two tests that exclude each other but nobody else.

**Both rules are checked, not remembered.** `TestEveryTestThatTouchesSharedStateLeasesIt` reads every test's source and fails if it touches something shared without leasing it; `TestEveryTestRunsInParallelUnlessItSaysWhy` fails on a test that is neither parallel nor listed in `serialTests` with a reason. A test that runs alone gets the whole account to itself for free, because Go finishes every non-parallel test before any parallel one resumes - which is exactly what the one test that rewrites the shared ID cache file needs.

When a new conflict appears - and it will appear as a test failing somewhere it has nothing to do with, like `Number of calendars exceeded limit` - the fix is one line in the vocabulary and one `lease` call, and the guard finds every test that needs it.

## What a run costs, and how to know

A run is spent almost entirely **inside invocations of the binary** - measured, 100% of the wall clock, with no idle time between them. So there are exactly three ways to make the suite faster: fewer invocations, cheaper invocations, or invocations that overlap.

An invocation's cost is the **depth of its request graph**, not the number of assertions in the test. A command that asks Proton for eight things one after another waits for all eight; one that asks for them together waits for the slowest. Do not guess which is which - measure it:

```bash
just test-report                    # per command: invocations, time, requests, overlap
just test-report TestCalendar       # or one slice of it
just test-coverage                  # every METHOD + path template the run reached
```

`overlap` is 1.0 for a strict chain and higher when requests were made together. The report ends with **chains worth flattening**: commands with several requests and an overlap near 1.0. That list is the work, in order.

`just test-coverage` is the guard rail on all of it: it prints the API surface the run actually exercised. An optimisation is only acceptable if that set does not shrink - a response nobody parsed is a response Proton could change without a test noticing.

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
- **Never clean up a seeded fixture.** The account holds it between runs on purpose; deleting it makes the next run send it again and spend the allowance this exists to protect.
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
| `sendTestMail(t, subject)` | Send a mail to self, register per-test cleanup, return inbox ID. **Only when the send itself is the subject** |
| `plainMail(t)` / `quotedMail(t)` / `sharedAttachment(t)` / `mutableMail(t)` | Seeded mail fixtures (see the sending-allowance section) |
| `waitFor(timeout, interval, check)` | Poll `check` (checks first, then sleeps) until true or timeout |
| `messageIDInFolder(folder, subject)` | `t`-free: first message ID in a folder matching subject, or `""` |
| `selfEmail()` | The primary account's address |
| `looksLikeID(s)` | Heuristic: Proton base64 IDs end in `==` |
| `secondaryEmail()` | The second account's address |
| `runSecondary` / `runOKSecondary` / `runJSONSecondary` / `runJSONArraySecondary` / `cleanupRunSecondary` | The same runners, as the second account |
| `lease(t, ...)` | Take exclusive use of shared state for this test (see Leases) |
| `externalRecipient(t)` | A non-Proton recipient; skips the test when none is configured |

## The sending allowance, and the fixtures that protect it

The accounts are on the free plan, which allows **50 messages an hour and 150 a day**, counted per recipient. A run sends about 19. That, not the wall clock, is what decides how often the suite can be run: a suite that sends 30 can be run once an hour however fast it is.

So a message is only sent when the sending is the thing being tested. Everything that merely needs *a* message of some shape reads one the account already holds: `tests/fixture` declares them, `scripts/seed` puts them there, and both read the same declaration so they cannot drift. Nothing sends them per run.

### Seeded fixtures (read-only tests)

| Fixture accessor | What it gives you | Use for |
|---|---|---|
| `plainMail(t)` | `(msgID, convID, subject)` - a delivered self-mail with a plain body (no quote markers, no attachments); its body contains its subject | reading, formats, body-only, redirects, summaries, search-hit |
| `quotedMail(t)` | `(msgID, subject)` - body carries the canonical `On <date>, <name> <addr> wrote:` reply block | strip-quotes assertions |
| `sharedAttachment(t)` (aka `findMessageWithAttachment(t)`) | `(msgID, attID, attName)` - a delivered mail carrying one regular attachment | attachment list/download/footer tests |
| `sharedMixedAttachment(t)` (aka `findMessageWithMixedAttachments(t)`) | `msgID` - the same message, which also carries an **inline** image | inline-vs-attachment disposition filter tests |
| `mutableMail(t)` | `msgID` of a message this test may change and change back | mark / star / move / trash round-trips |

The lookups happen at most once per run under `sync.OnceValues`, and are safe to call from parallel tests. `mutableMail` hands out one message from a pool, so two tests running together never change the same one; the state is put back if the test failed before it could.

**Rule of thumb:**

- **Read-only** test (never mutates its message) → use a seeded fixture.
- **Mutating** test (mark / star / move / trash) → `mutableMail(t)`.
- A test of the **send path itself** (attachments, inline images, HTML, scheduled, expiring, encrypted-for-outside, `--eml`, cross-account) → send your own. Each distinct send shape keeps exactly one such test: that is what makes a change to Proton's send path fail something.
- A test whose subject is a **flag on the parent** (`reply` setting `IsReplied`) → send your own, because a seeded parent would already be flagged and the assertion would pass for the wrong reason.

```go
func TestMailMessagesGetBodyOnly(t *testing.T) {
    msgID, _, subject := plainMail(t)    // seeded, no send, no delivery wait
    stdout := runOK(t, "mail", "messages", "get", "--body-only", msgID)
    assertContains(t, stdout, subject)
}
```

Adding a fixture means adding it to `tests/fixture` and running `just seed`. A test whose fixture is missing says so and names the command.

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

Anything judgeable from the command line alone must fail without a session: an unknown setting key, a value outside a declared domain, a colour off Proton's palette, a missing required flag, a selection that names nothing. A test for one of those belongs in `tests/offline`, where it costs 5ms and no credentials, and should assert the whole accepted domain appears in the message.

This holds because **no step asserts that an account exists**. The requirement belongs to the request, and the client holds it there (`SetSessionGuard`), so a command body judges what it can judge and only then finds out whether anyone is signed in. Two places keep the requirement earlier, on purpose:

- **unlocking keys**, because there are none to unlock for an account nobody is signed in to, and asking for a password to open them would be asking the wrong question;
- **a dry run of a mutation**, because a preview is a claim about what the command would do, and without an account it would not do it.

So a command that declares `kit.StepUnlock` answers "not signed in" before its own checks unless it declares them as a step first - which is what `kit.StepSelection` is for, and what `drive items delete` and `pass items trash` use. If you add a check to a key-using command and want it judged from the command line, put it in a step ahead of `StepUnlock` and move its test to `tests/offline`.

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

## Paid-plan features

The accounts are free ones, and Proton gates some features behind a plan. A test that reaches one skips rather than fails, because no seeding can make a free account able to do it:

| Feature | How it refuses |
| --- | --- |
| Contact groups | HTTP 401, Proton code `2027` |
| Auto-reply | a message naming "upgrade", "paid" or "subscription" |

Match the code where there is one; the sentence is Proton's to reword.

Auto-reply is worth a note: Proton refuses it with 9100, the same code it uses for a missing scope, so the CLI elevates the session first and only then hears the real reason. That is why `mail settings autoreply set` carries the credential flags even though the answer is a subscription and not a password.

## Known Limitations

- `calendar settings calendars delete` hits an endpoint Proton guards behind an elevated session. Nothing in the command arranges that: the client elevates when the server asks, using the password `runAs` supplies through `--password-file`, and drops the scope again.
- `drive trash empty` may not clear items from non-default volumes (e.g. Photos share).
- Proton only allows specific hex colors for labels, folders, calendars and contact groups (e.g. `#8080FF`, `#3CBB3A`) - see `ACCENT_COLORS` in the WebClients source. The CLI refuses anything else locally, before a request.
- `just test-report` says where the time went. The report ends with the request chains still worth flattening; `drive items upload` and `pass items delete` are genuinely deep and the rest is measured.
- Mail-delivery latency is inherent: a self-mail's inbox copy lands a few seconds after send. Only the tests whose subject is the send pay it.
- `TestDriveItemsUploadManyBlocks` uploads 44 MiB and takes about 20 seconds. It is the only test that makes the CLI ask for more than one batch of block links, so it is the only one that would notice if Proton lowered the number of links a single request may ask for. It stays.
