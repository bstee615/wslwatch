package wsl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseListVerbose(t *testing.T) {
	t.Run("basic case", func(t *testing.T) {
		output := "  NAME                   STATE           VERSION\n" +
			"* Ubuntu-22.04           Running         2\n" +
			"  Ubuntu-20.04           Stopped         2\n" +
			"  docker-desktop         Running         2\n" +
			"  docker-desktop-data    Stopped         2\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 4)

		assert.Equal(t, "Ubuntu-22.04", distros[0].Name)
		assert.Equal(t, StateRunning, distros[0].State)
		assert.True(t, distros[0].Default)
		assert.Equal(t, 2, distros[0].Version)

		assert.Equal(t, "Ubuntu-20.04", distros[1].Name)
		assert.Equal(t, StateStopped, distros[1].State)
		assert.False(t, distros[1].Default)
		assert.Equal(t, 2, distros[1].Version)

		assert.Equal(t, "docker-desktop", distros[2].Name)
		assert.Equal(t, StateRunning, distros[2].State)
		assert.False(t, distros[2].Default)
		assert.Equal(t, 2, distros[2].Version)

		assert.Equal(t, "docker-desktop-data", distros[3].Name)
		assert.Equal(t, StateStopped, distros[3].State)
		assert.False(t, distros[3].Default)
		assert.Equal(t, 2, distros[3].Version)
	})

	t.Run("default marker", func(t *testing.T) {
		output := "  NAME          STATE     VERSION\n" +
			"  distro-a      Stopped   1\n" +
			"* distro-b      Running   2\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 2)

		assert.False(t, distros[0].Default)
		assert.True(t, distros[1].Default)
		assert.Equal(t, "distro-b", distros[1].Name)
	})

	t.Run("Installing state", func(t *testing.T) {
		output := "  NAME       STATE        VERSION\n" +
			"  new-distro Installing   2\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 1)
		assert.Equal(t, StateInstalling, distros[0].State)
	})

	t.Run("unknown state", func(t *testing.T) {
		output := "  NAME      STATE    VERSION\n" +
			"  my-distro Pending  2\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 1)
		assert.Equal(t, StateUnknown, distros[0].State)
	})

	t.Run("empty output", func(t *testing.T) {
		output := "  NAME    STATE   VERSION\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Empty(t, distros)
	})

	t.Run("completely empty output", func(t *testing.T) {
		distros, err := ParseListVerbose("")
		assert.NoError(t, err)
		assert.Empty(t, distros)
	})

	t.Run("unicode names", func(t *testing.T) {
		output := "  NAME              STATE     VERSION\n" +
			"  Ubuntu-日本語      Running   2\n" +
			"  Debian-中文        Stopped   1\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 2)
		assert.Equal(t, "Ubuntu-日本語", distros[0].Name)
		assert.Equal(t, StateRunning, distros[0].State)
		assert.Equal(t, "Debian-中文", distros[1].Name)
		assert.Equal(t, StateStopped, distros[1].State)
	})

	t.Run("windows CRLF line endings", func(t *testing.T) {
		output := "  NAME          STATE     VERSION\r\n" +
			"* Ubuntu-22.04   Running   2\r\n" +
			"  Ubuntu-20.04   Stopped   2\r\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 2)
		assert.Equal(t, "Ubuntu-22.04", distros[0].Name)
		assert.True(t, distros[0].Default)
	})

	t.Run("WSL version 1", func(t *testing.T) {
		output := "  NAME      STATE     VERSION\n" +
			"  my-distro Running   1\n"

		distros, err := ParseListVerbose(output)
		assert.NoError(t, err)
		assert.Len(t, distros, 1)
		assert.Equal(t, 1, distros[0].Version)
	})
}

func TestParseListQuiet(t *testing.T) {
	t.Run("basic case", func(t *testing.T) {
		output := "Ubuntu-22.04\nUbuntu-20.04\ndocker-desktop\n"

		names, err := ParseListQuiet(output)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Ubuntu-22.04", "Ubuntu-20.04", "docker-desktop"}, names)
	})

	t.Run("blank lines at start and between entries", func(t *testing.T) {
		output := "\nUbuntu-22.04\n\nUbuntu-20.04\n"

		names, err := ParseListQuiet(output)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Ubuntu-22.04", "Ubuntu-20.04"}, names)
	})

	t.Run("single entry", func(t *testing.T) {
		output := "Ubuntu-22.04\n"

		names, err := ParseListQuiet(output)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Ubuntu-22.04"}, names)
	})

	t.Run("empty output", func(t *testing.T) {
		names, err := ParseListQuiet("")
		assert.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("windows CRLF line endings", func(t *testing.T) {
		output := "Ubuntu-22.04\r\nUbuntu-20.04\r\n"

		names, err := ParseListQuiet(output)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Ubuntu-22.04", "Ubuntu-20.04"}, names)
	})

	t.Run("blank line at top (wsl --list --quiet quirk)", func(t *testing.T) {
		output := "\nUbuntu-22.04\nUbuntu-20.04\n"

		names, err := ParseListQuiet(output)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Ubuntu-22.04", "Ubuntu-20.04"}, names)
	})
}
