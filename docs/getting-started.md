# Getting started

## Sign in

```console
$ proton account login
Email:            you@proton.me
Password:
Two-factor code:  123456

✓ Signed in as you@proton.me.
```

That is the whole setup. Signing in saves the session **and** unlocks your keys, so your password is needed once on this machine and not again.

No terminal to answer the prompts? Name the account and point at the password:

```bash
proton account login --user you@proton.me --password-file /run/secrets/proton
```

A password is never a flag value, because argv is readable by every user on the machine through `ps`. Use `--password-file`, or pipe it in with `--password-stdin`.

Two-factor codes rotate every thirty seconds, so `--totp` is only useful at the moment you sign in - otherwise let proton ask.

Check where you stand at any time:

```console
$ proton account get
Email:       you@proton.me
Name:        Roman
Storage:     ━━━━━───────────────   26%  128.4 GB of 500.0 GB
Max Upload:  5.0 GB
Profile:     default
Session:     valid
Unlocked:    yes
ID:          Kd91mQxT…
```

## Do something

```bash
proton mail messages list --unread
proton mail messages get "Invoice #2291"
proton drive items list /Documents
proton calendar events list
proton pass items get github.com
proton contacts list
```

Every command documents itself, and the grammar is the same throughout - see [The language](language.md).

## Turn on completion

Completion covers every command and flag, and offers real values as you type: your folder names, item types, output formats and setting keys.

```bash
# zsh
proton completion zsh > "${fpath[1]}/_proton"

# bash
proton completion bash | sudo tee /etc/bash_completion.d/proton

# fish
proton completion fish > ~/.config/fish/completions/proton.fish
```

## More than one account

Each profile keeps its own session, so a personal and a work account never mix:

```console
$ proton --profile work account login
Email:     you@company.com
Password:
✓ Signed in as you@company.com (profile "work").

$ proton --profile work mail messages list
```

See what is signed in on this machine:

```console
$ proton account profiles list
PROFILE   EMAIL             UNLOCKED  SAVED             ACTIVE
────────  ────────────────  ────────  ────────────────  ──────
default   you@proton.me     yes       2026-04-15 14:31  ✓
work      you@company.com   yes       2026-04-15 15:02
```

Make one the default for a shell with `export PROTON_PROFILE=work`. More in [Configuration](configuration.md).

## Sign out

```bash
proton account logout             # forget the session on this machine
proton account logout --revoke    # and invalidate it at Proton
proton account logout --all       # every profile
```

Revoking also makes the credentials saved on this machine useless, even to someone who already copied them. See [Security](../SECURITY.md).

## When something goes wrong

Errors say what happened and what to try:

```console
$ proton mail messages move 5bH2mQxK --into Work
Error: "Work" is a label, not a folder - moving needs a folder.
Try:   to attach the label instead, use `label --label Work`.
       To see the folders, run `proton mail settings folders list`.
```

Before changing anything in bulk, ask what would happen:

```bash
proton mail messages trash --from newsletter@example.com --older-than 90d --dry-run
```

## Where next

| Page | What's in it |
| --- | --- |
| [The language](language.md) | The grammar, the verbs, the filters |
| [Output](output.md) | The four response kinds, JSON, exit codes |
| [References](references.md) | Names, short IDs, compound IDs |
| [Command reference](commands/README.md) | Every command in the tree |
| [Scripting](scripting.md) | Pipelines, `jq`, cron |
