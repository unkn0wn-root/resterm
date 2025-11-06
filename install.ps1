$ErrorActionPreference = "Stop"
$REPO = "unkn0wn-root/resterm"
$BINARY_NAME = "resterm.exe"

function Write-Info {
    param($Message)
    Write-Host "[INFO] " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param($Message)
    Write-Host "[WARN] " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-Err {
    param($Message)
    Write-Host "[ERROR] " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64" }
        default { Write-Err "Unsupported architecture: $arch" }
    }
}

function Get-LatestRelease {
    try {
        Write-Info "Fetching latest release..."
        $response = Invoke-RestMethod -Uri "https://api.github.com/repos/$REPO/releases/latest"
        return $response.tag_name
    }
    catch {
        Write-Err "Failed to fetch latest release: $_"
    }
}

function Get-InstallDir {
    $possibleDirs = @(
        "$env:USERPROFILE\bin",
        "$env:LOCALAPPDATA\Programs\resterm"
    )

    foreach ($dir in $possibleDirs) {
        if (Test-Path $dir) {
            return $dir
        }
    }

    return "$env:USERPROFILE\bin"
}

function Test-InPath {
    param($Directory)
    $pathDirs = $env:PATH -split ';'
    return $pathDirs -contains $Directory
}

function Add-ToPath {
    param($Directory)

    Write-Info "Adding $Directory to PATH..."

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$Directory*") {
        $newPath = "$userPath;$Directory"
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        $env:PATH = "$env:PATH;$Directory"
        Write-Info "Added to PATH. You may need to restart your terminal."
        return $true
    }
    return $false
}

function Main {
    Write-Info "Starting Resterm installation..."

    $arch = Get-Architecture
    Write-Info "Detected Architecture: $arch"

    $version = Get-LatestRelease
    if (-not $version) {
        Write-Err "Failed to fetch latest release version"
    }
    Write-Info "Latest version: $version"

    $binaryFilename = "resterm_Windows_${arch}.exe"
    $downloadUrl = "https://github.com/$REPO/releases/download/$version/$binaryFilename"

    $installDir = Get-InstallDir
    Write-Info "Install directory: $installDir"

    if (-not (Test-Path $installDir)) {
        Write-Info "Creating directory $installDir..."
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }

    $installPath = Join-Path $installDir $BINARY_NAME

    Write-Info "Downloading from: $downloadUrl"
    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $installPath
    }
    catch {
        Write-Err "Download failed: $_"
    }

    if (-not (Test-Path $installPath)) {
        Write-Err "Installation failed: Binary not found at $installPath"
    }

    $inPath = Test-InPath $installDir
    if (-not $inPath) {
        Write-Warn "Install directory is not in PATH"
        $addPath = Read-Host "Add $installDir to PATH? (Y/n)"
        if ($addPath -ne 'n' -and $addPath -ne 'N') {
            Add-ToPath $installDir
        }
        else {
            Write-Warn "You will need to manually add $installDir to PATH"
        }
    }

    Write-Info "Successfully installed Resterm!"
    Write-Info "Location: $installPath"
    Write-Host ""
    Write-Info "Run 'resterm --help' to get started"

    if (-not $inPath) {
        Write-Host ""
        Write-Warn "Note: You may need to restart your terminal for PATH changes to take effect"
    }
}

Main
