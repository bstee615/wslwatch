# build.ps1 - Build and optionally sign wslwatch
#
# Usage:
#   .\scripts\build.ps1
#   .\scripts\build.ps1 -Sign -CertThumbprint <thumbprint>

param(
    [switch]$Sign,
    [string]$CertThumbprint = "",
    [string]$OutputDir = "dist",
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path $PSScriptRoot -Parent
$BinaryName  = "wslwatch.exe"
$OutputPath  = Join-Path $ProjectRoot $OutputDir $BinaryName

# Ensure output directory exists.
New-Item -ItemType Directory -Force -Path (Join-Path $ProjectRoot $OutputDir) | Out-Null

Write-Host "Building wslwatch $Version..."

$env:GOOS   = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$ldflags = "-s -w -X main.version=$Version"

Push-Location $ProjectRoot
try {
    go build -ldflags $ldflags -o $OutputPath ./cmd/wslwatch
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go build failed with exit code $LASTEXITCODE"
        exit $LASTEXITCODE
    }
} finally {
    Pop-Location
}

Write-Host "Built: $OutputPath"

# Optionally sign the binary.
if ($Sign) {
    if (-not $CertThumbprint) {
        Write-Error "-CertThumbprint is required when -Sign is specified"
        exit 1
    }

    Write-Host "Signing $OutputPath with cert $CertThumbprint..."
    $signtool = Get-Command signtool.exe -ErrorAction SilentlyContinue
    if (-not $signtool) {
        # Try well-known SDK paths.
        $candidates = @(
            "C:\Program Files (x86)\Windows Kits\10\bin\x64\signtool.exe",
            "C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\signtool.exe"
        )
        foreach ($c in $candidates) {
            if (Test-Path $c) { $signtool = $c; break }
        }
    }
    if (-not $signtool) {
        Write-Error "signtool.exe not found. Install Windows SDK."
        exit 1
    }

    & $signtool sign /sha1 $CertThumbprint /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 $OutputPath
    if ($LASTEXITCODE -ne 0) {
        Write-Error "signtool failed with exit code $LASTEXITCODE"
        exit $LASTEXITCODE
    }
    Write-Host "Signed successfully."
}

Write-Host "Done."
