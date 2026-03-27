package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/lock"
	"github.com/bstee615/wslwatch/internal/service"
	"github.com/bstee615/wslwatch/internal/status"
	"github.com/bstee615/wslwatch/internal/watchdog"
	"github.com/bstee615/wslwatch/internal/wsl"
)

var (
	cfgPath string
	version = "dev"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "wslwatch",
		Short:   "WSL Watchdog — monitors WSL2 distros and restarts them when they die",
		Version: version,
		RunE:    runForeground,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", "", "path to config file")

	// Commands
	rootCmd.AddCommand(installCmd())
	rootCmd.AddCommand(uninstallCmd())
	rootCmd.AddCommand(autoconfigCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(pauseCmd())
	rootCmd.AddCommand(resumeCmd())
	rootCmd.AddCommand(barkCmd())
	rootCmd.AddCommand(configSetCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getConfigPath() string {
	if cfgPath != "" {
		return cfgPath
	}
	return config.DefaultConfigPath()
}

func setupLogger(cfg *config.Config) *slog.Logger {
	var handler slog.Handler

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}

	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log file %s: %v\n", cfg.LogFile, err)
			handler = slog.NewTextHandler(os.Stdout, opts)
		} else {
			handler = slog.NewTextHandler(f, opts)
		}
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// runForeground starts the watchdog in the foreground (no flags).
func runForeground(cmd *cobra.Command, args []string) error {
	// Check if running as Windows service
	if service.IsWindowsService() {
		cfg, err := config.Load(getConfigPath())
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		logger := setupLogger(cfg)
		return service.RunService(cfg, getConfigPath(), logger)
	}

	// Foreground mode
	cfg, err := config.Load(getConfigPath())
	if err != nil {
		// If config doesn't exist, try to create a default one
		if os.IsNotExist(err) {
			fmt.Println("No config file found. Run 'wslwatch autoconfig' to generate one.")
			fmt.Println("Using default configuration...")
			cfg = config.DefaultConfig()
		} else {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	logger := setupLogger(cfg)

	// Single-instance check
	instanceLock := lock.NewLock()
	acquired, err := instanceLock.Acquire()
	if err != nil {
		return fmt.Errorf("checking instance lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("another instance of wslwatch is already running")
	}
	defer instanceLock.Release()

	// Create watchdog
	runner := wsl.NewExecRunner()
	w := watchdog.New(cfg, runner, logger)

	// Start IPC server
	ipcCtx, ipcCancel := context.WithCancel(context.Background())
	defer ipcCancel()

	ipcHandler := func(req ipc.Request) ipc.Response {
		return service.HandleIPCRequest(w, cfg, getConfigPath(), logger, req)
	}
	ipcServer := ipc.NewServer(ipcHandler, logger)
	go func() {
		if err := ipcServer.ListenAndServe(ipcCtx); err != nil {
			logger.Error("IPC server error", "error", err)
		}
	}()

	// Handle signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	// Run watchdog
	return w.Run(ctx)
}

func installCmd() *cobra.Command {
	var addToPath bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install wslwatch as a Windows Service",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			// Check if config exists, if not run autoconfig
			path := getConfigPath()
			if _, err := os.Stat(path); os.IsNotExist(err) {
				fmt.Println("No config found. Running autoconfig first...")
				if err := runAutoconfig(logger); err != nil {
					return err
				}
			}

			if err := service.Install(logger); err != nil {
				return err
			}

			if addToPath {
				fmt.Println("Note: --add-to-path requires manual PATH update on this platform")
			}

			color.Green("✓ wslwatch installed and started as a Windows Service")
			return nil
		},
	}
	cmd.Flags().BoolVar(&addToPath, "add-to-path", false, "add install directory to system PATH")
	return cmd
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall wslwatch Windows Service",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			bold := color.New(color.Bold)
			bold.Println("wslwatch uninstaller")
			fmt.Println(strings.Repeat("─", 20))
			fmt.Println("  [1] Remove wslwatch only (keep config and logs)")
			fmt.Println("  [2] Remove everything (config, logs, install dir)")
			fmt.Println("  [q] Cancel")
			fmt.Println()
			fmt.Print("Choice: ")

			var choice string
			fmt.Scanln(&choice)

			switch strings.TrimSpace(choice) {
			case "1":
				return service.Uninstall(logger, false)
			case "2":
				return service.Uninstall(logger, true)
			case "q", "Q":
				fmt.Println("Cancelled.")
				return nil
			default:
				return fmt.Errorf("invalid choice: %s", choice)
			}
		},
	}
}

func autoconfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "autoconfig",
		Short: "Auto-detect WSL distros and generate config",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			return runAutoconfig(logger)
		},
	}
}

func runAutoconfig(logger *slog.Logger) error {
	runner := wsl.NewExecRunner()
	distros, err := wsl.ListDistros(runner, 10*time.Second)
	if err != nil {
		return fmt.Errorf("enumerating distros: %w", err)
	}

	cfg := config.DefaultConfig()

	for _, d := range distros {
		if wsl.IsDockerDistro(d.Name) || wsl.IsInstallingDistro(d) {
			// Add to ignored list if not already there
			if !cfg.IsIgnored(d.Name) {
				cfg.IgnoredDistros = append(cfg.IgnoredDistros, d.Name)
			}
			continue
		}

		cfg.Distros = append(cfg.Distros, config.DistroConfig{
			Name:    d.Name,
			Enabled: true,
		})
	}

	path := getConfigPath()

	// Show generated config
	fmt.Println("Generated configuration:")
	fmt.Println(strings.Repeat("─", 40))

	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	fmt.Println()

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Config file already exists at %s\n", path)
		fmt.Print("Overwrite? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	if err := config.Save(cfg, path); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	color.Green("✓ Config written to %s", path)
	logger.Info("autoconfig completed", "path", path, "distros", len(cfg.Distros))
	return nil
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show watchdog status",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := ipc.SendRequest(ipc.Request{Cmd: "status"})
			if err != nil {
				status.RenderNotRunning(os.Stdout)
				return nil
			}

			if !resp.OK {
				return fmt.Errorf("status request failed: %s", resp.Error)
			}

			var ws watchdog.WatchdogStatus
			if err := json.Unmarshal(resp.Data, &ws); err != nil {
				return fmt.Errorf("parsing status: %w", err)
			}

			// Load config for ignored distros
			cfg, _ := config.Load(getConfigPath())
			var ignored []string
			if cfg != nil {
				ignored = cfg.IgnoredDistros
			}

			status.Render(os.Stdout, ws, ignored)
			return nil
		},
	}
}

func pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <distro>",
		Short: "Pause management of a distro",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := ipc.SendRequest(ipc.Request{
				Cmd:    "pause",
				Distro: args[0],
			})
			if err != nil {
				return fmt.Errorf("connecting to watchdog: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("pause failed: %s", resp.Error)
			}
			color.Yellow("⏸ %s paused", args[0])
			return nil
		},
	}
}

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <distro>",
		Short: "Resume management of a distro",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := ipc.SendRequest(ipc.Request{
				Cmd:    "resume",
				Distro: args[0],
			})
			if err != nil {
				return fmt.Errorf("connecting to watchdog: %w", err)
			}
			if !resp.OK {
				return fmt.Errorf("resume failed: %s", resp.Error)
			}
			color.Green("▶ %s resumed", args[0])
			return nil
		},
	}
}

func barkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bark",
		Short: "Woof!",
		Run: func(cmd *cobra.Command, args []string) {
			printSpike()
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := getConfigPath()
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if err := cfg.SetByKey(args[0], args[1]); err != nil {
				return fmt.Errorf("setting config: %w", err)
			}

			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid config after change: %w", err)
			}

			if err := config.Save(cfg, path); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			color.Green("✓ %s = %s", args[0], args[1])

			// Try to signal reload via IPC
			resp, err := ipc.SendRequest(ipc.Request{Cmd: "reload"})
			if err != nil {
				fmt.Println("Note: watchdog not running, config saved but not reloaded")
			} else if resp.OK {
				fmt.Println("Config reloaded in running watchdog")
			}

			return nil
		},
	}
}

func printSpike() {
	spikeArt := color.YellowString(`
         / \__
        (    @\___
        /         O
       /   (_____/
      /_____/   U
    `) + color.CyanString(`
    ╔═══════════════════════╗
    ║   WOOF! I'm Spike!   ║
    ║   Watching your WSL   ║
    ║   distros, pal!       ║
    ╚═══════════════════════╝
`)
	fmt.Print(spikeArt)
}
