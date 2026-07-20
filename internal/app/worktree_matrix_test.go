package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yunsang/gitgit/internal/apperr"
	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestWorktreeRandomizedLifecycle(t *testing.T) {
	t.Parallel()
	rng := deterministicRand(t, 202)
	root := newTestRepository(t)
	directories := make([]string, 0, 12)
	for range 12 {
		directory := randomWord(rng, "dir")
		directories = append(directories, directory)
		writeTestFile(t, root, filepath.Join(directory, "content.txt"), directory+"\n")
	}
	writeTestFile(t, root, "root.txt", "root\n")
	gitCommitAs(t, root, "Fixture User", "fixture@example.com", "2025-01-01T00:00:00Z", "seed randomized directories")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)
	linked := filepath.Join(t.TempDir(), "linked "+randomWord(rng, "tree"))
	moved := filepath.Join(filepath.Dir(linked), "moved "+randomWord(rng, "tree"))
	added, err := service.Add(context.Background(), AddWorktreeOptions{Path: linked, Detach: true, Sync: "ff-only"})
	if err != nil {
		t.Fatal(err)
	}
	if added.Path != linked || len(added.Warnings) != 1 || !strings.Contains(added.Warnings[0], "no remotes configured") {
		t.Fatalf("unexpected add result: %#v", added)
	}

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resolvedLinked, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || !items[1].Detached || items[1].Path != resolvedLinked {
		t.Fatalf("unexpected worktree list: %#v", items)
	}

	mutation, err := service.Move(context.Background(), linked, moved)
	if err != nil {
		t.Fatal(err)
	}
	if mutation.Action != "moved" || mutation.Path != moved {
		t.Fatalf("unexpected move result: %#v", mutation)
	}
	if _, err := os.Stat(linked); !os.IsNotExist(err) {
		t.Fatalf("old path still exists after move: %v", err)
	}

	selected := slices.Clone(directories[:4])
	state, err := service.SparseSet(context.Background(), moved, selected)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(selected)
	if !state.Enabled || !state.Cone || !slices.Equal(state.Directories, selected) {
		t.Fatalf("unexpected sparse set state: %#v", state)
	}
	listed, err := service.SparseList(context.Background(), moved)
	if err != nil || !slices.Equal(listed.Directories, selected) {
		t.Fatalf("sparse list = %#v, %v", listed, err)
	}
	if _, err := os.Stat(filepath.Join(moved, directories[0], "content.txt")); err != nil {
		t.Fatalf("selected directory was not materialized: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, directories[8], "content.txt")); !os.IsNotExist(err) {
		t.Fatalf("unselected directory is unexpectedly materialized: %v", err)
	}

	state, err = service.SparseExpand(context.Background(), moved, directories[4:7])
	if err != nil {
		t.Fatal(err)
	}
	selected = append(selected, directories[4:7]...)
	slices.Sort(selected)
	if !slices.Equal(state.Directories, selected) {
		t.Fatalf("expanded directories = %v, want %v", state.Directories, selected)
	}
	state, err = service.SparseContract(context.Background(), moved, directories[1:3])
	if err != nil {
		t.Fatal(err)
	}
	selected = removeStrings(selected, directories[1:3]...)
	if !slices.Equal(state.Directories, selected) {
		t.Fatalf("contracted directories = %v, want %v", state.Directories, selected)
	}

	disabled, err := service.SparseDisable(context.Background(), moved)
	if err != nil || disabled.Enabled {
		t.Fatalf("disable state = %#v, %v", disabled, err)
	}
	if _, err := os.Stat(filepath.Join(moved, directories[8], "content.txt")); err != nil {
		t.Fatalf("disable did not materialize all files: %v", err)
	}

	if _, err := service.Remove(context.Background(), moved, false); appErrorCode(err) != "confirmation_required" {
		t.Fatalf("remove without confirmation error = %v", err)
	}
	removed, err := service.Remove(context.Background(), moved, true)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Action != "removed" || removed.Path != moved {
		t.Fatalf("unexpected remove result: %#v", removed)
	}
	items, err = service.List(context.Background())
	if err != nil || len(items) != 1 || items[0].Path != repo.Root {
		t.Fatalf("final worktree list = %#v, %v", items, err)
	}
}

func TestSparseRandomizedStateMachine(t *testing.T) {
	t.Parallel()
	rng := deterministicRand(t, 303)
	root := newTestRepository(t)
	directories := make([]string, 0, 18)
	for range 18 {
		directory := randomWord(rng, "component")
		directories = append(directories, directory)
		writeTestFile(t, root, filepath.Join(directory, "tracked.txt"), directory+"\n")
	}
	gitCommitAs(t, root, "State User", "state@example.com", "2025-02-01T00:00:00Z", "seed sparse state machine")
	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)
	model := map[string]bool{}

	for step := range 36 {
		var state SparseState
		if len(model) == 0 || rng.Intn(2) == 0 {
			candidate := directories[rng.Intn(len(directories))]
			state, err = service.SparseExpand(context.Background(), root, []string{candidate})
			model[candidate] = true
		} else {
			present := mapKeys(model)
			candidate := present[rng.Intn(len(present))]
			state, err = service.SparseContract(context.Background(), root, []string{candidate})
			delete(model, candidate)
		}
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		want := mapKeys(model)
		if !state.Enabled || !state.Cone || !slices.Equal(state.Directories, want) {
			t.Fatalf("step %d state = %#v, want %v", step, state, want)
		}
		for _, directory := range directories {
			_, statErr := os.Stat(filepath.Join(root, directory, "tracked.txt"))
			if model[directory] && statErr != nil {
				t.Fatalf("step %d selected %s is missing: %v", step, directory, statErr)
			}
			if !model[directory] && !os.IsNotExist(statErr) {
				t.Fatalf("step %d unselected %s is materialized: %v", step, directory, statErr)
			}
		}
	}
}

func TestWorktreeValidationMatrix(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "a/a.txt", "a\n")
	gitCommitAs(t, root, "Test User", "test@example.com", "2025-01-01T00:00:00Z", "initial")
	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorktreeService(repo)

	tests := []struct {
		name string
		run  func() error
		code string
	}{
		{name: "missing add path", run: func() error { _, err := service.Add(context.Background(), AddWorktreeOptions{}); return err }, code: "missing_worktree"},
		{name: "branch and detach", run: func() error {
			_, err := service.Add(context.Background(), AddWorktreeOptions{Path: "unused", NewBranch: "branch", Detach: true})
			return err
		}, code: "invalid_arguments"},
		{name: "invalid sync", run: func() error {
			_, err := service.Add(context.Background(), AddWorktreeOptions{Path: "unused", Sync: "merge"})
			return err
		}, code: "invalid_sync"},
		{name: "missing move source", run: func() error { _, err := service.Move(context.Background(), "", "destination"); return err }, code: "missing_worktree_path"},
		{name: "missing remove path", run: func() error { _, err := service.Remove(context.Background(), "", true); return err }, code: "missing_worktree"},
		{name: "remove confirmation", run: func() error { _, err := service.Remove(context.Background(), "unused", false); return err }, code: "confirmation_required"},
		{name: "expand without directories", run: func() error { _, err := service.SparseExpand(context.Background(), root, nil); return err }, code: "missing_directories"},
		{name: "contract while disabled", run: func() error { _, err := service.SparseContract(context.Background(), root, []string{"a"}); return err }, code: "sparse_disabled"},
		{name: "absolute sparse directory", run: func() error {
			_, err := service.SparseSet(context.Background(), root, []string{"/tmp/outside"})
			return err
		}, code: "invalid_sparse_directory"},
		{name: "parent sparse directory", run: func() error {
			_, err := service.SparseSet(context.Background(), root, []string{"../outside"})
			return err
		}, code: "invalid_sparse_directory"},
		{name: "newline sparse directory", run: func() error { _, err := service.SparseSet(context.Background(), root, []string{"a\nb"}); return err }, code: "invalid_sparse_directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := appErrorCode(test.run()); got != test.code {
				t.Fatalf("error code = %q, want %q", got, test.code)
			}
		})
	}
}

func TestParseWorktreeListVariants(t *testing.T) {
	data := []byte(strings.Join([]string{
		"worktree /tmp/main", "HEAD 111111", "branch refs/heads/main", "",
		"worktree /tmp/feature with spaces", "HEAD 222222", "detached", "locked maintenance", "prunable stale", "",
		"worktree /tmp/bare", "HEAD 333333", "bare", "",
	}, "\x00"))
	items, err := parseWorktreeList(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("item count = %d, want 3: %#v", len(items), items)
	}
	if items[0].Branch != "main" || items[0].Detached || items[0].Bare {
		t.Fatalf("unexpected branch item: %#v", items[0])
	}
	if items[1].Path != "/tmp/feature with spaces" || !items[1].Detached || !items[1].Locked || !items[1].Prunable {
		t.Fatalf("unexpected detached item: %#v", items[1])
	}
	if !items[2].Bare {
		t.Fatalf("unexpected bare item: %#v", items[2])
	}
	if _, err := parseWorktreeList([]byte("HEAD orphan\x00")); err == nil {
		t.Fatal("attribute before worktree unexpectedly parsed")
	}
}

func TestParsePorcelainPathsAndSparseNormalization(t *testing.T) {
	porcelain := []byte(" M modified.txt\x00?? untracked file.txt\x00R  renamed.txt\x00old.txt\x00C  copied.txt\x00source.txt\x00")
	wantPaths := []string{"modified.txt", "untracked file.txt", "renamed.txt", "old.txt", "copied.txt", "source.txt"}
	if got := parsePorcelainPaths(porcelain); !slices.Equal(got, wantPaths) {
		t.Fatalf("porcelain paths = %v, want %v", got, wantPaths)
	}

	directories, err := normalizeSparseDirectories([]string{" z ", "a/../b", "./a", "a", "", ".", "z"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b", "z"}; !slices.Equal(directories, want) {
		t.Fatalf("normalized directories = %v, want %v", directories, want)
	}
}

func FuzzParseWorktreeList(f *testing.F) {
	f.Add([]byte("worktree /tmp/main\x00HEAD abc\x00branch refs/heads/main\x00\x00"))
	f.Add([]byte("HEAD orphan\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseWorktreeList(data)
	})
}

func FuzzParsePorcelainPaths(f *testing.F) {
	f.Add([]byte(" M file.txt\x00"))
	f.Add([]byte("R  new.txt\x00old.txt\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = parsePorcelainPaths(data)
	})
}

func FuzzNormalizeSparseDirectories(f *testing.F) {
	f.Add("a,b,c")
	f.Add("../outside")
	f.Add("a/../b, a ,b")
	f.Fuzz(func(t *testing.T, value string) {
		directories, err := normalizeSparseDirectories(strings.Split(value, ","))
		if err != nil {
			return
		}
		if !slices.IsSorted(directories) {
			t.Fatalf("directories are not sorted: %v", directories)
		}
		seen := map[string]bool{}
		for _, directory := range directories {
			if directory == "" || directory == "." || filepath.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, "../") {
				t.Fatalf("invalid normalized directory: %q", directory)
			}
			if seen[directory] {
				t.Fatalf("duplicate normalized directory: %q", directory)
			}
			seen[directory] = true
		}
	})
}

func appErrorCode(err error) string {
	if err == nil {
		return ""
	}
	code, _, _ := apperr.Details(err)
	return code
}

func removeStrings(values []string, removals ...string) []string {
	remove := map[string]bool{}
	for _, value := range removals {
		remove[value] = true
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !remove[value] {
			result = append(result, value)
		}
	}
	return result
}

func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func TestParseWorktreeListRandomizedPaths(t *testing.T) {
	rng := deterministicRand(t, 404)
	var input bytes.Buffer
	want := make([]string, 0, 40)
	for index := range 40 {
		path := fmt.Sprintf("/tmp/%02d %s", index, randomWord(rng, "worktree"))
		want = append(want, path)
		fmt.Fprintf(&input, "worktree %s\x00HEAD %040d\x00detached\x00\x00", path, index)
	}
	items, err := parseWorktreeList(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Path)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("parsed paths differ\ngot:  %v\nwant: %v", got, want)
	}
}
