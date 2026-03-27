package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/ipc"
	"github.com/bstee615/wslwatch/internal/lock"
	"github.com/bstee615/wslwatch/internal/service"
	"github.com/bstee615/wslwatch/internal/status"
	"github.com/bstee615/wslwatch/internal/wsl"
)

func main() {
	args := os.Args[1:]

	// Extract global --config-file flag.
	configFile, args := extractStringFlag(args, "--config-file")

	if len(args) == 0 {
		runDefault(configFile)
		return
	}

	switch args[0] {
	case "--install":
		cmdInstall(configFile, args[1:])
	case "--uninstall":
		cmdUninstall()
	case "--autoconfig":
		cmdAutoconfig(configFile)
	case "--config":
		cmdConfig(configFile, args[1:])
	case "--status":
		cmdStatus()
	case "--pause":
		cmdPause(args[1:])
	case "--resume":
		cmdResume(args[1:])
	case "--bark":
		cmdBark()
	case "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[0])
		printUsage()
		os.Exit(1)
	}
}

// runDefault is the no-flags entry point. On Windows it checks if running as a
// Windows Service; if so it hands off to the SCM handler. Otherwise it runs
// the watchdog in the foreground.
func runDefault(configFile string) {
	if isWindowsService() {
		cfg, logger := loadConfigAndLogger(configFile)
		if err := service.RunService(cfg, configFile, logger); err != nil {
			fmt.Fprintf(os.Stderr, "service error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cfg, logger := loadConfigAndLogger(configFile)

	lk, err := lock.Acquire()
	if err != nil {
		fmt.Fprintln(os.Stderr, "another instance of wslwatch is already running")
		os.Exit(1)
	}
	defer lk.Release()

	stopCh := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stopCh)
	}()

	fmt.Println("wslwatch running in foreground. Press Ctrl+C to stop.")
	if err := service.RunForeground(cfg, configFile, logger, stopCh); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// cmdInstall installs wslwatch as a Windows Service.
func cmdInstall(configFile string, args []string) {
	addToPath := false
	for _, a := range args {
		if a == "--add-to-path" {
			addToPath = true
		}
	}

	// Ensure config exists; if not, run autoconfig first.
	if _, err := os.Stat(config.DefaultPath()); os.IsNotExist(err) {
		fmt.Println("No config found. Running --autoconfig first...")
		cmdAutoconfig(configFile)
		fmt.Println("\nPlease review the generated config and re-run --install.")
		return
	}

	// Validate the config.
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config validation error: %v\n", err)
		os.Exit(1)
	}

	if !service.IsElevated() {
		fmt.Println("Requesting elevation...")
		if err := service.RelaunchElevated(os.Args[1:]...); err != nil {
			fmt.Fprintf(os.Stderr, "failed to relaunch elevated: %v\n", err)
			os.Exit(1)
		}
		return
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine executable path: %v\n", err)
		os.Exit(1)
	}

	if err := service.Install(exe, addToPath); err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		os.Exit(1)
	}
}

// cmdUninstall removes the wslwatch service interactively.
func cmdUninstall() {
	fmt.Println("wslwatch uninstaller")
	fmt.Println("────────────────────")
	fmt.Println("  [1] Remove wslwatch only (keep config and logs)")
	fmt.Println("  [2] Remove everything (config, logs, install dir)")
	fmt.Println("  [q] Cancel")
	fmt.Print("\nChoice: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "1":
		if !service.IsElevated() {
			fmt.Println("Requesting elevation...")
			if err := service.RelaunchElevated(os.Args[1:]...); err != nil {
				fmt.Fprintf(os.Stderr, "failed to relaunch elevated: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if err := service.Uninstall(false); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("wslwatch service removed. Config and logs preserved.")
	case "2":
		if !service.IsElevated() {
			fmt.Println("Requesting elevation...")
			if err := service.RelaunchElevated(os.Args[1:]...); err != nil {
				fmt.Fprintf(os.Stderr, "failed to relaunch elevated: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if err := service.Uninstall(true); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("wslwatch completely removed.")
	case "q", "Q", "":
		fmt.Println("Cancelled.")
	default:
		fmt.Println("Invalid choice. Cancelled.")
	}
}

// cmdAutoconfig generates wslwatch.yaml from installed distros.
func cmdAutoconfig(configFile string) {
	runner := wsl.NewWSLRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	distros, err := runner.ListDistros(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to enumerate distros: %v\n", err)
		fmt.Println("Generating config with no distros configured.")
		distros = nil
	}

	cfg := config.Default()

	// Filter out ignored distros and add the rest.
	ignoredSet := make(map[string]bool)
	for _, name := range cfg.IgnoredDistros {
		ignoredSet[strings.ToLower(name)] = true
	}

	for _, d := range distros {
		if ignoredSet[strings.ToLower(d.Name)] {
			continue
		}
		// Skip distros in Installing state.
		if d.State == wsl.StateInstalling {
			continue
		}
		cfg.Distros = append(cfg.Distros, config.DistroConfig{
			Name:    d.Name,
			Enabled: true,
		})
	}

	// Show the generated config.
	yamlBytes, err := marshalConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Generated configuration:")
	fmt.Println("────────────────────────")
	fmt.Println(string(yamlBytes))

	// Determine output path.
	outPath := configFile
	if outPath == "" {
		outPath = config.DefaultPath()
	}

	// Check if config already exists.
	if _, err := os.Stat(outPath); err == nil {
		fmt.Printf("Config already exists at %s\n", outPath)
		fmt.Print("Overwrite? [y/N]: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			fmt.Println("Cancelled.")
			return
		}
	} else {
		fmt.Printf("Write to %s? [Y/n]: ", outPath)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if answer == "n" || answer == "no" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := cfg.Save(outPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config written to %s\n", outPath)
}

// cmdConfig sets a config value by dotted key path.
func cmdConfig(configFile string, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: wslwatch --config <key> <value>")
		os.Exit(1)
	}
	key, value := args[0], args[1]

	path := configFile
	if path == "" {
		path = config.DefaultPath()
	}

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.SetByKey(key, value); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set %s: %v\n", key, err)
		os.Exit(1)
	}

	if err := cfg.Save(path); err != nil {
		fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Set %s = %s\n", key, value)

	// Signal running service to reload.
	client := ipc.NewClientWithTimeout(2 * time.Second)
	if client.IsRunning() {
		resp, err := client.Send(ipc.Request{Cmd: "reload"})
		if err != nil || !resp.OK {
			fmt.Println("(note: could not signal running service to reload config)")
		} else {
			fmt.Println("Running service notified to reload config.")
		}
	}
}

// cmdStatus displays the watchdog status via IPC.
func cmdStatus() {
	client := ipc.NewClientWithTimeout(5 * time.Second)
	if !client.IsRunning() {
		status.RenderNotRunning(os.Stdout)
		return
	}

	resp, err := client.Send(ipc.Request{Cmd: "status"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "IPC error: %v\n", err)
		status.RenderNotRunning(os.Stdout)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "error from service: %s\n", resp.Error)
		os.Exit(1)
	}

	var data ipc.StatusData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode status data: %v\n", err)
		os.Exit(1)
	}

	status.RenderStatus(os.Stdout, &data)
}

// cmdPause pauses a distro via IPC.
func cmdPause(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: wslwatch --pause <distro>")
		os.Exit(1)
	}
	distroName := args[0]

	client := ipc.NewClientWithTimeout(5 * time.Second)
	resp, err := client.Send(ipc.Request{Cmd: "pause", Distro: distroName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "IPC error: %v\n", err)
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error)
		os.Exit(1)
	}
	fmt.Printf("Paused %s\n", distroName)
}

// cmdResume resumes a paused distro via IPC.
func cmdResume(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: wslwatch --resume <distro>")
		os.Exit(1)
	}
	distroName := args[0]

	client := ipc.NewClientWithTimeout(5 * time.Second)
	resp, err := client.Send(ipc.Request{Cmd: "resume", Distro: distroName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "IPC error: %v\n", err)
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "error: %s\n", resp.Error)
		os.Exit(1)
	}
	fmt.Printf("Resumed %s\n", distroName)
}

// cmdBark prints Spike in ASCII art.
func cmdBark() {
	data, err := os.ReadFile("assets/spike.txt")
	if err != nil {
		// Embedded fallback.
		fmt.Println(spikeASCII)
		return
	}
	fmt.Print(string(data))
}

// loadConfigAndLogger loads config and sets up a logger.
func loadConfigAndLogger(configFile string) (*config.Config, *slog.Logger) {
	cfg, err := config.Load(configFile)
	if err != nil {
		// Use defaults if config not found.
		cfg = config.Default()
	}

	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var handler slog.Handler
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open log file %s: %v\n", cfg.LogFile, err)
			handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
		} else {
			handler = slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
		}
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}

	return cfg, slog.New(handler)
}

// extractStringFlag extracts a named flag and its value from args, returning
// the value and the remaining args with the flag removed.
func extractStringFlag(args []string, name string) (string, []string) {
	var out []string
	var value string
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			value = args[i+1]
			i++
		} else {
			out = append(out, args[i])
		}
	}
	return value, out
}

// marshalConfig marshals a config to YAML bytes.
func marshalConfig(cfg *config.Config) ([]byte, error) {
	// Save to a temp path and read back, or use yaml directly.
	// We use cfg.Save internals via a temp file.
	tmp, err := os.CreateTemp("", "wslwatch-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := cfg.Save(tmp.Name()); err != nil {
		return nil, err
	}
	return os.ReadFile(tmp.Name())
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `wslwatch - WSL2 distro watchdog

Usage:
  wslwatch                          Run watchdog in foreground
  wslwatch --install [--add-to-path] Install as Windows Service
  wslwatch --uninstall              Remove the Windows Service
  wslwatch --autoconfig             Generate config from installed distros
  wslwatch --config <key> <value>   Set a config value
  wslwatch --status                 Show watchdog status
  wslwatch --pause <distro>         Pause monitoring a distro
  wslwatch --resume <distro>        Resume monitoring a distro
  wslwatch --bark                   Print Spike

Global flags:
  --config-file <path>              Override config file path
`)
}

// spikeASCII is the fallback ASCII art when assets/spike.txt is not found.
const spikeASCII = `
   / \__
  (    @\___
  /         O
 /   (_____/
/_____/   U

  Spike says: WOOF! Watching your distros.
`
