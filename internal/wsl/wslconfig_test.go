package wsl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureVMIdleTimeout_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[wsl2]\r\nvmIdleTimeout=0\r\n", string(got))
}

func TestEnsureVMIdleTimeout_ExistingSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")
	os.WriteFile(path, []byte("[wsl2]\r\nmemory=4GB\r\n"), 0644)

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[wsl2]\r\nmemory=4GB\r\nvmIdleTimeout=0\r\n", string(got))
}

func TestEnsureVMIdleTimeout_AlreadySet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")
	original := "[wsl2]\r\nvmIdleTimeout=0\r\n"
	os.WriteFile(path, []byte(original), 0644)

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(got))
}

func TestEnsureVMIdleTimeout_WrongValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")
	os.WriteFile(path, []byte("[wsl2]\r\nvmIdleTimeout=60000\r\n"), 0644)

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[wsl2]\r\nvmIdleTimeout=0\r\n", string(got))
}

func TestEnsureVMIdleTimeout_OtherSectionsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")
	os.WriteFile(path, []byte("[experimental]\r\nautoMemoryReclaim=gradual\r\n"), 0644)

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[experimental]\r\nautoMemoryReclaim=gradual\r\n\r\n[wsl2]\r\nvmIdleTimeout=0\r\n", string(got))
}

func TestEnsureVMIdleTimeout_MultipleSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".wslconfig")
	os.WriteFile(path, []byte("[wsl2]\r\nmemory=4GB\r\n[experimental]\r\nautoMemoryReclaim=gradual\r\n"), 0644)

	err := ensureVMIdleTimeoutInFile(path)
	require.NoError(t, err)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[wsl2]\r\nmemory=4GB\r\nvmIdleTimeout=0\r\n[experimental]\r\nautoMemoryReclaim=gradual\r\n", string(got))
}
