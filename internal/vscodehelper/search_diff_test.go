package vscodehelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestSearchRunMatchesMessageDiffFileAndGroupedExpression(t *testing.T) {
	fixture := newMethodFixture(t)
	tests := []struct {
		name            string
		patterns        []map[string]any
		wantMessage     string
		wantSources     []string
		wantFileSources []string
	}{
		{
			name:        "message",
			patterns:    []map[string]any{{"source": "msg", "value": "*rename file*"}},
			wantMessage: "rename file",
			wantSources: []string{"msg"},
		},
		{
			name:            "diff",
			patterns:        []map[string]any{{"source": "diff", "value": "*third updated*"}},
			wantMessage:     "update new file",
			wantSources:     []string{"diff"},
			wantFileSources: []string{"diff"},
		},
		{
			name:            "file",
			patterns:        []map[string]any{{"source": "file", "value": "*new.txt*"}},
			wantMessage:     "update new file",
			wantSources:     []string{"file"},
			wantFileSources: []string{"file"},
		},
		{
			name: "and or groups",
			patterns: []map[string]any{
				{"source": "msg", "value": "*does-not-match*", "openGroups": 1},
				{"source": "file", "value": "*new.txt*", "join": "or", "closeGroups": 1},
				{"source": "diff", "value": "*third updated*", "join": "and"},
			},
			wantMessage:     "update new file",
			wantSources:     []string{"file", "diff"},
			wantFileSources: []string{"file", "diff"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := serveRPCMessages(t, fixture.server, "search.run", map[string]any{
				"repositoryRoot": fixture.root,
				"patterns":       test.patterns,
				"engine":         "glob",
				"scope":          "HEAD",
				"limit":          1,
				"context":        1,
			})
			response, progress := splitRPCMessages(t, messages)
			if response.Error != nil {
				t.Fatalf("search.run error: %#v", response.Error)
			}
			var result SearchRunResult
			decodeResult(t, response, &result)
			if result.Scope != "HEAD" || result.AllRefs || result.Count != 1 || len(result.Results) != 1 {
				t.Fatalf("search result = %#v", result)
			}
			match := result.Results[0]
			if match.Message != test.wantMessage || !reflect.DeepEqual(match.MatchSources, test.wantSources) {
				t.Fatalf("match = %#v, want message %q sources %#v", match, test.wantMessage, test.wantSources)
			}
			if match.Commit == "" || match.ShortCommit != shortObjectID(match.Commit) || match.Author.Name == "" || match.Date == "" || match.ChangedFiles == nil || match.MatchedFiles == nil {
				t.Fatalf("incomplete commit-first match: %#v", match)
			}
			if test.wantFileSources == nil {
				if len(match.MatchedFiles) != 0 {
					t.Fatalf("message-only match exposed matched files: %#v", match.MatchedFiles)
				}
			} else if len(match.MatchedFiles) != 1 || !reflect.DeepEqual(match.MatchedFiles[0].MatchSources, test.wantFileSources) {
				t.Fatalf("matched files = %#v, want one file with sources %#v", match.MatchedFiles, test.wantFileSources)
			}
			assertTerminalProgress(t, progress, false)
		})
	}
}

func TestSearchRunLimitIsCommitBasedAndReportsHasMore(t *testing.T) {
	fixture := newMethodFixture(t)
	messages := serveRPCMessages(t, fixture.server, "search.run", map[string]any{
		"repositoryRoot": fixture.root,
		"patterns":       []map[string]any{{"source": "msg", "value": "*file*"}},
		"limit":          1,
	})
	response, progress := splitRPCMessages(t, messages)
	if response.Error != nil {
		t.Fatalf("search.run error: %#v", response.Error)
	}
	var result SearchRunResult
	decodeResult(t, response, &result)
	if result.Count != 1 || len(result.Results) != 1 || !result.HasMore {
		t.Fatalf("limited result = %#v, want one commit and hasMore", result)
	}
	if len(result.Results[0].MatchedFiles) != 0 || result.Results[0].ChangedFiles == nil {
		t.Fatalf("message-only commit-first files = %#v", result.Results[0])
	}
	assertTerminalProgress(t, progress, false)
}

func TestSearchRunToleratesDesktopSnakeCaseAliasesAndEmitsCamelCase(t *testing.T) {
	fixture := newMethodFixture(t)
	request := mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      41,
		"method":  "search.run",
		"params": map[string]any{
			"repository_root": fixture.root,
			"patterns": []map[string]any{
				{"source": "msg", "value": "*does-not-match*", "open_groups": 1},
				{"source": "file", "value": "*new.txt*", "join": "or", "close_groups": 1},
			},
			"all_refs":      true,
			"follow_rename": false,
			"limit":         1,
		},
	})
	var output bytes.Buffer
	if err := fixture.server.Serve(context.Background(), strings.NewReader(request+"\n"), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeNDJSONMessages(t, output.String())
	response, progress := splitRPCMessages(t, messages)
	if response.Error != nil {
		t.Fatalf("search.run error: %#v", response.Error)
	}
	assertTerminalProgress(t, progress, false)

	var wire map[string]any
	if err := json.Unmarshal(response.Result, &wire); err != nil {
		t.Fatalf("decode search wire result: %v", err)
	}
	if _, ok := wire["hasMore"].(bool); !ok || wire["allRefs"] != true || wire["scope"] != "All refs" {
		t.Fatalf("camelCase scope fields = %#v", wire)
	}
	if _, exists := wire["all_refs"]; exists {
		t.Fatalf("snake_case leaked into search response: %#v", wire)
	}
	results, ok := wire["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("wire results = %#v", wire["results"])
	}
	match := results[0].(map[string]any)
	if _, ok := match["shortCommit"]; !ok {
		t.Fatalf("shortCommit missing: %#v", match)
	}
	if _, ok := match["matchSources"]; !ok {
		t.Fatalf("matchSources missing: %#v", match)
	}
	for _, required := range []string{"matchedFiles", "changedFiles"} {
		if _, ok := match[required]; !ok {
			t.Fatalf("%s missing: %#v", required, match)
		}
	}
	for _, forbidden := range []string{"short_commit", "match_sources", "file", "files", "diff"} {
		if _, exists := match[forbidden]; exists {
			t.Fatalf("snake_case %q leaked into match: %#v", forbidden, match)
		}
	}
}

func TestProgressReporterCapsUpdatesAndAddsTerminalDone(t *testing.T) {
	var output bytes.Buffer
	sink := &responseSink{writer: &output}
	reporter := newProgressReporter(json.RawMessage("99"), sink)
	for index := 1; index <= 1_000; index++ {
		reporter.report(appProgress(index, 1_000))
	}
	reporter.done(1_000, false)
	reporter.done(1_000, false)

	messages := decodeNDJSONMessages(t, output.String())
	if len(messages) != maxProgressUpdates+1 {
		t.Fatalf("progress message count = %d, want %d", len(messages), maxProgressUpdates+1)
	}
	for index, message := range messages {
		if message.Method != "$/progress" || string(message.Params.RequestID) != "99" {
			t.Fatalf("progress %d = %#v", index, message)
		}
		if index < len(messages)-1 && message.Params.Done {
			t.Fatalf("non-terminal progress marked done: %#v", message)
		}
	}
	last := messages[len(messages)-1]
	if !last.Params.Done || last.Params.Cancelled || last.Params.Scanned != 1_000 || last.Params.Total != 1_000 {
		t.Fatalf("terminal progress = %#v", last)
	}
}

func TestSearchRunCancellationDiscardsPartialResult(t *testing.T) {
	repositoryRoot := t.TempDir()
	started := make(chan struct{})
	server := newServer(func(ctx context.Context) (gitInstallation, error) {
		close(started)
		<-ctx.Done()
		return gitInstallation{}, ctx.Err()
	})
	reader, writer := ioPipe(t)
	type outcome struct {
		output string
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		var output bytes.Buffer
		err := server.Serve(context.Background(), reader, &output)
		done <- outcome{output: output.String(), err: err}
	}()

	request := mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      72,
		"method":  "search.run",
		"params": map[string]any{
			"repositoryRoot": repositoryRoot,
			"patterns":       []map[string]any{{"source": "msg", "value": "*match*"}},
		},
	})
	if _, err := fmt.Fprintln(writer, request); err != nil {
		t.Fatalf("write search request: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("search request did not start")
	}
	if _, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":72}}`); err != nil {
		t.Fatalf("write cancel request: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close request stream: %v", err)
	}

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("serve: %v", outcome.err)
		}
		messages := decodeNDJSONMessages(t, outcome.output)
		response, progress := splitRPCMessages(t, messages)
		if response.Error == nil || response.Error.Data.Code != "request_cancelled" {
			t.Fatalf("cancel response = %#v", response)
		}
		if len(response.Result) != 0 && string(response.Result) != "null" {
			t.Fatalf("cancelled search leaked a partial result: %s", response.Result)
		}
		assertTerminalProgress(t, progress, true)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled search did not finish")
	}
}

func TestDiffFileRootModificationDeletionRenameAndBinary(t *testing.T) {
	fixture := newDiffFixture(t)
	tests := []struct {
		name        string
		commit      string
		path        string
		oldPath     string
		wantParent  string
		wantRanges  []TargetRange
		wantAnchors []DeletionAnchor
		contains    string
	}{
		{
			name:        "root commit",
			commit:      fixture.rootCommit,
			path:        "file.txt",
			wantParent:  "",
			wantRanges:  []TargetRange{{StartLine: 1, EndLine: 3}},
			wantAnchors: []DeletionAnchor{},
			contains:    "new file mode",
		},
		{
			name:        "modification",
			commit:      fixture.modifyCommit,
			path:        "file.txt",
			wantParent:  fixture.rootCommit,
			wantRanges:  []TargetRange{{StartLine: 2, EndLine: 2}, {StartLine: 4, EndLine: 4}},
			wantAnchors: []DeletionAnchor{{AnchorLine: 2}},
			contains:    "+BETA",
		},
		{
			name:        "deletion",
			commit:      fixture.deleteCommit,
			path:        "file.txt",
			wantParent:  fixture.modifyCommit,
			wantRanges:  []TargetRange{},
			wantAnchors: []DeletionAnchor{{AnchorLine: 3}},
			contains:    "-gamma",
		},
		{
			name:        "rename",
			commit:      fixture.renameCommit,
			path:        "renamed.txt",
			oldPath:     "file.txt",
			wantParent:  fixture.deleteCommit,
			wantRanges:  []TargetRange{},
			wantAnchors: []DeletionAnchor{},
			contains:    "rename from file.txt",
		},
		{
			name:        "binary",
			commit:      fixture.binaryCommit,
			path:        "binary.dat",
			wantParent:  fixture.renameCommit,
			wantRanges:  []TargetRange{},
			wantAnchors: []DeletionAnchor{},
			contains:    "Binary files",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         test.commit,
				"path":           test.path,
				"context":        1,
			}
			if test.oldPath != "" {
				params["oldPath"] = test.oldPath
			}
			response := serveRPC(t, fixture.server, "diff.file", params)
			if response.Error != nil {
				t.Fatalf("diff.file error: %#v", response.Error)
			}
			var result DiffFileResult
			decodeResult(t, response, &result)
			if result.Parent != test.wantParent || !reflect.DeepEqual(result.TargetRanges, test.wantRanges) || !reflect.DeepEqual(result.DeletionAnchors, test.wantAnchors) {
				t.Fatalf("diff result = %#v, want parent=%q ranges=%#v anchors=%#v", result, test.wantParent, test.wantRanges, test.wantAnchors)
			}
			if !strings.Contains(result.Diff, test.contains) {
				t.Fatalf("diff does not contain %q:\n%s", test.contains, result.Diff)
			}
		})
	}
}

func TestDiffFileDisablesConfiguredTextconv(t *testing.T) {
	fixture := newDiffFixture(t)
	writeFixtureFile(t, filepath.Join(fixture.root, ".gitattributes"), []byte("textconv.txt diff=gitgittextconv\n"))
	writeFixtureFile(t, filepath.Join(fixture.root, "textconv.txt"), []byte("before textconv\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", ".gitattributes", "textconv.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "add textconv fixture")

	runGit(t, fixture.gitPath, fixture.root, "config", "diff.gitgittextconv.textconv", "git gitgit-textconv-must-not-run")
	writeFixtureFile(t, filepath.Join(fixture.root, "textconv.txt"), []byte("after textconv\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "textconv.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "update textconv fixture")
	commit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         commit,
		"path":           "textconv.txt",
	})
	if response.Error != nil {
		t.Fatalf("diff.file executed configured textconv: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if !strings.Contains(result.Diff, "-before textconv") || !strings.Contains(result.Diff, "+after textconv") {
		t.Fatalf("diff.file did not return the raw file diff:\n%s", result.Diff)
	}
}

func TestDiffFileTreatsRenamePathsAsLiteralPathspecs(t *testing.T) {
	fixture := newDiffFixture(t)
	oldPath := "old[ab].txt"
	newPath := "new[ab].txt"
	writeFixtureFile(t, filepath.Join(fixture.root, oldPath), []byte("literal rename\n"))
	writeFixtureFile(t, filepath.Join(fixture.root, "olda.txt"), []byte("old wildcard neighbor\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", oldPath, "olda.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "add literal pathspec fixture")

	runGit(t, fixture.gitPath, fixture.root, "mv", oldPath, newPath)
	writeFixtureFile(t, filepath.Join(fixture.root, "olda.txt"), []byte("changed old wildcard neighbor\n"))
	writeFixtureFile(t, filepath.Join(fixture.root, "newa.txt"), []byte("new wildcard neighbor\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "olda.txt", "newa.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "rename literal pathspec fixture")
	commit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         commit,
		"path":           newPath,
		"oldPath":        oldPath,
	})
	if response.Error != nil {
		t.Fatalf("diff.file error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if !strings.Contains(result.Diff, "rename from "+oldPath) || !strings.Contains(result.Diff, "rename to "+newPath) {
		t.Fatalf("literal rename is missing from diff:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "wildcard neighbor") || strings.Contains(result.Diff, "newa.txt") || strings.Contains(result.Diff, "olda.txt") {
		t.Fatalf("wildcard pathspec leaked neighboring files into diff:\n%s", result.Diff)
	}
}

func TestDiffFileDoesNotLeakDescendantsOfRenamedPath(t *testing.T) {
	fixture := newDiffFixture(t)
	writeFixtureFile(t, filepath.Join(fixture.root, "foo"), []byte("renamed content\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "foo")
	commitFixture(t, fixture.gitPath, fixture.root, "add prefix fixture")

	runGit(t, fixture.gitPath, fixture.root, "mv", "foo", "renamed-prefix-source")
	if err := os.Mkdir(filepath.Join(fixture.root, "foo"), 0o700); err != nil {
		t.Fatalf("create renamed prefix directory: %v", err)
	}
	runGit(t, fixture.gitPath, fixture.root, "mv", "renamed-prefix-source", "foo/bar")
	writeFixtureFile(t, filepath.Join(fixture.root, "foo", "baz"), []byte("must not leak\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "foo/baz")
	commitFixture(t, fixture.gitPath, fixture.root, "rename prefix fixture")
	commit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         commit,
		"path":           "foo/bar",
		"oldPath":        "foo",
	})
	if response.Error != nil {
		t.Fatalf("diff.file error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if !strings.Contains(result.Diff, "rename from foo") || !strings.Contains(result.Diff, "rename to foo/bar") {
		t.Fatalf("selected rename is missing from diff:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "foo/baz") || strings.Contains(result.Diff, "must not leak") {
		t.Fatalf("descendant-prefix path leaked into diff:\n%s", result.Diff)
	}
}

func TestDiffFileReturnsDeletionWhenFileBecomesDirectory(t *testing.T) {
	fixture := newDiffFixture(t)
	writeFixtureFile(t, filepath.Join(fixture.root, "foo"), []byte("deleted preimage\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "foo")
	commitFixture(t, fixture.gitPath, fixture.root, "add file directory fixture")
	parent := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	runGit(t, fixture.gitPath, fixture.root, "rm", "foo")
	if err := os.Mkdir(filepath.Join(fixture.root, "foo"), 0o700); err != nil {
		t.Fatalf("create replacement directory: %v", err)
	}
	writeFixtureFile(t, filepath.Join(fixture.root, "foo", "bar"), []byte("replacement descendant\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "foo/bar")
	commitFixture(t, fixture.gitPath, fixture.root, "replace file with directory")
	commit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         commit,
		"path":           "foo",
	})
	if response.Error != nil {
		t.Fatalf("diff.file error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if result.Parent != parent || !strings.Contains(result.Diff, "deleted file mode") || !strings.Contains(result.Diff, "-deleted preimage") {
		t.Fatalf("file-to-directory deletion = %#v\n%s", result, result.Diff)
	}
	if strings.Contains(result.Diff, "foo/bar") || strings.Contains(result.Diff, "replacement descendant") {
		t.Fatalf("replacement descendant leaked into deletion diff:\n%s", result.Diff)
	}
}

func TestDiffFileReturnsFirstParentMergeChange(t *testing.T) {
	fixture := newDiffFixture(t)
	writeFixtureFile(t, filepath.Join(fixture.root, "merge.txt"), []byte("before merge\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "merge.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "add merge fixture")
	mainBranch := gitOutput(t, fixture.gitPath, fixture.root, "branch", "--show-current")
	runGit(t, fixture.gitPath, fixture.root, "branch", "merge-side")

	writeFixtureFile(t, filepath.Join(fixture.root, "main-only.txt"), []byte("first parent\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "main-only.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "advance first parent")
	firstParent := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	runGit(t, fixture.gitPath, fixture.root, "checkout", "merge-side")
	writeFixtureFile(t, filepath.Join(fixture.root, "merge.txt"), []byte("after merge\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "merge.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "change merge fixture")
	runGit(t, fixture.gitPath, fixture.root, "checkout", mainBranch)
	runGit(t, fixture.gitPath, fixture.root, "-c", "commit.gpgsign=false", "merge", "--no-ff", "--no-edit", "merge-side")
	mergeCommit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         mergeCommit,
		"path":           "merge.txt",
	})
	if response.Error != nil {
		t.Fatalf("diff.file merge error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if result.Parent != firstParent || !strings.Contains(result.Diff, "-before merge") || !strings.Contains(result.Diff, "+after merge") {
		t.Fatalf("first-parent merge diff = %#v\n%s", result, result.Diff)
	}
}

func TestMergeFileChangesExcludeSecondParentOnlyPaths(t *testing.T) {
	fixture := newDiffFixture(t)
	mainBranch := gitOutput(t, fixture.gitPath, fixture.root, "branch", "--show-current")
	runGit(t, fixture.gitPath, fixture.root, "branch", "second-parent")

	writeFixtureFile(t, filepath.Join(fixture.root, "second-parent-diff-only.txt"), []byte("second parent comparison only\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "second-parent-diff-only.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "advance first parent for isolation")
	firstParent := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	runGit(t, fixture.gitPath, fixture.root, "checkout", "second-parent")
	writeFixtureFile(t, filepath.Join(fixture.root, "first-parent-diff.txt"), []byte("first parent diff\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", "first-parent-diff.txt")
	commitFixture(t, fixture.gitPath, fixture.root, "add second parent only path")
	runGit(t, fixture.gitPath, fixture.root, "checkout", mainBranch)
	runGit(t, fixture.gitPath, fixture.root, "-c", "commit.gpgsign=false", "merge", "--no-ff", "--no-edit", "second-parent")
	mergeCommit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	repository, openError := gitexec.OpenRepository(context.Background(), &gitexec.Runner{Binary: fixture.gitPath}, fixture.root)
	if openError != nil {
		t.Fatalf("open fixture repository: %v", openError)
	}
	filesByCommit, responseError := readCommitFilesBatch(context.Background(), repository, []string{mergeCommit})
	if responseError != nil {
		t.Fatalf("readCommitFilesBatch error: %#v", responseError)
	}
	changes := filesByCommit[mergeCommit]
	if len(changes) != 1 || changes[0].Status != "A" || changes[0].Path != "first-parent-diff.txt" {
		t.Fatalf("first-parent merge changes = %#v", changes)
	}

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         mergeCommit,
		"path":           "second-parent-diff-only.txt",
	})
	if response.Error == nil || response.Error.Data.Code != "diff_not_found" {
		t.Fatalf("second-parent-only comparison leaked into diff.file: %#v", response.Error)
	}
	response = serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         mergeCommit,
		"path":           "first-parent-diff.txt",
	})
	if response.Error != nil {
		t.Fatalf("first-parent merge addition error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if result.Parent != firstParent || !strings.Contains(result.Diff, "+first parent diff") {
		t.Fatalf("first-parent merge addition = %#v\n%s", result, result.Diff)
	}
}

func TestReadCommitFilesBatchIncludesRootChanges(t *testing.T) {
	fixture := newDiffFixture(t)
	repository, err := gitexec.OpenRepository(context.Background(), &gitexec.Runner{Binary: fixture.gitPath}, fixture.root)
	if err != nil {
		t.Fatalf("open fixture repository: %v", err)
	}
	filesByCommit, responseError := readCommitFilesBatch(context.Background(), repository, []string{fixture.rootCommit})
	if responseError != nil {
		t.Fatalf("readCommitFilesBatch root error: %#v", responseError)
	}
	changes := filesByCommit[fixture.rootCommit]
	if len(changes) != 1 || changes[0].Status != "A" || changes[0].Path != "file.txt" {
		t.Fatalf("root changes = %#v", changes)
	}
}

func TestDiffFileReturnsBlobToGitlinkTypeChange(t *testing.T) {
	fixture := newDiffFixture(t)
	const path = "type-change.txt"
	writeFixtureFile(t, filepath.Join(fixture.root, path), []byte("blob before gitlink\n"))
	runGit(t, fixture.gitPath, fixture.root, "add", path)
	commitFixture(t, fixture.gitPath, fixture.root, "add type change blob")
	parent := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	runGit(t, fixture.gitPath, fixture.root, "rm", path)
	runGit(
		t,
		fixture.gitPath,
		fixture.root,
		"update-index",
		"--add",
		"--cacheinfo",
		"160000,"+parent+","+path,
	)
	commitFixture(t, fixture.gitPath, fixture.root, "replace blob with gitlink")
	commit := gitOutput(t, fixture.gitPath, fixture.root, "rev-parse", "HEAD")

	response := serveRPC(t, fixture.server, "diff.file", map[string]any{
		"repositoryRoot": fixture.root,
		"commit":         commit,
		"path":           path,
	})
	if response.Error != nil {
		t.Fatalf("diff.file type change error: %#v", response.Error)
	}
	var result DiffFileResult
	decodeResult(t, response, &result)
	if result.Parent != parent {
		t.Fatalf("type change parent = %q, want %q", result.Parent, parent)
	}
	if strings.Count(result.Diff, "diff --git a/"+path+" b/"+path) != 2 ||
		!strings.Contains(result.Diff, "deleted file mode 100644") ||
		!strings.Contains(result.Diff, "new file mode 160000") ||
		!strings.Contains(result.Diff, "-blob before gitlink") ||
		!strings.Contains(result.Diff, "+Subproject commit "+parent) {
		t.Fatalf("blob-to-gitlink diff is incomplete:\n%s", result.Diff)
	}
}

func TestSelectExactFileDiffRejectsMalformedRawPatchOutput(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
	}{
		{name: "missing boundary", output: []byte(":100644 100644 old new M\x00file.txt\x00")},
		{name: "malformed raw", output: []byte("invalid\x00path\x00\x00diff --git a/path b/path\n")},
		{name: "patch count mismatch", output: []byte(":100644 100644 old new M\x00file.txt\x00\x00")},
	}
	selected := FileChange{Status: "M", Path: "file.txt"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := selectExactFileDiff(test.output, selected); err == nil {
				t.Fatal("selectExactFileDiff accepted malformed Git output")
			}
		})
	}
}

func TestDiffFileRejectsInjectionEscapeMissingAndOversizedOutput(t *testing.T) {
	fixture := newDiffFixture(t)
	tests := []struct {
		name       string
		params     map[string]any
		stableCode string
	}{
		{
			name: "commit option",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         "--all",
				"path":           "renamed.txt",
			},
			stableCode: "invalid_revision",
		},
		{
			name: "path traversal",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         fixture.renameCommit,
				"path":           "../renamed.txt",
			},
			stableCode: "invalid_file_path",
		},
		{
			name: "old path option",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         fixture.renameCommit,
				"path":           "renamed.txt",
				"oldPath":        `C:\escape.txt`,
			},
			stableCode: "invalid_file_path",
		},
		{
			name: "not changed",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         fixture.binaryCommit,
				"path":           "renamed.txt",
			},
			stableCode: "diff_not_found",
		},
		{
			name: "output bound",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"commit":         fixture.largeCommit,
				"path":           "large.txt",
			},
			stableCode: "output_too_large",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveRPC(t, fixture.server, "diff.file", test.params)
			if response.Error == nil || response.Error.Data.Code != test.stableCode {
				t.Fatalf("error = %#v, want data.code %q", response.Error, test.stableCode)
			}
		})
	}
}

type decodedMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  ProgressParams  `json:"params"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

func appProgress(scanned, total int) app.SearchProgress {
	return app.SearchProgress{Scanned: scanned, Total: total}
}

func serveRPCMessages(t *testing.T, server *Server, method string, params any) []decodedMessage {
	t.Helper()
	request := mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(request+"\n"), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return decodeNDJSONMessages(t, output.String())
}

func decodeNDJSONMessages(t *testing.T, output string) []decodedMessage {
	t.Helper()
	messages := make([]decodedMessage, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message decodedMessage
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("decode NDJSON message: %v\n%s", err, line)
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan NDJSON output: %v", err)
	}
	return messages
}

func splitRPCMessages(t *testing.T, messages []decodedMessage) (wireResponse, []decodedMessage) {
	t.Helper()
	var response wireResponse
	responseCount := 0
	progress := make([]decodedMessage, 0)
	for _, message := range messages {
		if message.Method == "$/progress" {
			progress = append(progress, message)
			continue
		}
		if len(message.ID) != 0 {
			responseCount++
			response = wireResponse{
				JSONRPC: message.JSONRPC,
				ID:      message.ID,
				Result:  message.Result,
				Error:   message.Error,
			}
		}
	}
	if responseCount != 1 {
		t.Fatalf("response count = %d, messages = %#v", responseCount, messages)
	}
	return response, progress
}

func assertTerminalProgress(t *testing.T, progress []decodedMessage, cancelled bool) {
	t.Helper()
	if len(progress) == 0 || len(progress) > maxProgressUpdates+1 {
		t.Fatalf("progress count = %d", len(progress))
	}
	terminal := progress[len(progress)-1].Params
	if !terminal.Done || terminal.Cancelled != cancelled {
		t.Fatalf("terminal progress = %#v, want cancelled=%v", terminal, cancelled)
	}
	for _, message := range progress[:len(progress)-1] {
		if message.Params.Done {
			t.Fatalf("intermediate progress marked done: %#v", message)
		}
	}
}

type diffFixture struct {
	root         string
	gitPath      string
	server       *Server
	rootCommit   string
	modifyCommit string
	deleteCommit string
	renameCommit string
	binaryCommit string
	largeCommit  string
}

func newDiffFixture(t *testing.T) diffFixture {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	gitPath, err = filepath.Abs(gitPath)
	if err != nil {
		t.Fatalf("resolve Git path: %v", err)
	}
	root := filepath.Join(t.TempDir(), "repository")
	runGit(t, gitPath, "", "init", root)
	runGit(t, gitPath, root, "config", "user.name", "GitGit Test")
	runGit(t, gitPath, root, "config", "user.email", "gitgit@example.invalid")

	writeFixtureFile(t, filepath.Join(root, "file.txt"), []byte("alpha\nbeta\ngamma\n"))
	runGit(t, gitPath, root, "add", "file.txt")
	commitFixture(t, gitPath, root, "root file")
	rootCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	writeFixtureFile(t, filepath.Join(root, "file.txt"), []byte("alpha\nBETA\ngamma\ndelta\n"))
	runGit(t, gitPath, root, "add", "file.txt")
	commitFixture(t, gitPath, root, "modify file")
	modifyCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	writeFixtureFile(t, filepath.Join(root, "file.txt"), []byte("alpha\nBETA\ndelta\n"))
	runGit(t, gitPath, root, "add", "file.txt")
	commitFixture(t, gitPath, root, "delete line")
	deleteCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	runGit(t, gitPath, root, "mv", "file.txt", "renamed.txt")
	commitFixture(t, gitPath, root, "rename file")
	renameCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	writeFixtureFile(t, filepath.Join(root, "binary.dat"), []byte{'b', 'i', 'n', 0, 'a', 'r', 'y'})
	runGit(t, gitPath, root, "add", "binary.dat")
	commitFixture(t, gitPath, root, "add binary")
	binaryCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	large := bytes.Repeat([]byte{'x'}, maxTextBytes+1)
	writeFixtureFile(t, filepath.Join(root, "large.txt"), large)
	runGit(t, gitPath, root, "add", "large.txt")
	commitFixture(t, gitPath, root, "add large")
	largeCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	server := newServer(func(context.Context) (gitInstallation, error) {
		return gitInstallation{Path: gitPath, Source: "test", Version: "test"}, nil
	})
	return diffFixture{
		root:         canonicalPath(root),
		gitPath:      gitPath,
		server:       server,
		rootCommit:   rootCommit,
		modifyCommit: modifyCommit,
		deleteCommit: deleteCommit,
		renameCommit: renameCommit,
		binaryCommit: binaryCommit,
		largeCommit:  largeCommit,
	}
}
