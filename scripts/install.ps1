<#
.SYNOPSIS
  proton-cli installer for Windows.

.DESCRIPTION
  Downloads the latest proton-cli release from GitHub Releases, verifies its
  SHA-256 checksum, and installs it into a user directory. No package manager
  required. (winget remains the recommended Windows channel:
  `winget install Roman-16.ProtonCLI`.)

.EXAMPLE
  irm https://raw.githubusercontent.com/roman-16/proton-cli/main/scripts/install.ps1 | iex

.EXAMPLE
  # Pin a version or install directory (run the script directly, not piped):
  .\install.ps1 -Version 1.9.11 -InstallDir "C:\tools\proton-cli"
#>
[CmdletBinding()]
param(
    [string]$Version = $env:PROTON_CLI_VERSION,
    [string]$InstallDir = $(if ($env:PROTON_CLI_INSTALL_DIR) { $env:PROTON_CLI_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\proton-cli" })
)

$ErrorActionPreference = 'Stop'
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

$repo = 'roman-16/proton-cli'
$asset = 'proton-cli_windows_amd64.exe'   # runs natively on x64 and under emulation on ARM64

$base = if ($Version) {
    "https://github.com/$repo/releases/download/v$($Version.TrimStart('v'))"
} else {
    "https://github.com/$repo/releases/latest/download"
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("proton-cli-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
try {
    Write-Host "Downloading $asset$(if ($Version) { " (v$($Version.TrimStart('v')))" })..."
    Invoke-WebRequest -Uri "$base/$asset" -OutFile "$tmp\$asset" -UseBasicParsing
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

    $expected = (Select-String -Path "$tmp\checksums.txt" -Pattern "\s$([regex]::Escape($asset))$" |
        Select-Object -First 1).Line -split '\s+' | Select-Object -First 1
    if (-not $expected) { throw "no checksum entry for $asset in checksums.txt" }
    $actual = (Get-FileHash -Algorithm SHA256 -Path "$tmp\$asset").Hash.ToLower()
    if ($expected.ToLower() -ne $actual) { throw "checksum mismatch for $asset (expected $expected, got $actual)" }
    Write-Host "Checksum verified."

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $dest = Join-Path $InstallDir 'proton-cli.exe'
    Move-Item -Path "$tmp\$asset" -Destination $dest -Force

    $installed = (& $dest --version 2>$null)
    Write-Host "Installed $installed to $dest" -ForegroundColor Green
}
finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $InstallDir) {
    Write-Warning "$InstallDir is not on your PATH. To add it permanently, run:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"`$([Environment]::GetEnvironmentVariable('Path','User'));$InstallDir`", 'User')"
    # Make proton-cli usable in the current session without persisting anything.
    $env:Path = "$env:Path;$InstallDir"
}

Write-Host "Enable shell completions with: proton-cli completion powershell"
Write-Host "Uninstall any time with: proton-cli uninstall"
