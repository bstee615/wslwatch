package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/wsl"
)

// DistroManagementState tracks runtime state per distro.
type DistroManagementState struct {
	Name         string
	RestartCount int
	Exhausted    bool      // true if max_restarts reached
	StartedAt    time.Time // when the distro was last observed Running
	tracker      *FailureTracker
}

// DistroStatus is the public status view of a distro.
type DistroStatus struct {
	Name         string
	State        string // "healthy", "unhealthy", "paused", "exhausted", "ignored", "installing"
	Uptime       time.Duration
	RestartCount int
	InBackoff    bool
	BackoffUntil time.Time
	Exhausted    bool
	FailureTimes []time.Time
}

// Status is the overall watchdog status.
type Status struct {
	Running   bool
	Uptime    time.Duration
	StartedAt time.Time
	Distros   []DistroStatus
}

// Watchdog is the core WSL monitor.
type Watchdog struct {
	cfg    *config.Config
	runner wsl.Runner
	logger *slog.Logger

	mu        sync.RWMutex
	states    map[string]*DistroManagementState
	startedAt time.Time
	paused    bool // global pause (from service Pause/Continue)

	stopCh chan struct{}
	doneCh chan struct{}
}

// New creates a new Watchdog with the given config, runner, and logger.
// States are initialized for each enabled distro in cfg.Distros.
func New(cfg *config.Config, runner wsl.Runner, logger *slog.Logger) *Watchdog {
	w := &Watchdog{
		cfg:    cfg,
		runner: runner,
		logger: logger,
		states: make(map[string]*DistroManagementState),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}

	for _, d := range cfg.Distros {
		if !d.Enabled {
			continue
		}
		w.states[d.Name] = &DistroManagementState{
			Name: d.Name,
			tracker: NewFailureTracker(
				cfg.FailureWindow.Duration,
				cfg.FailureThreshold,
				cfg.BackoffDuration.Duration,
			),
		}
	}

	return w
}

// Start begins the watch loop in a goroutine.
func (w *Watchdog) Start() {
	go w.run()
}

// Stop signals the watch loop to exit and waits for it to finish.
func (w *Watchdog) Stop() {
	close(w.stopCh)
	<-w.doneCh
}

// PauseAll pauses all distro management (global pause for service Pause signal).
func (w *Watchdog) PauseAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paused = true
}

// ResumeAll resumes all distro management.
func (w *Watchdog) ResumeAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paused = false
}

// PauseDistro pauses management of a specific distro (sets cfg.Distros[].Pause = true).
func (w *Watchdog) PauseDistro(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, d := range w.cfg.Distros {
		if d.Name == name {
			w.cfg.Distros[i].Pause = true
			return nil
		}
	}
	return fmt.Errorf("distro %q not found in config", name)
}

// ResumeDistro resumes management of a specific distro.
func (w *Watchdog) ResumeDistro(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, d := range w.cfg.Distros {
		if d.Name == name {
			w.cfg.Distros[i].Pause = false
			return nil
		}
	}
	return fmt.Errorf("distro %q not found in config", name)
}

// ReloadConfig updates the watchdog config and adjusts internal state.
// New enabled distros get a fresh DistroManagementState; removed distros are cleaned up.
func (w *Watchdog) ReloadConfig(cfg *config.Config) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Add states for any newly enabled distros.
	newStateKeys := make(map[string]bool)
	for _, d := range cfg.Distros {
		if !d.Enabled {
			continue
		}
		newStateKeys[d.Name] = true
		if _, exists := w.states[d.Name]; !exists {
			w.states[d.Name] = &DistroManagementState{
				Name: d.Name,
				tracker: NewFailureTracker(
					cfg.FailureWindow.Duration,
					cfg.FailureThreshold,
					cfg.BackoffDuration.Duration,
				),
			}
		}
	}

	// Remove states for distros no longer in config.
	for name := range w.states {
		if !newStateKeys[name] {
			delete(w.states, name)
		}
	}

	w.cfg = cfg
}

// GetStatus returns current status.
func (w *Watchdog) GetStatus() Status {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var distroStatuses []DistroStatus

	// Add ignored distros first.
	for _, name := range w.cfg.IgnoredDistros {
		distroStatuses = append(distroStatuses, DistroStatus{
			Name:  name,
			State: "ignored",
		})
	}

	for _, d := range w.cfg.Distros {
		if !d.Enabled {
			continue
		}

		state, exists := w.states[d.Name]
		if !exists {
			continue
		}

		var uptime time.Duration
		if !state.StartedAt.IsZero() {
			uptime = time.Since(state.StartedAt)
		}

		var statusState string
		switch {
		case state.Exhausted:
			statusState = "exhausted"
		case d.Pause || w.paused:
			statusState = "paused"
		case state.tracker.InBackoff():
			statusState = "unhealthy"
		default:
			statusState = "healthy"
		}

		distroStatuses = append(distroStatuses, DistroStatus{
			Name:         d.Name,
			State:        statusState,
			Uptime:       uptime,
			RestartCount: state.RestartCount,
			InBackoff:    state.tracker.InBackoff(),
			BackoffUntil: state.tracker.BackoffUntil(),
			Exhausted:    state.Exhausted,
			FailureTimes: state.tracker.FailureTimes(),
		})
	}

	var uptime time.Duration
	if !w.startedAt.IsZero() {
		uptime = time.Since(w.startedAt)
	}

	return Status{
		Running:   !w.startedAt.IsZero(),
		Uptime:    uptime,
		StartedAt: w.startedAt,
		Distros:   distroStatuses,
	}
}

// IsIgnored returns true if name is in cfg.IgnoredDistros.
func (w *Watchdog) IsIgnored(name string) bool {
	for _, ignored := range w.cfg.IgnoredDistros {
		if ignored == name {
			return true
		}
	}
	return false
}

// run is the main watch loop, executed in a goroutine by Start.
func (w *Watchdog) run() {
	defer close(w.doneCh)

	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.logger.Error("watchdog panic recovered", "panic", r)
					time.Sleep(5 * time.Second)
				}
			}()

			w.mu.Lock()
			w.startedAt = time.Now()
			w.mu.Unlock()

			ticker := time.NewTicker(w.cfg.CheckInterval.Duration)
			defer ticker.Stop()

			for {
				select {
				case <-w.stopCh:
					return
				case <-ticker.C:
					w.checkAll()
				}
			}
		}()

		// Check if we should exit after panic recovery.
		select {
		case <-w.stopCh:
			return
		default:
		}
	}
}

// checkAll checks all configured distros and restarts any that are unhealthy.
func (w *Watchdog) checkAll() {
	ctx := context.Background()

	w.mu.RLock()
	globalPaused := w.paused
	cfg := w.cfg
	w.mu.RUnlock()

	// List distros once for all checks in this tick.
	distros, err := w.runner.ListDistros(ctx)
	if err != nil {
		w.logger.Error("failed to list distros", "error", err)
		return
	}

	for _, distro := range cfg.Distros {
		if !distro.Enabled {
			continue
		}
		if globalPaused {
			continue
		}
		if distro.Pause {
			continue
		}

		w.mu.RLock()
		state, exists := w.states[distro.Name]
		w.mu.RUnlock()

		if !exists {
			continue
		}

		if state.Exhausted {
			w.logger.Debug("skipping exhausted distro", "distro", distro.Name)
			continue
		}

		if state.tracker.InBackoff() {
			w.logger.Debug("skipping distro in backoff",
				"distro", distro.Name,
				"backoff_until", state.tracker.BackoffUntil(),
			)
			continue
		}

		healthy := w.queryDistroHealth(ctx, distro.Name, distros)

		if healthy {
			w.mu.Lock()
			if state.StartedAt.IsZero() {
				state.StartedAt = time.Now()
			}
			w.mu.Unlock()
			continue
		}

		// Unhealthy: record failure and possibly restart.
		state.tracker.RecordFailure()

		if distro.MaxRestarts > 0 && state.RestartCount >= distro.MaxRestarts {
			w.mu.Lock()
			state.Exhausted = true
			w.mu.Unlock()
			w.logger.Warn("distro has exhausted max restarts",
				"distro", distro.Name,
				"max_restarts", distro.MaxRestarts,
			)
			continue
		}

		w.restartDistro(ctx, distro)

		w.mu.Lock()
		state.RestartCount++
		w.mu.Unlock()
	}
}

// queryDistroHealth checks if a distro is healthy using the pre-fetched distro list.
func (w *Watchdog) queryDistroHealth(ctx context.Context, name string, distros []wsl.DistroInfo) bool {
	// Find the distro in the list.
	var found *wsl.DistroInfo
	for i := range distros {
		if distros[i].Name == name {
			found = &distros[i]
			break
		}
	}

	if found == nil {
		return false
	}

	if found.State == wsl.StateInstalling {
		return false
	}

	if found.State != wsl.StateRunning {
		return false
	}

	// Create a context with probe timeout.
	probeCtx, cancel := context.WithTimeout(ctx, w.cfg.ProbeTimeout.Duration)
	defer cancel()

	if err := w.runner.Probe(probeCtx, name); err != nil {
		w.logger.Debug("probe failed", "distro", name, "error", err)
		return false
	}

	return true
}

// restartDistro terminates and restarts a distro.
func (w *Watchdog) restartDistro(ctx context.Context, distro config.DistroConfig) {
	w.logger.Info("restarting distro", "distro", distro.Name)

	if err := w.runner.Terminate(ctx, distro.Name); err != nil {
		w.logger.Error("failed to terminate distro", "distro", distro.Name, "error", err)
	}

	w.mu.RLock()
	restartDelay := w.cfg.RestartDelay.Duration
	w.mu.RUnlock()

	time.Sleep(restartDelay)

	if err := w.runner.Start(ctx, distro.Name); err != nil {
		w.logger.Error("failed to start distro", "distro", distro.Name, "error", err)
	}

	if distro.StartCommand != "" {
		if _, err := w.runner.Exec(ctx, distro.Name, "sh", "-c", distro.StartCommand); err != nil {
			w.logger.Error("failed to exec start command",
				"distro", distro.Name,
				"command", distro.StartCommand,
				"error", err,
			)
		}
	}
}
