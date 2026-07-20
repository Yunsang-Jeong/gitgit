package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestSearchFilenameDiffAndFollowRename(t *testing.T) {
	t.Parallel()
	root := newTestRepository(t)
	writeTestFile(t, root, "internal/old.go", "package internal\n\nconst Value = \"old\"\n")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "commit", "-m", "add old implementation")

	writeTestFile(t, root, "internal/old.go", "package internal\n\nconst Value = \"needle\"\n")
	gitTestCommand(t, root, "add", ".")
	gitTestCommand(t, root, "commit", "-m", "update implementation")

	if err := os.Rename(filepath.Join(root, "internal/old.go"), filepath.Join(root, "internal/new.go")); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, root, "add", "-A")
	gitTestCommand(t, root, "commit", "-m", "rename implementation")

	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(repo)

	filename, err := service.Search(context.Background(), SearchOptions{
		Files: []string{"**/new.go"}, Engine: "glob", Limit: 10, Context: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filename.Count != 1 {
		t.Fatalf("expected one filename match, got %d: %#v", filename.Count, filename.Results)
	}
	if filename.Results[0].File.OldPath != "internal/old.go" || filename.Results[0].File.Path != "internal/new.go" {
		t.Fatalf("unexpected rename: %#v", filename.Results[0].File)
	}

	diff, err := service.Search(context.Background(), SearchOptions{
		Diffs: []string{"*needle*"}, Engine: "glob", Limit: 10, Context: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Count != 1 || diff.Results[0].File.Path != "internal/old.go" {
		t.Fatalf("unexpected diff results: %#v", diff.Results)
	}
	if !strings.Contains(diff.Results[0].Diff, "+const Value = \"needle\"") {
		t.Fatalf("missing diff line: %s", diff.Results[0].Diff)
	}

	follow, err := service.Search(context.Background(), SearchOptions{
		Files: []string{"internal/new.go"}, Engine: "glob", FollowRename: true, Limit: 10, Context: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if follow.Count != 3 {
		t.Fatalf("expected three followed commits, got %d: %#v", follow.Count, follow.Results)
	}
}

func TestSearchRejectsRevisionOptionWithoutWritingOutput(t *testing.T) {
	root := newTestRepository(t)
	writeTestFile(t, root, "tracked.txt", "tracked\n")
	gitTestCommand(t, root, "add", "tracked.txt")
	gitTestCommand(t, root, "commit", "-m", "seed history")
	repo, err := gitexec.OpenRepository(context.Background(), gitexec.NewRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "must-not-change")
	const sentinel = "sentinel\n"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = NewSearchService(repo).Search(context.Background(), SearchOptions{
		Messages: []string{"*seed*"}, Engine: "glob", Revision: "--output=" + target, Limit: 10, Context: 3,
	})
	if err == nil {
		t.Fatal("revision option unexpectedly succeeded")
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != sentinel {
		t.Fatalf("revision option changed output file: %q", content)
	}
}

func newTestRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitTestCommand(t, root, "init", "-b", "main")
	gitTestCommand(t, root, "config", "user.name", "Test User")
	gitTestCommand(t, root, "config", "user.email", "test@example.com")
	gitTestCommand(t, root, "config", "commit.gpgsign", "false")
	gitTestCommand(t, root, "config", "tag.gpgsign", "false")
	gitTestCommand(t, root, "config", "core.autocrlf", "false")
	return root
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = isolatedGitEnvironment()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
