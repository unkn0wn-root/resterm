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

function Download-File {
    param($Url, $OutputPath, $Label)

    Write-Info "Downloading from: $Url"
    try {
        Invoke-WebRequest -Uri $Url -OutFile $OutputPath
    }
    catch {
        Write-Err "$Label download failed: $_"
    }
}

function Get-ExpectedChecksum {
    param($ChecksumPath, $BinaryFilename)

    $line = Get-Content -Path $ChecksumPath -TotalCount 1
    if (-not $line) {
        Write-Err "Checksum file is empty"
    }

    $fields = $line.Trim() -split '\s+'
    if ($fields.Count -ne 1 -and $fields.Count -ne 2) {
        Write-Err "Invalid checksum line"
    }

    $expected = $fields[0].ToLowerInvariant()
    if ($expected -notmatch '^[0-9a-f]{64}$') {
        Write-Err "Invalid SHA-256 digest: $($fields[0])"
    }

    if ($fields.Count -eq 2) {
        $name = $fields[1].TrimStart([char[]]"*")
        if ($name -ne $BinaryFilename) {
            Write-Err "Checksum names $name, want $BinaryFilename"
        }
    }

    return $expected
}

function Verify-Checksum {
    param($FilePath, $ChecksumPath, $BinaryFilename)

    $expected = Get-ExpectedChecksum $ChecksumPath $BinaryFilename
    $actual = (Get-FileHash -Algorithm SHA256 -Path $FilePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) {
        Write-Err "Checksum mismatch for $FilePath`: expected $expected, got $actual"
    }
    Write-Info "Checksum verified"
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

    try {
        $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) "resterm-install-$([guid]::NewGuid().ToString('N'))"
        New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

        $tempBinary = Join-Path $tempDir $BINARY_NAME
        $checksumPath = "$tempBinary.sha256"

        Download-File $downloadUrl $tempBinary "Binary"
        Download-File "${downloadUrl}.sha256" $checksumPath "Checksum"
        Verify-Checksum $tempBinary $checksumPath $binaryFilename

        Move-Item -Path $tempBinary -Destination $installPath -Force
    }
    finally {
        if ($tempDir -and (Test-Path $tempDir)) {
            Remove-Item -Path $tempDir -Recurse -Force
        }
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
