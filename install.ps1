# gitdash Windows one-line installer
#
# Server CLI:
#   & $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1)))
# Command-line client (gitdash-cli):
#   & $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1))) cli
# Self-hosted CI runner:
#   & $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1))) runner
# Copilot agent runtime:
#   & $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1))) agent
#
# Installing gitdash also installs the bundled agent runtime (if present), so copilot works out of the box.
#
# Integrity: always downloads checksums.txt from the same release and verifies SHA256.
#
# Environment variables:
#   GITDASH_VERSION      pin a version (e.g. v0.1.0), defaults to latest release
#   GITDASH_INSTALL_DIR  install directory, defaults to %LOCALAPPDATA%\Programs\gitdash

$ErrorActionPreference = "Stop"

$Repo = "holihur/gitdash"
if ($args.Count -gt 0 -and $args[0] -eq "runner") { $BinName = "gitdash-runner.exe" }
elseif ($args.Count -gt 0 -and $args[0] -eq "agent") { $BinName = "agent.exe" }
elseif ($args.Count -gt 0 -and $args[0] -eq "cli") { $BinName = "gitdash-cli.exe" }
else { $BinName = "gitdash.exe" }
$Version = $env:GITDASH_VERSION
$InstallDir = if ($env:GITDASH_INSTALL_DIR) { $env:GITDASH_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\gitdash" }

function Write-Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

# Resolve architecture
switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# Resolve latest version
if (-not $Version) {
    Write-Step "Querying latest version..."
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
        -Headers @{ "User-Agent" = "gitdash-install" }
    $Version = $rel.tag_name
    if (-not $Version) { throw "Failed to query latest version; set GITDASH_VERSION and retry" }
}
$Ver = $Version.TrimStart("v")
$ArchiveName = "gitdash_${Ver}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$ArchiveName"
$SumsUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("gitdash-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    Write-Step "Downloading $Url"
    $Zip = Join-Path $Tmp "gitdash.zip"
    Invoke-WebRequest -Uri $Url -OutFile $Zip -UserAgent "gitdash-install"

    Write-Step "Verifying SHA256..."
    $SumsFile = Join-Path $Tmp "checksums.txt"
    Invoke-WebRequest -Uri $SumsUrl -OutFile $SumsFile -UserAgent "gitdash-install"
    $line = Get-Content $SumsFile | Where-Object { $_ -match "\s+$([regex]::Escape($ArchiveName))$" } | Select-Object -First 1
    if (-not $line) { throw "checksums.txt does not contain $ArchiveName" }
    $want = ($line -split '\s+')[0]
    $got = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash
    if ($got.ToLower() -ne $want.ToLower()) { throw "SHA256 mismatch; download may be tampered with" }
    Write-Step "SHA256 OK"

    Write-Step "Extracting"
    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    $Exe = Join-Path $Tmp $BinName
    if (-not (Test-Path $Exe)) { throw "Archive does not contain $BinName" }

    Write-Step "Installing to $InstallDir"
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $Exe -Destination (Join-Path $InstallDir $BinName) -Force
    # gitdash 同时装上同压缩包内的 copilot agent（存在则装）
    if ($BinName -eq "gitdash.exe") {
        $AgentExe = Join-Path $Tmp "agent.exe"
        if (Test-Path $AgentExe) {
            Copy-Item -Path $AgentExe -Destination (Join-Path $InstallDir "agent.exe") -Force
            Write-Step "Installed agent.exe (copilot runtime; override path with GITDASH_COPILOT_AGENT_BIN)"
        }
    }

    # Add to user PATH if not present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Step "Added $InstallDir to user PATH (restart your terminal to take effect)"
    }

    Write-Host ""
    Write-Host "Installed $BinName $Version -> $InstallDir\$BinName" -ForegroundColor Green
    Write-Host ""
    if ($BinName -eq "gitdash-cli.exe") {
        Write-Host "Quick start:"
        Write-Host "  gitdash-cli login                          # browser OAuth device flow (or --method pat)"
        Write-Host "  gitdash-cli --host http://localhost:8080 login"
        Write-Host "  gitdash-cli me                             # show the authenticated user"
        Write-Host "  gitdash-cli repo list                      # list your repositories"
    } else {
        Write-Host "Quick start:"
        Write-Host "  gitdash serve                 # http://localhost:8080 / ssh :2222"
        Write-Host "  GITDASH_TOKEN=secret gitdash serve"
        Write-Host ""
        Write-Host "Run as a Windows service: see packaging/gitdash.windows.md in the repository"
    }
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
