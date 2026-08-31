# Quickstart

Sign in once, then read your mail, move files in and out of Drive, check your calendar and pull a two-factor code, all from the shell. This is the five minutes from a fresh install to a useful command.

## Sign in

```console
$ proton account login
Email:            you@proton.me
Password:
Two-factor code:  123456

✓ Signed in as you@proton.me.
```

That is the whole setup. Signing in saves the session **and** unlocks your keys, so your password is needed once on this machine and not again.

No terminal to answer the prompts? Name the account and point at the password ([never a flag value](design-notes.md#why-a-password-is-never-a-flag-value)):

```bash
proton account login --user you@proton.me --password-file /run/secrets/proton
```

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

Every command reads the same way - `proton <app> <collection> <verb>` - and anywhere one wants an ID, a subject, name, path or address works too. That grammar is the whole trick: [learn it once](language.md) and you can guess the rest.

Every command also documents itself, and points at its own page in the reference:

```console
$ proton mail messages send --help
Compose and send a message
…
Global flags:    proton --help
Full reference:  https://proton-cli.lerchster.dev/commands/mail/#messages-send
```

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

## Look before you leap

Every command that changes something takes `--dry-run`, which resolves references, applies filters, and shows you the rows it would touch:

```bash
proton mail messages trash --from newsletter@example.com --older-than 90d --dry-run
```

Anything that removes permanently, or removes what a filter picked out rather than what you named, shows those rows and asks first.

## Sign out

```bash
proton account logout             # forget the session on this machine
proton account logout --revoke    # and invalidate it at Proton
```

Revoking also makes the credentials saved on this machine useless, even to someone who already copied them.

## Where next

| Page | What's in it |
| --- | --- |
| [How commands read](language.md) | The grammar, the verbs, the filters, dry runs, naming a thing |
| [Mail](apps/mail.md) · [Drive](apps/drive.md) · [Calendar](apps/calendar.md) · [Pass](apps/pass.md) · [Contacts](apps/contacts.md) | One page per app, task by task |
| [Accounts and sessions](apps/account.md) | Two accounts side by side, unattended sign-in, revoking a device |
| [Scripting](scripting.md) | Pipelines, `jq`, cron and systemd |
| [Command reference](commands/README.md) | Every command, argument and flag |
| [FAQ](faq.md) | Is this official? How does it differ from Bridge? Is my password safe? |
