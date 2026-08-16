# Installation

proton is a single self-contained binary. Pick whichever line matches your system.

The command is `proton`. Every install also puts `proton-cli` beside it as a second name, so a line written either way runs.

## Linux

**Any distribution** (installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh | sh
```

The script takes `--version X.Y.Z` and `--install-dir DIR` (or the `PROTON_CLI_VERSION` and `PROTON_CLI_INSTALL_DIR` environment variables):

```bash
curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh | sh -s -- --install-dir /usr/local/bin
```

**Arch Linux** (AUR):

```bash
yay -S proton-cli-bin      # or: paru -S proton-cli-bin
```

**Debian, Ubuntu, Linux Mint** (APT repository, gets updates with the rest of your system):

```bash
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://roman-16.github.io/proton-cli/gpg.key | sudo tee /etc/apt/keyrings/proton-cli.asc >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/proton-cli.asc] https://roman-16.github.io/proton-cli stable main" | sudo tee /etc/apt/sources.list.d/proton-cli.list
sudo apt update && sudo apt install proton-cli
```

**Fedora, RHEL, Alpine** - download the package from the [latest release](https://github.com/roman-16/proton-cli/releases/latest) and install it:

```bash
sudo dnf install ./proton-cli_*.rpm                  # Fedora, RHEL
sudo apk add --allow-untrusted ./proton-cli_*.apk    # Alpine
```

**Nix** - the [`proton-cli`](https://search.nixos.org/packages?query=proton-cli) package is in nixpkgs:

```nix
environment.systemPackages = [ pkgs.proton-cli ];
```

To track the latest release instead of your nixpkgs channel, use the flake:

```nix
inputs = {
  proton = {
    url = "github:roman-16/proton-cli";
    inputs.nixpkgs.follows = "nixpkgs";
  };
};

# in a NixOS module
environment.systemPackages = [
  proton.packages.${pkgs.stdenv.hostPlatform.system}.default
];
```

## macOS

```bash
brew install --cask roman-16/tap/proton-cli
```

Or the install script, which puts the binary in `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.sh | sh
```

## Windows

```powershell
winget install Roman-16.ProtonCLI
```

Or the PowerShell installer, which installs to `%LOCALAPPDATA%\Programs\proton-cli`:

```powershell
irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1 | iex
```

It accepts `-Version` and `-InstallDir`:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1))) -InstallDir "C:\tools\proton-cli"
```

## Cross-platform

**npm** - handy if you already manage tooling with Node:

```bash
npm install -g @roman-16/proton-cli
```

**Go** - builds from source:

```bash
go install github.com/roman-16/proton-cli/cmd/proton@latest
```

> [!NOTE]
> `go install` builds don't embed the CAPTCHA helper that release binaries carry. If Proton asks for human verification at login, use a release binary instead. See [Human verification](human-verification.md).

## Manual download

Every release ships raw binaries and archives on the [releases page](https://github.com/roman-16/proton-cli/releases/latest).

| Platform | Binary |
| --- | --- |
| Linux x86_64 | `proton-cli_linux_amd64` |
| Linux ARM64 | `proton-cli_linux_arm64` |
| macOS Apple Silicon | `proton-cli_darwin_arm64` |
| macOS Intel | `proton-cli_darwin_amd64` |
| Windows x86_64 | `proton-cli_windows_amd64.exe` |

```bash
curl -LO https://github.com/roman-16/proton-cli/releases/latest/download/proton-cli_linux_amd64
chmod +x proton-cli_linux_amd64
sudo mv proton-cli_linux_amd64 /usr/local/bin/proton
sudo ln -s proton /usr/local/bin/proton-cli    # the second name, if you want it
```

On Windows, download the `.exe`, rename it to `proton.exe`, and put its folder on your `PATH`.

The `proton-cli_<version>_<os>_<arch>.tar.gz` / `.zip` archives bundle the binary, the licence, and shell completions for bash, zsh, and fish. The `.zip` also carries `proton-cli.exe`, a small launcher for `proton.exe`: an archive cannot hold a link, so on Windows the second name travels as a file of its own.

### Verifying a download

Each release includes `checksums.txt`:

```bash
curl -LO https://github.com/roman-16/proton-cli/releases/latest/download/checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

## Shell completions

Package installs (APT, AUR, Homebrew, RPM, APK) wire completions up for you. For a manual install, generate them yourself:

```bash
proton completion bash > /etc/bash_completion.d/proton
proton completion zsh  > "${fpath[1]}/_proton"
proton completion fish > ~/.config/fish/completions/proton.fish
```

One script covers both names. Fish is the exception: it looks for a file named after the command being typed, so the second name needs one of its own.

```bash
echo 'complete -c proton-cli -w proton' > ~/.config/fish/completions/proton-cli.fish
```

## Updating

If you used a package manager, update with it (`apt upgrade`, `brew upgrade`, `winget upgrade`, `yay -Syu`, …).

If you used the install script or a manual download, proton updates itself:

```bash
proton update             # install the latest release
proton update --check     # only report whether an update exists
proton update 1.9.13      # install a specific version
proton update --reinstall # install again even if already current
```

## Uninstalling

Package installs go out the way they came in (`apt remove proton-cli`, `brew uninstall --cask proton-cli`, …).

Script and manual installs can remove themselves:

```bash
proton uninstall --dry-run       # show what would be removed
proton uninstall                 # ask, then remove the binary
proton uninstall --yes           # remove it without asking
proton uninstall --yes --purge   # also delete saved sessions and the ID cache
```

Uninstalling cannot be undone from here, so it asks first, like every other removal ([why](language.md#when-it-asks-first)). `--yes` is the answer given in advance.

## Building from source

Needs Go 1.26 or newer:

```bash
git clone https://github.com/roman-16/proton-cli.git
cd proton
go build .
```

That plain build has no CAPTCHA helper. For a release-shaped binary, see [Contributing](../CONTRIBUTING.md).
