# Getting started

## Sign in

```console
$ proton-cli account login
Email:            you@proton.me
Password:
Two-factor code:  123456

✓ Signed in as you@proton.me.
```

That is the whole setup. Signing in saves the session **and** unlocks your keys, so your password is needed once on this machine and not again.

Anything already in the environment is used instead of being asked for:

```bash
export PROTON_USER=you@proton.me
export PROTON_PASSWORD='your-password'
proton-cli account login
```

Two-factor codes rotate every thirty seconds, so `PROTON_TOTP` is rarely useful - let proton-cli ask.

Check where you stand at any time:

```console
$ proton-cli account get
Email:       you@proton.me
Name:        Roman
Storage:     128.4 GB of 500 GB (26%)
Max Upload:  5.0 GB
Profile:     default
Session:     valid
Unlocked:    yes
ID:          Kd91mQxT…
```

## Do something

```bash
proton-cli mail messages list --unread
proton-cli mail messages get "Invoice #2291"
proton-cli drive items list /Documents
proton-cli calendar events list
proton-cli pass items get github.com
proton-cli contacts list
```

Every command documents itself, and the grammar is the same throughout - see [The language](language.md).

## Turn on completion

Completion covers every command and flag, and offers real values as you type: your folder names, item types, output formats and setting keys.

```bash
# zsh
proton-cli completion zsh > "${fpath[1]}/_proton-cli"

# bash
proton-cli completion bash | sudo tee /etc/bash_completion.d/proton-cli

# fish
proton-cli completion fish > ~/.config/fish/completions/proton-cli.fish
```

## More than one account

Each profile keeps its own session, so a personal and a work account never mix:

```console
$ proton-cli --profile work account login
Email:     you@company.com
Password:
✓ Signed in as you@company.com (profile "work").

$ proton-cli --profile work mail messages list
```

See what is signed in on this machine:

```console
$ proton-cli account profiles list
PROFILE   EMAIL             UNLOCKED  SAVED             ACTIVE
────────  ────────────────  ────────  ────────────────  ──────
default   you@proton.me     yes       2026-04-15 14:31  ✓
work      you@company.com   yes       2026-04-15 15:02
```

Make one the default for a shell with `export PROTON_PROFILE=work`. More in [Configuration](configuration.md).

## Sign out

```bash
proton-cli account logout             # forget the session on this machine
proton-cli account logout --revoke    # and invalidate it at Proton
proton-cli account logout --all       # every profile
```

Revoking also makes the credentials saved on this machine useless, even to someone who already copied them. See [Security](../SECURITY.md).

## When something goes wrong

Errors say what happened and what to try:

```console
$ proton-cli mail messages move 5bH2mQxK --into Work
Error: "Work" is a label, not a folder - moving needs a folder.
Try:   to attach the label instead, use `label --label Work`.
       To see the folders, run `proton-cli mail settings folders list`.
```

Before changing anything in bulk, ask what would happen:

```bash
proton-cli mail messages trash --from newsletter@example.com --older-than 90d --dry-run
```

## Where next

| Page | What's in it |
| --- | --- |
| [The language](language.md) | The grammar, the verbs, the filters |
| [Output](output.md) | The four response kinds, JSON, exit codes |
| [References](references.md) | Names, short IDs, compound IDs |
| [Command reference](commands/README.md) | Every command in the tree |
| [Scripting](scripting.md) | Pipelines, `jq`, cron |
