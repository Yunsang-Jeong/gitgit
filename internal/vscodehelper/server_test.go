package vscodehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type protocolManifest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Transport       string   `json:"transport"`
	LineBase        int      `json:"lineBase"`
	ReadOnly        bool     `json:"readOnly"`
	Network         bool     `json:"network"`
	MaxOutputBytes  int      `json:"maxOutputBytes"`
	Methods         []string `json:"methods"`
	Notifications   []string `json:"notifications"`
}

type wireResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

func TestProtocolMatchesCanonicalManifest(t *testing.T) {
	t.Parallel()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	manifestPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "testdata", "vscode-contract", "protocol-v1.json")
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read canonical manifest: %v", err)
	}
	var manifest protocolManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode canonical manifest: %v", err)
	}

	if manifest.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocolVersion = %d, want %d", ProtocolVersion, manifest.ProtocolVersion)
	}
	if manifest.Transport != Transport {
		t.Fatalf("transport = %q, want %q", Transport, manifest.Transport)
	}
	if manifest.LineBase != LineBase {
		t.Fatalf("lineBase = %d, want %d", LineBase, manifest.LineBase)
	}
	if manifest.ReadOnly != ReadOnly {
		t.Fatalf("readOnly = %v, want %v", ReadOnly, manifest.ReadOnly)
	}
	if manifest.Network != Network {
		t.Fatalf("network = %v, want %v", Network, manifest.Network)
	}
	if manifest.MaxOutputBytes != MaxOutputBytes {
		t.Fatalf("maxOutputBytes = %d, want %d", MaxOutputBytes, manifest.MaxOutputBytes)
	}
	if !reflect.DeepEqual(manifest.Methods, Methods) {
		t.Fatalf("methods = %#v, want %#v", Methods, manifest.Methods)
	}
	if !reflect.DeepEqual(manifest.Notifications, Notifications) {
		t.Fatalf("notifications = %#v, want %#v", Notifications, manifest.Notifications)
	}
}

func TestInitializeAcceptsNumericAndStringProtocolVersions(t *testing.T) {
	t.Parallel()

	for _, version := range []string{`1`, `"1.0"`} {
		version := version
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			server := newServer(func(context.Context) (gitInstallation, error) {
				return gitInstallation{
					Path:    "/test/bin/git",
					Source:  "test",
					Version: "git version test",
				}, nil
			})
			response := serveSingle(t, server, fmt.Sprintf(
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%s}}`,
				version,
			))
			if response.Error != nil {
				t.Fatalf("initialize error: %#v", response.Error)
			}

			var result InitializeResult
			if err := json.Unmarshal(response.Result, &result); err != nil {
				t.Fatalf("decode initialize result: %v", err)
			}
			if result.ProtocolVersion != ProtocolVersion || !result.ReadOnly || result.Network {
				t.Fatalf("unexpected flat capabilities: %#v", result)
			}
			if result.LineBase != LineBase || result.MaxOutputBytes != MaxOutputBytes {
				t.Fatalf("unexpected line/output contract: %#v", result)
			}
			if !reflect.DeepEqual(result.Methods, Methods) {
				t.Fatalf("initialize methods = %#v, want %#v", result.Methods, Methods)
			}
			if !result.Capabilities.ReadOnly || result.Capabilities.Network {
				t.Fatalf("unexpected nested capabilities: %#v", result.Capabilities)
			}
			if !result.Diagnostics.Available || result.Diagnostics.ExecutableSource != "test" {
				t.Fatalf("unexpected Git diagnostics: %#v", result.Diagnostics)
			}
		})
	}
}

func TestRepositoryDiscoverReturnsCanonicalRepositoryMetadata(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	gitPath, err = filepath.Abs(gitPath)
	if err != nil {
		t.Fatalf("resolve Git path: %v", err)
	}

	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	runGit(t, gitPath, "", "init", repositoryRoot)
	runGit(t, gitPath, repositoryRoot, "config", "user.name", "GitGit Test")
	runGit(t, gitPath, repositoryRoot, "config", "user.email", "gitgit@example.invalid")
	if err := os.WriteFile(filepath.Join(repositoryRoot, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	runGit(t, gitPath, repositoryRoot, "add", "README.md")
	runGit(t, gitPath, repositoryRoot, "-c", "commit.gpgsign=false", "commit", "--no-gpg-sign", "-m", "initial")
	nested := filepath.Join(repositoryRoot, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	server := newServer(func(context.Context) (gitInstallation, error) {
		return gitInstallation{Path: gitPath, Source: "test", Version: "test"}, nil
	})
	response := serveSingle(t, server, fmt.Sprintf(
		`{"jsonrpc":"2.0","id":"discover","method":"repository.discover","params":{"path":%s}}`,
		mustJSON(t, nested),
	))
	if response.Error != nil {
		t.Fatalf("repository.discover error: %#v", response.Error)
	}

	var result RepositoryDiscoverResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode repository.discover result: %v", err)
	}
	wantRoot := canonicalPath(repositoryRoot)
	if result.Root != wantRoot {
		t.Fatalf("root = %q, want %q", result.Root, wantRoot)
	}
	if result.CommonDir == "" || result.GitDir == "" || result.Branch == "" || result.Head == "" {
		t.Fatalf("incomplete repository metadata: %#v", result)
	}
	if len(result.Head) != 40 {
		t.Fatalf("head = %q, want a full object id", result.Head)
	}
	for _, path := range []string{result.CommonDir, result.GitDir} {
		if !filepath.IsAbs(path) {
			t.Fatalf("metadata path is not absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("metadata path %q does not exist: %v", path, err)
		}
	}
}

func TestRepositoryDiscoverRejectsUnsafeOrInvalidPaths(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	server := newServer(func(context.Context) (gitInstallation, error) {
		return gitInstallation{Path: gitPath, Source: "test", Version: "test"}, nil
	})

	filePath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write path fixture: %v", err)
	}
	for _, test := range []struct {
		name       string
		path       string
		stableCode string
	}{
		{name: "relative", path: "../repository", stableCode: "invalid_repository_path"},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing"), stableCode: "invalid_repository_path"},
		{name: "file", path: filePath, stableCode: "invalid_repository_path"},
		{name: "not repository", path: t.TempDir(), stableCode: "not_git_repository"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := serveSingle(t, server, fmt.Sprintf(
				`{"jsonrpc":"2.0","id":1,"method":"repository.discover","params":{"path":%s}}`,
				mustJSON(t, test.path),
			))
			if response.Error == nil {
				t.Fatal("repository.discover unexpectedly succeeded")
			}
			if response.Error.Data.Code != test.stableCode {
				t.Fatalf("error data.code = %q, want %q", response.Error.Data.Code, test.stableCode)
			}
		})
	}
}

func TestCancelRequestCancelsInFlightRequest(t *testing.T) {
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

	if _, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":1}}`); err != nil {
		t.Fatalf("write initialize request: %v", err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initialize request did not start")
	}
	if _, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":7}}`); err != nil {
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
		t.Fatal("cancelled request did not finish")
	}
}

func TestUnknownMethodHasStableMethodNotFoundError(t *testing.T) {
	t.Parallel()

	server := newServer(func(context.Context) (gitInstallation, error) {
		return gitInstallation{}, nil
	})
	response := serveSingle(t, server, `{"jsonrpc":"2.0","id":1,"method":"git.raw","params":{}}`)
	if response.Error == nil || response.Error.Data.Code != "method_not_found" {
		t.Fatalf("response error = %#v", response.Error)
	}
}

func TestDiscoverGitExecutableFindsWindowsInstallations(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		environment string
		relative    []string
	}{
		{
			name:        "machine installation",
			environment: "ProgramFiles",
			relative:    []string{"Git", "cmd", "git.exe"},
		},
		{
			name:        "per-user installation",
			environment: "LocalAppData",
			relative:    []string{"Programs", "Git", "cmd", "git.exe"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			gitPath := filepath.Join(append([]string{base}, testCase.relative...)...)
			if err := os.MkdirAll(filepath.Dir(gitPath), 0o755); err != nil {
				t.Fatalf("create Git directory: %v", err)
			}
			if err := os.WriteFile(gitPath, []byte("test Git executable"), 0o600); err != nil {
				t.Fatalf("create Git executable: %v", err)
			}

			getenv := func(key string) string {
				if key == testCase.environment {
					return base
				}
				return ""
			}
			resolved, source, err := discoverGitExecutable(
				"windows",
				getenv,
				func(string) (string, error) { return "", exec.ErrNotFound },
			)
			if err != nil {
				t.Fatalf("discover Git: %v", err)
			}
			want, err := filepath.EvalSymlinks(gitPath)
			if err != nil {
				t.Fatalf("resolve expected Git path: %v", err)
			}
			if resolved != want || source != testCase.environment {
				t.Fatalf("Git discovery = (%q, %q), want (%q, %q)", resolved, source, want, testCase.environment)
			}
		})
	}
}

func serveSingle(t *testing.T, server *Server, request string) wireResponse {
	t.Helper()
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(request+"\n"), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return decodeSingleResponse(t, output.String())
}

func decodeSingleResponse(t *testing.T, output string) wireResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("response lines = %#v, want one", lines)
	}
	var response wireResponse
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return string(payload)
}

func runGit(t *testing.T, gitPath, directory string, args ...string) {
	t.Helper()
	command := exec.Command(gitPath, args...)
	if directory != "" {
		command.Dir = directory
	}
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func ioPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create request pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return reader, writer
}
