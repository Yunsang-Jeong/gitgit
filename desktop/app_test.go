package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	desktopcore "github.com/yunsang/gitgit/internal/desktop"
)

func TestDesktopAppDiscoversProjectsOnlyUnderExplicitDirectory(t *testing.T) {
	root := t.TempDir()
	discoveryDirectory := filepath.Join(root, "discover-here")
	insideRepository := filepath.Join(discoveryDirectory, "nested-repository")
	outsideRepository := filepath.Join(root, "outside-repository")
	for _, repository := range []string{insideRepository, outsideRepository} {
		if err := os.MkdirAll(repository, 0o755); err != nil {
			t.Fatalf("create %s: %v", repository, err)
		}
		command := exec.Command("git", "init", "-q", "-b", "main")
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", repository, err, output)
		}
	}
	canonicalInsideRepository, err := filepath.EvalSymlinks(insideRepository)
	if err != nil {
		t.Fatalf("resolve %s: %v", insideRepository, err)
	}

	app := &DesktopApp{
		context:  context.Background(),
		projects: desktopcore.NewProjectStore(filepath.Join(root, "projects.json")),
	}
	result, err := app.DiscoverProjects(discoveryDirectory)
	if err != nil {
		t.Fatalf("DiscoverProjects(%q): %v", discoveryDirectory, err)
	}
	if result.Directory != discoveryDirectory || result.Found != 1 || result.Added != 1 || len(result.Projects) != 1 || result.Projects[0].Root != canonicalInsideRepository {
		t.Fatalf("DiscoverProjects(%q) = %+v, want only %q", discoveryDirectory, result, canonicalInsideRepository)
	}
	if _, err := app.DiscoverProjects(" "); err == nil {
		t.Fatal("DiscoverProjects with an empty directory unexpectedly succeeded")
	}
}

func TestDesktopAppDefersPersistenceUntilStartup(t *testing.T) {
	constructed := NewDesktopApp()
	if constructed.service != nil || constructed.projects != nil || constructed.openProjects == nil || constructed.openCache == nil {
		t.Fatal("NewDesktopApp must configure deferred persistence without opening it")
	}

	projectStoreOpens := 0
	cacheOpens := 0
	app := &DesktopApp{
		openProjects: func() (*desktopcore.ProjectStore, error) {
			projectStoreOpens++
			return desktopcore.NewProjectStore(filepath.Join(t.TempDir(), "projects.json")), nil
		},
		openCache: func() (*desktopcore.PersistentCache, error) {
			cacheOpens++
			return desktopcore.OpenPersistentCache(filepath.Join(t.TempDir(), "cache"))
		},
	}

	if projectStoreOpens != 0 || cacheOpens != 0 || app.service != nil || app.projects != nil {
		t.Fatal("constructing DesktopApp must not open user persistence")
	}

	app.startup(context.Background())
	t.Cleanup(func() { app.shutdown(context.Background()) })
	if projectStoreOpens != 1 || cacheOpens != 1 || app.service == nil || app.projects == nil {
		t.Fatalf("startup initialization = projects:%d cache:%d service:%t store:%t", projectStoreOpens, cacheOpens, app.service != nil, app.projects != nil)
	}

	app.startup(context.Background())
	if projectStoreOpens != 1 || cacheOpens != 1 {
		t.Fatalf("startup initialized persistence more than once: projects:%d cache:%d", projectStoreOpens, cacheOpens)
	}
}

func TestIDECommand(t *testing.T) {
	tests := []struct {
		ide         string
		application string
		command     string
	}{
		{ide: "vscode", application: "Visual Studio Code", command: "code"},
		{ide: "cursor", application: "Cursor", command: "cursor"},
		{ide: "zed", application: "Zed", command: "zed"},
		{ide: "idea", application: "IntelliJ IDEA", command: "idea"},
		{ide: "xcode", application: "Xcode", command: "xed"},
	}
	for _, test := range tests {
		application, command, err := ideCommand(test.ide)
		if err != nil || application != test.application || command != test.command {
			t.Fatalf("ideCommand(%q) = %q, %q, %v", test.ide, application, command, err)
		}
	}
	if _, _, err := ideCommand("unknown"); err == nil {
		t.Fatal("unknown IDE should fail")
	}
}

func TestIDEOpenPathsStartWithProjectRootAndSelectFile(t *testing.T) {
	root := "/repo"
	file := "/repo/internal/search.go"
	paths := ideOpenPaths(root, file)
	if len(paths) != 2 || paths[0] != root || paths[1] != file {
		t.Fatalf("ideOpenPaths() = %v", paths)
	}
	if paths := ideOpenPaths(root, root); len(paths) != 1 || paths[0] != root {
		t.Fatalf("ideOpenPaths(root, root) = %v", paths)
	}
}

func TestTerminalApplication(t *testing.T) {
	tests := map[string]string{
		"terminal": "Terminal",
		"iterm2":   "iTerm",
		"warp":     "Warp",
		"ghostty":  "Ghostty",
		"wezterm":  "WezTerm",
	}
	for preference, want := range tests {
		got, err := terminalApplication(preference)
		if err != nil || got != want {
			t.Fatalf("terminalApplication(%q) = %q, %v", preference, got, err)
		}
	}
	if _, err := terminalApplication("unknown"); err == nil {
		t.Fatal("unknown Terminal application should fail")
	}
}

func TestValidateExternalURLAllowsOnlyAbsoluteHTTPLinks(t *testing.T) {
	got, err := validateExternalURL(" https://github.com/acme/repo/pull/12 ")
	if err != nil || got != "https://github.com/acme/repo/pull/12" {
		t.Fatalf("validateExternalURL() = %q, %v", got, err)
	}
	for _, value := range []string{"file:///tmp/repo", "javascript:alert(1)", "/relative/path", "https:///missing-host"} {
		if _, err := validateExternalURL(value); err == nil {
			t.Fatalf("validateExternalURL(%q) unexpectedly succeeded", value)
		}
	}
}

func TestBrowserDevelopmentModeRequiresExplicitEnvironmentValue(t *testing.T) {
	if !browserDevelopmentMode("1") {
		t.Fatal("GITGIT_BROWSER_DEV=1 should keep the native development window hidden")
	}
	for _, value := range []string{"", "0", "true", "browser"} {
		if browserDevelopmentMode(value) {
			t.Fatalf("browserDevelopmentMode(%q) unexpectedly enabled", value)
		}
	}
}
