package wsl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseListVerbose_Normal(t *testing.T) {
	output := `  NAME                   STATE           VERSION
* Ubuntu-22.04           Running         2
  Ubuntu-20.04           Stopped         2
  docker-desktop         Running         2
  docker-desktop-data    Stopped         2`

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 4)

	assert.Equal(t, "Ubuntu-22.04", distros[0].Name)
	assert.Equal(t, StateRunning, distros[0].State)
	assert.True(t, distros[0].IsDefault)
	assert.Equal(t, 2, distros[0].Version)

	assert.Equal(t, "Ubuntu-20.04", distros[1].Name)
	assert.Equal(t, StateStopped, distros[1].State)
	assert.False(t, distros[1].IsDefault)

	assert.Equal(t, "docker-desktop", distros[2].Name)
	assert.Equal(t, StateRunning, distros[2].State)

	assert.Equal(t, "docker-desktop-data", distros[3].Name)
	assert.Equal(t, StateStopped, distros[3].State)
}

func TestParseListVerbose_InstallingState(t *testing.T) {
	output := `  NAME              STATE           VERSION
  MyDistro          Installing      2`

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 1)
	assert.Equal(t, "MyDistro", distros[0].Name)
	assert.Equal(t, StateInstalling, distros[0].State)
}

func TestParseListVerbose_EmptyOutput(t *testing.T) {
	distros := ParseListVerbose("")
	assert.Empty(t, distros)
}

func TestParseListVerbose_HeaderOnly(t *testing.T) {
	output := `  NAME                   STATE           VERSION`
	distros := ParseListVerbose(output)
	assert.Empty(t, distros)
}

func TestParseListVerbose_UnknownState(t *testing.T) {
	output := `  NAME              STATE           VERSION
  Weird             SomeState       2`

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 1)
	assert.Equal(t, StateUnknown, distros[0].State)
}

func TestParseListVerbose_WithBOM(t *testing.T) {
	// Simulate BOM that wsl.exe sometimes outputs
	output := "\xef\xbb\xbf  NAME                   STATE           VERSION\n* Ubuntu           Running         2"

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 1)
	assert.Equal(t, "Ubuntu", distros[0].Name)
	assert.Equal(t, StateRunning, distros[0].State)
	assert.True(t, distros[0].IsDefault)
}

func TestParseListVerbose_WithNullBytes(t *testing.T) {
	output := " \x00 N\x00A\x00M\x00E\x00 \x00 S\x00T\x00A\x00T\x00E\x00\n\x00* Ubuntu Running 2"

	distros := ParseListVerbose(output)
	// Should handle null bytes gracefully
	assert.NotEmpty(t, distros)
}

func TestParseListVerbose_WSL1(t *testing.T) {
	output := `  NAME              STATE           VERSION
  Legacy            Running         1`

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 1)
	assert.Equal(t, 1, distros[0].Version)
}

func TestParseListVerbose_Unicode(t *testing.T) {
	output := `  NAME              STATE           VERSION
  Ubuntu-日本語       Running         2`

	distros := ParseListVerbose(output)
	assert.Len(t, distros, 1)
	assert.Equal(t, "Ubuntu-日本語", distros[0].Name)
}

func TestParseListQuiet(t *testing.T) {
	output := "Ubuntu-22.04\nUbuntu-20.04\ndocker-desktop"
	names := parseListQuiet(output)
	assert.Equal(t, []string{"Ubuntu-22.04", "Ubuntu-20.04", "docker-desktop"}, names)
}

func TestParseListQuiet_Empty(t *testing.T) {
	names := parseListQuiet("")
	assert.Empty(t, names)
}

func TestIsDockerDistro(t *testing.T) {
	assert.True(t, IsDockerDistro("docker-desktop"))
	assert.True(t, IsDockerDistro("Docker-Desktop"))
	assert.True(t, IsDockerDistro("docker-desktop-data"))
	assert.False(t, IsDockerDistro("Ubuntu-22.04"))
}

func TestIsInstallingDistro(t *testing.T) {
	assert.True(t, IsInstallingDistro(DistroInfo{State: StateInstalling}))
	assert.False(t, IsInstallingDistro(DistroInfo{State: StateRunning}))
	assert.False(t, IsInstallingDistro(DistroInfo{State: StateStopped}))
}
