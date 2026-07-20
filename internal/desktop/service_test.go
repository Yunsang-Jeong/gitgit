package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yunsang/gitgit/internal/app"
	"github.com/yunsang/gitgit/internal/gitexec"
)

func TestServiceOpensRepositoryAndEnrichesSearchResults(t *testing.T) {
	repository := createRepository(t)
	service := NewService(nil)

	state, err := service.Open(context.Background(), repository)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	if state.Name != filepath.Base(repository) || state.Branch != "main" || state.Dirty {
		t.Fatalf("unexpected repository state: %#v", state)
	}
	if state.User.Name != "GitGit Test" || state.User.Email != "gitgit@example.com" {
		t.Fatalf("unexpected repository user: %#v", state.User)
	}
	if len(state.Worktrees) != 1 {
		t.Fatalf("unexpected worktrees: %#v", state.Worktrees)
	}
	if state.ProjectRoot != state.Root {
		t.Fatalf("project root = %q, want %q", state.ProjectRoot, state.Root)
	}
	worktreePath, pathErr := filepath.EvalSymlinks(state.Worktrees[0].Path)
	repositoryPath, repoPathErr := filepath.EvalSymlinks(repository)
	if pathErr != nil || repoPathErr != nil || worktreePath != repositoryPath {
		t.Fatalf("unexpected worktree path: %q", state.Worktrees[0].Path)
	}

	history, err := service.History(context.Background(), HistoryRequest{Scope: "main", Limit: 100})
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if history.Scope != "main" || history.Total != 2 || len(history.Commits) != 2 {
		t.Fatalf("unexpected history response: %#v", history)
	}
	if history.Commits[0].Message != "fix: update search implementation" || len(history.Commits[0].Files) != 1 {
		t.Fatalf("unexpected latest commit: %#v", history.Commits[0])
	}
	if history.Commits[1].Parents == nil || len(history.Commits[1].Parents) != 0 {
		t.Fatalf("root commit parents must be an empty array: %#v", history.Commits[1].Parents)
	}
	if history.Commits[0].Branches != nil {
		t.Fatalf("initial history should defer containing branches: %v", history.Commits[0].Branches)
	}
	branchResponse, err := service.HistoryBranches(context.Background(), []string{history.Commits[0].Commit})
	if err != nil {
		t.Fatalf("read containing branches: %v", err)
	}
	containingBranches := branchResponse.Branches[history.Commits[0].Commit]
	if len(containingBranches) != 1 || containingBranches[0] != "main" {
		t.Fatalf("unexpected containing branches: %v", containingBranches)
	}
	detail, err := service.CommitDetail(context.Background(), history.Commits[0].Commit, "internal/search.go")
	if err != nil {
		t.Fatalf("read commit detail: %v", err)
	}
	if detail.File.Path != "internal/search.go" || !strings.Contains(detail.Diff, `const engine = "regex"`) {
		t.Fatalf("unexpected commit detail: %#v", detail)
	}

	response, err := service.Search(context.Background(), SearchRequest{
		Patterns: []Pattern{{Source: "file", Value: "**/*.go"}},
		Engine:   "glob",
		Scope:    "HEAD",
		Limit:    20,
		Context:  3,
	})
	if err != nil {
		t.Fatalf("search repository: %v", err)
	}
	if response.Scanned != 2 || response.Count != 2 {
		t.Fatalf("unexpected response counts: %#v", response)
	}
	if response.Results[0].Message != "fix: update search implementation" {
		t.Fatalf("unexpected message: %q", response.Results[0].Message)
	}
	if response.Results[0].Date == "" || response.Results[0].ShortCommit == "" {
		t.Fatalf("missing enriched metadata: %#v", response.Results[0])
	}
	if len(response.Results[0].Files) != 1 || response.Results[0].Files[0].Path != "internal/search.go" {
		t.Fatalf("unexpected changed files: %#v", response.Results[0].Files)
	}
}

func TestHistoryCacheReturnsIndependentValuesAndInvalidatesBranchMembership(t *testing.T) {
	repository := createRepository(t)
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}
	request := HistoryRequest{Scope: "main", Limit: 24}
	first, err := service.History(context.Background(), request)
	if err != nil {
		t.Fatalf("read initial history: %v", err)
	}
	wantMessage := first.Commits[0].Message
	first.Commits[0].Message = "mutated caller value"
	second, err := service.History(context.Background(), request)
	if err != nil {
		t.Fatalf("read cached history: %v", err)
	}
	if second.Commits[0].Message != wantMessage {
		t.Fatalf("cached history leaked caller mutation: %q", second.Commits[0].Message)
	}

	oid := second.Commits[0].Commit
	before, err := service.HistoryBranches(context.Background(), []string{oid})
	if err != nil {
		t.Fatalf("read cached branch membership: %v", err)
	}
	if got := before.Branches[oid]; len(got) != 1 || got[0] != "main" {
		t.Fatalf("branches before ref change = %v", got)
	}
	runGit(t, repository, nil, "branch", "feature/cache-invalidation", oid)
	after, err := service.HistoryBranches(context.Background(), []string{oid})
	if err != nil {
		t.Fatalf("refresh branch membership after ref change: %v", err)
	}
	if got := after.Branches[oid]; len(got) != 2 || got[0] != "feature/cache-invalidation" || got[1] != "main" {
		t.Fatalf("branches after ref change = %v", got)
	}
}

func TestHistoryRejectsRevisionOptionWithoutWritingOutput(t *testing.T) {
	repository := createRepository(t)
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "must-not-change")
	const sentinel = "sentinel\n"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := service.History(context.Background(), HistoryRequest{Scope: "--output=" + target, Limit: 24}); err == nil {
		t.Fatal("revision option unexpectedly succeeded")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != sentinel {
		t.Fatalf("revision option changed output file: %q", content)
	}
}

func TestRepositorySwitchCancelsRegisteredReadOperations(t *testing.T) {
	service := NewService(nil)
	operationContext, finishOperation := service.beginOperation(context.Background())
	defer finishOperation()
	switchContext, generation, finishSwitch := service.beginRepositorySwitch(context.Background())
	defer finishSwitch()
	defer service.failRepositorySwitch(generation)

	select {
	case <-operationContext.Done():
	case <-time.After(time.Second):
		t.Fatal("repository switch did not cancel the active operation")
	}
	if err := switchContext.Err(); err != nil {
		t.Fatalf("new repository operation was canceled with the previous operation: %v", err)
	}
}

func TestLatestFailedRepositorySwitchDoesNotInstallAnOlderRequest(t *testing.T) {
	firstRepository := createRepository(t)
	secondRepository := createRepository(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "second-open-started")
	release := filepath.Join(t.TempDir(), "release-second-open")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" -C $TEST_SECOND_REPOSITORY --no-pager rev-parse --show-toplevel "*)
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
	t.Setenv("TEST_SECOND_REPOSITORY", secondRepository)
	t.Setenv("TEST_MARKER", marker)
	t.Setenv("TEST_RELEASE", release)
	service := NewService(&gitexec.Runner{Binary: wrapper})
	if _, err := service.Open(context.Background(), firstRepository); err != nil {
		t.Fatal(err)
	}

	olderResult := make(chan error, 1)
	go func() {
		_, openErr := service.Open(context.Background(), secondRepository)
		olderResult <- openErr
	}()
	waitForFile(t, marker)

	latestResult := make(chan error, 1)
	go func() {
		_, openErr := service.Open(context.Background(), filepath.Join(t.TempDir(), "missing-repository"))
		latestResult <- openErr
	}()
	deadline := time.Now().Add(time.Second)
	for {
		service.mu.RLock()
		generation := service.repositoryGeneration
		service.mu.RUnlock()
		if generation >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("latest repository switch was not reserved")
		}
		time.Sleep(10 * time.Millisecond)
	}
	writeFile(t, release, "release\n")
	if err := <-olderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("older repository switch error = %v, want context canceled", err)
	}
	if err := <-latestResult; err == nil {
		t.Fatal("latest invalid repository switch unexpectedly succeeded")
	}
	state, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstRepositoryResolved, resolveErr := filepath.EvalSymlinks(firstRepository)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if state.Root != firstRepositoryResolved {
		t.Fatalf("active repository = %q, want original %q", state.Root, firstRepositoryResolved)
	}
}

func TestRepositorySwitchTerminatesInFlightGitRead(t *testing.T) {
	firstRepository := createRepository(t)
	secondRepository := createRepository(t)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "branch-read-started")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" branch --contains "*)
    : > "$TEST_MARKER"
    exec sleep 30
    ;;
esac
exec "$TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(wrapperSource), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_REAL_GIT", realGit)
	t.Setenv("TEST_MARKER", marker)
	service := NewService(&gitexec.Runner{Binary: wrapper})
	if _, err := service.Open(context.Background(), firstRepository); err != nil {
		t.Fatalf("open first repository: %v", err)
	}
	history, err := service.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24})
	if err != nil {
		t.Fatalf("read first history: %v", err)
	}

	branchResult := make(chan error, 1)
	go func() {
		_, branchErr := service.HistoryBranches(context.Background(), []string{history.Commits[0].Commit})
		branchResult <- branchErr
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, statErr := os.Stat(marker); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("branch membership Git process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	started := time.Now()
	if _, err := service.Open(context.Background(), secondRepository); err != nil {
		t.Fatalf("open second repository: %v", err)
	}
	select {
	case branchErr := <-branchResult:
		if branchErr == nil {
			t.Fatal("canceled branch membership unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("project switch did not terminate the in-flight Git process")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("repository switch waited too long for canceled Git process: %s", elapsed)
	}
}

func TestSyncRemotesFetchesWithoutMovingLocalBranch(t *testing.T) {
	repository := createRepository(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, remote, nil, "init", "-q", "--bare")
	runGit(t, repository, nil, "remote", "add", "origin", remote)
	runGit(t, repository, nil, "push", "-q", "-u", "origin", "main")

	peerParent := t.TempDir()
	peer := filepath.Join(peerParent, "peer")
	runGit(t, peerParent, nil, "clone", "-q", remote, peer)
	runGit(t, peer, nil, "config", "user.name", "Remote Test")
	runGit(t, peer, nil, "config", "user.email", "remote@example.com")
	writeFile(t, filepath.Join(peer, "remote.txt"), "remote update\n")
	runGit(t, peer, nil, "add", "--all")
	runGit(t, peer, nil, "commit", "-q", "-m", "feat: remote update")
	runGit(t, peer, nil, "push", "-q", "origin", "main")

	localHead := gitOutput(t, repository, "rev-parse", "HEAD")
	remoteHead := gitOutput(t, peer, "rev-parse", "HEAD")
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}
	result, err := service.SyncRemotes(context.Background())
	if err != nil {
		t.Fatalf("sync remotes: %v", err)
	}
	if got := gitOutput(t, repository, "rev-parse", "HEAD"); got != localHead {
		t.Fatalf("sync moved local HEAD from %s to %s", localHead, got)
	}
	if got := gitOutput(t, repository, "rev-parse", "origin/main"); got != remoteHead {
		t.Fatalf("fetched origin/main = %s, want %s", got, remoteHead)
	}
	if result.State.Behind != 1 || result.State.Ahead != 0 {
		t.Fatalf("sync state ahead/behind = %d/%d", result.State.Ahead, result.State.Behind)
	}
}

func TestServiceListsRepositoryTreeByRevisionAndDirectory(t *testing.T) {
	repository := createRepository(t)
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}

	root, err := service.RepositoryTree(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("list root tree: %v", err)
	}
	if root.Revision != "main" || root.Directory != "" || len(root.Entries) != 1 {
		t.Fatalf("unexpected root tree: %#v", root)
	}
	if root.Entries[0].Name != "internal" || root.Entries[0].Path != "internal" || root.Entries[0].ObjectType != "tree" {
		t.Fatalf("unexpected root entry: %#v", root.Entries[0])
	}

	internalTree, err := service.RepositoryTree(context.Background(), "main", "internal")
	if err != nil {
		t.Fatalf("list internal tree: %v", err)
	}
	if internalTree.Directory != "internal" || len(internalTree.Entries) != 1 {
		t.Fatalf("unexpected internal tree: %#v", internalTree)
	}
	if internalTree.Entries[0].Name != "search.go" || internalTree.Entries[0].Path != "internal/search.go" || internalTree.Entries[0].ObjectType != "blob" {
		t.Fatalf("unexpected file entry: %#v", internalTree.Entries[0])
	}

	if _, err := service.RepositoryTree(context.Background(), "missing-branch", ""); err == nil {
		t.Fatal("missing revision should fail")
	}
	if _, err := service.RepositoryTree(context.Background(), "main", "../outside"); err == nil {
		t.Fatal("escaping directory should fail")
	}
	if _, err := service.RepositoryTree(context.Background(), "main", "internal/search.go"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file path error = %v, want not a directory", err)
	}
}

func TestSearchRejectsUnknownPatternSource(t *testing.T) {
	_, _, err := searchOptions(SearchRequest{Patterns: []Pattern{{Source: "subject", Value: "fix"}}})
	if err == nil || !strings.Contains(err.Error(), "unsupported pattern source") {
		t.Fatalf("expected source error, got %v", err)
	}
}

func TestSearchOptionsPreservesPatternOrderAndJoins(t *testing.T) {
	options, _, err := searchOptions(SearchRequest{
		Patterns: []Pattern{
			{Source: "MSG", Value: "*alpha*"},
			{Source: "DIFF", Value: "*beta*", Join: "OR"},
			{Source: "FILE", Value: "**/*.go", Join: "AND"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []app.SearchPredicate{
		{Source: "msg", Value: "*alpha*"},
		{Source: "diff", Value: "*beta*", Join: "or"},
		{Source: "file", Value: "**/*.go", Join: "and"},
	}
	if !reflect.DeepEqual(options.Predicates, want) {
		t.Fatalf("predicates = %#v, want %#v", options.Predicates, want)
	}
}

func TestSearchRejectsUnknownPatternJoin(t *testing.T) {
	_, _, err := searchOptions(SearchRequest{Patterns: []Pattern{
		{Source: "msg", Value: "one"},
		{Source: "diff", Value: "two", Join: "xor"},
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported search join") {
		t.Fatalf("expected join error, got %v", err)
	}
}

func TestNormalizeHistoryScopeDistinguishesAllBranchesFromBranchNames(t *testing.T) {
	got, all := normalizeHistoryScope("All", false)
	if got != "All" || all {
		t.Fatalf("normalizeHistoryScope(All, false) = %q, %t; want branch All", got, all)
	}
	got, all = normalizeHistoryScope("main", true)
	if got != "All branches" || !all {
		t.Fatalf("normalizeHistoryScope(main, true) = %q, %t; want All branches", got, all)
	}
}

func TestHistoryAndSearchCanTargetBranchNamedAll(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "checkout", "-q", "-b", "All")
	writeFile(t, filepath.Join(repository, "literal-all.txt"), "literal All branch\n")
	runGit(t, repository, nil, "add", "literal-all.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: literal All branch")
	runGit(t, repository, nil, "checkout", "-q", "main")

	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatalf("open repository: %v", err)
	}

	history, err := service.History(context.Background(), HistoryRequest{Scope: "All", Limit: 100})
	if err != nil {
		t.Fatalf("read literal All branch history: %v", err)
	}
	if history.AllBranches || history.Scope != "All" || !historyContainsMessage(history, "feat: literal All branch") {
		t.Fatalf("literal All branch history = scope %q all=%t messages=%v", history.Scope, history.AllBranches, historyMessages(history))
	}

	search, err := service.Search(context.Background(), SearchRequest{
		Patterns: []Pattern{{Source: "msg", Value: "*literal All branch*"}},
		Engine:   "glob",
		Scope:    "All",
		Limit:    100,
		Context:  3,
	})
	if err != nil {
		t.Fatalf("search literal All branch: %v", err)
	}
	if search.AllRefs || search.Scope != "All" || search.Count != 1 {
		t.Fatalf("literal All branch search = scope %q all=%t count=%d", search.Scope, search.AllRefs, search.Count)
	}

	allHistory, err := service.History(context.Background(), HistoryRequest{Scope: "main", AllBranches: true, Limit: 100})
	if err != nil {
		t.Fatalf("read all branches history: %v", err)
	}
	if !allHistory.AllBranches || allHistory.Scope != "All branches" || !historyContainsMessage(allHistory, "feat: literal All branch") {
		t.Fatalf("all branches history = scope %q all=%t messages=%v", allHistory.Scope, allHistory.AllBranches, historyMessages(allHistory))
	}
}

func historyContainsMessage(history HistoryResponse, message string) bool {
	for _, commit := range history.Commits {
		if strings.Split(commit.Message, "\n")[0] == message {
			return true
		}
	}
	return false
}

func historyMessages(history HistoryResponse) []string {
	messages := make([]string, 0, len(history.Commits))
	for _, commit := range history.Commits {
		messages = append(messages, strings.Split(commit.Message, "\n")[0])
	}
	return messages
}

func TestAllHistoryIncludesLocalBranchesAndMatchingRemoteDefaultBranches(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "remote", "add", "origin", "https://github.com/hashicorp/example.git")

	runGit(t, repository, nil, "checkout", "-q", "-b", "feature/local")
	writeFile(t, filepath.Join(repository, "local.txt"), "local branch\n")
	runGit(t, repository, nil, "add", "local.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: local branch")
	runGit(t, repository, nil, "checkout", "-q", "main")

	runGit(t, repository, nil, "checkout", "-q", "-b", "fixture/remote-main")
	writeFile(t, filepath.Join(repository, "remote-main.txt"), "remote main\n")
	runGit(t, repository, nil, "add", "remote-main.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: remote default branch")
	remoteMain := gitOutput(t, repository, "rev-parse", "HEAD")
	runGit(t, repository, nil, "checkout", "-q", "main")
	runGit(t, repository, nil, "update-ref", "refs/remotes/origin/main", remoteMain)
	runGit(t, repository, nil, "branch", "-D", "fixture/remote-main")

	runGit(t, repository, nil, "checkout", "-q", "-b", "fixture/remote-feature")
	writeFile(t, filepath.Join(repository, "remote-feature.txt"), "remote feature\n")
	runGit(t, repository, nil, "add", "remote-feature.txt")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: remote-only branch")
	remoteFeature := gitOutput(t, repository, "rev-parse", "HEAD")
	runGit(t, repository, nil, "checkout", "-q", "main")
	runGit(t, repository, nil, "update-ref", "refs/remotes/origin/feature/remote-only", remoteFeature)
	runGit(t, repository, nil, "branch", "-D", "fixture/remote-feature")

	service := NewService(nil)
	state, err := service.Open(context.Background(), repository)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	if len(state.Remotes) != 1 || state.Remotes[0].Name != "origin" || state.Remotes[0].URL != "https://github.com/hashicorp/example.git" {
		t.Fatalf("unexpected remotes: %#v", state.Remotes)
	}

	history, err := service.History(context.Background(), HistoryRequest{Scope: "main", AllBranches: true, Limit: 100})
	if err != nil {
		t.Fatalf("read All branches history: %v", err)
	}
	messages := make(map[string]CommitSummary)
	for _, commit := range history.Commits {
		messages[strings.Split(commit.Message, "\n")[0]] = commit
	}
	if _, ok := messages["feat: local branch"]; !ok {
		t.Fatal("All branches history omitted a local branch commit")
	}
	remoteDefault, ok := messages["feat: remote default branch"]
	if !ok {
		t.Fatal("All branches history omitted origin/main")
	}
	if _, ok := messages["feat: remote-only branch"]; ok {
		t.Fatal("All branches history included a non-default remote branch")
	}
	if !containsString(remoteDefault.Refs, "origin/main") {
		t.Fatalf("remote default decorations = %v, want origin/main", remoteDefault.Refs)
	}
	if containsString(history.Branches, "origin/main") || containsString(history.Branches, "origin/feature/remote-only") {
		t.Fatalf("branch picker values contain remote refs: %v", history.Branches)
	}
	if !containsString(history.Branches, "main") || !containsString(history.Branches, "feature/local") {
		t.Fatalf("branch picker values omitted local refs: %v", history.Branches)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestServiceIdentifiesProjectRootFromLinkedWorktree(t *testing.T) {
	repository := createRepository(t)
	linked := filepath.Join(filepath.Dir(repository), "linked-worktree")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/linked", linked, "main")

	state, err := NewService(nil).Open(context.Background(), linked)
	if err != nil {
		t.Fatalf("open linked worktree: %v", err)
	}
	wantRoot, rootErr := filepath.EvalSymlinks(linked)
	wantProjectRoot, projectErr := filepath.EvalSymlinks(repository)
	if rootErr != nil || projectErr != nil || state.Root != wantRoot || state.ProjectRoot != wantProjectRoot {
		t.Fatalf("linked state root/project_root = %q/%q, want %q/%q", state.Root, state.ProjectRoot, wantRoot, wantProjectRoot)
	}
	if len(state.Worktrees) != 2 {
		t.Fatalf("linked state worktrees = %d, want 2", len(state.Worktrees))
	}
}

func TestDefaultBranchRemainsPrimaryWorktreeBranch(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "branch", "-m", "trunk")
	linked := filepath.Join(filepath.Dir(repository), "feature-default-fallback")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/fallback", linked, "trunk")

	state, err := NewService(nil).Open(context.Background(), linked)
	if err != nil {
		t.Fatalf("open linked worktree: %v", err)
	}
	if state.Branch != "feature/fallback" || state.DefaultBranch != "trunk" {
		t.Fatalf("linked branch/default = %q/%q, want feature/fallback/trunk", state.Branch, state.DefaultBranch)
	}
}

func TestServiceReportsGitNativeWorktreeMergeStateAndRelatedHistory(t *testing.T) {
	repository := createRepository(t)
	runGit(t, repository, nil, "switch", "-q", "-c", "feature/merged")
	writeFile(t, filepath.Join(repository, "merged.txt"), "merged branch\n")
	runGit(t, repository, nil, "add", "--all")
	runGit(t, repository, nil, "commit", "-q", "-m", "feat: merged worktree branch")
	runGit(t, repository, nil, "switch", "-q", "main")
	runGit(t, repository, nil, "merge", "-q", "--no-ff", "feature/merged", "-m", "merge: feature/merged")

	mergedWorktree := filepath.Join(filepath.Dir(repository), "merged-worktree")
	secondMergedWorktree := filepath.Join(filepath.Dir(repository), "second-merged-worktree")
	openWorktree := filepath.Join(filepath.Dir(repository), "open-worktree")
	dirtyMergedWorktree := filepath.Join(filepath.Dir(repository), "dirty-merged-worktree")
	runGit(t, repository, nil, "worktree", "add", "-q", mergedWorktree, "feature/merged")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/second-merged", secondMergedWorktree, "main")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/open", openWorktree, "main")
	writeFile(t, filepath.Join(openWorktree, "open.txt"), "unmerged branch\n")
	runGit(t, openWorktree, nil, "add", "--all")
	runGit(t, openWorktree, nil, "commit", "-q", "-m", "feat: open worktree branch")
	runGit(t, repository, nil, "worktree", "add", "-q", "-b", "feature/dirty-merged", dirtyMergedWorktree, "main")
	writeFile(t, filepath.Join(dirtyMergedWorktree, "untracked.txt"), "local changes\n")

	service := NewService(nil)
	state, err := service.Open(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	if state.DefaultBranch != "main" {
		t.Fatalf("default branch = %q, want main", state.DefaultBranch)
	}
	mergeState := map[string]bool{}
	for _, worktree := range state.Worktrees {
		mergeState[worktree.Branch] = worktree.MergedIntoDefault
	}
	if !mergeState["main"] || !mergeState["feature/merged"] || !mergeState["feature/dirty-merged"] || mergeState["feature/open"] {
		t.Fatalf("unexpected merge state: %#v", mergeState)
	}

	history, err := service.History(context.Background(), HistoryRequest{
		Scope: "feature/merged", RelatedScope: "main", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if history.Total != 4 || len(history.Commits) != 2 || len(history.Commits[0].Parents) != 2 || history.BranchPoint != history.Commits[1].Commit {
		t.Fatalf("related history did not stop at the branch point: %#v", history)
	}
	if len(history.Commits[0].Files) != 1 || history.Commits[0].Files[0].Path != "merged.txt" {
		t.Fatalf("merge commit first-parent files = %#v, want merged.txt", history.Commits[0].Files)
	}
	mergeDetail, err := service.CommitDetail(context.Background(), history.Commits[0].Commit, "merged.txt")
	if err != nil {
		t.Fatalf("read merge commit detail: %v", err)
	}
	if mergeDetail.File.Path != "merged.txt" || !strings.Contains(mergeDetail.Diff, "+merged branch") {
		t.Fatalf("merge commit detail = %#v, want first-parent file diff", mergeDetail)
	}

	openHistory, err := service.History(context.Background(), HistoryRequest{
		Scope: "feature/open", RelatedScope: "main", Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundOpenTip := false
	foundMainMerge := false
	for _, commit := range openHistory.Commits {
		foundOpenTip = foundOpenTip || commit.Message == "feat: open worktree branch"
		foundMainMerge = foundMainMerge || (commit.Message == "merge: feature/merged" && len(commit.Parents) == 2)
	}
	if openHistory.Total != 5 || len(openHistory.Commits) != 2 || openHistory.BranchPoint != openHistory.Commits[1].Commit || !foundOpenTip || !foundMainMerge {
		t.Fatalf("unmerged related history did not stop after both graph lanes reached the branch point: %#v", openHistory)
	}
	olderHistory, err := service.History(context.Background(), HistoryRequest{
		Scope: "feature/open", RelatedScope: "main", Limit: 20, Skip: len(openHistory.Commits),
	})
	if err != nil {
		t.Fatal(err)
	}
	if olderHistory.Total != 5 || olderHistory.BranchPoint != openHistory.BranchPoint || len(olderHistory.Commits) != 3 {
		t.Fatalf("history after the branch point did not resume normal pagination: %#v", olderHistory)
	}

	if _, err := service.RemoveMergedWorktree(context.Background(), dirtyMergedWorktree); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("dirty merged worktree removal error = %v", err)
	}
	if _, err := service.RemoveMergedWorktree(context.Background(), openWorktree); err == nil || !strings.Contains(err.Error(), "not merged") {
		t.Fatalf("unmerged worktree removal error = %v", err)
	}
	if _, err := service.RemoveMergedWorktrees(context.Background(), []string{mergedWorktree, dirtyMergedWorktree}); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("mixed batch removal error = %v", err)
	}
	if _, err := os.Stat(mergedWorktree); err != nil {
		t.Fatalf("valid worktree was removed before the mixed batch was fully validated: %v", err)
	}
	if output, err := exec.Command("git", "-C", repository, "show-ref", "--verify", "refs/heads/feature/merged").CombinedOutput(); err != nil {
		t.Fatalf("valid branch was removed before the mixed batch was fully validated: %v: %s", err, output)
	}

	state, err = service.RemoveMergedWorktrees(context.Background(), []string{mergedWorktree, secondMergedWorktree})
	if err != nil {
		t.Fatalf("remove merged worktrees and branches: %v", err)
	}
	if len(state.Worktrees) != 3 {
		t.Fatalf("worktrees after removal = %#v", state.Worktrees)
	}
	for _, removed := range []struct {
		path   string
		branch string
	}{
		{path: mergedWorktree, branch: "feature/merged"},
		{path: secondMergedWorktree, branch: "feature/second-merged"},
	} {
		if _, err := os.Stat(removed.path); !os.IsNotExist(err) {
			t.Fatalf("removed worktree %s still exists: %v", removed.path, err)
		}
		if output, err := exec.Command("git", "-C", repository, "show-ref", "--verify", "refs/heads/"+removed.branch).CombinedOutput(); err == nil {
			t.Fatalf("removed branch %s still exists: %s", removed.branch, output)
		}
	}
}

func TestDeleteWorktreeBranchRetainsItWhenDefaultBranchMoves(t *testing.T) {
	repositoryRoot := createRepository(t)
	defaultHead := gitOutput(t, repositoryRoot, "rev-parse", "main")
	runGit(t, repositoryRoot, nil, "branch", "feature/already-merged", defaultHead)
	writeFile(t, filepath.Join(repositoryRoot, "main-moved.txt"), "main moved\n")
	runGit(t, repositoryRoot, nil, "add", "main-moved.txt")
	runGit(t, repositoryRoot, nil, "commit", "-q", "-m", "feat: move default branch")

	repository, err := gitexec.OpenRepository(context.Background(), nil, repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	err = deleteBranchIfDefaultUnchanged(
		context.Background(), repository,
		"refs/heads/main", defaultHead,
		"refs/heads/feature/already-merged", defaultHead,
	)
	if err == nil || !strings.Contains(err.Error(), "atomically") {
		t.Fatalf("default branch movement error = %v", err)
	}
	if got := gitOutput(t, repositoryRoot, "rev-parse", "feature/already-merged"); got != defaultHead {
		t.Fatalf("feature branch moved after rejected deletion: got %s want %s", got, defaultHead)
	}
}

func TestRemoveMergedWorktreeSerializesRepositorySwitch(t *testing.T) {
	firstRepository := createRepository(t)
	secondRepository := createRepository(t)
	worktreePath := filepath.Join(filepath.Dir(firstRepository), "merged-switch-worktree")
	runGit(t, firstRepository, nil, "worktree", "add", "-q", "-b", "feature/remove-before-switch", worktreePath, "main")

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "worktree-remove-started")
	release := filepath.Join(t.TempDir(), "release-worktree-remove")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	wrapperSource := `#!/bin/sh
case " $* " in
  *" worktree remove "*)
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
	if _, err := service.Open(context.Background(), firstRepository); err != nil {
		t.Fatal(err)
	}

	removeResult := make(chan error, 1)
	go func() {
		_, removeErr := service.RemoveMergedWorktree(context.Background(), worktreePath)
		removeResult <- removeErr
	}()
	waitForFile(t, marker)

	openStarted := make(chan struct{})
	openResult := make(chan error, 1)
	go func() {
		close(openStarted)
		_, openErr := service.Open(context.Background(), secondRepository)
		openResult <- openErr
	}()
	<-openStarted
	select {
	case openErr := <-openResult:
		t.Fatalf("repository switch crossed active worktree removal: %v", openErr)
	case <-time.After(150 * time.Millisecond):
	}
	writeFile(t, release, "release\n")
	if err := <-removeResult; err != nil {
		t.Fatalf("remove merged worktree: %v", err)
	}
	if err := <-openResult; err != nil {
		t.Fatalf("open second repository: %v", err)
	}
	state, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if canonicalWorktreePath(state.Root) != canonicalWorktreePath(secondRepository) {
		t.Fatalf("current repository = %q, want %q", state.Root, secondRepository)
	}
}

func TestServiceCanonicalSubgitSearchScenarios(t *testing.T) {
	repository := createCanonicalSubgitFixture(t)
	service := NewService(nil)

	state, err := service.Open(context.Background(), repository)
	if err != nil {
		t.Fatalf("open canonical fixture: %v", err)
	}
	if state.Name != filepath.Base(repository) || state.Branch != "main" || state.Dirty || len(state.Worktrees) != 12 {
		t.Fatalf("unexpected canonical repository state: %#v", state)
	}
	if state.ProjectRoot != state.Root {
		t.Fatalf("canonical project root = %q, want %q", state.ProjectRoot, state.Root)
	}
	var dirty, locked, sparse, detached bool
	for _, worktree := range state.Worktrees {
		dirty = dirty || worktree.Dirty
		locked = locked || worktree.Locked
		sparse = sparse || worktree.Sparse.Enabled
		detached = detached || worktree.Detached
	}
	if !dirty || !locked || !sparse || !detached {
		t.Fatalf("fixture worktree states dirty=%t locked=%t sparse=%t detached=%t", dirty, locked, sparse, detached)
	}

	firstPage, err := service.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24})
	if err != nil {
		t.Fatalf("read first history page: %v", err)
	}
	secondPage, err := service.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24, Skip: 24})
	if err != nil {
		t.Fatalf("read second history page: %v", err)
	}
	if firstPage.Total != 100 || len(firstPage.Commits) != 24 || len(secondPage.Commits) != 24 {
		t.Fatalf("unexpected paged history sizes: first=%d second=%d total=%d", len(firstPage.Commits), len(secondPage.Commits), firstPage.Total)
	}
	if firstPage.Commits[0].Commit == secondPage.Commits[0].Commit {
		t.Fatal("history pages overlap at their first commit")
	}
	if _, err := service.History(context.Background(), HistoryRequest{Scope: "main", Limit: 24, Skip: -1}); err == nil {
		t.Fatal("negative history skip should fail")
	}

	tests := []struct {
		name        string
		request     SearchRequest
		wantScanned int
		wantCount   int
		check       func(*testing.T, SearchResponse)
	}{
		{
			name:        "message glob",
			request:     scenarioSearch("MSG", "*search*", "glob", "HEAD"),
			wantScanned: 100,
			wantCount:   2,
		},
		{
			name:        "diff glob",
			request:     scenarioSearch("DIFF", "*FIXTURE_REGEX_OMEGA*", "glob", "HEAD"),
			wantScanned: 100,
			wantCount:   1,
			check: func(t *testing.T, response SearchResponse) {
				if response.Results[0].File.Path != "internal/search/pipeline.txt" {
					t.Fatalf("diff result path = %q", response.Results[0].File.Path)
				}
			},
		},
		{
			name:        "file double-star glob",
			request:     scenarioSearch("FILE", "**/*.md", "glob", "HEAD"),
			wantScanned: 100,
			wantCount:   4,
		},
		{
			name:        "message regex",
			request:     scenarioSearch("MSG", `^fix\(`, "regex", "HEAD"),
			wantScanned: 100,
			wantCount:   2,
		},
		{
			name: "patterns across sources use OR",
			request: SearchRequest{
				Patterns: []Pattern{
					{Source: "DIFF", Value: "*PERF_BATCH_64*"},
					{Source: "FILE", Value: "docs/history.md"},
				},
				Engine: "glob", Scope: "HEAD", Limit: 100, Context: 3,
			},
			wantScanned: 100,
			wantCount:   2,
		},
		{
			name: "author and date filters intersect",
			request: SearchRequest{
				Patterns: []Pattern{{Source: "MSG", Value: "*search*"}},
				Engine:   "glob", Scope: "HEAD", Author: "Bob Example",
				Since: "2025-01-01", Until: "2025-12-31", Limit: 100, Context: 3,
			},
			wantScanned: 1,
			wantCount:   1,
		},
		{
			name:        "one exact revision",
			request:     scenarioSearch("DIFF", "FIXTURE_REGEX_OMEGA", "regex", "fixture-v2^!"),
			wantScanned: 1,
			wantCount:   1,
		},
		{
			name:        "side branch excluded from HEAD",
			request:     scenarioSearch("MSG", "*SIDE_BRANCH_ONLY*", "glob", "HEAD"),
			wantScanned: 100,
			wantCount:   0,
		},
		{
			name:        "all refs includes side branch",
			request:     scenarioSearch("MSG", "*SIDE_BRANCH_ONLY*", "glob", "All refs"),
			wantScanned: 106,
			wantCount:   1,
		},
		{
			name: "current filename follows case-only rename backwards",
			request: func() SearchRequest {
				request := scenarioSearch("FILE", "a/b/c/D.txt", "glob", "HEAD")
				request.FollowRename = true
				return request
			}(),
			wantScanned: 100,
			wantCount:   3,
			check: func(t *testing.T, response SearchResponse) {
				assertContainsRename(t, response, "a/b/c/d.txt", "a/b/c/D.txt")
			},
		},
		{
			name: "old filename exposes rename event",
			request: func() SearchRequest {
				request := scenarioSearch("FILE", "a/b/c/d.txt", "glob", "HEAD")
				request.FollowRename = true
				return request
			}(),
			wantScanned: 100,
			wantCount:   2,
			check: func(t *testing.T, response SearchResponse) {
				assertContainsRename(t, response, "a/b/c/d.txt", "a/b/c/D.txt")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := service.Search(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if response.Scanned != test.wantScanned || response.Count != test.wantCount {
				t.Fatalf("scanned/count = %d/%d, want %d/%d: %#v", response.Scanned, response.Count, test.wantScanned, test.wantCount, response.Results)
			}
			for _, result := range response.Results {
				if result.Commit == "" || result.ShortCommit == "" || result.Message == "" || result.Date == "" || result.Diff == "" {
					t.Fatalf("result is missing inspector metadata: %#v", result)
				}
			}
			if test.check != nil {
				test.check(t, response)
			}
			t.Logf("scope=%s scanned=%d matches=%d", response.Scope, response.Scanned, response.Count)
		})
	}
}

func scenarioSearch(source, value, engine, scope string) SearchRequest {
	return SearchRequest{
		Patterns: []Pattern{{Source: source, Value: value}},
		Engine:   engine, Scope: scope, AllRefs: scope == "All refs", Limit: 100, Context: 3,
	}
}

func assertContainsRename(t *testing.T, response SearchResponse, oldPath, newPath string) {
	t.Helper()
	for _, result := range response.Results {
		if result.File.OldPath == oldPath && result.File.Path == newPath && strings.HasPrefix(result.File.Status, "R") {
			return
		}
	}
	t.Fatalf("rename %s -> %s is missing: %#v", oldPath, newPath, response.Results)
}

func TestCanonicalSubgitSideBranchHistoryStopsAtDefaultBranchPoint(t *testing.T) {
	repository := createCanonicalSubgitFixture(t)
	service := NewService(nil)
	if _, err := service.Open(context.Background(), repository); err != nil {
		t.Fatal(err)
	}
	sideHead := gitOutput(t, repository, "rev-parse", "scenario-side")
	mainHead := gitOutput(t, repository, "rev-parse", "main")

	history, err := service.History(context.Background(), HistoryRequest{
		Scope: "scenario-side", RelatedScope: "main", Limit: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	if history.Total != 101 || history.BranchPoint != mainHead || len(history.Commits) != 2 {
		t.Fatalf("side branch boundary history = %#v", history)
	}
	if history.Commits[0].Commit != sideHead || history.Commits[0].Message != "feat: SIDE_BRANCH_ONLY" || history.Commits[1].Commit != mainHead {
		t.Fatalf("side branch commits through branch point = %#v", history.Commits)
	}

	older, err := service.History(context.Background(), HistoryRequest{
		Scope: "scenario-side", RelatedScope: "main", Limit: 24, Skip: len(history.Commits),
	})
	if err != nil {
		t.Fatal(err)
	}
	if older.BranchPoint != mainHead || len(older.Commits) != 24 || older.Commits[0].Commit == mainHead {
		t.Fatalf("side branch continuation = %#v", older)
	}

	_, err = service.PrepareCommitEdit(context.Background(), sideHead)
	if err == nil || !strings.Contains(err.Error(), "scenario-side") || !strings.Contains(err.Error(), "main checked out") {
		t.Fatalf("side branch edit context = %v", err)
	}
}

func createCanonicalSubgitFixture(t *testing.T) string {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", "testdata", "subgit", "setup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceRepository, err := filepath.Abs(filepath.Join("..", "..", "subgit"))
	if err != nil {
		t.Fatal(err)
	}
	workspaceCommand := exec.Command("sh", script, "create", workspaceRepository)
	if _, err := workspaceCommand.CombinedOutput(); err == nil {
		return workspaceRepository
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("resolve test fixture cache: %v", err)
	}
	repository := filepath.Join(cacheRoot, "GitGit", "test-fixtures", "subgit-v4-100")
	if err := os.MkdirAll(filepath.Dir(repository), 0o755); err != nil {
		t.Fatalf("create test fixture cache directory: %v", err)
	}
	command := exec.Command("sh", script, "create", repository)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("prepare cached canonical subgit fixture: %v\n%s", err, output)
	}
	return repository
}

func createRepository(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, nil, "init", "-q", "-b", "main")
	runGit(t, root, nil, "config", "user.name", "GitGit Test")
	runGit(t, root, nil, "config", "user.email", "gitgit@example.com")

	writeFile(t, filepath.Join(root, "internal", "search.go"), "package internal\n\nconst engine = \"glob\"\n")
	runGit(t, root, nil, "add", "--all")
	commitEnv := []string{
		"GIT_AUTHOR_DATE=2026-07-01T10:00:00+09:00",
		"GIT_COMMITTER_DATE=2026-07-01T10:00:00+09:00",
	}
	runGit(t, root, commitEnv, "commit", "-q", "-m", "feat: add search implementation")

	writeFile(t, filepath.Join(root, "internal", "search.go"), "package internal\n\nconst engine = \"regex\"\n")
	runGit(t, root, nil, "add", "--all")
	commitEnv = []string{
		"GIT_AUTHOR_DATE=2026-07-02T10:00:00+09:00",
		"GIT_COMMITTER_DATE=2026-07-02T10:00:00+09:00",
	}
	runGit(t, root, commitEnv, "commit", "-q", "-m", "fix: update search implementation")
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, directory string, environment []string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
