# wslwatch

A single-binary Windows Service that monitors WSL2 distros and automatically restarts them when they die.

## Features

- **Automatic restart** — detects stopped or unresponsive distros and restarts them
- **Windows Service** — runs as a background service with auto-start on login
- **YAML config** — per-distro settings, failure thresholds, backoff, and more
- **IPC status** — live status, pause/resume, and config reload via named pipe
- **Single binary** — no runtime dependencies, self-installs and self-uninstalls

## Quick Start

```powershell
# Generate config from installed distros
wslwatch --autoconfig

# Install as Windows Service (requires admin)
wslwatch --install

# Check status
wslwatch --status
```

## Commands

| Command | Description |
|---|---|
| `wslwatch` | Run watchdog in foreground (Ctrl+C to stop) |
| `wslwatch --install [--add-to-path]` | Install as Windows Service |
| `wslwatch --uninstall` | Remove the Windows Service |
| `wslwatch --autoconfig` | Generate config from installed distros |
| `wslwatch --config <key> <value>` | Set a config value |
| `wslwatch --status` | Show live status |
| `wslwatch --pause <distro>` | Pause monitoring a distro |
| `wslwatch --resume <distro>` | Resume monitoring a distro |
| `wslwatch --bark` | Important |

### Global Flags

```
--config-file <path>    Override config file path (default: %ProgramData%\wslwatch\wslwatch.yaml)
```

## Configuration

Default path: `%ProgramData%\wslwatch\wslwatch.yaml`

```yaml
log_level: info              # debug | info | warn | error
log_file: ""                 # empty = stderr; path = file

check_interval: 15s          # how often to poll distro health
probe_timeout: 8s            # timeout for liveness probe per distro
restart_delay: 3s            # delay between terminate and restart

failure_window: 60s          # sliding window for failure rate tracking
failure_threshold: 5         # failures within window before backoff kicks in
backoff_duration: 0s         # how long to stop retrying (0 = retry forever)

distros:
  - name: Ubuntu-22.04
    enabled: true
    max_restarts: 0          # 0 = unlimited
    pause: false
    start_command: ""        # optional: shell command to run after distro starts

ignored_distros:
  - docker-desktop
  - docker-desktop-data
```

### Config via CLI

```powershell
wslwatch --config check_interval 30s
wslwatch --config distros.Ubuntu-22.04.max_restarts 5
wslwatch --config failure_threshold 3
```

## Status Output

```
wslwatch  ● running  uptime 3d 14h 22m
──────────────────────────────────────────────────────
  Ubuntu-22.04         ● healthy   uptime 2d 4h 11m   restarts 1
  Ubuntu-20.04         ⏸ paused
  docker-desktop       ─ ignored

Failure history (last 60m) — Ubuntu-22.04
  ██░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░

Legend: █ failure  ░ healthy  ─ no data
```

## Building

```powershell
# Windows (from repo root)
.\scripts\build.ps1

# With code signing
.\scripts\build.ps1 -Sign -CertThumbprint <thumbprint>
```

Or manually:

```powershell
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -o wslwatch.exe ./cmd/wslwatch
```

## Testing

```powershell
# Unit tests
go test ./internal/...

# Integration tests (requires WSL2 with at least one distro)
go test ./test/integration/... -tags integration -v

# Target a specific distro
go test ./test/integration/... -tags integration -run TestLiveWatchdogStartsDeadDistro -v -distro Ubuntu-22.04
```

## Requirements

- Windows 10/11 with WSL2
- Go 1.21+
- No CGo required

## Architecture

```
wslwatch/
├── cmd/wslwatch/          # Entry point + CLI dispatch
├── internal/
│   ├── config/            # YAML config: load, validate, save, set-by-key
│   ├── wsl/               # wsl.exe invocation, distro enumeration
│   ├── watchdog/          # Core watch loop + sliding-window failure tracker
│   ├── service/           # Windows SCM install/uninstall + service handler
│   ├── ipc/               # Named pipe server/client (\\.\pipe\wslwatch)
│   ├── status/            # Color-coded terminal status renderer
│   └── lock/              # Named mutex for single-instance enforcement
└── test/integration/      # Live distro integration tests
```

## Distro Classification

The following WSL "distros" are infrastructure integrations and are never restarted:

| Pattern | Reason |
|---|---|
| `docker-desktop` | Docker Desktop host integration |
| `docker-desktop-data` | Docker Desktop data volume |
| Any `Installing` distro | Mid-install, unsafe to touch |

Add additional patterns to `ignored_distros` in the config.
