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

// DistroStatus holds runtime state for a managed distro.
type DistroStatus struct {
	Name         string
	Healthy      bool
	Paused       bool
	Exhausted    bool
	RestartCount int
	LastSeen     time.Time
	StartedAt    time.Time
	Tracker      *FailureTracker
}

// Watchdog is the core watch loop that monitors and restarts WSL distros.
type Watchdog struct {
	mu       sync.RWMutex
	cfg      *config.Config
	runner   wsl.Runner
	logger   *slog.Logger
	statuses map[string]*DistroStatus
	started  time.Time

	// Control channels
	stopCh   chan struct{}
	pauseAll bool

	// Callbacks for testing
	OnTick    func()
	OnRestart func(distro string)
}

// New creates a new Watchdog with the given configuration and runner.
func New(cfg *config.Config, runner wsl.Runner, logger *slog.Logger) *Watchdog {
	w := &Watchdog{
		cfg:      cfg,
		runner:   runner,
		logger:   logger,
		statuses: make(map[string]*DistroStatus),
		stopCh:   make(chan struct{}),
	}

	// Initialize distro statuses
	for _, d := range cfg.Distros {
		if !d.Enabled {
			continue
		}
		w.statuses[d.Name] = &DistroStatus{
			Name:      d.Name,
			Paused:    d.Pause,
			StartedAt: time.Now(),
			Tracker: NewFailureTracker(
				cfg.FailureWindow,
				cfg.FailureThreshold,
				cfg.BackoffDuration,
			),
		}
	}

	return w
}

// Run starts the watchdog loop. It blocks until ctx is cancelled or Stop() is called.
func (w *Watchdog) Run(ctx context.Context) error {
	w.started = time.Now()
	w.logger.Info("watchdog started",
		"check_interval", w.cfg.CheckInterval,
		"distro_count", len(w.statuses),
	)

	ticker := time.NewTicker(w.cfg.CheckInterval)
	defer ticker.Stop()

	// Do an initial check immediately
	w.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("watchdog stopping: context cancelled")
			return ctx.Err()
		case <-w.stopCh:
			w.logger.Info("watchdog stopping: stop signal received")
			return nil
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

// Stop signals the watchdog to stop.
func (w *Watchdog) Stop() {
	select {
	case w.stopCh <- struct{}{}:
	default:
	}
}

// tick performs one iteration of the watch loop.
func (w *Watchdog) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("panic recovered in watch loop", "panic", fmt.Sprintf("%v", r))
		}
	}()

	w.mu.RLock()
	pauseAll := w.pauseAll
	w.mu.RUnlock()

	if pauseAll {
		w.logger.Debug("all distros paused, skipping tick")
		return
	}

	// Query all distro states in one call
	distros, err := wsl.ListDistros(w.runner, w.cfg.ProbeTimeout)
	if err != nil {
		w.logger.Error("failed to list distros", "error", err)
		return
	}

	distroMap := make(map[string]*wsl.DistroInfo)
	for i := range distros {
		distroMap[distros[i].Name] = &distros[i]
	}

	w.mu.RLock()
	statuses := make([]*DistroStatus, 0, len(w.statuses))
	for _, s := range w.statuses {
		statuses = append(statuses, s)
	}
	w.mu.RUnlock()

	for _, status := range statuses {
		if ctx.Err() != nil {
			return
		}

		dcfg := w.cfg.FindDistro(status.Name)
		if dcfg == nil || !dcfg.Enabled {
			continue
		}

		if status.Paused {
			w.logger.Debug("distro paused, skipping", "distro", status.Name)
			continue
		}

		if status.Tracker.InBackoff() {
			w.logger.Debug("distro in backoff, skipping", "distro", status.Name)
			continue
		}

		info, found := distroMap[status.Name]
		healthy := false
		if found && info.State == wsl.StateRunning {
			// Send liveness probe
			ok, err := wsl.ProbeDistro(w.runner, status.Name, w.cfg.ProbeTimeout)
			if err != nil {
				w.logger.Warn("liveness probe failed", "distro", status.Name, "error", err)
			} else if ok {
				healthy = true
			}
		}

		w.mu.Lock()
		status.Healthy = healthy
		if healthy {
			status.LastSeen = time.Now()
		}
		w.mu.Unlock()

		if !healthy {
			w.logger.Warn("distro unhealthy", "distro", status.Name,
				"found", found,
				"state", func() string {
					if info != nil {
						return string(info.State)
					}
					return "not found"
				}(),
			)

			status.Tracker.RecordFailure()

			// Check max_restarts
			if dcfg.MaxRestarts > 0 && status.RestartCount >= dcfg.MaxRestarts {
				w.logger.Warn("distro exhausted max restarts",
					"distro", status.Name,
					"max_restarts", dcfg.MaxRestarts,
					"restart_count", status.RestartCount,
				)
				w.mu.Lock()
				status.Exhausted = true
				w.mu.Unlock()
				continue
			}

			// Restart the distro
			w.restartDistro(status, dcfg)
		}
	}

	if w.OnTick != nil {
		w.OnTick()
	}
}

// restartDistro performs the restart sequence for a distro.
func (w *Watchdog) restartDistro(status *DistroStatus, dcfg *config.DistroConfig) {
	w.logger.Info("restarting distro", "distro", status.Name)

	// Step 1: Terminate
	if err := wsl.TerminateDistro(w.runner, status.Name, w.cfg.ProbeTimeout); err != nil {
		w.logger.Error("failed to terminate distro", "distro", status.Name, "error", err)
		// Continue anyway — maybe it's already stopped
	}

	// Step 2: Wait
	time.Sleep(w.cfg.RestartDelay)

	// Step 3: Start
	if err := wsl.StartDistro(w.runner, status.Name, w.cfg.ProbeTimeout); err != nil {
		w.logger.Error("failed to start distro", "distro", status.Name, "error", err)
		return
	}

	// Step 4: Run start_command if configured
	if dcfg.StartCommand != "" {
		output, err := wsl.RunInDistro(w.runner, status.Name, w.cfg.ProbeTimeout, dcfg.StartCommand)
		if err != nil {
			w.logger.Error("start_command failed", "distro", status.Name, "error", err, "output", output)
		} else {
			w.logger.Info("start_command succeeded", "distro", status.Name, "output", output)
		}
	}

	w.mu.Lock()
	status.RestartCount++
	status.StartedAt = time.Now()
	w.mu.Unlock()

	w.logger.Info("distro restarted", "distro", status.Name, "restart_count", status.RestartCount)

	if w.OnRestart != nil {
		w.OnRestart(status.Name)
	}
}

// PauseDistro pauses management of a distro.
func (w *Watchdog) PauseDistro(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, ok := w.statuses[name]
	if !ok {
		return fmt.Errorf("distro %q not found", name)
	}
	s.Paused = true
	return nil
}

// ResumeDistro resumes management of a distro.
func (w *Watchdog) ResumeDistro(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	s, ok := w.statuses[name]
	if !ok {
		return fmt.Errorf("distro %q not found", name)
	}
	s.Paused = false
	return nil
}

// PauseAll pauses all distro management.
func (w *Watchdog) PauseAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pauseAll = true
}

// ResumeAll resumes all distro management.
func (w *Watchdog) ResumeAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pauseAll = false
}

// Status returns a snapshot of all distro statuses.
func (w *Watchdog) Status() WatchdogStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	ds := make([]DistroStatusSnapshot, 0, len(w.statuses))
	for _, s := range w.statuses {
		ds = append(ds, DistroStatusSnapshot{
			Name:         s.Name,
			Healthy:      s.Healthy,
			Paused:       s.Paused,
			Exhausted:    s.Exhausted,
			RestartCount: s.RestartCount,
			StartedAt:    s.StartedAt,
			LastSeen:     s.LastSeen,
			FailureCount: s.Tracker.FailureCount(),
		})
	}

	return WatchdogStatus{
		Running:   true,
		StartedAt: w.started,
		PausedAll: w.pauseAll,
		Distros:   ds,
	}
}

// ReloadConfig replaces the watchdog config and adjusts distro statuses.
func (w *Watchdog) ReloadConfig(cfg *config.Config) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.cfg = cfg

	// Add new distros
	for _, d := range cfg.Distros {
		if !d.Enabled {
			continue
		}
		if _, ok := w.statuses[d.Name]; !ok {
			w.statuses[d.Name] = &DistroStatus{
				Name:      d.Name,
				Paused:    d.Pause,
				StartedAt: time.Now(),
				Tracker: NewFailureTracker(
					cfg.FailureWindow,
					cfg.FailureThreshold,
					cfg.BackoffDuration,
				),
			}
		}
	}
}

// WatchdogStatus is a snapshot of the full watchdog state.
type WatchdogStatus struct {
	Running   bool                  `json:"running"`
	StartedAt time.Time             `json:"started_at"`
	PausedAll bool                  `json:"paused_all"`
	Distros   []DistroStatusSnapshot `json:"distros"`
}

// DistroStatusSnapshot is a snapshot of a single distro's state.
type DistroStatusSnapshot struct {
	Name         string    `json:"name"`
	Healthy      bool      `json:"healthy"`
	Paused       bool      `json:"paused"`
	Exhausted    bool      `json:"exhausted"`
	RestartCount int       `json:"restart_count"`
	StartedAt    time.Time `json:"started_at"`
	LastSeen     time.Time `json:"last_seen"`
	FailureCount int       `json:"failure_count"`
}
