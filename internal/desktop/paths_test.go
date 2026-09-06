package desktop

import (
	"path/filepath"
	"testing"
)

func TestXDGDirectoriesPreferTheEnvironmentAndStayApart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	config, err := ConfigDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "gitgit"); config != want {
		t.Fatalf("config directory = %q, want %q", config, want)
	}
	cache, err := CacheDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".cache", "gitgit"); cache != want {
		t.Fatalf("cache directory = %q, want %q", cache, want)
	}
	// The cache is a growing database; it must not sit inside the directory
	// people back up and sync as their settings.
	if pathContains(config, cache) {
		t.Fatalf("the cache directory %q is nested inside the config directory %q", cache, config)
	}

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "elsewhere"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "scratch"))
	config, err = ConfigDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "elsewhere", "gitgit"); config != want {
		t.Fatalf("config directory = %q, want %q", config, want)
	}
	cache, err = CacheDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "scratch", "gitgit"); cache != want {
		t.Fatalf("cache directory = %q, want %q", cache, want)
	}
}

// The specification says a relative XDG value must be ignored, which is what
// stops a stray export from scattering state into the working directory.
func TestXDGDirectoriesIgnoreRelativeOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("XDG_CACHE_HOME", "relative/cache")

	config, err := ConfigDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "gitgit"); config != want {
		t.Fatalf("config directory = %q, want %q", config, want)
	}
	cache, err := CacheDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".cache", "gitgit"); cache != want {
		t.Fatalf("cache directory = %q, want %q", cache, want)
	}
}
