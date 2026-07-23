package desktop

import (
	"context"
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

func TestProjectStoreRemovesRegisteredAndMissingProjects(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, directory := range []string{first, second} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := NewProjectStore(filepath.Join(root, "config", "projects.json"))
	if _, added, err := store.AddMany([]string{first, second}); err != nil || added != 2 {
		t.Fatalf("register projects = added:%d err:%v", added, err)
	}
	registered, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	missingRoot := registered[0].Root
	remainingRoot := registered[1].Root
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}
	projects, err := store.Remove(missingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Root != remainingRoot {
		t.Fatalf("remaining projects = %#v, want only %q", projects, remainingRoot)
	}
	if _, err := store.Remove(missingRoot); err == nil {
		t.Fatal("expected removing an unregistered project to fail")
	}
	reloaded, err := NewProjectStore(filepath.Join(root, "config", "projects.json")).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 1 || reloaded[0].Root != remainingRoot {
		t.Fatalf("reloaded projects = %#v, want only %q", reloaded, remainingRoot)
	}
}

func TestProjectStorePrunesUnavailableProjects(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid")
	missing := filepath.Join(root, "missing")
	notGit := filepath.Join(root, "not-git")
	for _, directory := range []string{valid, missing, notGit} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, valid, nil, "init", "-q", "-b", "main")

	store := NewProjectStore(filepath.Join(root, "config", "projects.json"))
	registered, added, err := store.AddMany([]string{valid, missing, notGit})
	if err != nil || added != 3 {
		t.Fatalf("register projects = added:%d err:%v", added, err)
	}
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}

	projects, removed, err := store.PruneUnavailable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Root != registered[0].Root {
		t.Fatalf("retained projects = %#v, want only %#v", projects, registered[0])
	}
	if len(removed) != 2 {
		t.Fatalf("removed projects = %#v, want missing and non-Git projects", removed)
	}
	removedRoots := map[string]bool{}
	for _, project := range removed {
		removedRoots[project.Root] = true
	}
	if !removedRoots[registered[1].Root] || !removedRoots[registered[2].Root] {
		t.Fatalf("removed projects = %#v, want %#v and %#v", removed, registered[1], registered[2])
	}

	reloaded, err := NewProjectStore(filepath.Join(root, "config", "projects.json")).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 1 || reloaded[0] != projects[0] {
		t.Fatalf("reloaded projects = %#v, want %#v", reloaded, projects)
	}
}
