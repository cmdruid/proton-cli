# Install on Linux, macOS and Windows

proton-cli is one self-contained binary, on Homebrew, winget, APT, AUR, Nix, npm, Go and as a signed download. Pick the line that matches your system.

The command is `proton`. Every install also puts `proton-cli` beside it as a second name, so a line written either way runs.

| Platform | Install |
| --- | --- |
| **Linux, macOS** | `curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh \| sh` |
| **Windows** | `irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1 \| iex` |
| **Homebrew** | `brew install --cask roman-16/tap/proton-cli` |
| **winget** | `winget install Roman-16.ProtonCLI` |
| **Arch** | `yay -S proton-cli-bin` |
| **Fedora, RHEL** | `sudo dnf install ./proton-cli_*.rpm` |
| **Alpine** | `sudo apk add --allow-untrusted ./proton-cli_*.apk` |
| **Nix** | `environment.systemPackages = [ pkgs.proton-cli ];` |
| **npm** | `npm install -g @roman-16/proton-cli` |
| **Go** | `go install github.com/roman-16/proton-cli/cmd/proton@latest` |

The install scripts drop the binary in `~/.local/bin` (Linux, macOS) or `%LOCALAPPDATA%\Programs\proton-cli` (Windows). Both take a version and a directory:

```bash
curl -fsSL …/install.sh | sh -s -- --install-dir /usr/local/bin --version 2.5.0
```

```powershell
& ([scriptblock]::Create((irm …/install.ps1))) -InstallDir "C:\tools\proton-cli"
```

> [!NOTE]
> `go install` builds don't embed the CAPTCHA helper that release binaries carry. If Proton asks for human verification at login, use a release binary instead. See [Troubleshooting](troubleshooting.md#proton-asks-for-a-captcha).

## Debian, Ubuntu, Linux Mint

The APT repository updates proton with the rest of your system:

```bash
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://roman-16.github.io/proton-cli/gpg.key | sudo tee /etc/apt/keyrings/proton-cli.asc >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/proton-cli.asc] https://roman-16.github.io/proton-cli stable main" | sudo tee /etc/apt/sources.list.d/proton-cli.list
sudo apt update && sudo apt install proton-cli
```

## Nix flake

To track the latest release rather than your nixpkgs channel:

```nix
inputs.proton = {
  url = "github:roman-16/proton-cli";
  inputs.nixpkgs.follows = "nixpkgs";
};

environment.systemPackages = [
  proton.packages.${pkgs.stdenv.hostPlatform.system}.default
];
```

## Downloading a binary

Every release ships raw binaries, archives and `checksums.txt` on the [releases page](https://github.com/roman-16/proton-cli/releases/latest), named `proton-cli_<os>_<arch>` for `linux`, `darwin` and `windows` on `amd64` and `arm64`.

```bash
curl -LO https://github.com/roman-16/proton-cli/releases/latest/download/proton-cli_linux_amd64
curl -LO https://github.com/roman-16/proton-cli/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
chmod +x proton-cli_linux_amd64
sudo mv proton-cli_linux_amd64 /usr/local/bin/proton
```

On Windows, download the `.exe`, rename it to `proton.exe`, and put its folder on your `PATH`.

The `.tar.gz` / `.zip` archives also bundle the licence and shell completions. The `.zip` carries `proton-cli.exe` as a real file, since an archive cannot hold a link.

## Shell completions

Package installs wire these up for you. For a manual install:

```bash
proton completion bash > /etc/bash_completion.d/proton
proton completion zsh  > "${fpath[1]}/_proton"
proton completion fish > ~/.config/fish/completions/proton.fish
```

One script covers both names, except in fish, which looks for a file named after the command being typed:

```bash
echo 'complete -c proton-cli -w proton' > ~/.config/fish/completions/proton-cli.fish
```

## Updating

Installed with a package manager? Update with it. Otherwise proton updates itself:

```bash
proton update             # install the latest release
proton update --check     # only report whether an update exists
proton update 2.5.0       # install a specific version
```

An install that no package manager owns has nothing to tell it a release happened, so proton says so itself, **once a day**, after the command has finished:

```console
$ proton mail messages list
…

proton 2.4.1 → 2.5.0 available.
Run `proton update` to install it, or `proton changelog 2.5.0` for what changed.
```

It stays quiet when a package manager owns this copy, when stderr is not a terminal, and under `--quiet`. `PROTON_NO_UPDATE_CHECK=1` ends it for good.

`proton changelog` prints what each release changed - the whole file, one version, or `--since 2.3.0 --until 2.4.0` for a range.

## Uninstalling

Package installs go out the way they came in. Script and manual installs remove themselves:

```bash
proton uninstall --dry-run       # show what would be removed
proton uninstall --yes           # remove the binary without asking
proton uninstall --yes --purge   # also delete saved sessions and the ID cache
```

Uninstalling cannot be undone from here, so it asks first, like every other permanent removal.

## Building from source

Needs Go 1.26 or newer:

```bash
git clone https://github.com/roman-16/proton-cli.git
cd proton-cli
go build ./cmd/proton
```

That plain build has no CAPTCHA helper. For a release-shaped binary, see [Contributing](../CONTRIBUTING.md).
