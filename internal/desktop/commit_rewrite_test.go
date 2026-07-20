package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestRewriteCommitsReordersMessagesAndFileContent(t *testing.T) {
	repository := createRepository(t)
	base := gitOutput(t, repository, "rev-parse", "HEAD")
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/rewrite")

	writeFile(t, filepath.Join(repository, "value.txt"), "a = \"1\"\n")
	runGit(t, repository, nil, "add", "--all")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: add value")
	first := gitOutput(t, repository, "rev-parse", "HEAD")

	writeFile(t, filepath.Join(repository, "second.txt"), "second\n")
	runGit(t, repository, nil, "add", "--all")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: add second")
	second := gitOutput(t, repository, "rev-parse", "HEAD")

	service := NewService(nil)
	state, err := service.Open(context.Background(), repository)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	if state.DefaultBranch != "main" {
		t.Fatalf("default branch = %q, want main", state.DefaultBranch)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), first)
	if err != nil {
		t.Fatalf("prepare commit edit: %v", err)
	}
	if stack.Branch != "feature/rewrite" || stack.DefaultBranchTarget || stack.Base != base || stack.Head != second {
		t.Fatalf("unexpected edit stack: %#v", stack)
	}
	if len(stack.Commits) != 2 || stack.Commits[0].Commit != first || stack.Commits[1].Commit != second {
		t.Fatalf("unexpected editable commits: %#v", stack.Commits)
	}
	content, err := service.CommitFileContent(context.Background(), first, "value.txt")
	if err != nil {
		t.Fatalf("read editable content: %v", err)
	}
	if !content.Editable || !content.Exists || content.Content != "a = \"1\"\n" {
		t.Fatalf("unexpected editable content: %#v", content)
	}

	result, err := service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch:       stack.Branch,
		ExpectedHead: stack.Head,
		Base:         stack.Base,
		Commits: []RewriteCommit{
			{Commit: second, Message: "feat: add second"},
			{Commit: first, Message: "feat: replace value", FileEdits: []CommitFileEdit{{Path: "value.txt", Content: "b = \"2\"\n"}}},
		},
	})
	if err != nil {
		t.Fatalf("rewrite commits: %v", err)
	}
	if result.Head == second || result.State.Head != result.Head || result.State.Dirty {
		t.Fatalf("unexpected rewrite result: %#v", result)
	}
	if got := gitOutput(t, repository, "rev-parse", result.BackupRef); got != second {
		t.Fatalf("backup ref = %s, want old HEAD %s", got, second)
	}
	messages := strings.Split(gitOutput(t, repository, "log", "--reverse", "--format=%s", base+"..HEAD"), "\n")
	if len(messages) != 2 || messages[0] != "feat: add second" || messages[1] != "feat: replace value" {
		t.Fatalf("rewritten messages = %v", messages)
	}
	value, err := os.ReadFile(filepath.Join(repository, "value.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "b = \"2\"\n" {
		t.Fatalf("rewritten value = %q", value)
	}
	if status := gitOutput(t, repository, "status", "--porcelain"); status != "" {
		t.Fatalf("worktree is dirty after rewrite: %q", status)
	}
}

func TestRewriteCommitsRequiresDefaultBranchConfirmation(t *testing.T) {
	repository := createRepository(t)
	head := gitOutput(t, repository, "rev-parse", "HEAD")
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatalf("prepare commit edit: %v", err)
	}
	if !stack.DefaultBranchTarget || len(stack.Commits) != 1 {
		t.Fatalf("unexpected default branch stack: %#v", stack)
	}
	_, err = service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{{Commit: head, Message: "fix: edited without confirmation"}},
	})
	if err == nil || !strings.Contains(err.Error(), "explicit confirmation") {
		t.Fatalf("default branch rewrite error = %v", err)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != head {
		t.Fatalf("default branch moved without confirmation: %s", got)
	}
	result, err := service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base, ConfirmDefaultBranch: true,
		Commits: []RewriteCommit{{Commit: head, Message: "fix: edited with confirmation"}},
	})
	if err != nil {
		t.Fatalf("confirmed default branch rewrite: %v", err)
	}
	if result.Head == head || gitOutput(t, repository, "log", "-1", "--format=%s") != "fix: edited with confirmation" {
		t.Fatalf("confirmed rewrite result = %#v", result)
	}
}

func TestPrepareCommitEditAllowsDirtyAndRejectsDetachedWorktrees(t *testing.T) {
	repository := createRepository(t)
	head := gitOutput(t, repository, "rev-parse", "HEAD")
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}
	writeFile(t, filepath.Join(repository, "dirty.txt"), "dirty\n")
	if _, err := service.PrepareCommitEdit(context.Background(), head); err != nil {
		t.Fatalf("prepare commit edit with dirty worktree: %v", err)
	}
	if err := os.Remove(filepath.Join(repository, "dirty.txt")); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, nil, "checkout", "--detach", "-q", head)
	if _, err := service.PrepareCommitEdit(context.Background(), head); err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("detached HEAD error = %v", err)
	}
}

func TestRewriteCommitsPreservesNonOverlappingWorktreeAndIndexChanges(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/dirty-rewrite")
	writeFile(t, filepath.Join(repository, "staged.txt"), "committed staged\n")
	writeFile(t, filepath.Join(repository, "unstaged.txt"), "committed unstaged\n")
	runGit(t, repository, nil, "add", "staged.txt", "unstaged.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "chore: add local change fixtures")
	writeFile(t, filepath.Join(repository, "rewrite.txt"), "committed rewrite\n")
	runGit(t, repository, nil, "add", "rewrite.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: rewrite target")
	head := gitOutput(t, repository, "rev-parse", "HEAD")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository, "staged.txt"), "local staged\n")
	runGit(t, repository, nil, "add", "staged.txt")
	writeFile(t, filepath.Join(repository, "unstaged.txt"), "local unstaged\n")
	writeFile(t, filepath.Join(repository, "untracked.txt"), "local untracked\n")

	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatalf("prepare dirty commit edit: %v", err)
	}
	result, err := service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{{
			Commit: head, Message: "feat: rewritten while dirty",
			FileEdits: []CommitFileEdit{{Path: "rewrite.txt", Content: "rewritten content\n"}},
		}},
	})
	if err != nil {
		t.Fatalf("rewrite with non-overlapping local changes: %v", err)
	}
	if !result.State.Dirty {
		t.Fatalf("rewritten repository should still report local changes: %#v", result.State)
	}
	for path, want := range map[string]string{
		"staged.txt":    "local staged\n",
		"unstaged.txt":  "local unstaged\n",
		"untracked.txt": "local untracked\n",
		"rewrite.txt":   "rewritten content\n",
	} {
		content, readErr := os.ReadFile(filepath.Join(repository, path))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(content) != want {
			t.Fatalf("%s content = %q, want %q", path, content, want)
		}
	}
	status := gitOutput(t, repository, "status", "--porcelain")
	for _, want := range []string{"M  staged.txt", " M unstaged.txt", "?? untracked.txt"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status %q does not preserve %q", status, want)
		}
	}
	if got := gitOutput(t, repository, "log", "-1", "--format=%s"); got != "feat: rewritten while dirty" {
		t.Fatalf("rewritten message = %q", got)
	}
}

func TestRewriteCommitsRejectsOverlappingLocalChangesBeforeMovingBranch(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/overlapping-rewrite")
	writeFile(t, filepath.Join(repository, "value.txt"), "committed\n")
	runGit(t, repository, nil, "add", "value.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: add value")
	head := gitOutput(t, repository, "rev-parse", "HEAD")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repository, "value.txt"), "local change\n")
	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatalf("prepare overlapping commit edit: %v", err)
	}
	_, err = service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{{
			Commit: head, Message: "feat: rewritten value",
			FileEdits: []CommitFileEdit{{Path: "value.txt", Content: "rewritten\n"}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot preserve") {
		t.Fatalf("overlapping local change error = %v", err)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != head {
		t.Fatalf("branch moved despite overlapping local change: got %s want %s", got, head)
	}
	content, readErr := os.ReadFile(filepath.Join(repository, "value.txt"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "local change\n" {
		t.Fatalf("overlapping local change was modified: %q", content)
	}
	if refs := gitOutput(t, repository, "for-each-ref", "--format=%(refname)", "refs/gitgit/backups"); refs != "" {
		t.Fatalf("backup ref was created before overlap rejection: %q", refs)
	}
}

func TestPrepareCommitEditExplainsSelectedBranchMismatch(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "switch", "-q", "-c", "scenario-side")
	writeFile(t, filepath.Join(repository, "side.txt"), "side branch only\n")
	runGit(t, repository, nil, "add", "side.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: side branch only")
	sideCommit := gitOutput(t, repository, "rev-parse", "HEAD")
	runGit(t, repository, nil, "switch", "-q", "main")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	_, err := service.PrepareCommitEdit(context.Background(), sideCommit)
	if err == nil || !strings.Contains(err.Error(), "scenario-side") || !strings.Contains(err.Error(), "main checked out") || !strings.Contains(err.Error(), "open a worktree") {
		t.Fatalf("branch mismatch error = %v", err)
	}
}

func TestRewriteCommitsLeavesBranchUntouchedWhenReorderConflicts(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/conflict")
	writeFile(t, filepath.Join(repository, "sequence.txt"), "one\n")
	runGit(t, repository, nil, "add", "--all")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: sequence one")
	first := gitOutput(t, repository, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(repository, "sequence.txt"), "two\n")
	runGit(t, repository, nil, "add", "--all")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: sequence two")
	second := gitOutput(t, repository, "rev-parse", "HEAD")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), first)
	if err != nil {
		t.Fatalf("prepare commit edit: %v", err)
	}
	_, err = service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{
			{Commit: second, Message: "feat: sequence two"},
			{Commit: first, Message: "feat: sequence one"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "new position") {
		t.Fatalf("conflicting reorder error = %v", err)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != second {
		t.Fatalf("branch moved after failed rewrite: %s", got)
	}
	if got := gitOutput(t, repository, "status", "--porcelain"); got != "" {
		t.Fatalf("original worktree changed after failed rewrite: %q", got)
	}
}

func TestCommitFileEditRejectsSymlinkPreviewAndApply(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/symlink-edit")
	victim := filepath.Join(t.TempDir(), "victim.txt")
	writeFile(t, victim, "sentinel\n")
	linkPath := filepath.Join(repository, "linked-config")
	if err := os.Symlink(victim, linkPath); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, nil, "add", "linked-config")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: add config link")
	head := gitOutput(t, repository, "rev-parse", "HEAD")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	content, err := service.CommitFileContent(context.Background(), head, "linked-config")
	if err != nil {
		t.Fatal(err)
	}
	if content.Editable || !strings.Contains(content.Reason, "Symbolic links") {
		t.Fatalf("symlink content = %#v", content)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{{
			Commit: head, Message: "feat: edit config link",
			FileEdits: []CommitFileEdit{{Path: "linked-config", Content: "overwritten\n"}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "Symbolic links") {
		t.Fatalf("symlink rewrite error = %v", err)
	}
	victimContent, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(victimContent) != "sentinel\n" {
		t.Fatalf("symlink target was changed: %q", victimContent)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != head {
		t.Fatalf("branch moved after rejected symlink edit: %s", got)
	}
}

func TestRewriteFilePathRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked-parent")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareRewriteFilePath(root, "linked-parent/file.txt", true); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink parent error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file was created: %v", err)
	}
}

func TestApplyCommitFileEditsRejectsOversizeAndNULContent(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "oversize", content: strings.Repeat("x", maximumEditableFile+1), want: "2 MiB"},
		{name: "NUL", content: "before\x00after", want: "NUL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := createRepository(t)
			writeFile(t, filepath.Join(repository, "editable.txt"), "original\n")
			runGit(t, repository, nil, "add", "editable.txt")
			runGit(t, repository, nil, "commit", "-q", "-m", "feat: editable file")
			head := gitOutput(t, repository, "rev-parse", "HEAD")
			repo, err := gitexec.OpenRepository(context.Background(), nil, repository)
			if err != nil {
				t.Fatal(err)
			}
			summary, err := readCommitSummary(context.Background(), repo, head)
			if err != nil {
				t.Fatal(err)
			}
			err = applyCommitFileEdits(context.Background(), repo, CommitEditCommit{
				Author: summary.Author, Commit: summary.Commit, ShortCommit: summary.ShortCommit,
				Message: summary.Message, Date: summary.Date, Files: summary.Files, Parents: summary.Parents,
			}, []CommitFileEdit{{Path: "editable.txt", Content: test.content}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("apply error = %v, want %q", err, test.want)
			}
			content, err := os.ReadFile(filepath.Join(repository, "editable.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "original\n" {
				t.Fatalf("rejected edit changed file: %q", content)
			}
		})
	}
}

func TestRewriteCommitsPreservesConcurrentWorktreeChangeAndRollsBackRef(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/concurrent-install")
	writeFile(t, filepath.Join(repository, "value.txt"), "committed\n")
	runGit(t, repository, nil, "add", "value.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: committed value")
	head := gitOutput(t, repository, "rev-parse", "HEAD")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "branch-update-started")
	release := filepath.Join(t.TempDir(), "release-branch-update")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" update-ref refs/heads/feature/concurrent-install "*)
    : > "$TEST_MARKER"
    while test ! -e "$TEST_RELEASE"; do sleep 0.01; done
    ;;
esac
exec "$TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(wrapperSource), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REAL_GIT", realGit)
	t.Setenv("TEST_MARKER", marker)
	t.Setenv("TEST_RELEASE", release)
	service := NewService(&gitexec.Runner{Binary: wrapper})
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, rewriteErr := service.RewriteCommits(context.Background(), RewriteCommitsRequest{
			Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
			Commits: []RewriteCommit{{
				Commit: head, Message: "feat: rewritten value",
				FileEdits: []CommitFileEdit{{Path: "value.txt", Content: "rewritten committed\n"}},
			}},
		})
		result <- rewriteErr
	}()
	waitForFile(t, marker)
	writeFile(t, filepath.Join(repository, "value.txt"), "local change\n")
	writeFile(t, release, "release\n")
	select {
	case rewriteErr := <-result:
		if rewriteErr == nil || !strings.Contains(rewriteErr.Error(), "update worktree") {
			t.Fatalf("concurrent rewrite error = %v", rewriteErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("rewrite did not finish after releasing branch update")
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != head {
		t.Fatalf("branch was not rolled back: got %s want %s", got, head)
	}
	content, err := os.ReadFile(filepath.Join(repository, "value.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "local change\n" {
		t.Fatalf("concurrent worktree change was lost: %q", content)
	}
}

func TestRewriteCommitsKeepsInstalledRewriteWhenRefreshFails(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/snapshot-refresh")
	writeFile(t, filepath.Join(repository, "refresh.txt"), "refresh\n")
	runGit(t, repository, nil, "add", "refresh.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: refresh state")
	head := gitOutput(t, repository, "rev-parse", "HEAD")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(t.TempDir(), "rewrite-installed")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" update-ref refs/heads/feature/snapshot-refresh "*)
    "$TEST_REAL_GIT" "$@"
    code=$?
    if test "$code" -eq 0; then : > "$TEST_INSTALLED"; fi
    exit "$code"
    ;;
  *" status --porcelain=v2 -z "*)
    if test -e "$TEST_INSTALLED"; then
      echo "injected post-install snapshot failure" >&2
      exit 91
    fi
    ;;
esac
exec "$TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(wrapperSource), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REAL_GIT", realGit)
	t.Setenv("TEST_INSTALLED", installed)
	service := NewService(&gitexec.Runner{Binary: wrapper})
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	stack, err := service.PrepareCommitEdit(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RewriteCommits(context.Background(), RewriteCommitsRequest{
		Branch: stack.Branch, ExpectedHead: stack.Head, Base: stack.Base,
		Commits: []RewriteCommit{{Commit: head, Message: "feat: rewritten despite refresh failure"}},
	})
	if err != nil {
		t.Fatalf("installed rewrite returned an error: %v", err)
	}
	if result.Head == head || result.State.Head != result.Head || result.Warning == "" || !result.State.Dirty {
		t.Fatalf("unexpected fallback rewrite result: %#v", result)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != result.Head {
		t.Fatalf("installed branch was rolled back: got %s want %s", got, result.Head)
	}
	if got := gitOutput(t, repository, "log", "-1", "--format=%s"); got != "feat: rewritten despite refresh failure" {
		t.Fatalf("installed commit message = %q", got)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
