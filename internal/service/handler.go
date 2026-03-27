package service

import (
	"encoding/json"
	"log/slog"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
)

// HandleIPCRequest processes an IPC request against the watchdog.
// This is shared between the Windows service and foreground mode.
func HandleIPCRequest(w *watchdog.Watchdog, cfg *config.Config, cfgPath string, logger *slog.Logger, req ipc.Request) ipc.Response {
	return handleIPCRequest(w, cfg, cfgPath, logger, req)
}

func handleIPCRequest(w *watchdog.Watchdog, cfg *config.Config, cfgPath string, logger *slog.Logger, req ipc.Request) ipc.Response {
	switch req.Cmd {
	case "status":
		status := w.Status()
		data, err := json.Marshal(status)
		if err != nil {
			return ipc.Response{OK: false, Error: "marshaling status: " + err.Error()}
		}
		return ipc.Response{OK: true, Data: data}

	case "pause":
		if req.Distro == "" {
			return ipc.Response{OK: false, Error: "distro name required"}
		}
		if err := w.PauseDistro(req.Distro); err != nil {
			return ipc.Response{OK: false, Error: err.Error()}
		}
		// Update config
		d := cfg.FindDistro(req.Distro)
		if d != nil {
			d.Pause = true
			if err := config.Save(cfg, cfgPath); err != nil {
				logger.Warn("failed to save config after pause", "error", err)
			}
		}
		return ipc.Response{OK: true}

	case "resume":
		if req.Distro == "" {
			return ipc.Response{OK: false, Error: "distro name required"}
		}
		if err := w.ResumeDistro(req.Distro); err != nil {
			return ipc.Response{OK: false, Error: err.Error()}
		}
		// Update config
		d := cfg.FindDistro(req.Distro)
		if d != nil {
			d.Pause = false
			if err := config.Save(cfg, cfgPath); err != nil {
				logger.Warn("failed to save config after resume", "error", err)
			}
		}
		return ipc.Response{OK: true}

	case "reload":
		newCfg, err := config.Load(cfgPath)
		if err != nil {
			return ipc.Response{OK: false, Error: "loading config: " + err.Error()}
		}
		w.ReloadConfig(newCfg)
		*cfg = *newCfg
		logger.Info("config reloaded via IPC")
		return ipc.Response{OK: true}

	default:
		return ipc.Response{OK: false, Error: "unknown command: " + req.Cmd}
	}
}
