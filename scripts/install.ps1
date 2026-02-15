param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:LOCALAPPDATA\Vandor\bin",
    [switch]$AddToPath
)

$ErrorActionPreference = "Stop"

$RepoOwner = "alfariiizi"
$RepoName = "vandor"
$BinaryName = "vandor.exe"

function Resolve-LatestTag {
    $latestUrl = "https://github.com/$RepoOwner/$RepoName/releases/latest"
    $response = Invoke-WebRequest -Uri $latestUrl -MaximumRedirection 0 -ErrorAction SilentlyContinue
    if ($response.StatusCode -ge 300 -and $response.StatusCode -lt 400) {
        $location = $response.Headers.Location
        if (-not $location) {
            throw "Failed to resolve latest release location."
        }
        return ($location -split "/")[-1]
    }
    throw "Failed to resolve latest release tag."
}

if ($Version -eq "latest") {
    $Version = Resolve-LatestTag
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -match "ARM64") { "arm64" } else { "amd64" }
if ($arch -eq "arm64") {
    throw "windows/arm64 release is not published yet. Use amd64 machine or WSL for now."
}
$versionNoV = $Version.TrimStart("v")
$archiveName = "vandor_${versionNoV}_windows_${arch}.zip"
$downloadUrl = "https://github.com/$RepoOwner/$RepoName/releases/download/$Version/$archiveName"

Write-Host "Installing vandor $Version (windows/$arch)"
Write-Host "Download: $downloadUrl"
Write-Host "Install dir: $InstallDir"

$tempRoot = Join-Path $env:TEMP ("vandor-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tempRoot | Out-Null

try {
    $archivePath = Join-Path $tempRoot $archiveName
    Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath

    Expand-Archive -Path $archivePath -DestinationPath $tempRoot -Force

    $binaryPath = Join-Path $tempRoot $BinaryName
    if (-not (Test-Path $binaryPath)) {
        throw "Binary $BinaryName not found in archive."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path $binaryPath -Destination (Join-Path $InstallDir $BinaryName) -Force
}
finally {
    Remove-Item -Path $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Installed: $(Join-Path $InstallDir $BinaryName)"

if ($AddToPath) {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($null -eq $currentPath) { $currentPath = "" }
    if ($currentPath -notlike "*$InstallDir*") {
        if ($currentPath -eq "") {
            [Environment]::SetEnvironmentVariable("Path", $InstallDir, "User")
        }
        else {
            [Environment]::SetEnvironmentVariable("Path", "$currentPath;$InstallDir", "User")
        }
        Write-Host "Added install dir to User PATH."
    }
    else {
        Write-Host "Install dir already exists in User PATH."
    }
}
else {
    Write-Host ""
    Write-Host "PATH not modified (safe mode)."
    Write-Host "Add manually via Windows Environment Variables:"
    Write-Host "  $InstallDir"
}

Write-Host ""
Write-Host "Optional official registry env (PowerShell):"
Write-Host '  setx VANDOR_VPKG_REGISTRY_OFFICIAL "https://vpkg.vercel.app"'
Write-Host ""
Write-Host "Verify in a new terminal:"
Write-Host "  vandor --version"
