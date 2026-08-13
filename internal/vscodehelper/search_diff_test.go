package vscodehelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yunsang/gitgit/internal/app"
)

func TestSearchRunMatchesMessageDiffFileAndGroupedExpression(t *testing.T) {
	fixture := newMethodFixture(t)
	tests := []struct {
		name        string
		patterns    []map[string]any
		wantMessage string
		wantSources []string
	}{
		{
			name:        "message",
			patterns:    []map[string]any{{"source": "msg", "value": "*rename file*"}},
			wantMessage: "rename file",
			wantSources: []string{"msg"},
		},
		{
			name:        "diff",
			patterns:    []map[string]any{{"source": "diff", "value": "*third updated*"}},
			wantMessage: "update new file",
			wantSources: []string{"diff"},
		},
		{
			name:        "file",
			patterns:    []map[string]any{{"source": "file", "value": "*new.txt*"}},
			wantMessage: "update new file",
			wantSources: []string{"file"},
		},
		{
			name: "and or groups",
			patterns: []map[string]any{
				{"source": "msg", "value": "*does-not-match*", "openGroups": 1},
				{"source": "file", "value": "*new.txt*", "join": "or", "closeGroups": 1},
				{"source": "diff", "value": "*third updated*", "join": "and"},
			},
			wantMessage: "update new file",
			wantSources: []string{"file", "diff"},
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
			if match.Commit == "" || match.ShortCommit != shortObjectID(match.Commit) || match.Author.Name == "" || match.Date == "" || match.File.Path == "" || match.Files == nil {
				t.Fatalf("incomplete Desktop-shaped match: %#v", match)
			}
			assertTerminalProgress(t, progress, false)
		})
	}
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
	if wire["allRefs"] != true || wire["scope"] != "All refs" {
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
	for _, forbidden := range []string{"short_commit", "match_sources"} {
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
		server:       server,
		rootCommit:   rootCommit,
		modifyCommit: modifyCommit,
		deleteCommit: deleteCommit,
		renameCommit: renameCommit,
		binaryCommit: binaryCommit,
		largeCommit:  largeCommit,
	}
}
