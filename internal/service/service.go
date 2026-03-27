//go:build windows

package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

const (
	// CtrlReload is the custom control code for config reload.
	CtrlReload svc.Cmd = 128
)

// WslWatchService implements svc.Handler and runs the watchdog loop.
type WslWatchService struct {
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger
}

// New creates a new WslWatchService.
func New(cfg *config.Config, cfgPath string, logger *slog.Logger) *WslWatchService {
	return &WslWatchService{cfg: cfg, cfgPath: cfgPath, logger: logger}
}

// Execute implements svc.Handler. It is called by the Windows SCM.
func (s *WslWatchService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	const acceptedCmds = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	status <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

	if err := wsl.EnsureVMIdleTimeout(); err != nil {
		s.logger.Warn("failed to set vmIdleTimeout in .wslconfig", "error", err)
	}

	runner := wsl.NewWSLRunner()
	w := watchdog.New(s.cfg, runner, s.logger)
	w.Start()

	handler := makeHandler(w, s.cfg, s.cfgPath, s.logger)
	ipcServer := ipc.NewServer(handler)
	if err := ipcServer.Start(); err != nil {
		s.logger.Warn("IPC server failed to start", "error", err)
	}

	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			w.Stop()
			ipcServer.Stop()
			return false, 0

		case svc.Pause:
			w.PauseAll()
			status <- svc.Status{State: svc.Paused, Accepts: acceptedCmds}

		case svc.Continue:
			w.ResumeAll()
			status <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

		case CtrlReload:
			newCfg, err := config.Load("")
			if err != nil {
				s.logger.Warn("failed to reload config", "error", err)
			} else {
				s.cfg = newCfg
				w.ReloadConfig(newCfg)
			}

		default:
			s.logger.Warn("unexpected control request", "cmd", c.Cmd)
		}
	}

	return false, 0
}

// makeHandler creates the IPC handler that delegates to the watchdog.
func makeHandler(w *watchdog.Watchdog, cfg *config.Config, cfgPath string, logger *slog.Logger) ipc.Handler {
	return func(req ipc.Request) ipc.Response {
		switch req.Cmd {
		case "status":
			s := w.GetStatus()
			data := watchdogStatusToIPCStatus(s)
			raw, err := json.Marshal(data)
			if err != nil {
				return ipc.Response{OK: false, Error: "marshaling status: " + err.Error()}
			}
			return ipc.Response{OK: true, Data: raw}

		case "pause":
			if req.Distro == "" {
				return ipc.Response{OK: false, Error: "distro name required"}
			}
			if err := w.PauseDistro(req.Distro); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			persistPauseState(cfg, cfgPath, req.Distro, true, logger)
			return ipc.Response{OK: true}

		case "resume":
			if req.Distro == "" {
				return ipc.Response{OK: false, Error: "distro name required"}
			}
			if err := w.ResumeDistro(req.Distro); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			persistPauseState(cfg, cfgPath, req.Distro, false, logger)
			return ipc.Response{OK: true}

		case "reload":
			newCfg, err := config.Load(cfgPath)
			if err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			w.ReloadConfig(newCfg)
			return ipc.Response{OK: true}

		default:
			return ipc.Response{OK: false, Error: "unknown command: " + req.Cmd}
		}
	}
}

// persistPauseState saves the pause/resume state back to config on disk so it
// survives a service restart.
func persistPauseState(cfg *config.Config, cfgPath string, distroName string, paused bool, logger *slog.Logger) {
	for i := range cfg.Distros {
		if cfg.Distros[i].Name == distroName {
			cfg.Distros[i].Pause = paused
			break
		}
	}
	if err := cfg.Save(cfgPath); err != nil {
		logger.Warn("failed to persist pause state to config", "distro", distroName, "error", err)
	}
}

// formatDuration formats a duration with at most 1 decimal place of seconds.
func formatDuration(d time.Duration) string {
	d = d.Truncate(100 * time.Millisecond)
	return d.String()
}

// watchdogStatusToIPCStatus converts watchdog.Status to ipc.StatusData.
func watchdogStatusToIPCStatus(s watchdog.Status) ipc.StatusData {
	var distros []ipc.DistroData
	for _, d := range s.Distros {
		distros = append(distros, ipc.DistroData{
			Name:         d.Name,
			State:        d.State,
			Uptime:       formatDuration(d.Uptime),
			RestartCount: d.RestartCount,
			InBackoff:    d.InBackoff,
			BackoffUntil: d.BackoffUntil,
			Exhausted:    d.Exhausted,
			FailureTimes: d.FailureTimes,
		})
	}
	return ipc.StatusData{
		Running:   s.Running,
		Uptime:    formatDuration(s.Uptime),
		StartedAt: s.StartedAt,
		Distros:   distros,
	}
}

// RunService runs the service in SCM mode (called when started by SCM).
func RunService(cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	elog, err := eventlog.Open(ServiceName)
	if err == nil {
		defer elog.Close()
	}
	s := New(cfg, cfgPath, logger)
	return svc.Run(ServiceName, s)
}

// RunDebug runs the service in debug mode (foreground, for testing).
func RunDebug(cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	s := New(cfg, cfgPath, logger)
	return debug.Run(ServiceName, s)
}

// RunForeground is provided for API compatibility with the stub; on Windows
// it delegates to RunDebug.
func RunForeground(cfg *config.Config, cfgPath string, logger *slog.Logger, stopCh <-chan struct{}) error {
	_ = stopCh
	return RunDebug(cfg, cfgPath, logger)
}

// Ensure context is used (imported for watchdog.run loop usage).
var _ = context.Background
