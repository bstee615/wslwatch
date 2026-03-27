//go:build windows

package service

import (
	"context"
	"log/slog"

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
	cfg    *config.Config
	logger *slog.Logger
}

// New creates a new WslWatchService.
func New(cfg *config.Config, logger *slog.Logger) *WslWatchService {
	return &WslWatchService{cfg: cfg, logger: logger}
}

// Execute implements svc.Handler. It is called by the Windows SCM.
func (s *WslWatchService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	const acceptedCmds = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue

	status <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

	runner := wsl.NewWSLRunner()
	w := watchdog.New(s.cfg, runner, s.logger)
	w.Start()

	handler := makeHandler(w, s.cfg)
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
func makeHandler(w *watchdog.Watchdog, cfg *config.Config) ipc.Handler {
	return func(req ipc.Request) ipc.Response {
		switch req.Cmd {
		case "status":
			status := w.GetStatus()
			data := watchdogStatusToIPCStatus(status)
			return ipc.Response{OK: true, Data: data}

		case "pause":
			if req.Distro == "" {
				return ipc.Response{OK: false, Error: "distro name required"}
			}
			if err := w.PauseDistro(req.Distro); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true}

		case "resume":
			if req.Distro == "" {
				return ipc.Response{OK: false, Error: "distro name required"}
			}
			if err := w.ResumeDistro(req.Distro); err != nil {
				return ipc.Response{OK: false, Error: err.Error()}
			}
			return ipc.Response{OK: true}

		case "reload":
			newCfg, err := config.Load("")
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

// watchdogStatusToIPCStatus converts watchdog.Status to ipc.StatusData.
func watchdogStatusToIPCStatus(s watchdog.Status) ipc.StatusData {
	var distros []ipc.DistroData
	for _, d := range s.Distros {
		distros = append(distros, ipc.DistroData{
			Name:         d.Name,
			State:        d.State,
			Uptime:       d.Uptime.String(),
			RestartCount: d.RestartCount,
			InBackoff:    d.InBackoff,
			BackoffUntil: d.BackoffUntil,
			Exhausted:    d.Exhausted,
		})
	}
	return ipc.StatusData{
		Running:   s.Running,
		Uptime:    s.Uptime.String(),
		StartedAt: s.StartedAt,
		Distros:   distros,
	}
}

// RunService runs the service in SCM mode (called when started by SCM).
func RunService(cfg *config.Config, logger *slog.Logger) error {
	elog, err := eventlog.Open(ServiceName)
	if err == nil {
		defer elog.Close()
	}
	s := New(cfg, logger)
	return svc.Run(ServiceName, s)
}

// RunDebug runs the service in debug mode (foreground, for testing).
func RunDebug(cfg *config.Config, logger *slog.Logger) error {
	s := New(cfg, logger)
	return debug.Run(ServiceName, s)
}

// RunForeground is provided for API compatibility with the stub; on Windows
// it delegates to RunDebug.
func RunForeground(cfg *config.Config, logger *slog.Logger, stopCh <-chan struct{}) error {
	_ = stopCh
	return RunDebug(cfg, logger)
}

// Ensure context is used (imported for watchdog.run loop usage).
var _ = context.Background
