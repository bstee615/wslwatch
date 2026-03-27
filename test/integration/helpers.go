//go:build integration

package integration

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/bstee615/wslwatch/internal/wsl"
)

var targetDistro = flag.String("distro", "", "WSL distro to use for integration tests")

// requireDistro returns the target distro name for integration tests.
// If -distro is provided, it uses that; otherwise it picks the first non-ignored running distro.
func requireDistro(t *testing.T) string {
	t.Helper()

	if *targetDistro != "" {
		return *targetDistro
	}

	runner := wsl.NewWSLRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	distros, err := runner.ListDistros(ctx)
	if err != nil {
		t.Fatalf("failed to list distros: %v", err)
	}

	ignored := map[string]bool{
		"docker-desktop":      true,
		"docker-desktop-data": true,
	}

	for _, d := range distros {
		if !ignored[d.Name] && d.State != wsl.StateInstalling {
			return d.Name
		}
	}

	t.Skip("no suitable WSL distro found for integration testing")
	return ""
}

// waitForState polls until the distro reaches the desired state or the timeout expires.
func waitForState(t *testing.T, runner wsl.Runner, distroName string, desiredState wsl.DistroState, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		distros, err := runner.ListDistros(ctx)
		cancel()
		if err == nil {
			for _, d := range distros {
				if d.Name == distroName && d.State == desiredState {
					return
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("distro %q did not reach state %s within %s", distroName, desiredState, timeout)
}
