#Requires -Version 5.1
<#
.SYNOPSIS
    Sets up a Windows build environment for Orb (Tauri desktop binary + Go backend).

.DESCRIPTION
    Installs and configures:
      - Git
      - Go  (for the services/ backend)
      - Rust (stable MSVC toolchain)
      - Visual Studio Build Tools 2022 (C++ workload + Windows SDK)
      - Bun  (frontend package manager)
      - WebView2 Runtime (if not already present)
    Then installs frontend npm dependencies and verifies the toolchain.

.NOTES
    Run from an elevated (Administrator) PowerShell prompt.
    Re-running is safe; already-installed components are skipped.
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Helpers ───────────────────────────────────────────────────────────────────

function Write-Step  { param($msg) Write-Host "`n==> $msg" -ForegroundColor Cyan }
function Write-Ok    { param($msg) Write-Host "    [OK] $msg" -ForegroundColor Green }
function Write-Skip  { param($msg) Write-Host "    [--] $msg (already installed)" -ForegroundColor DarkGray }
function Write-Warn  { param($msg) Write-Host "    [!!] $msg" -ForegroundColor Yellow }
function Write-Fail  { param($msg) Write-Host "    [XX] $msg" -ForegroundColor Red }

function Test-Command { param($cmd) return [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }

function Refresh-Path {
    $env:Path = [System.Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [System.Environment]::GetEnvironmentVariable('Path', 'User')
}

function Install-WingetPackage {
    param(
        [string]$Id,
        [string]$Name,
        [string[]]$ExtraArgs = @()
    )
    Write-Step "Installing $Name"
    $result = winget install --id $Id --exact --accept-source-agreements --accept-package-agreements `
              --silent @ExtraArgs 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Ok "$Name installed"
    } elseif ($result -match 'already installed') {
        Write-Skip $Name
    } else {
        Write-Warn "$Name install returned exit code $LASTEXITCODE — may already be present or need a reboot."
    }
    Refresh-Path
}

# ── Require elevation ─────────────────────────────────────────────────────────

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
           ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Fail "This script must be run as Administrator."
    Write-Host "  Right-click PowerShell and choose 'Run as administrator', then re-run."
    exit 1
}

# ── Require winget ────────────────────────────────────────────────────────────

Write-Step "Checking winget"
if (-not (Test-Command 'winget')) {
    Write-Fail "winget not found. Install 'App Installer' from the Microsoft Store, then re-run."
    exit 1
}
Write-Ok "winget $(winget --version)"

# ── Git ───────────────────────────────────────────────────────────────────────

Write-Step "Checking Git"
if (Test-Command 'git') {
    Write-Skip "git $(git --version)"
} else {
    Install-WingetPackage -Id 'Git.Git' -Name 'Git'
}

# ── Go ────────────────────────────────────────────────────────────────────────

Write-Step "Checking Go"
if (Test-Command 'go') {
    Write-Skip "go $(go version)"
} else {
    Install-WingetPackage -Id 'GoLang.Go' -Name 'Go'
    Refresh-Path
}

# ── Visual Studio Build Tools 2022 ───────────────────────────────────────────
# Rust's MSVC toolchain requires the C++ build tools and a Windows SDK.

Write-Step "Checking Visual Studio Build Tools"
$vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
$hasVS = $false
if (Test-Path $vswhere) {
    $vsInfo = & $vswhere -products * -requires Microsoft.VisualCpp.Tools.HostX64.TargetX64 -format json 2>$null | ConvertFrom-Json
    $hasVS = ($null -ne $vsInfo -and @($vsInfo).Count -gt 0)
}

if ($hasVS) {
    Write-Skip "Visual Studio Build Tools (C++ components found)"
} else {
    Write-Step "Installing Visual Studio Build Tools 2022"
    Write-Host "    This downloads ~3-6 GB and may take several minutes..." -ForegroundColor DarkYellow

    $vsArgs = @(
        '--add', 'Microsoft.VisualStudio.Workload.VCTools',
        '--add', 'Microsoft.VisualStudio.Component.Windows11SDK.22621',
        '--includeRecommended',
        '--quiet', '--wait', '--norestart'
    )
    Install-WingetPackage -Id 'Microsoft.VisualStudio.2022.BuildTools' `
                          -Name 'VS Build Tools 2022' `
                          -ExtraArgs $vsArgs
}

# ── Rust ──────────────────────────────────────────────────────────────────────

Write-Step "Checking Rust"
if (Test-Command 'rustup') {
    Write-Skip "rustup $(rustup --version 2>&1 | Select-String 'rustup' | Select-Object -First 1)"
    $ErrorActionPreference = 'Continue'
    rustup update stable
    $ErrorActionPreference = 'Stop'
    Write-Ok "Rust toolchain updated"
} else {
    Write-Step "Installing Rust via rustup-init"
    $rustupInit = "$env:TEMP\rustup-init.exe"
    Invoke-WebRequest -Uri 'https://win.rustup.rs/x86_64' -OutFile $rustupInit -UseBasicParsing
    & $rustupInit -y --default-toolchain stable --default-host x86_64-pc-windows-msvc
    Remove-Item $rustupInit -Force
    Refresh-Path
    Write-Ok "Rust installed"
}

# Ensure the MSVC toolchain is the default (idempotent)
Write-Step "Configuring Rust toolchain"
$ErrorActionPreference = 'Continue'
rustup default stable-x86_64-pc-windows-msvc
$ErrorActionPreference = 'Stop'
Write-Ok "Default toolchain: stable-x86_64-pc-windows-msvc"

# ── Bun ───────────────────────────────────────────────────────────────────────

Write-Step "Checking Bun"
if (Test-Command 'bun') {
    Write-Skip "bun $(bun --version)"
} else {
    Write-Step "Installing Bun"
    # Official Bun Windows installer
    Invoke-RestMethod 'https://bun.sh/install.ps1' | Invoke-Expression
    Refresh-Path
    if (Test-Command 'bun') {
        Write-Ok "Bun $(bun --version) installed"
    } else {
        Write-Warn "Bun installer ran but 'bun' not found in PATH yet. You may need to restart your terminal."
    }
}

# ── WebView2 Runtime ──────────────────────────────────────────────────────────
# Required by Tauri. Pre-installed on Windows 11 and most Windows 10 machines.

Write-Step "Checking WebView2 Runtime"
$wv2Key = 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
$wv2User = 'HKCU:\Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
$hasWV2 = (Test-Path $wv2Key) -or (Test-Path $wv2User)

if ($hasWV2) {
    Write-Skip "WebView2 Runtime"
} else {
    Write-Step "Installing WebView2 Runtime"
    $wv2Installer = "$env:TEMP\MicrosoftEdgeWebview2Setup.exe"
    Invoke-WebRequest `
        -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' `
        -OutFile $wv2Installer -UseBasicParsing
    Start-Process -FilePath $wv2Installer -ArgumentList '/silent /install' -Wait
    Remove-Item $wv2Installer -Force
    Write-Ok "WebView2 Runtime installed"
}

# ── Frontend dependencies ─────────────────────────────────────────────────────

Write-Step "Installing frontend dependencies (bun install)"
$scriptDir  = Split-Path -Parent $MyInvocation.MyCommand.Definition
$repoRoot   = Split-Path -Parent $scriptDir
$webDir     = Join-Path $repoRoot 'web'

if (-not (Test-Path $webDir)) {
    Write-Warn "web/ directory not found at $webDir — skipping bun install"
} elseif (Test-Command 'bun') {
    Push-Location $webDir
    bun install --force
    Pop-Location
    Write-Ok "Frontend dependencies installed"
} else {
    Write-Warn "bun not in PATH — skipping bun install (restart terminal and run 'bun install' in web/)"
}

# ── Verification ──────────────────────────────────────────────────────────────

Write-Step "Toolchain verification"

$checks = @(
    @{ Cmd = 'git';   Label = 'git';   Args = @('--version') },
    @{ Cmd = 'go';    Label = 'go';    Args = @('version')   },
    @{ Cmd = 'rustc'; Label = 'rustc'; Args = @('--version') },
    @{ Cmd = 'cargo'; Label = 'cargo'; Args = @('--version') },
    @{ Cmd = 'bun';   Label = 'bun';   Args = @('--version') }
)

$allOk = $true
$ErrorActionPreference = 'Continue'
foreach ($c in $checks) {
    if (Test-Command $c.Cmd) {
        $ver = & $c.Cmd @($c.Args) 2>&1 | Select-Object -First 1
        Write-Ok "$($c.Label): $ver"
    } else {
        Write-Fail "$($c.Label) not found in PATH"
        $allOk = $false
    }
}
$ErrorActionPreference = 'Stop'

# Check MSVC linker
$clPath = & "$vswhere" -latest -products * -requires Microsoft.VisualCpp.Tools.HostX64.TargetX64 `
           -find 'VC\Tools\MSVC\**\bin\HostX64\x64\cl.exe' 2>$null |
           Select-Object -Last 1
if ($clPath) {
    Write-Ok "MSVC cl.exe: $clPath"
} else {
    Write-Warn "MSVC cl.exe not found via vswhere — Rust may not link correctly"
    $allOk = $false
}

# ── Summary ───────────────────────────────────────────────────────────────────

Write-Host ""
if ($allOk) {
    Write-Host "All tools ready. Build commands:" -ForegroundColor Green
} else {
    Write-Host "Setup complete with warnings. Fix any [XX] items above, then:" -ForegroundColor Yellow
}

Write-Host @"

  # Tauri desktop binary (produces web/src-tauri/target/release/orb.exe):
  cd web
  bunx tauri build

  # Go backend (produces services binary):
  cd services
  go build ./cmd/...

"@ -ForegroundColor White

Write-Host "NOTE: If any PATH changes were made, close and reopen your terminal before building." -ForegroundColor DarkYellow
