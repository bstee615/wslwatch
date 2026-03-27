# wslwatch 🐕

**WSL Watchdog** — monitors WSL2 distros and restarts them when they die.

A single-binary Go tool that installs itself as a Windows Service, is configured via YAML, and exposes a CLI for management, status, and diagnostics.

## Features

- **Automatic restart** — Detects when WSL distros go down and brings them back up
- **Windows Service** — Runs in the background as a proper Windows Service
- **Failure tracking** — Sliding window failure rate tracking with configurable backoff
- **IPC control** — Pause, resume, and query status via named pipe IPC
- **Auto-configuration** — Detects installed distros and generates config
- **Single instance** — Named mutex prevents duplicate watchdogs
- **Zero dependencies at runtime** — Single binary, no runtime requirements

## Quick Start

```powershell
# Build
go build -o wslwatch.exe ./cmd/wslwatch/

# Auto-detect distros and generate config
wslwatch autoconfig

# Run in foreground (for debugging)
wslwatch

# Install as Windows Service
wslwatch install

# Check status
wslwatch status

# Pause/resume a distro
wslwatch pause Ubuntu-22.04
wslwatch resume Ubuntu-22.04
```

## Configuration

Default location: `%ProgramData%\wslwatch\wslwatch.yaml`

```yaml
log_level: info              # debug | info | warn | error
log_file: ""                 # empty = stdout; path = file

check_interval: 15s          # how often to poll distro health
probe_timeout: 8s            # timeout for liveness probe per distro
restart_delay: 3s            # delay between terminate and restart

failure_window: 60s          # sliding window for failure rate tracking
failure_threshold: 5         # failures within window before backoff kicks in
backoff_duration: 0s         # how long to stop retrying (0 = no limit)

distros:
  - name: Ubuntu-22.04
    enabled: true
    max_restarts: 0          # 0 = unlimited
    pause: false
    start_command: ""         # optional: shell command to run after distro starts

ignored_distros:
  - docker-desktop
  - docker-desktop-data
```

## Commands

| Command | Description |
|---------|-------------|
| `wslwatch` | Start watchdog in foreground |
| `wslwatch install` | Install as Windows Service |
| `wslwatch uninstall` | Uninstall Windows Service |
| `wslwatch autoconfig` | Auto-detect distros and generate config |
| `wslwatch status` | Show watchdog and distro status |
| `wslwatch pause <distro>` | Pause management of a distro |
| `wslwatch resume <distro>` | Resume management of a distro |
| `wslwatch set <key> <value>` | Set a config value |
| `wslwatch bark` | 🐕 Woof! |

## Building

```powershell
# Simple build
go build -o wslwatch.exe ./cmd/wslwatch/

# Or use the build script
.\scripts\build.ps1 -Version "1.0.0"
```

## Testing

```powershell
# Unit tests
go test ./internal/... -v

# Integration tests (requires WSL with at least one distro)
go test ./test/integration/... -tags integration -v

# Target a specific distro
go test ./test/integration/... -tags integration -run TestLiveWatchdogStartsDeadDistro -v -distro Ubuntu-22.04
```

## Architecture

```
wslwatch/
├── cmd/wslwatch/          # CLI entry point
├── internal/
│   ├── config/            # YAML config loading/validation/saving
│   ├── wsl/               # WSL distro enumeration, state queries, runner
│   ├── watchdog/          # Core watch loop, failure tracking
│   ├── service/           # Windows Service integration (SCM)
│   ├── ipc/               # Named pipe IPC server/client
│   ├── status/            # Color-coded terminal output
│   └── lock/              # Single-instance mutex
├── test/integration/      # Live WSL integration tests
├── assets/                # ASCII art and resources
└── scripts/               # Build scripts
```

## Requirements

- Windows with WSL2
- Go 1.21+
- No CGo

## License

MIT
