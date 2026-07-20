package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectStorePersistsUniqueCanonicalRoots(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "shared-name")
	second := filepath.Join(root, "second", "shared-name")
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "first-alias")
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "config", "projects.json")
	store := NewProjectStore(path)
	if _, err := store.Add(first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(alias); err != nil {
		t.Fatal(err)
	}
	projects, err := store.Add(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("registered projects = %#v", projects)
	}
	if projects[0].Name != "shared-name" || projects[1].Name != "shared-name" || projects[0].Root == projects[1].Root {
		t.Fatalf("unexpected registered projects: %#v", projects)
	}
	projects, err = store.SetFavorite(alias, true)
	if err != nil {
		t.Fatal(err)
	}
	if !projects[0].Favorite || projects[1].Favorite {
		t.Fatalf("unexpected favorite state: %#v", projects)
	}
	projects, err = store.SetFavorite(first, false)
	if err != nil {
		t.Fatal(err)
	}
	if projects[0].Favorite {
		t.Fatalf("favorite was not cleared: %#v", projects)
	}
	projects, err = store.SetFavorite(first, true)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := NewProjectStore(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 2 || reloaded[0] != projects[0] || reloaded[1] != projects[1] {
		t.Fatalf("reloaded projects = %#v, want %#v", reloaded, projects)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("project store mode = %o", info.Mode().Perm())
	}
}

func TestProjectStoreRejectsInvalidDataAndMissingRoots(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProjectStore(path).List(); err == nil {
		t.Fatal("expected invalid project store error")
	}
	if _, err := NewProjectStore(filepath.Join(root, "clean.json")).Add(filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected missing project root error")
	}
	registered := filepath.Join(root, "registered")
	if err := os.Mkdir(registered, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewProjectStore(filepath.Join(root, "favorites.json"))
	if _, err := store.SetFavorite(registered, true); err == nil {
		t.Fatal("expected unregistered project error")
	}
}

func TestProjectStoreAddsManyProjectsOnce(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, directory := range []string{first, second} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := NewProjectStore(filepath.Join(root, "config", "projects.json"))
	projects, added, err := store.AddMany([]string{second, first, first})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 || len(projects) != 2 {
		t.Fatalf("added/projects = %d/%#v, want 2 projects", added, projects)
	}
	projects, added, err = store.AddMany([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 || len(projects) != 2 {
		t.Fatalf("second add added/projects = %d/%#v, want 0/2", added, projects)
	}
}
