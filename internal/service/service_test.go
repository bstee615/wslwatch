package service_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bstee615/wslwatch/internal/config"
	"github.com/bstee615/wslwatch/internal/service"
)

func TestInstallReturnsErrorOnNonWindows(t *testing.T) {
	err := service.Install("", false)
	assert.Error(t, err)
}

func TestUninstallReturnsErrorOnNonWindows(t *testing.T) {
	err := service.Uninstall(false)
	assert.Error(t, err)
}

func TestRunServiceReturnsErrorOnNonWindows(t *testing.T) {
	cfg := config.Default()
	logger := slog.Default()
	err := service.RunService(cfg, logger)
	assert.Error(t, err)
}
