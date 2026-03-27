//go:build !windows

package service

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

type WslWatchService struct {
	cfg     *config.Config
	cfgPath string
	logger  *slog.Logger
}

func New(cfg *config.Config, cfgPath string, logger *slog.Logger) *WslWatchService {
	return &WslWatchService{cfg: cfg, cfgPath: cfgPath, logger: logger}
}

// RunForeground runs the watchdog and IPC server without Windows SCM.
func RunForeground(cfg *config.Config, cfgPath string, logger *slog.Logger, stopCh <-chan struct{}) error {
	runner := wsl.NewWSLRunner()
	w := watchdog.New(cfg, runner, logger)
	w.Start()

	handler := makeHandler(w, cfg, cfgPath, logger)
	server := ipc.NewServer(handler)
	if err := server.Start(); err != nil {
		logger.Warn("IPC server failed to start", "error", err)
	}

	<-stopCh
	w.Stop()
	server.Stop()
	return nil
}

func RunService(cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	return errors.New("Windows service mode is only supported on Windows")
}

func RunDebug(cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	return errors.New("Windows service debug mode is only supported on Windows")
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

// persistPauseState saves the pause/resume state back to config on disk.
func persistPauseState(cfg *config.Config, cfgPath string, distroName string, paused bool, logger *slog.Logger) {
	for i := range cfg.Distros {
		if cfg.Distros[i].Name == distroName {
			cfg.Distros[i].Pause = paused
			break
		}
	}
	if cfgPath != "" {
		if err := cfg.Save(cfgPath); err != nil {
			logger.Warn("failed to persist pause state to config", "distro", distroName, "error", err)
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
