package vscodehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type methodFixture struct {
	root         string
	gitPath      string
	head         string
	fileCommit   string
	fileContents string
	server       *Server
}

func TestRefsListReturnsLocalRemoteAndCurrentRefs(t *testing.T) {
	fixture := newMethodFixture(t)
	response := serveRPC(t, fixture.server, "refs.list", map[string]any{
		"repositoryRoot": fixture.root,
	})
	if response.Error != nil {
		t.Fatalf("refs.list error: %#v", response.Error)
	}
	var result RefsListResult
	decodeResult(t, response, &result)
	if len(result.Refs) < 3 {
		t.Fatalf("refs = %#v, want current, feature, and remote refs", result.Refs)
	}
	currentCount := 0
	foundRemote := false
	for _, ref := range result.Refs {
		if ref.Ref == "" || ref.Ref != ref.FullName || ref.Name == "" || ref.Commit == "" {
			t.Fatalf("incomplete ref: %#v", ref)
		}
		if ref.Current {
			currentCount++
			if ref.Kind != "local" {
				t.Fatalf("current ref is not local: %#v", ref)
			}
		}
		if ref.FullName == "refs/remotes/origin/main" && ref.Kind == "remote" {
			foundRemote = true
		}
	}
	if currentCount != 1 {
		t.Fatalf("current ref count = %d, want 1: %#v", currentCount, result.Refs)
	}
	if !foundRemote {
		t.Fatalf("remote ref missing: %#v", result.Refs)
	}
}

func TestBlameLinesSupportsUnsavedUTF8ContentsAndCommittedMetadata(t *testing.T) {
	fixture := newMethodFixture(t)
	unsaved := "first\nunsaved 한글\nthird updated\n"
	response := serveRPC(t, fixture.server, "blame.lines", map[string]any{
		"repositoryRoot": fixture.root,
		"relativePath":   "new.txt",
		"startLine":      2,
		"endLine":        2,
		"contents":       unsaved,
	})
	if response.Error != nil {
		t.Fatalf("blame.lines unsaved error: %#v", response.Error)
	}
	var unsavedResult BlameLinesResult
	decodeResult(t, response, &unsavedResult)
	if len(unsavedResult.Lines) != 1 || unsavedResult.Lines[0].Line != 2 || unsavedResult.Lines[0].Content != "unsaved 한글" {
		t.Fatalf("unsaved blame = %#v", unsavedResult.Lines)
	}
	if unsavedResult.Lines[0].Commit == "" || unsavedResult.Lines[0].Author.Name == "" || unsavedResult.Lines[0].Message == "" {
		t.Fatalf("unsaved blame metadata is incomplete: %#v", unsavedResult.Lines[0])
	}

	response = serveRPC(t, fixture.server, "blame.lines", map[string]any{
		"repositoryRoot": fixture.root,
		"relativePath":   "new.txt",
		"startLine":      3,
		"endLine":        3,
		"revision":       fixture.head,
	})
	if response.Error != nil {
		t.Fatalf("blame.lines committed error: %#v", response.Error)
	}
	var committedResult BlameLinesResult
	decodeResult(t, response, &committedResult)
	if len(committedResult.Lines) != 1 {
		t.Fatalf("committed blame lines = %#v", committedResult.Lines)
	}
	line := committedResult.Lines[0]
	if line.Commit != fixture.fileCommit || line.Author.Name != "GitGit Test" || line.Author.Email != "gitgit@example.invalid" || line.Date == "" || line.Message != "update new file" {
		t.Fatalf("committed blame metadata = %#v", line)
	}
}

func TestBlameLinesReturnsStableRangeBinaryAndUntrackedErrors(t *testing.T) {
	fixture := newMethodFixture(t)
	for _, test := range []struct {
		name       string
		params     map[string]any
		stableCode string
	}{
		{
			name: "range",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"relativePath":   "new.txt",
				"startLine":      1,
				"endLine":        99,
			},
			stableCode: "invalid_line_range",
		},
		{
			name: "binary",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"relativePath":   "binary.dat",
				"startLine":      1,
				"endLine":        1,
			},
			stableCode: "binary_file",
		},
		{
			name: "untracked",
			params: map[string]any{
				"repositoryRoot": fixture.root,
				"relativePath":   "untracked.txt",
				"startLine":      1,
				"endLine":        1,
			},
			stableCode: "untracked_file",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveRPC(t, fixture.server, "blame.lines", test.params)
			if response.Error == nil || response.Error.Data.Code != test.stableCode {
				t.Fatalf("error = %#v, want data.code %q", response.Error, test.stableCode)
			}
		})
	}
}

func TestHistoryFileFollowsRenameAndReturnsCommitFiles(t *testing.T) {
	fixture := newMethodFixture(t)
	response := serveRPC(t, fixture.server, "history.file", map[string]any{
		"repositoryRoot": fixture.root,
		"relativePath":   "new.txt",
		"limit":          50,
	})
	if response.Error != nil {
		t.Fatalf("history.file error: %#v", response.Error)
	}
	var result HistoryFileResult
	decodeResult(t, response, &result)
	if len(result.Commits) != 3 {
		t.Fatalf("history commits = %#v, want update, rename, and original commits", result.Commits)
	}
	wantMessages := []string{"update new file", "rename file", "add old file"}
	for index, commit := range result.Commits {
		if commit.Message != wantMessages[index] {
			t.Fatalf("commit %d message = %q, want %q", index, commit.Message, wantMessages[index])
		}
		if commit.Commit == "" || commit.ShortCommit == "" || commit.Author.Name == "" || commit.Date == "" || commit.Parents == nil || commit.Files == nil {
			t.Fatalf("incomplete history commit: %#v", commit)
		}
	}
	foundRename := false
	for _, file := range result.Commits[1].Files {
		if strings.HasPrefix(file.Status, "R") && file.OldPath == "old.txt" && file.Path == "new.txt" {
			foundRename = true
		}
	}
	if !foundRename {
		t.Fatalf("rename metadata missing: %#v", result.Commits[1].Files)
	}
}

func TestRevisionContentReturnsBoundedUTF8Text(t *testing.T) {
	fixture := newMethodFixture(t)
	response := serveRPC(t, fixture.server, "revision.content", map[string]any{
		"repositoryRoot": fixture.root,
		"relativePath":   "new.txt",
		"revision":       "HEAD",
	})
	if response.Error != nil {
		t.Fatalf("revision.content error: %#v", response.Error)
	}
	var result RevisionContentResult
	decodeResult(t, response, &result)
	if result.Content != fixture.fileContents {
		t.Fatalf("content = %q, want %q", result.Content, fixture.fileContents)
	}

	binaryResponse := serveRPC(t, fixture.server, "revision.content", map[string]any{
		"repositoryRoot": fixture.root,
		"relativePath":   "binary.dat",
		"revision":       "HEAD",
	})
	if binaryResponse.Error == nil || binaryResponse.Error.Data.Code != "binary_file" {
		t.Fatalf("binary revision error = %#v", binaryResponse.Error)
	}
	if responseError := validateText(make([]byte, maxTextBytes+1)); responseError == nil || responseError.Data.Code != "file_too_large" {
		t.Fatalf("oversized text error = %#v", responseError)
	}
}

func TestFileMethodsRejectTraversalAbsoluteSymlinkAndWindowsEscapes(t *testing.T) {
	fixture := newMethodFixture(t)
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	paths := []string{
		"../secret.txt",
		"dir/../new.txt",
		outsideFile,
		`C:\Windows\secret.txt`,
		`\\server\share\secret.txt`,
	}
	if err := os.Symlink(outside, filepath.Join(fixture.root, "escape")); err == nil {
		paths = append(paths, "escape/secret.txt")
	} else {
		t.Logf("symlink escape case unavailable: %v", err)
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := serveRPC(t, fixture.server, "revision.content", map[string]any{
				"repositoryRoot": fixture.root,
				"relativePath":   path,
				"revision":       "HEAD",
			})
			if response.Error == nil || response.Error.Data.Code != "invalid_file_path" {
				t.Fatalf("path %q error = %#v", path, response.Error)
			}
		})
	}

	nested := filepath.Join(fixture.root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	response := serveRPC(t, fixture.server, "refs.list", map[string]any{"repositoryRoot": nested})
	if response.Error == nil || response.Error.Data.Code != "repository_root_mismatch" {
		t.Fatalf("nested repositoryRoot error = %#v", response.Error)
	}
}

func TestFileMethodsRejectRevisionOptionInjection(t *testing.T) {
	fixture := newMethodFixture(t)
	for _, method := range []string{"blame.lines", "history.file", "revision.content"} {
		t.Run(method, func(t *testing.T) {
			params := map[string]any{
				"repositoryRoot": fixture.root,
				"relativePath":   "new.txt",
				"revision":       "--all",
			}
			if method == "blame.lines" {
				params["startLine"] = 1
				params["endLine"] = 1
			}
			response := serveRPC(t, fixture.server, method, params)
			if response.Error == nil || response.Error.Data.Code != "invalid_revision" {
				t.Fatalf("error = %#v", response.Error)
			}
		})
	}
}

func TestHistoryFileCancellationUsesStandardCancelNotification(t *testing.T) {
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
		"id":      "history-request",
		"method":  "history.file",
		"params": map[string]any{
			"repositoryRoot": repositoryRoot,
			"relativePath":   "file.txt",
		},
	})
	if _, err := fmt.Fprintln(writer, request); err != nil {
		t.Fatalf("write history request: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("history request did not start")
	}
	cancel := mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "$/cancelRequest",
		"params":  map[string]any{"id": "history-request"},
	})
	if _, err := fmt.Fprintln(writer, cancel); err != nil {
		t.Fatalf("write cancellation: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close request stream: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("serve: %v", result.err)
		}
		response := decodeSingleResponse(t, result.output)
		if response.Error == nil || response.Error.Data.Code != "request_cancelled" {
			t.Fatalf("cancellation response = %#v", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled history request did not finish")
	}
}

func newMethodFixture(t *testing.T) methodFixture {
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

	writeFixtureFile(t, filepath.Join(root, "old.txt"), []byte("first\nsecond\nthird\n"))
	runGit(t, gitPath, root, "add", "old.txt")
	commitFixture(t, gitPath, root, "add old file")
	runGit(t, gitPath, root, "mv", "old.txt", "new.txt")
	commitFixture(t, gitPath, root, "rename file")
	fileContents := "first\nsecond\nthird updated\n"
	writeFixtureFile(t, filepath.Join(root, "new.txt"), []byte(fileContents))
	runGit(t, gitPath, root, "add", "new.txt")
	commitFixture(t, gitPath, root, "update new file")
	fileCommit := gitOutput(t, gitPath, root, "rev-parse", "HEAD")

	writeFixtureFile(t, filepath.Join(root, "binary.dat"), []byte{'b', 'i', 'n', 0, 'a', 'r', 'y', '\n'})
	runGit(t, gitPath, root, "add", "binary.dat")
	commitFixture(t, gitPath, root, "add binary")
	head := gitOutput(t, gitPath, root, "rev-parse", "HEAD")
	runGit(t, gitPath, root, "branch", "feature")
	runGit(t, gitPath, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	writeFixtureFile(t, filepath.Join(root, "untracked.txt"), []byte("untracked\n"))

	server := newServer(func(context.Context) (gitInstallation, error) {
		return gitInstallation{Path: gitPath, Source: "test", Version: "test"}, nil
	})
	return methodFixture{
		root:         canonicalPath(root),
		gitPath:      gitPath,
		head:         head,
		fileCommit:   fileCommit,
		fileContents: fileContents,
		server:       server,
	}
}

func writeFixtureFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture %q: %v", path, err)
	}
}

func commitFixture(t *testing.T, gitPath, root, message string) {
	t.Helper()
	runGit(t, gitPath, root, "-c", "commit.gpgsign=false", "commit", "--no-gpg-sign", "-m", message)
}

func gitOutput(t *testing.T, gitPath, root string, args ...string) string {
	t.Helper()
	command := exec.Command(gitPath, args...)
	command.Dir = root
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func serveRPC(t *testing.T, server *Server, method string, params any) wireResponse {
	t.Helper()
	request := mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	})
	return serveSingle(t, server, request)
}

func decodeResult(t *testing.T, response wireResponse, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Result, destination); err != nil {
		t.Fatalf("decode result: %v", err)
	}
}
