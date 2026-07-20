package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestPersistentCacheSurvivesReopenAndInvalidatesOnlyRefData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache")
	cache, err := OpenPersistentCache(path)
	if err != nil {
		t.Fatal(err)
	}
	repository := "/tmp/example/.git"
	history := HistoryResponse{Scope: "main", Total: 1, Commits: []CommitSummary{{Commit: "abc", Message: "cached"}}}
	detail := CommitDetail{CommitSummary: CommitSummary{Commit: "abc", Message: "detail"}, Diff: "+cached"}
	if err := cache.storeHistory(repository, "refs-a", "page", history); err != nil {
		t.Fatal(err)
	}
	if err := cache.storeBranches(repository, "refs-a", "abc", []string{"main"}); err != nil {
		t.Fatal(err)
	}
	if err := cache.storeDetail(repository, "abc", detail); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}

	cache, err = OpenPersistentCache(path)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	loadedHistory, found, err := cache.loadHistory(repository, "refs-a", "page")
	if err != nil || !found || loadedHistory.Commits[0].Message != "cached" {
		t.Fatalf("load persisted history = %#v, %t, %v", loadedHistory, found, err)
	}
	loadedBranches, found, err := cache.loadBranches(repository, "refs-a", "abc")
	if err != nil || !found || len(loadedBranches) != 1 || loadedBranches[0] != "main" {
		t.Fatalf("load persisted branches = %v, %t, %v", loadedBranches, found, err)
	}

	if _, found, err := cache.loadHistory(repository, "refs-b", "page"); err != nil || found {
		t.Fatalf("history survived ref invalidation: found=%t err=%v", found, err)
	}
	if _, found, err := cache.loadBranches(repository, "refs-b", "abc"); err != nil || found {
		t.Fatalf("branches survived ref invalidation: found=%t err=%v", found, err)
	}
	loadedDetail, found, err := cache.loadDetail(repository, "abc")
	if err != nil || !found || loadedDetail.Diff != "+cached" {
		t.Fatalf("immutable detail was invalidated = %#v, %t, %v", loadedDetail, found, err)
	}
}

func TestPersistentCacheIsSharedAcrossLinkedWorktrees(t *testing.T) {
	repository := createRepository(t)
	linked := filepath.Join(filepath.Dir(repository), "linked-cache-worktree")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/cache-worktree", linked, "main")

	cachePath := filepath.Join(t.TempDir(), "cache")
	cache, err := OpenPersistentCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	first := NewServiceWithCache(nil, cache)
	if _, err := first.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	history, err := first.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24})
	if err != nil {
		t.Fatal(err)
	}
	oid := history.Commits[0].Commit
	if _, err := first.HistoryBranches(context.Background(), []string{oid}); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CommitDetail(context.Background(), oid, "internal/search.go"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" log --topo-order "*|*" branch --all --contains "*|*" show "*)
    echo "persistent cache miss triggered an expensive Git read" >&2
    exit 91
    ;;
esac
exec "$TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(wrapperSource), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REAL_GIT", realGit)
	cache, err = OpenPersistentCache(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	second := NewServiceWithCache(&gitexec.Runner{Binary: wrapper}, cache)
	defer second.Close()
	if _, err := second.Open(context.Background(), linked); err != nil {
		t.Fatal(err)
	}
	loadedHistory, err := second.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24})
	if err != nil || len(loadedHistory.Commits) != len(history.Commits) {
		t.Fatalf("load shared history: commits=%d err=%v", len(loadedHistory.Commits), err)
	}
	if _, err := second.HistoryBranches(context.Background(), []string{oid}); err != nil {
		t.Fatalf("load shared branches: %v", err)
	}
	loadedDetail, err := second.CommitDetail(context.Background(), oid, "internal/search.go")
	if err != nil || loadedDetail.Commit != oid {
		t.Fatalf("load shared detail: commit=%q err=%v", loadedDetail.Commit, err)
	}
}
