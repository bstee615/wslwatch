//go:build windows

package service

import (
	"context"
	"log/slog"

	"golang.org/x/sys/windows/svc"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

const customReloadControl = 128

// wslwatchService implements svc.Handler.
type wslwatchService struct {
	cfg    *config.Config
	cfgPath string
	logger *slog.Logger
}

// RunService starts the SCM service handler.
func RunService(cfg *config.Config, cfgPath string, logger *slog.Logger) error {
	return svc.Run(serviceName, &wslwatchService{
		cfg:     cfg,
		cfgPath: cfgPath,
		logger:  logger,
	})
}

// Execute is the SCM service handler entrypoint.
func (s *wslwatchService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Create watchdog
	runner := wsl.NewExecRunner()
	w := watchdog.New(s.cfg, runner, s.logger)

	// Create IPC handler
	ipcHandler := makeIPCHandler(w, s.cfg, s.cfgPath, s.logger)
	ipcServer := ipc.NewServer(ipcHandler, s.logger)

	// Start IPC server in background
	ipcCtx, ipcCancel := context.WithCancel(context.Background())
	defer ipcCancel()
	go func() {
		if err := ipcServer.ListenAndServe(ipcCtx); err != nil {
			s.logger.Error("IPC server error", "error", err)
		}
	}()

	// Start watchdog in background
	watchCtx, watchCancel := context.WithCancel(context.Background())
	defer watchCancel()
	watchDone := make(chan error, 1)
	go func() {
		watchDone <- w.Run(watchCtx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				watchCancel()
				ipcCancel()
				<-watchDone
				return
			case svc.Pause:
				w.PauseAll()
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				w.ResumeAll()
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				if c.Cmd == svc.Cmd(customReloadControl) {
					// Reload config
					newCfg, err := config.Load(s.cfgPath)
					if err != nil {
						s.logger.Error("config reload failed", "error", err)
					} else {
						w.ReloadConfig(newCfg)
						s.cfg = newCfg
						s.logger.Info("config reloaded")
					}
				}
			}
		case err := <-watchDone:
			if err != nil {
				s.logger.Error("watchdog exited with error", "error", err)
			}
			return
		}
	}
}

// IsWindowsService detects if we're running as a Windows service.
func IsWindowsService() bool {
	interactive, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return interactive
}

func makeIPCHandler(w *watchdog.Watchdog, cfg *config.Config, cfgPath string, logger *slog.Logger) ipc.Handler {
	return func(req ipc.Request) ipc.Response {
		return handleIPCRequest(w, cfg, cfgPath, logger, req)
	}
}
