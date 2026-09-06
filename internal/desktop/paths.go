package desktop

import (
	"errors"
	"os"
	"path/filepath"
)

// GitGit follows the XDG base directory convention rather than the macOS
// Application Support and Caches layout, so its state sits beside the other
// developer tools a user already keeps in ~/.config and ~/.cache.
//
// Config and cache stay in separate roots on purpose. The cache is a Pebble
// database that grows with the repositories being browsed, and burying it
// under the config directory would sweep it into every settings backup and
// dotfile sync.
const applicationDirectoryName = "gitgit"

func ConfigDirectory() (string, error) {
	return xdgDirectory("XDG_CONFIG_HOME", ".config")
}

func CacheDirectory() (string, error) {
	return xdgDirectory("XDG_CACHE_HOME", ".cache")
}

// The specification requires an absolute path and says a relative one must be
// ignored, which is what keeps a stray export from scattering state into the
// working directory.
func xdgDirectory(variable, fallback string) (string, error) {
	if base := os.Getenv(variable); filepath.IsAbs(base) {
		return filepath.Join(filepath.Clean(base), applicationDirectoryName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("locate the home directory for " + variable)
	}
	return filepath.Join(home, fallback, applicationDirectoryName), nil
}
