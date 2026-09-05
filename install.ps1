# gitdash Windows one-line installer
#
#   irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1 | iex
#
# Environment variables:
#   GITDASH_VERSION      pin a version (e.g. v0.1.0), defaults to latest release
#   GITDASH_INSTALL_DIR  install directory, defaults to %LOCALAPPDATA%\Programs\gitdash

$ErrorActionPreference = "Stop"

$Repo = "holihur/gitdash"
$BinName = "gitdash.exe"
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
$Url = "https://github.com/$Repo/releases/download/$Version/gitdash_${Ver}_windows_${Arch}.zip"

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("gitdash-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    Write-Step "Downloading $Url"
    $Zip = Join-Path $Tmp "gitdash.zip"
    Invoke-WebRequest -Uri $Url -OutFile $Zip -UserAgent "gitdash-install"

    Write-Step "Extracting"
    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    $Exe = Join-Path $Tmp $BinName
    if (-not (Test-Path $Exe)) { throw "Archive does not contain $BinName" }

    Write-Step "Installing to $InstallDir"
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $Exe -Destination (Join-Path $InstallDir $BinName) -Force

    # Add to user PATH if not present
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Step "Added $InstallDir to user PATH (restart your terminal to take effect)"
    }

    Write-Host ""
    Write-Host "Installed $BinName $Version -> $InstallDir\$BinName" -ForegroundColor Green
    Write-Host ""
    Write-Host "Quick start:"
    Write-Host "  gitdash serve                 # http://localhost:8080 / ssh :2222"
    Write-Host "  GITDASH_TOKEN=secret gitdash serve"
    Write-Host ""
    Write-Host "Run as a Windows service: see packaging/gitdash.windows.md in the repository"
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
