# Release Runbook — wslwatch (Windows binary)

## How It Works

Every push to `main` triggers the [Release workflow](../../actions/workflows/release.yml), which:

1. Runs tests on Windows
2. Auto-generates a version tag (`v0.0.0-YYYYMMDDHHMMSS-sha`)
3. Builds `wslwatch.exe` with the version baked in via `-ldflags`
4. Tags the commit and creates a GitHub Release with auto-generated notes

**No manual tagging needed** — just merge to `main`.

## Promoting a Stable Release

To publish a named release (e.g. `v1.0.0`), tag manually:

```powershell
$Version = "1.0.0"
git tag -a "v$Version" -m "Release v$Version"
git push origin "v$Version"
```

Then create the release from the existing artifact:

```powershell
# Download the latest binary from the auto-release
gh release download --pattern "wslwatch.exe" --dir dist
gh release create "v$Version" dist/wslwatch.exe --title "v$Version" --generate-notes
```

## Manual Release (if CI is down)

### Build locally

```powershell
.\scripts\build.ps1 -Version $Version -OutputDir dist
```

### Smoke-test

```powershell
.\dist\wslwatch.exe --help
.\dist\wslwatch.exe --bark
```

### (Optional) Sign the binary

```powershell
.\scripts\build.ps1 -Version $Version -OutputDir dist -Sign -CertThumbprint <thumbprint>
```

### Create release manually

```powershell
gh release create "v$Version" .\dist\wslwatch.exe `
    --title "v$Version" `
    --generate-notes
```

## Hotfix Process

1. Branch from the release tag: `git checkout -b hotfix/v1.0.1 v1.0.0`
2. Fix, commit, push, open PR to `main`
3. After merge, repeat steps 3–9 with the patch version

## Rollback

To yank a bad release:

```powershell
gh release delete "v$Version" --yes
git push --delete origin "v$Version"
git tag -d "v$Version"
```
