# wslwatch build script
# Usage: .\build.ps1

param(
    [string]$Version = "dev",
    [switch]$Sign
)

$ErrorActionPreference = "Stop"

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

$outDir = "bin"
$outFile = "$outDir\wslwatch.exe"

Write-Host "Building wslwatch $Version..." -ForegroundColor Cyan

if (-not (Test-Path $outDir)) {
    New-Item -ItemType Directory -Path $outDir | Out-Null
}

go build -ldflags "-s -w -X main.version=$Version" -o $outFile ./cmd/wslwatch/

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

Write-Host "Built $outFile" -ForegroundColor Green

if ($Sign) {
    Write-Host "Signing $outFile..." -ForegroundColor Cyan
    # Add your signing command here, e.g.:
    # signtool sign /sha1 <thumbprint> /t http://timestamp.digicert.com $outFile
    Write-Host "Signing not configured. Skipping." -ForegroundColor Yellow
}

Write-Host "Done!" -ForegroundColor Green
