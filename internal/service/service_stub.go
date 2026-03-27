//go:build !windows

package service

import (
	"errors"
	"log/slog"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

type WslWatchService struct {
	cfg    *config.Config
	logger *slog.Logger
}

func New(cfg *config.Config, logger *slog.Logger) *WslWatchService {
	return &WslWatchService{cfg: cfg, logger: logger}
}

// RunForeground runs the watchdog and IPC server without Windows SCM
// (used on non-Windows for testing, and also called by main when running foreground on Windows before SCM check)
func RunForeground(cfg *config.Config, logger *slog.Logger, stopCh <-chan struct{}) error {
	runner := wsl.NewWSLRunner()
	w := watchdog.New(cfg, runner, logger)
	w.Start()

	handler := makeHandler(w, cfg)
	server := ipc.NewServer(handler)
	if err := server.Start(); err != nil {
		logger.Warn("IPC server failed to start", "error", err)
	}

	<-stopCh
	w.Stop()
	server.Stop()
	return nil
}

func RunService(cfg *config.Config, logger *slog.Logger) error {
	return errors.New("Windows service mode is only supported on Windows")
}

func RunDebug(cfg *config.Config, logger *slog.Logger) error {
	return errors.New("Windows service debug mode is only supported on Windows")
}

// makeHandler creates the IPC handler that delegates to the watchdog
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
			return ipc.Response{OK: true}
		default:
			return ipc.Response{OK: false, Error: "unknown command: " + req.Cmd}
		}
	}
}

// watchdogStatusToIPCStatus converts watchdog.Status to ipc.StatusData
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
