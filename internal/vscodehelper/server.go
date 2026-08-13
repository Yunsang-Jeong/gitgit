package vscodehelper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/yunsang/gitgit/internal/gitexec"
)

const (
	codeParseError       = -32700
	codeInvalidRequest   = -32600
	codeMethodNotFound   = -32601
	codeInvalidParams    = -32602
	codeInternalError    = -32603
	codeNotImplemented   = -32001
	codeGitNotFound      = -32010
	codeNotGitRepository = -32011
	codeRequestCancelled = -32800
)

type gitInstallation struct {
	Path    string
	Source  string
	Version string
}

type gitResolver func(context.Context) (gitInstallation, error)

type Server struct {
	resolveGit gitResolver

	mu            sync.Mutex
	cancellations map[string]context.CancelFunc
}

func NewServer() *Server {
	return newServer(resolveGitInstallation)
}

func newServer(resolver gitResolver) *Server {
	return &Server{
		resolveGit:    resolver,
		cancellations: make(map[string]context.CancelFunc),
	}
}

// Serve reads one JSON-RPC 2.0 message per line and writes one response per
// request. Requests run concurrently so a later $/cancelRequest notification
// can cancel an in-flight Git command.
func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	sink := &responseSink{writer: output}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), MaxOutputBytes)

	var requests sync.WaitGroup
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		req, requestError := decodeRequest(line)
		if requestError != nil {
			_ = sink.write(response{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   requestError,
			})
			continue
		}
		if req.Method == "$/cancelRequest" {
			s.handleCancel(req.Params)
			continue
		}

		requestContext, cancel := context.WithCancel(ctx)
		wantsResponse := len(req.ID) != 0
		registered := false
		if wantsResponse {
			registered = s.registerCancellation(req.ID, cancel)
			if !registered {
				cancel()
				_ = sink.write(response{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: rpcError(
						codeInvalidRequest,
						"duplicate_request_id",
						"request id is already in flight",
					),
				})
				continue
			}
		}

		requests.Add(1)
		go func(req request, requestContext context.Context, cancel context.CancelFunc, registered bool) {
			defer requests.Done()
			defer cancel()
			if registered {
				defer s.finishRequest(req.ID)
			}

			result, responseError := s.handle(requestContext, req, sink)
			if len(req.ID) == 0 {
				return
			}
			if responseError != nil {
				result = nil
			}
			_ = sink.write(response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
				Error:   responseError,
			})
		}(req, requestContext, cancel, registered)
	}

	scanError := scanner.Err()
	if scanError != nil || ctx.Err() != nil {
		s.cancelAll()
	}
	requests.Wait()
	if err := sink.err(); err != nil {
		return err
	}
	if scanError != nil {
		return fmt.Errorf("read JSON-RPC input: %w", scanError)
	}
	return ctx.Err()
}

func decodeRequest(line []byte) (request, *RPCError) {
	if !json.Valid(line) {
		return request{}, rpcError(codeParseError, "parse_error", "invalid JSON")
	}
	if len(line) == 0 || line[0] != '{' {
		return request{}, rpcError(codeInvalidRequest, "invalid_request", "request must be a JSON object")
	}

	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return request{}, rpcError(codeInvalidRequest, "invalid_request", "invalid JSON-RPC request")
	}
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return request{}, rpcError(codeInvalidRequest, "invalid_request", "jsonrpc must be 2.0 and method is required")
	}
	if len(req.ID) != 0 && !validRequestID(req.ID) {
		return request{}, rpcError(codeInvalidRequest, "invalid_request_id", "id must be a string, number, or null")
	}
	return req, nil
}

func validRequestID(raw json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	switch value.(type) {
	case string, json.Number:
		return true
	default:
		return false
	}
}

func (s *Server) handle(ctx context.Context, req request, sink *responseSink) (any, *RPCError) {
	switch req.Method {
	case "initialize":
		return s.initialize(ctx, req.Params)
	case "repository.discover":
		return s.discoverRepository(ctx, req.Params)
	case "refs.list":
		return s.listRefs(ctx, req.Params)
	case "blame.lines":
		return s.blameLines(ctx, req.Params)
	case "history.file":
		return s.fileHistory(ctx, req.Params)
	case "revision.content":
		return s.revisionContent(ctx, req.Params)
	case "search.run":
		progress := newProgressReporter(req.ID, sink)
		result, responseError := s.searchRun(ctx, req.Params, progress)
		progress.done(result.Scanned, ctx.Err() != nil)
		return result, responseError
	case "diff.file":
		return s.diffFile(ctx, req.Params)
	default:
		if methodAllowed(req.Method) {
			return nil, rpcError(
				codeNotImplemented,
				"method_not_implemented",
				fmt.Sprintf("method %q is not implemented in this helper slice", req.Method),
			)
		}
		return nil, rpcError(codeMethodNotFound, "method_not_found", "method not found")
	}
}

func (s *Server) initialize(ctx context.Context, rawParams json.RawMessage) (InitializeResult, *RPCError) {
	var params InitializeParams
	if len(rawParams) == 0 || bytes.Equal(bytes.TrimSpace(rawParams), []byte("null")) {
		return InitializeResult{}, rpcError(codeInvalidParams, "protocol_version_required", "protocolVersion is required")
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return InitializeResult{}, rpcError(codeInvalidParams, "invalid_initialize_params", "initialize params must be an object")
	}
	version, err := parseProtocolVersion(params.ProtocolVersion)
	if err != nil {
		return InitializeResult{}, rpcError(codeInvalidParams, "invalid_protocol_version", err.Error())
	}
	if version != ProtocolVersion {
		return InitializeResult{}, rpcError(
			codeInvalidParams,
			"unsupported_protocol_version",
			fmt.Sprintf("unsupported protocolVersion %d", version),
		)
	}

	diagnostics := GitDiagnostics{}
	installation, gitError := s.resolveGit(ctx)
	if gitError != nil {
		if ctx.Err() != nil || errors.Is(gitError, context.Canceled) {
			return InitializeResult{}, cancelledError()
		}
		diagnostics.Error = gitError.Error()
	} else {
		diagnostics = GitDiagnostics{
			Available:        true,
			Version:          installation.Version,
			Executable:       installation.Path,
			ExecutableSource: installation.Source,
		}
	}

	methods := append([]string(nil), Methods...)
	notifications := append([]string(nil), Notifications...)
	return InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Transport:       Transport,
		LineBase:        LineBase,
		ReadOnly:        ReadOnly,
		Network:         Network,
		MaxOutputBytes:  MaxOutputBytes,
		Methods:         methods,
		Notifications:   notifications,
		Server: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Capabilities: Capabilities{
			ReadOnly:       ReadOnly,
			Network:        Network,
			LineBase:       LineBase,
			MaxOutputBytes: MaxOutputBytes,
			Methods:        append([]string(nil), methods...),
			Notifications:  append([]string(nil), notifications...),
		},
		Diagnostics: diagnostics,
	}, nil
}

func parseProtocolVersion(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, errors.New("protocolVersion is required")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, errors.New("protocolVersion must be 1 or \"1.0\"")
	}

	switch typed := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil || parsed != float64(ProtocolVersion) {
			return 0, errors.New("protocolVersion must be 1 or \"1.0\"")
		}
		return ProtocolVersion, nil
	case string:
		if typed == "1" || typed == "1.0" {
			return ProtocolVersion, nil
		}
	}
	return 0, errors.New("protocolVersion must be 1 or \"1.0\"")
}

func (s *Server) discoverRepository(ctx context.Context, rawParams json.RawMessage) (RepositoryDiscoverResult, *RPCError) {
	var params RepositoryDiscoverParams
	if len(rawParams) == 0 || json.Unmarshal(rawParams, &params) != nil {
		return RepositoryDiscoverResult{}, rpcError(codeInvalidParams, "invalid_repository_params", "repository.discover params must be an object")
	}
	path := strings.TrimSpace(params.Path)
	if path == "" {
		return RepositoryDiscoverResult{}, rpcError(codeInvalidParams, "repository_path_required", "path is required")
	}
	if !filepath.IsAbs(path) {
		return RepositoryDiscoverResult{}, rpcError(codeInvalidParams, "invalid_repository_path", "path must be absolute")
	}

	resolvedPath, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return RepositoryDiscoverResult{}, rpcError(codeInvalidParams, "invalid_repository_path", "path must exist")
	}
	info, err := os.Stat(resolvedPath)
	if err != nil || !info.IsDir() {
		return RepositoryDiscoverResult{}, rpcError(codeInvalidParams, "invalid_repository_path", "path must be an existing directory")
	}

	installation, err := s.resolveGit(ctx)
	if err != nil {
		return RepositoryDiscoverResult{}, rpcError(codeGitNotFound, "git_not_found", err.Error())
	}
	runner := &gitexec.Runner{Binary: installation.Path}
	repository, err := gitexec.OpenRepository(ctx, runner, resolvedPath)
	if err != nil {
		if ctx.Err() != nil {
			return RepositoryDiscoverResult{}, cancelledError()
		}
		return RepositoryDiscoverResult{}, rpcError(codeNotGitRepository, "not_git_repository", err.Error())
	}

	gitDirOutput, err := runReadOnlyGit(ctx, repository, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return RepositoryDiscoverResult{}, gitCommandError(ctx, err)
	}
	gitDir := strings.TrimSpace(string(gitDirOutput))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repository.Root, gitDir)
	}
	gitDir = canonicalPath(gitDir)

	branch := ""
	if branchOutput, branchError := runReadOnlyGit(ctx, repository, nil, "symbolic-ref", "--quiet", "--short", "HEAD"); branchError == nil {
		branch = strings.TrimSpace(string(branchOutput))
	} else if ctx.Err() != nil {
		return RepositoryDiscoverResult{}, cancelledError()
	}

	head := ""
	if headOutput, headError := runReadOnlyGit(ctx, repository, nil, "rev-parse", "--verify", "HEAD"); headError == nil {
		head = strings.TrimSpace(string(headOutput))
	} else if ctx.Err() != nil {
		return RepositoryDiscoverResult{}, cancelledError()
	}

	return RepositoryDiscoverResult{
		Root:      canonicalPath(repository.Root),
		CommonDir: canonicalPath(repository.CommonDir),
		GitDir:    gitDir,
		Branch:    branch,
		Head:      head,
	}, nil
}

func canonicalPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func gitCommandError(ctx context.Context, err error) *RPCError {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return cancelledError()
	}
	return rpcError(codeInternalError, "git_error", err.Error())
}

func cancelledError() *RPCError {
	return rpcError(codeRequestCancelled, "request_cancelled", "request cancelled")
}

func methodAllowed(method string) bool {
	for _, allowed := range Methods {
		if method == allowed {
			return true
		}
	}
	return false
}

func rpcError(code int, stableCode, message string) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		Data:    ErrorData{Code: stableCode},
	}
}

func requestIDKey(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return "string:" + typed, true
	case json.Number:
		return "number:" + typed.String(), true
	default:
		return "", false
	}
}

func (s *Server) registerCancellation(id json.RawMessage, cancel context.CancelFunc) bool {
	key, ok := requestIDKey(id)
	if !ok {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.cancellations[key]; exists {
		return false
	}
	s.cancellations[key] = cancel
	return true
}

func (s *Server) finishRequest(id json.RawMessage) {
	key, ok := requestIDKey(id)
	if !ok {
		return
	}
	s.mu.Lock()
	delete(s.cancellations, key)
	s.mu.Unlock()
}

func (s *Server) handleCancel(rawParams json.RawMessage) {
	var params struct {
		ID json.RawMessage `json:"id"`
	}
	if len(rawParams) == 0 || json.Unmarshal(rawParams, &params) != nil {
		return
	}
	key, ok := requestIDKey(params.ID)
	if !ok {
		return
	}
	s.mu.Lock()
	cancel := s.cancellations[key]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) cancelAll() {
	s.mu.Lock()
	cancellations := make([]context.CancelFunc, 0, len(s.cancellations))
	for _, cancel := range s.cancellations {
		cancellations = append(cancellations, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
}

func resolveGitInstallation(ctx context.Context) (gitInstallation, error) {
	path, source, err := discoverGitExecutable(runtime.GOOS, os.Getenv, exec.LookPath)
	if err != nil {
		return gitInstallation{}, err
	}
	runner := &gitexec.Runner{Binary: path}
	version, err := runner.Version(ctx)
	if err != nil {
		return gitInstallation{}, fmt.Errorf("read Git version: %w", err)
	}
	return gitInstallation{Path: path, Source: source, Version: version}, nil
}

func discoverGitExecutable(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
) (string, string, error) {
	if candidate, err := lookPath("git"); err == nil {
		if resolved, ok := regularAbsoluteFile(candidate); ok {
			return resolved, "PATH", nil
		}
	}
	if goos == "windows" {
		candidates := []struct {
			base     string
			relative []string
			source   string
		}{
			{
				base:     getenv("ProgramFiles"),
				relative: []string{"Git", "cmd", "git.exe"},
				source:   "ProgramFiles",
			},
			{
				base:     getenv("LocalAppData"),
				relative: []string{"Programs", "Git", "cmd", "git.exe"},
				source:   "LocalAppData",
			},
		}
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate.base) == "" {
				continue
			}
			path := filepath.Join(append([]string{candidate.base}, candidate.relative...)...)
			if resolved, ok := regularAbsoluteFile(path); ok {
				return resolved, candidate.source, nil
			}
		}
	}
	return "", "", errors.New("Git executable not found")
}

func regularAbsoluteFile(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil || !filepath.IsAbs(abs) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return filepath.Clean(resolved), true
}

type responseSink struct {
	writer io.Writer

	mu       sync.Mutex
	writeErr error
}

func (s *responseSink) write(value response) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload)+1 > MaxOutputBytes {
		payload, err = json.Marshal(response{
			JSONRPC: "2.0",
			ID:      value.ID,
			Error:   rpcError(codeInternalError, "output_too_large", "response exceeds maxOutputBytes"),
		})
		if err != nil {
			return err
		}
	}
	return s.writePayload(payload)
}

func (s *responseSink) writeNotification(value ProgressNotification) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload)+1 > MaxOutputBytes {
		return errors.New("notification exceeds maxOutputBytes")
	}
	return s.writePayload(payload)
}

func (s *responseSink) writePayload(payload []byte) error {
	payload = append(payload, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	written, err := s.writer.Write(payload)
	if err == nil && written != len(payload) {
		err = io.ErrShortWrite
	}
	if err != nil {
		s.writeErr = err
	}
	return err
}

func (s *responseSink) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeErr
}
