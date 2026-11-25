package bolt

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nextdhcp/nextdhcp/core/lease/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func open(t *testing.T, options map[string][]string) (storage.LeaseStorage, error) {
	t.Helper()

	store, err := storageFactory(options)
	if err == nil {
		t.Cleanup(func() { assert.NoError(t, store.(*Storage).Close()) })
	}

	return store, err
}

func TestStorageFactoryFile(t *testing.T) {
	dir := t.TempDir()

	_, err := open(t, map[string][]string{"__args__": {filepath.Join(dir, "args.db")}})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "args.db"))

	_, err = open(t, map[string][]string{"file": {filepath.Join(dir, "file.db")}})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "file.db"))
}

func TestStorageFactoryMode(t *testing.T) {
	dir := t.TempDir()

	for _, mode := range []string{"0600", "600", "0o600"} {
		file := filepath.Join(dir, mode+".db")
		_, err := open(t, map[string][]string{"file": {file}, "mode": {mode}})
		require.NoError(t, err, mode)

		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), mode)
	}
}

// TestStorageFactoryLocked checks that opening a database file that is
// already open fails after the timeout instead of waiting forever.
func TestStorageFactoryLocked(t *testing.T) {
	file := filepath.Join(t.TempDir(), "leases.db")

	_, err := open(t, map[string][]string{"file": {file}})
	require.NoError(t, err)

	start := time.Now()
	_, err = open(t, map[string][]string{"file": {file}, "timeout": {"100ms"}})
	assert.ErrorContains(t, err, "locked")
	assert.Less(t, time.Since(start), 5*time.Second)

	// without the timeout option, the default timeout applies
	orig := defaultTimeout
	defaultTimeout = 100 * time.Millisecond
	defer func() { defaultTimeout = orig }()

	_, err = open(t, map[string][]string{"file": {file}})
	assert.ErrorContains(t, err, "locked")
}

func TestStorageFactoryInvalidOptions(t *testing.T) {
	file := filepath.Join(t.TempDir(), "leases.db")

	for name, options := range map[string]map[string][]string{
		"no file":             {},
		"file without value":  {"file": {}},
		"two files":           {"file": {file, file}},
		"two file options":    {"__args__": {file}, "file": {file}},
		"two positional args": {"__args__": {file, "other"}},
		"invalid timeout":     {"file": {file}, "timeout": {"forever"}},
		"negative timeout":    {"file": {file}, "timeout": {"-1s"}},
		"two timeouts":        {"file": {file}, "timeout": {"1s", "2s"}},
		"invalid mode":        {"file": {file}, "mode": {"rw"}},
		"mode out of range":   {"file": {file}, "mode": {"1777"}},
		"hexadecimal mode":    {"file": {file}, "mode": {"0x1ff"}},
		"unknown option":      {"file": {file}, "permissions": {"0600"}},
	} {
		_, err := open(t, options)
		assert.Error(t, err, name)
	}

	assert.NoFileExists(t, file)
}
